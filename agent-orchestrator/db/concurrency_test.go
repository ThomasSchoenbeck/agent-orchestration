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
	p := &db.Project{Name: "Concurrency Test Project", Status: "planned"}
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
		Status:    "planned",
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

	// Verify the task is in_progress with exactly one assigned agent.
	updated, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != "in_progress" {
		t.Errorf("expected task status %q, got %q", "in_progress", updated.Status)
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

	// Create agents.
	agentIDs := make([]string, numAgents)
	for i := 0; i < numAgents; i++ {
		agentIDs[i] = createAgent(t, d, fmt.Sprintf("worker-%d", i))
	}

	// Create tasks.
	taskIDs := make([]string, numTasks)
	for i := 0; i < numTasks; i++ {
		task := &db.Task{
			ProjectID: projectID,
			Type:      "implement",
			Role:      "worker",
			Status:    "planned",
			Payload:   map[string]interface{}{"index": i},
		}
		if err := d.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task[%d]: %v", i, err)
		}
		taskIDs[i] = task.ID
	}

	// Each agent tries to claim every task concurrently.
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

	// Each task should be claimed exactly once → totalClaims == numTasks.
	if int(totalClaims) != numTasks {
		t.Errorf("expected %d total claims (one per task), got %d", numTasks, totalClaims)
	}

	// Verify each task has exactly one assigned agent and is in_progress.
	claimedBy := make(map[string]int) // agent → claim count
	for _, tid := range taskIDs {
		task, err := d.GetTask(ctx, tid)
		if err != nil {
			t.Fatalf("get task %s: %v", tid, err)
		}
		if task.Status != "in_progress" {
			t.Errorf("task %s: expected status in_progress, got %q", tid, task.Status)
		}
		if task.AssignedAgentID == "" {
			t.Errorf("task %s: expected assigned_agent_id to be set", tid)
		}
		claimedBy[task.AssignedAgentID]++
	}

	// Verify all claimants are real agents.
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
		Status:    "planned",
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// First claim succeeds.
	if err := d.ClaimTask(ctx, task.ID, agentA); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	// Second claim must fail.
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
		Status:    "completed",
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

	// Create an in-progress task backdated by 600s.
	task := &db.Task{
		ProjectID: projectID,
		Type:      "implement",
		Role:      "worker",
		Status:    "in_progress",
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	// Backdate updated_at.
	_, err := d.RawDB().ExecContext(ctx,
		`UPDATE tasks SET updated_at=datetime('now','-600 seconds') WHERE id=?`, task.ID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Requeue tasks older than 300s.
	n, err := d.RequeueTimedOutTasks(ctx, 300)
	if err != nil {
		t.Fatalf("RequeueTimedOutTasks: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 task requeued, got %d", n)
	}

	// Verify the task is now planned.
	updated, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != "planned" {
		t.Errorf("expected status %q, got %q", "planned", updated.Status)
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
		Status:    "in_progress",
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	// Task is fresh (just created), timeout is 300s.
	n, err := d.RequeueTimedOutTasks(ctx, 300)
	if err != nil {
		t.Fatalf("RequeueTimedOutTasks: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 tasks requeued (task is fresh), got %d", n)
	}
}
