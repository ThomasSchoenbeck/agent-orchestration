package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// TestResult_CodeTaskCannotCompleteSkippingReview reproduces B1: a code
// (non-planner) task must not reach COMPLETED via the result endpoint, because
// that skips the review → merge gate and leaves the branch unmerged. Both an
// empty status (which defaults to "completed") and an explicit "completed" are
// redirected to AWAITING_REVIEW.
func TestResult_CodeTaskCannotCompleteSkippingReview(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()
	workerID := seedRole(t, database, "worker") // no creates_tasks capability
	pid := newProject(t, database)

	cases := []struct {
		name string
		body interface{}
	}{
		{"empty status defaults to completed", map[string]interface{}{}},
		{"explicit completed", map[string]interface{}{"status": "completed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &db.Task{ProjectID: pid, Role: workerID, Status: db.TaskStatusDeveloping}
			if err := database.CreateTask(ctx, task); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}

			w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/result", tc.body)
			if w.Code != http.StatusOK {
				t.Fatalf("result: got %d (%s)", w.Code, w.Body.String())
			}

			got, err := database.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if got.Status == db.TaskStatusCompleted {
				t.Fatalf("worker task reached COMPLETED via result endpoint — skipped review/merge")
			}
			if got.Status != db.TaskStatusAwaitingReview {
				t.Errorf("status = %q, want AWAITING_REVIEW", got.Status)
			}
		})
	}
}

// TestResult_PlannerTaskCompletesDirectly verifies the guard does not block the
// legitimate direct-complete path: a creates_tasks (planner) task produces no
// code and completes directly via the result endpoint.
func TestResult_PlannerTaskCompletesDirectly(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := context.Background()
	planner := &db.RoleDefinition{
		Name: "planner", Label: "Planner",
		Capabilities: []string{"creates_tasks"}, Enabled: true,
	}
	if err := database.CreateRoleDefinition(ctx, planner); err != nil {
		t.Fatalf("CreateRoleDefinition: %v", err)
	}
	pid := newProject(t, database)
	task := &db.Task{ProjectID: pid, Role: planner.ID, Status: db.TaskStatusDeveloping}
	if err := database.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/result",
		map[string]interface{}{"status": "completed"})
	if w.Code != http.StatusOK {
		t.Fatalf("result: got %d (%s)", w.Code, w.Body.String())
	}

	got, err := database.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != db.TaskStatusCompleted {
		t.Errorf("planner status = %q, want COMPLETED", got.Status)
	}
}
