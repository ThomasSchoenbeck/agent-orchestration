package db_test

// Tests for Bug 8 / Bug 10: agent identity is resolved and persisted on task
// log events and comments, and status transitions are mirrored into the event
// log so the status-history timeline is complete.

import (
	"context"
	"strings"
	"testing"

	"agent-orchestrator/db"
)

// TestLogTaskEvent_StoresAgentName verifies that claiming a task (which logs a
// status-change event with an agent ID) resolves and stores the agent's display
// name and the branch in the event's metadata, surfaced as TaskLog.AgentName.
func TestLogTaskEvent_StoresAgentName(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	agent := &db.Agent{Name: "worker-7", Roles: []string{"worker"}, Status: "online"}
	if err := d.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	task := &db.Task{ProjectID: "p1", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := d.ClaimTask(ctx, task.ID, agent.ID); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	logs, err := d.ListTaskLogs(ctx, db.TaskLogFilters{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListTaskLogs: %v", err)
	}
	var claimed *db.TaskLog
	for _, l := range logs {
		if l.EventType == "task_claimed" {
			claimed = l
			break
		}
	}
	if claimed == nil {
		t.Fatalf("no task_claimed event logged; got %d events", len(logs))
	}
	if claimed.AgentName != "worker-7" {
		t.Errorf("AgentName = %q, want %q", claimed.AgentName, "worker-7")
	}
	if claimed.AgentID != agent.ID {
		t.Errorf("AgentID = %q, want %q", claimed.AgentID, agent.ID)
	}
	if !strings.Contains(claimed.Metadata, "worker-7") {
		t.Errorf("metadata missing agent_name: %s", claimed.Metadata)
	}
	if !strings.Contains(claimed.Metadata, "task/"+task.ID) {
		t.Errorf("metadata missing branch: %s", claimed.Metadata)
	}
}

// TestTransitionTaskState_LogsStatusEvent verifies that a state transition is
// mirrored into the event log with old/new status and agent identity, so the
// status-history timeline (Bug 10) captures review/merge transitions.
func TestTransitionTaskState_LogsStatusEvent(t *testing.T) {
	d := openTestDBWithLogs(t)
	ctx := context.Background()

	agent := &db.Agent{Name: "reviewer-3", Roles: []string{"reviewer"}, Status: "online"}
	if err := d.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	task := &db.Task{ProjectID: "p1", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := d.TransitionTaskState(ctx, task.ID,
		db.TaskStatusBacklog, db.TaskStatusDeveloping, agent.ID, "claimed by agent"); err != nil {
		t.Fatalf("TransitionTaskState: %v", err)
	}

	logs, err := d.ListTaskLogs(ctx, db.TaskLogFilters{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListTaskLogs: %v", err)
	}
	var transition *db.TaskLog
	for _, l := range logs {
		if l.EventType == "task_transition" {
			transition = l
			break
		}
	}
	if transition == nil {
		t.Fatalf("no task_transition event logged; got %d events", len(logs))
	}
	if transition.OldStatus != db.TaskStatusBacklog || transition.NewStatus != db.TaskStatusDeveloping {
		t.Errorf("transition statuses: got %s → %s", transition.OldStatus, transition.NewStatus)
	}
	if transition.AgentName != "reviewer-3" {
		t.Errorf("AgentName = %q, want %q", transition.AgentName, "reviewer-3")
	}
}

// TestCreateComment_StoresAuthorName verifies that an agent-authored comment has
// its author display name resolved and persisted at write time, and returned on
// read.
func TestCreateComment_StoresAuthorName(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	agent := &db.Agent{Name: "reviewer-2", Roles: []string{"reviewer"}, Status: "online"}
	if err := d.CreateAgent(ctx, agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	task := &db.Task{ProjectID: "p1", Role: "reviewer"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	c := &db.TaskComment{
		TaskID:     task.ID,
		AuthorType: "agent",
		AuthorRole: "reviewer",
		AuthorID:   agent.ID,
		Body:       "looks good",
	}
	if err := d.CreateComment(ctx, c); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if c.AuthorName != "reviewer-2" {
		t.Errorf("AuthorName after create = %q, want %q", c.AuthorName, "reviewer-2")
	}

	list, err := d.ListComments(ctx, task.ID, "")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(list))
	}
	if list[0].AuthorName != "reviewer-2" {
		t.Errorf("AuthorName on read = %q, want %q", list[0].AuthorName, "reviewer-2")
	}
}

// TestCreateComment_UserAuthorNoLookup verifies that a user-authored comment is
// not subjected to an agent-name lookup (its author ID is not an agent).
func TestCreateComment_UserAuthorNoLookup(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	task := &db.Task{ProjectID: "p1", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	c := &db.TaskComment{
		TaskID:     task.ID,
		AuthorType: "user",
		AuthorID:   "user-123",
		Body:       "please prioritise this",
	}
	if err := d.CreateComment(ctx, c); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if c.AuthorName != "" {
		t.Errorf("user comment AuthorName = %q, want empty", c.AuthorName)
	}
}
