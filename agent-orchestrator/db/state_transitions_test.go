package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestTransitionTaskState_RecordsHistory(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "transition-test", Status: "active"}
	if err := d.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task := &db.Task{
		ProjectID: proj.ID,
		Type:      "implement",
		Role:      "worker",
		Status:    db.TaskStatusBacklog,
		Priority:  5,
		Payload:   map[string]interface{}{},
	}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Transition BACKLOG → DEVELOPING.
	if err := d.TransitionTaskState(ctx, task.ID,
		db.TaskStatusBacklog, db.TaskStatusDeveloping,
		"agent-abc", "claimed by agent"); err != nil {
		t.Fatalf("TransitionTaskState: %v", err)
	}

	// Verify task status changed.
	updated, err := d.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updated.Status != db.TaskStatusDeveloping {
		t.Errorf("task status = %q, want %q", updated.Status, db.TaskStatusDeveloping)
	}

	// Verify transition recorded.
	transitions, err := d.ListStateTransitions(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListStateTransitions: %v", err)
	}
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	tr := transitions[0]
	if tr.FromState != db.TaskStatusBacklog {
		t.Errorf("from_state = %q, want %q", tr.FromState, db.TaskStatusBacklog)
	}
	if tr.ToState != db.TaskStatusDeveloping {
		t.Errorf("to_state = %q, want %q", tr.ToState, db.TaskStatusDeveloping)
	}
	if tr.ActorAgentID != "agent-abc" {
		t.Errorf("actor_agent_id = %q, want %q", tr.ActorAgentID, "agent-abc")
	}
	if tr.Reason != "claimed by agent" {
		t.Errorf("reason = %q, want %q", tr.Reason, "claimed by agent")
	}
}

func TestTransitionTaskState_WrongFromState(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "bad-from", Status: "active"}
	_ = d.CreateProject(ctx, proj)
	task := &db.Task{
		ProjectID: proj.ID, Type: "implement", Role: "worker",
		Status: db.TaskStatusBacklog, Priority: 1, Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(ctx, task)

	// Try to transition from DEVELOPING (wrong — task is BACKLOG).
	err := d.TransitionTaskState(ctx, task.ID,
		db.TaskStatusDeveloping, db.TaskStatusAwaitingReview,
		"", "")
	if err == nil {
		t.Fatal("expected error for wrong fromState, got nil")
	}
}

func TestTransitionTaskState_MultipleTransitions(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	proj := &db.Project{Name: "multi-transition", Status: "active"}
	_ = d.CreateProject(ctx, proj)
	task := &db.Task{
		ProjectID: proj.ID, Type: "implement", Role: "worker",
		Status: db.TaskStatusBacklog, Priority: 1, Payload: map[string]interface{}{},
	}
	_ = d.CreateTask(ctx, task)

	transitions := []struct{ from, to string }{
		{db.TaskStatusBacklog, db.TaskStatusDeveloping},
		{db.TaskStatusDeveloping, db.TaskStatusAwaitingReview},
		{db.TaskStatusAwaitingReview, db.TaskStatusReviewing},
		{db.TaskStatusReviewing, db.TaskStatusAwaitingMerge},
		{db.TaskStatusAwaitingMerge, db.TaskStatusMerging},
		{db.TaskStatusMerging, db.TaskStatusCompleted},
	}

	for _, tr := range transitions {
		if err := d.TransitionTaskState(ctx, task.ID, tr.from, tr.to, "", ""); err != nil {
			t.Fatalf("TransitionTaskState %s→%s: %v", tr.from, tr.to, err)
		}
	}

	recorded, err := d.ListStateTransitions(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListStateTransitions: %v", err)
	}
	if len(recorded) != len(transitions) {
		t.Errorf("expected %d transitions, got %d", len(transitions), len(recorded))
	}
	if recorded[len(recorded)-1].ToState != db.TaskStatusCompleted {
		t.Errorf("last state = %q, want COMPLETED", recorded[len(recorded)-1].ToState)
	}
}
