package workflow

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"agent-orchestrator/db"
)

// Scheduler manages the task lifecycle: detects completions, creates follow-on tasks,
// handles retries with exponential backoff, and requeues timed-out tasks.
type Scheduler struct {
	db         *db.Database
	timeoutSec int
	maxRetries int
	interval   time.Duration
	stop       chan struct{}
}

// NewScheduler creates a Scheduler backed by the given database.
// timeoutSec: seconds before an in_progress task is considered timed out.
// maxRetries: maximum number of times a failed task is automatically retried.
// interval: how often the scheduler polls for work (e.g. 30s).
func NewScheduler(database *db.Database, timeoutSec, maxRetries int, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{
		db:         database,
		timeoutSec: timeoutSec,
		maxRetries: maxRetries,
		interval:   interval,
		stop:       make(chan struct{}),
	}
}

// Start launches the background scheduling loop.
func (s *Scheduler) Start(ctx context.Context) {
	go s.run(ctx)
}

// Stop signals the background loop to exit.
func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				log.Printf("[scheduler] tick error: %v", err)
			}
		}
	}
}

// Tick performs one scheduling cycle:
//  1. Requeue timed-out tasks.
//  2. Retry eligible failed tasks (exponential backoff).
//  3. Create follow-on tasks for newly completed tasks.
func (s *Scheduler) Tick(ctx context.Context) error {
	// 1. Requeue timed-out in_progress tasks.
	requeued, err := s.db.RequeueTimedOutTasks(ctx, s.timeoutSec)
	if err != nil {
		return fmt.Errorf("requeue timed out tasks: %w", err)
	}
	if requeued > 0 {
		log.Printf("[scheduler] requeued %d timed-out tasks", requeued)
	}

	// 2. Retry failed tasks within retry limit.
	if err := s.retryFailedTasks(ctx); err != nil {
		return fmt.Errorf("retry failed tasks: %w", err)
	}

	// 3. Create follow-on tasks for completed tasks that need them.
	if err := s.createFollowOnTasks(ctx); err != nil {
		return fmt.Errorf("create follow-on tasks: %w", err)
	}

	return nil
}

// retryFailedTasks promotes failed tasks back to planned if under the retry limit.
// Uses exponential backoff: a task is only retried after 2^(attempts-1) minutes.
func (s *Scheduler) retryFailedTasks(ctx context.Context) error {
	tasks, err := s.db.ListTasks(ctx, db.TaskFilters{Status: "failed"})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, t := range tasks {
		if t.Attempts >= s.maxRetries {
			continue // permanently failed
		}
		// Exponential backoff: wait 2^(attempts-1) minutes since last update.
		backoffMin := math.Pow(2, float64(t.Attempts-1))
		backoffDur := time.Duration(backoffMin * float64(time.Minute))
		if now.Sub(t.UpdatedAt) < backoffDur {
			continue // not ready yet
		}
		// Promote back to planned.
		t.Status = "planned"
		t.AssignedAgentID = ""
		if err := s.db.UpdateTask(ctx, t); err != nil {
			log.Printf("[scheduler] failed to requeue task %s: %v", t.ID, err)
		} else {
			log.Printf("[scheduler] retrying task %s (attempt %d/%d)", t.ID, t.Attempts, s.maxRetries)
		}
	}
	return nil
}

// createFollowOnTasks inspects recently completed tasks and spawns follow-on
// tasks according to the workflow rules (implement→review, review→test, etc.).
func (s *Scheduler) createFollowOnTasks(ctx context.Context) error {
	// Find completed tasks that need a follow-on (last updated within the tick window
	// times 2 to avoid missing tasks when the scheduler restarts).
	tasks, err := s.db.ListTasks(ctx, db.TaskFilters{Status: "completed"})
	if err != nil {
		return err
	}

	for _, t := range tasks {
		followType, ok := FollowOnType(TaskType(t.Type), "completed")
		if !ok {
			continue
		}
		// Check if a follow-on task already exists to avoid duplicates.
		existing, err := s.db.ListTasks(ctx, db.TaskFilters{
			ProjectID: t.ProjectID,
			Status:    "planned",
		})
		if err != nil {
			continue
		}
		alreadyExists := false
		for _, e := range existing {
			if e.Type == string(followType) {
				// Look for a task whose payload references this parent.
				if pid, ok := e.Payload["parent_task_id"].(string); ok && pid == t.ID {
					alreadyExists = true
					break
				}
			}
		}
		if alreadyExists {
			continue
		}

		// Also check non-planned states (in_progress, etc.) to avoid duplicates.
		for _, status := range []string{"in_progress", "needs_review", "approved"} {
			more, err := s.db.ListTasks(ctx, db.TaskFilters{
				ProjectID: t.ProjectID,
				Status:    status,
			})
			if err != nil {
				continue
			}
			for _, e := range more {
				if e.Type == string(followType) {
					if pid, ok := e.Payload["parent_task_id"].(string); ok && pid == t.ID {
						alreadyExists = true
					}
				}
			}
		}
		if alreadyExists {
			continue
		}

		if err := s.CreateFollowOnTask(ctx, t, followType); err != nil {
			log.Printf("[scheduler] failed to create follow-on task for %s: %v", t.ID, err)
		}
	}
	return nil
}

// CreateFollowOnTask creates a new task that follows up on a completed parent task.
func (s *Scheduler) CreateFollowOnTask(ctx context.Context, parent *db.Task, followType TaskType) error {
	role := RoleForType(followType)

	// Inherit parent payload and add context.
	payload := make(map[string]interface{})
	for k, v := range parent.Payload {
		payload[k] = v
	}
	payload["parent_task_id"] = parent.ID
	payload["parent_task_type"] = parent.Type

	followOn := &db.Task{
		ProjectID: parent.ProjectID,
		Type:      string(followType),
		Role:      role,
		Status:    "planned",
		Priority:  parent.Priority, // inherit priority
		Payload:   payload,
	}

	if err := s.db.CreateTask(ctx, followOn); err != nil {
		return fmt.Errorf("create follow-on %s task: %w", followType, err)
	}
	log.Printf("[scheduler] created follow-on %s task %s for completed %s task %s",
		followType, followOn.ID, parent.Type, parent.ID)
	return nil
}

// HandleReviewResult processes the outcome of a review task:
//   - "approved": marks review as approved, creates a test task
//   - "changes": marks review as needs_review, re-queues the implement task
func (s *Scheduler) HandleReviewResult(ctx context.Context, reviewTask *db.Task, outcome string) error {
	switch outcome {
	case "approved":
		reviewTask.Status = "approved"
		if err := s.db.UpdateTask(ctx, reviewTask); err != nil {
			return fmt.Errorf("update review task to approved: %w", err)
		}
		// Create a test task.
		return s.CreateFollowOnTask(ctx, reviewTask, TypeTest)

	case "changes":
		reviewTask.Status = "needs_review"
		if err := s.db.UpdateTask(ctx, reviewTask); err != nil {
			return fmt.Errorf("update review task to needs_review: %w", err)
		}
		// Re-queue the original implement task.
		parentID, _ := reviewTask.Payload["parent_task_id"].(string)
		if parentID == "" {
			return nil
		}
		implTask, err := s.db.GetTask(ctx, parentID)
		if err != nil {
			return fmt.Errorf("get parent implement task %s: %w", parentID, err)
		}
		if implTask.Attempts >= s.maxRetries {
			log.Printf("[scheduler] implement task %s has reached max retries, not re-queuing", implTask.ID)
			return nil
		}
		implTask.Status = "planned"
		implTask.AssignedAgentID = ""
		return s.db.UpdateTask(ctx, implTask)

	default:
		return fmt.Errorf("unknown review outcome %q; expected 'approved' or 'changes'", outcome)
	}
}

// RetryDelay returns the exponential backoff duration for a given attempt count.
// attempt=1 → 1m, attempt=2 → 2m, attempt=3 → 4m, etc.
func RetryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Minute
	}
	minutes := math.Pow(2, float64(attempt-1))
	return time.Duration(minutes * float64(time.Minute))
}
