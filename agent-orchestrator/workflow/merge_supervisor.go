package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"agent-orchestrator/db"
	"agent-orchestrator/git"
	"agent-orchestrator/storage"
)

// MergeSupervisor polls for AWAITING_MERGE tasks, checks file-lock conflicts,
// and orchestrates the merge into main.
type MergeSupervisor struct {
	database    *db.Database
	paths       *storage.Paths
	intervalSec int
}

// NewMergeSupervisor creates a supervisor with the given DB, storage paths,
// and polling interval.
func NewMergeSupervisor(database *db.Database, paths *storage.Paths, intervalSec int) *MergeSupervisor {
	if intervalSec <= 0 {
		intervalSec = 10
	}
	return &MergeSupervisor{
		database:    database,
		paths:       paths,
		intervalSec: intervalSec,
	}
}

// TickOnce runs one supervisor cycle synchronously. For testing only.
func (ms *MergeSupervisor) TickOnce(ctx context.Context) {
	ms.tick(ctx)
}

// Run starts the merge supervisor loop. It blocks until ctx is cancelled.
func (ms *MergeSupervisor) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(ms.intervalSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ms.tick(ctx)
		}
	}
}

func (ms *MergeSupervisor) tick(ctx context.Context) {
	tasks, err := ms.database.ListTasks(ctx, db.TaskFilters{
		Status: db.TaskStatusAwaitingMerge,
		Limit:  50,
	})
	if err != nil {
		log.Printf("merge_supervisor: ListTasks: %v", err)
		return
	}
	for _, t := range tasks {
		ms.processTask(ctx, t)
	}
}

func (ms *MergeSupervisor) processTask(ctx context.Context, task *db.Task) {
	project, err := ms.database.GetProject(ctx, task.ProjectID)
	if err != nil {
		log.Printf("merge_supervisor: GetProject %q: %v", task.ProjectID, err)
		return
	}

	repoPath := ms.paths.RepoPath(project.ID)
	branchName := task.Branch
	if branchName == "" {
		branchName = fmt.Sprintf("task/%s", task.ID)
	}

	// Detect changed files for this branch.
	changed, err := git.ChangedFiles(repoPath, branchName, "main")
	if err != nil {
		log.Printf("merge_supervisor: ChangedFiles task %q: %v", task.ID, err)
		return
	}

	// Check against existing merge locks held by other tasks.
	if conflict, otherTaskID := ms.checkFileLocks(ctx, task.ID, changed); conflict {
		log.Printf("merge_supervisor: task %q blocked by file-lock conflict with %q", task.ID, otherTaskID)
		return // leave in AWAITING_MERGE; retry next tick
	}

	// Acquire a merge lock for this task's files.
	if err := ms.acquireMergeLock(ctx, task.ID, changed); err != nil {
		log.Printf("merge_supervisor: acquireMergeLock task %q: %v", task.ID, err)
		return
	}

	// Transition to MERGING.
	if err := ms.database.TransitionTaskState(ctx, task.ID,
		db.TaskStatusAwaitingMerge, db.TaskStatusMerging,
		"", "merge supervisor"); err != nil {
		_ = ms.releaseMergeLock(ctx, task.ID)
		log.Printf("merge_supervisor: TransitionTaskState %q: %v", task.ID, err)
		return
	}
	task.Status = db.TaskStatusMerging // keep in-memory copy in sync with DB

	// Perform the merge in a temporary worktree.
	mergeWorktreePath := filepath.Join(ms.paths.WorktreePath(task.ID) + "-merge")
	headSHA, mergeErr := git.MergeIntoMain(repoPath, mergeWorktreePath, branchName)

	_ = ms.releaseMergeLock(ctx, task.ID)

	if mergeErr != nil {
		log.Printf("merge_supervisor: merge failed for task %q: %v", task.ID, mergeErr)
		ms.handleMergeFailure(ctx, task, mergeErr)
		return
	}

	// Record the merge SHA.
	task.BranchHeadSHA = headSHA
	now := time.Now().UTC()
	task.LastPushAt = &now
	if err := ms.database.UpdateTask(ctx, task); err != nil {
		log.Printf("merge_supervisor: UpdateTask SHA task %q: %v", task.ID, err)
	}

	// Transition to COMPLETED.
	if err := ms.database.TransitionTaskState(ctx, task.ID,
		db.TaskStatusMerging, db.TaskStatusCompleted,
		"", "merge successful"); err != nil {
		log.Printf("merge_supervisor: TransitionTaskState COMPLETED %q: %v", task.ID, err)
		return
	}
	log.Printf("merge_supervisor: task %q merged and completed (sha=%s)", task.ID, headSHA)

	// Push to upstream if configured (non-blocking; failure is logged only).
	if project.RemoteURL != "" {
		go ms.pushUpstream(project, headSHA)
	}
}

func (ms *MergeSupervisor) handleMergeFailure(ctx context.Context, task *db.Task, mergeErr error) {
	// Create a synthetic review requesting revision with the merge-fail log.
	rev := &db.TaskReview{
		TaskID:     task.ID,
		AuthorType: "system",
		AuthorRole: "merge_supervisor",
		Status:     "revision_requested",
		Body:       fmt.Sprintf("## Merge Failed\n\n```\n%v\n```\n\nPlease resolve conflicts and resubmit.", mergeErr),
	}
	if err := ms.database.CreateTaskReview(ctx, rev); err != nil {
		log.Printf("merge_supervisor: CreateTaskReview (failure) task %q: %v", task.ID, err)
	}

	_ = ms.database.TransitionTaskState(ctx, task.ID,
		db.TaskStatusMerging, db.TaskStatusAwaitingRevision,
		"", "merge conflict — revision requested")
}

func (ms *MergeSupervisor) pushUpstream(project *db.Project, _ string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	repoPath := ms.paths.RepoPath(project.ID)
	if err := git.PushMain(repoPath, "upstream", ""); err != nil {
		log.Printf("merge_supervisor: upstream push project %q: %v (non-fatal)", project.ID, err)
		_ = ms.database.CreateLog(ctx, &db.LogEntry{
			ProjectID: project.ID,
			Level:     "warn",
			Message:   fmt.Sprintf("upstream push failed: %v", err),
			Metadata:  map[string]interface{}{"event": "project_upstream_sync_failed"},
		})
	}
}

// --- merge lock helpers ---

func (ms *MergeSupervisor) checkFileLocks(ctx context.Context, taskID string, changed []string) (conflict bool, otherTaskID string) {
	locks, err := ms.database.ListMergeLocks(ctx)
	if err != nil {
		log.Printf("merge_supervisor: ListMergeLocks: %v", err)
		return false, ""
	}
	for _, lock := range locks {
		if lock.TaskID == taskID {
			continue
		}
		if git.HasOverlap(lock.Paths, changed) {
			return true, lock.TaskID
		}
	}
	return false, ""
}

func (ms *MergeSupervisor) acquireMergeLock(ctx context.Context, taskID string, paths []string) error {
	return ms.database.CreateMergeLock(ctx, taskID, paths)
}

func (ms *MergeSupervisor) releaseMergeLock(ctx context.Context, taskID string) error {
	return ms.database.DeleteMergeLock(ctx, taskID)
}

// MergeLock is the in-memory representation of a merge_locks row.
type MergeLock struct {
	ID        string
	TaskID    string
	Paths     []string
	CreatedAt time.Time
}

// parseMergeLockPaths deserialises the JSON paths column.
func parseMergeLockPaths(pathsJSON string) []string {
	var paths []string
	_ = json.Unmarshal([]byte(pathsJSON), &paths)
	return paths
}
