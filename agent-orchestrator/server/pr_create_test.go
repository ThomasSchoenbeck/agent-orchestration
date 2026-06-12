package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// TestMergeClaim_AutoOpensPR: the PR opens when the merge phase is picked up — a
// task in AWAITING_MERGE claimed into MERGING — not when the code review starts.
func TestMergeClaim_AutoOpensPR(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()

	reviewer := &db.RoleDefinition{
		Name: "reviewer", Label: "Reviewer",
		Capabilities: []string{"handles_review", "handles_merge"}, Enabled: true,
	}
	if err := database.CreateRoleDefinition(ctx, reviewer); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}
	pid := newProject(t, database)

	aw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "rev", "roles": []string{"reviewer"},
	})
	var reg map[string]string
	_ = json.Unmarshal(aw.Body.Bytes(), &reg)

	// A code-review claim (AWAITING_REVIEW → REVIEWING) must NOT open a PR.
	reviewTask := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusAwaitingReview, Branch: "feature/r"}
	if err := database.CreateTask(ctx, reviewTask); err != nil {
		t.Fatalf("CreateTask review: %v", err)
	}
	do(t, srv, http.MethodPost, "/api/tasks/"+reviewTask.ID+"/claim",
		map[string]string{"agent_id": reg["agent_id"]})
	if prs, _ := database.ListPRsForTask(ctx, reviewTask.ID); len(prs) != 0 {
		t.Fatalf("a review claim must not open a PR, got %d", len(prs))
	}

	// A merge claim (AWAITING_MERGE → MERGING) opens the PR.
	mergeTask := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusAwaitingMerge, Branch: "feature/m"}
	if err := database.CreateTask(ctx, mergeTask); err != nil {
		t.Fatalf("CreateTask merge: %v", err)
	}
	cw := do(t, srv, http.MethodPost, "/api/tasks/"+mergeTask.ID+"/claim",
		map[string]string{"agent_id": reg["agent_id"]})
	if cw.Code != http.StatusOK {
		t.Fatalf("claim merge: %d (%s)", cw.Code, cw.Body.String())
	}
	prs, err := database.ListPRsForTask(ctx, mergeTask.ID)
	if err != nil {
		t.Fatalf("ListPRsForTask: %v", err)
	}
	if len(prs) != 1 || prs[0].Status != "open" || prs[0].Branch != "feature/m" {
		t.Fatalf("expected one open PR on feature/m, got %+v", prs)
	}
}

// TestCreatePR_Endpoint_MovesToAwaitingMerge: the human Create-PR endpoint opens
// a PR for a task in review and moves it to AWAITING_MERGE (Merge available).
func TestCreatePR_Endpoint_MovesToAwaitingMerge(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()
	pid := newProject(t, database)
	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusAwaitingReview, Branch: "feature/y"}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/pull-requests", nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create PR: %d (%s)", w.Code, w.Body.String())
	}

	got, err := database.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != db.TaskStatusAwaitingMerge {
		t.Errorf("status = %q, want AWAITING_MERGE", got.Status)
	}
	prs, _ := database.ListPRsForTask(ctx, task.ID)
	if len(prs) != 1 {
		t.Errorf("expected 1 PR, got %d", len(prs))
	}
}
