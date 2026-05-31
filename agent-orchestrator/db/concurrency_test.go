package db_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"agent-orchestrator/db"
)

// createConcurrencyProject creates a project for concurrency tests.
func createConcurrencyProject(t *testing.T, d *db.Database) string {
	t.Helper()
	p := &db.Project{Name: "Concurrency Test Project", Status: "active"}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p.ID
}

// createAgent creates a test agent entry.
func createAgent(t *testing.T, d *db.Database, name string) string {
	t.Helper()
	a := &db.Agent{
		Name:   name,
		Roles:  []string{"worker"},
		Status: "online",
	}
	if err := d.CreateAgent(context.Background(), a); err != nil {
		t.Fatalf("create agent %q: %v", name, err)
	}
	return a.ID
}

// TestClaimTask_NoConcurrentDoubleClaim verifies that concurrent goroutines
// racing to claim the same task result in exactly one success and one failure.
func TestClaimTask_NoConcurrentDoubleClaim(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	projectID := createConcurrencyProject(t, d)

	agentA := createAgent(t, d, "agent-a")
	agentB := createAgent(t, d, "agent-b")

	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusBacklog,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	var successCount int32
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := d.ClaimTask(ctx, task.ID, agentA); err == nil {
			atomic.AddInt32(&successCount, 1)
		}
	}()
	go func() {
		defer wg.Done()
		if err := d.ClaimTask(ctx, task.ID, agentB); err == nil {
			atomic.AddInt32(&successCount, 1)
		}
	}()

	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful claim, got %d", successCount)
	}

	updated, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != db.TaskStatusDeveloping {
		t.Errorf("expected task status %q, got %q", db.TaskStatusDeveloping, updated.Status)
	}
	if updated.AssignedAgentID == "" {
		t.Error("expected assigned_agent_id to be set")
	}
}

// TestClaimTask_ManyAgentsManyTasks verifies fair distribution and no double-claims
// when N agents race to claim M tasks.
func TestClaimTask_ManyAgentsManyTasks(t *testing.T) {
	const numAgents = 5
	const numTasks = 20

	d := openTestDB(t)
	ctx := context.Background()
	projectID := createConcurrencyProject(t, d)

	agentIDs := make([]string, numAgents)
	for i := 0; i < numAgents; i++ {
		agentIDs[i] = createAgent(t, d, fmt.Sprintf("worker-%d", i))
	}

	taskIDs := make([]string, numTasks)
	for i := 0; i < numTasks; i++ {
		task := &db.Task{
			ProjectID: projectID,
			Type:      "implement",
			Role:      "worker",
			Status:    db.TaskStatusBacklog,
			Payload:   map[string]interface{}{"index": i},
		}
		if err := d.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task[%d]: %v", i, err)
		}
		taskIDs[i] = task.ID
	}

	var totalClaims int32
	var wg sync.WaitGroup

	for _, agentID := range agentIDs {
		for _, taskID := range taskIDs {
			wg.Add(1)
			go func(aid, tid string) {
				defer wg.Done()
				if err := d.ClaimTask(ctx, tid, aid); err == nil {
					atomic.AddInt32(&totalClaims, 1)
				}
			}(agentID, taskID)
		}
	}
	wg.Wait()

	if int(totalClaims) != numTasks {
		t.Errorf("expected %d total claims (one per task), got %d", numTasks, totalClaims)
	}

	claimedBy := make(map[string]int)
	for _, tid := range taskIDs {
		task, err := d.GetTask(ctx, tid)
		if err != nil {
			t.Fatalf("get task %s: %v", tid, err)
		}
		if task.Status != db.TaskStatusDeveloping {
			t.Errorf("task %s: expected status %s, got %q", tid, db.TaskStatusDeveloping, task.Status)
		}
		if task.AssignedAgentID == "" {
			t.Errorf("task %s: expected assigned_agent_id to be set", tid)
		}
		claimedBy[task.AssignedAgentID]++
	}

	agentSet := make(map[string]bool, numAgents)
	for _, id := range agentIDs {
		agentSet[id] = true
	}
	for aid := range claimedBy {
		if !agentSet[aid] {
			t.Errorf("task claimed by unknown agent %q", aid)
		}
	}
}

// TestClaimTask_AlreadyClaimed verifies that a second ClaimTask call on an
// already in-progress task always returns an error.
func TestClaimTask_AlreadyClaimed(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	projectID := createConcurrencyProject(t, d)
	agentA := createAgent(t, d, "agent-already-a")
	agentB := createAgent(t, d, "agent-already-b")

	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusBacklog,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := d.ClaimTask(ctx, task.ID, agentA); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	if err := d.ClaimTask(ctx, task.ID, agentB); err == nil {
		t.Error("expected error on second claim of already-in-progress task")
	}
}

// TestClaimTask_CompletedTask verifies that a completed task cannot be claimed.
func TestClaimTask_CompletedTask(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	projectID := createConcurrencyProject(t, d)
	agentID := createAgent(t, d, "agent-comp")

	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusCompleted,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := d.ClaimTask(ctx, task.ID, agentID); err == nil {
		t.Error("expected error claiming a completed task")
	}
}

// TestRequeueTimedOutTasks verifies that timed-out tasks are re-queued.
func TestRequeueTimedOutTasks(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	projectID := createConcurrencyProject(t, d)

	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusDeveloping,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, err := d.RawDB().ExecContext(ctx,
		`UPDATE tasks SET updated_at=datetime('now','-600 seconds') WHERE id=?`, task.ID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	n, err := d.RequeueTimedOutTasks(ctx, 300)
	if err != nil {
		t.Fatalf("RequeueTimedOutTasks: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 task requeued, got %d", n)
	}

	updated, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != db.TaskStatusBacklog {
		t.Errorf("expected status %q, got %q", db.TaskStatusBacklog, updated.Status)
	}
}

// TestRequeueTimedOutTasks_ReviewingGoesToAwaitingReview verifies that a timed-out
// REVIEWING task returns to AWAITING_REVIEW (Bug 9 C), not BACKLOG.
func TestRequeueTimedOutTasks_ReviewingGoesToAwaitingReview(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	projectID := createConcurrencyProject(t, d)

	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusReviewing,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := d.RawDB().ExecContext(ctx,
		`UPDATE tasks SET updated_at=datetime('now','-600 seconds') WHERE id=?`, task.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if _, err := d.RequeueTimedOutTasks(ctx, 300); err != nil {
		t.Fatalf("RequeueTimedOutTasks: %v", err)
	}

	updated, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != db.TaskStatusAwaitingReview {
		t.Errorf("expected status %q, got %q", db.TaskStatusAwaitingReview, updated.Status)
	}
}

// TestRequeueTimedOutTasks_NotExpired verifies that non-expired tasks are not requeued.
func TestRequeueTimedOutTasks_NotExpired(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	projectID := createConcurrencyProject(t, d)

	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusDeveloping,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	n, err := d.RequeueTimedOutTasks(ctx, 300)
	if err != nil {
		t.Fatalf("RequeueTimedOutTasks: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 tasks requeued (task is fresh), got %d", n)
	}
}
