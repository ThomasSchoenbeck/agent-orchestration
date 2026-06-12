package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// TestClaimReview_RecordsReviewerRole: an AWAITING_REVIEW task with no explicit
// reviewer ("any reviewer") must, when claimed, record the claiming agent's
// handles_review role. The agent-side executor routes a review by review_role; if
// it stays empty the review runs with the task's own (worker) role and the task
// gets stuck in REVIEWING.
func TestClaimReview_RecordsReviewerRole(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()

	reviewer := &db.RoleDefinition{
		Name: "reviewer", Label: "Reviewer",
		Capabilities: []string{"handles_review"}, Enabled: true,
	}
	if err := database.CreateRoleDefinition(ctx, reviewer); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}
	pid := newProject(t, database)

	task := &db.Task{ProjectID: pid, Role: "worker", Status: db.TaskStatusAwaitingReview, ReviewRole: ""}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	aw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "rev-1", "roles": []string{"reviewer"},
	})
	var reg map[string]string
	_ = json.Unmarshal(aw.Body.Bytes(), &reg)
	if reg["agent_id"] == "" {
		t.Fatalf("register: no agent_id (%s)", aw.Body.String())
	}

	cw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/claim",
		map[string]string{"agent_id": reg["agent_id"]})
	if cw.Code != http.StatusOK {
		t.Fatalf("claim: %d (%s)", cw.Code, cw.Body.String())
	}

	got, err := database.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != db.TaskStatusReviewing {
		t.Errorf("status = %q, want REVIEWING", got.Status)
	}
	if got.ReviewRole != reviewer.ID {
		t.Errorf("review_role = %q, want reviewer id %q", got.ReviewRole, reviewer.ID)
	}
}
