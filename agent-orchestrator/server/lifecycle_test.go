package server_test

// lifecycle_test.go exercises the agentic dev lifecycle through the HTTP API:
//   worker claims → submits for review → reviewer claims → posts review
// It verifies state transitions are recorded and visible via the API.

import (
	"encoding/json"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// helpers: newTestServer, do, createTestProject are in handlers_test.go

func TestLifecycle_SubmitForReviewAndPostReview(t *testing.T) {
	srv, _ := newTestServer(t)

	projectID := createTestProject(t, srv)

	// --- Create task ---
	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID,
		"type":       "implement",
		"role":       "worker",
		"priority":   5,
	})
	if tw.Code != http.StatusCreated {
		t.Fatalf("create task: expected 201, got %d: %s", tw.Code, tw.Body.String())
	}
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)
	if task.Status != db.TaskStatusBacklog {
		t.Fatalf("initial status = %q, want BACKLOG", task.Status)
	}

	// --- Register worker agent ---
	aw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "worker-1", "roles": []string{"worker"},
	})
	var regW map[string]string
	_ = json.Unmarshal(aw.Body.Bytes(), &regW)
	workerID := regW["agent_id"]

	// --- Claim task (worker) → DEVELOPING ---
	cw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/claim", map[string]string{"agent_id": workerID})
	if cw.Code != http.StatusOK {
		t.Fatalf("claim: expected 200, got %d: %s", cw.Code, cw.Body.String())
	}
	// Verify DEVELOPING status.
	gw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID, nil)
	var claimed db.Task
	_ = json.Unmarshal(gw.Body.Bytes(), &claimed)
	if claimed.Status != db.TaskStatusDeveloping {
		t.Fatalf("after claim: status = %q, want DEVELOPING", claimed.Status)
	}

	// --- Submit for review → AWAITING_REVIEW (records transition) ---
	sw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/submit-for-review", nil)
	if sw.Code != http.StatusOK {
		t.Fatalf("submit-for-review: expected 200, got %d: %s", sw.Code, sw.Body.String())
	}
	var submitted db.Task
	_ = json.Unmarshal(sw.Body.Bytes(), &submitted)
	if submitted.Status != db.TaskStatusAwaitingReview {
		t.Fatalf("after submit: status = %q, want AWAITING_REVIEW", submitted.Status)
	}

	// --- Register reviewer agent ---
	ar := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "reviewer-1", "roles": []string{"reviewer"},
	})
	var regR map[string]string
	_ = json.Unmarshal(ar.Body.Bytes(), &regR)
	reviewerID := regR["agent_id"]

	// --- Claim for review (reviewer) → REVIEWING ---
	cr := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/claim", map[string]string{"agent_id": reviewerID})
	if cr.Code != http.StatusOK {
		t.Fatalf("reviewer claim: expected 200, got %d: %s", cr.Code, cr.Body.String())
	}
	gr := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID, nil)
	var reviewing db.Task
	_ = json.Unmarshal(gr.Body.Bytes(), &reviewing)
	if reviewing.Status != db.TaskStatusReviewing {
		t.Fatalf("after reviewer claim: status = %q, want REVIEWING", reviewing.Status)
	}

	// --- Post review (changes_requested) → AWAITING_REVISION (records transition) ---
	rw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/reviews", map[string]interface{}{
		"author_type": "agent",
		"author_role": "reviewer",
		"author_id":   reviewerID,
		"status":      "changes_requested",
		"body":        "Please handle the error case in line 42.",
	})
	if rw.Code != http.StatusCreated {
		t.Fatalf("post review: expected 201, got %d: %s", rw.Code, rw.Body.String())
	}
	final := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID, nil)
	var finalTask db.Task
	_ = json.Unmarshal(final.Body.Bytes(), &finalTask)
	if finalTask.Status != db.TaskStatusAwaitingRevision {
		t.Fatalf("after review: status = %q, want AWAITING_REVISION", finalTask.Status)
	}

	// --- Verify state-transition history ---
	tw2 := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/transitions", nil)
	if tw2.Code != http.StatusOK {
		t.Fatalf("transitions: expected 200, got %d", tw2.Code)
	}
	var transitions []db.StateTransition
	_ = json.Unmarshal(tw2.Body.Bytes(), &transitions)

	// submit-for-review records DEVELOPING→AWAITING_REVIEW;
	// POST review records REVIEWING→AWAITING_REVISION.
	if len(transitions) < 2 {
		t.Fatalf("expected at least 2 transitions, got %d: %+v", len(transitions), transitions)
	}

	var devToReview, reviewToRevision bool
	for _, tr := range transitions {
		if tr.FromState == db.TaskStatusDeveloping && tr.ToState == db.TaskStatusAwaitingReview {
			devToReview = true
		}
		if tr.FromState == db.TaskStatusReviewing && tr.ToState == db.TaskStatusAwaitingRevision {
			reviewToRevision = true
		}
	}
	if !devToReview {
		t.Error("missing transition DEVELOPING → AWAITING_REVIEW")
	}
	if !reviewToRevision {
		t.Error("missing transition REVIEWING → AWAITING_REVISION")
	}

	// --- Verify reviews endpoint ---
	lrw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/reviews", nil)
	var reviews []db.TaskReview
	_ = json.Unmarshal(lrw.Body.Bytes(), &reviews)
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].Status != "changes_requested" {
		t.Errorf("review status = %q, want changes_requested", reviews[0].Status)
	}
	if reviews[0].AuthorID != reviewerID {
		t.Errorf("review author_id = %q, want %q", reviews[0].AuthorID, reviewerID)
	}
}

func TestLifecycle_ApprovedReview_TransitionsToAwaitingMerge(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// Worker claims + submits.
	aw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "w2", "roles": []string{"worker"},
	})
	var rw map[string]string
	_ = json.Unmarshal(aw.Body.Bytes(), &rw)
	workerID := rw["agent_id"]

	do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/claim", map[string]string{"agent_id": workerID})
	do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/submit-for-review", nil)

	// Reviewer claims.
	ar := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "r2", "roles": []string{"reviewer"},
	})
	var rr map[string]string
	_ = json.Unmarshal(ar.Body.Bytes(), &rr)
	reviewerID := rr["agent_id"]

	do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/claim", map[string]string{"agent_id": reviewerID})

	// Reviewer approves.
	do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/reviews", map[string]interface{}{
		"status": "approved",
		"body":   "LGTM, ship it.",
	})

	gw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID, nil)
	var final db.Task
	_ = json.Unmarshal(gw.Body.Bytes(), &final)
	if final.Status != db.TaskStatusAwaitingMerge {
		t.Errorf("after approval: status = %q, want AWAITING_MERGE", final.Status)
	}
}

func TestAgent_OfflineEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	// Register an agent.
	rw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "agent-offline-test", "roles": []string{"worker"},
	})
	if rw.Code != http.StatusCreated && rw.Code != http.StatusOK {
		t.Fatalf("register: expected 2xx, got %d: %s", rw.Code, rw.Body.String())
	}
	var reg map[string]string
	_ = json.Unmarshal(rw.Body.Bytes(), &reg)
	agentID := reg["agent_id"]

	// Agent should start as online/idle.
	gw := do(t, srv, http.MethodGet, "/api/agents/"+agentID, nil)
	var a db.Agent
	_ = json.Unmarshal(gw.Body.Bytes(), &a)
	if a.Status == "offline" {
		t.Fatalf("expected agent to be online after registration, got %q", a.Status)
	}

	// POST /offline — simulates clean shutdown.
	ow := do(t, srv, http.MethodPost, "/api/agents/"+agentID+"/offline", nil)
	if ow.Code != http.StatusOK {
		t.Fatalf("offline: expected 200, got %d: %s", ow.Code, ow.Body.String())
	}

	// Agent should now be offline immediately.
	gw2 := do(t, srv, http.MethodGet, "/api/agents/"+agentID, nil)
	var a2 db.Agent
	_ = json.Unmarshal(gw2.Body.Bytes(), &a2)
	if a2.Status != "offline" {
		t.Errorf("expected agent status offline after deregistration, got %q", a2.Status)
	}
}

func TestAgent_OfflineEndpoint_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	ow := do(t, srv, http.MethodPost, "/api/agents/nonexistent-id/offline", nil)
	if ow.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown agent, got %d", ow.Code)
	}
}
