package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/server"
)

func newTestServer(t *testing.T) (*server.Server, *db.Database) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Database: config.DatabaseConfig{Path: path},
		Agents: config.AgentConfig{
			HeartbeatIntervalSec: 30,
			TaskTimeoutSec:       300,
		},
	}
	reg := llm.NewRegistry()
	srv := server.New(cfg, database, reg)
	return srv, database
}

func do(t *testing.T, srv *server.Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// Server must expose ServeHTTP for testing.
// (We add a thin method in server.go to delegate to s.mux.)

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/health", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Projects ---

func TestCreateProject(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/projects", map[string]string{
		"name":        "My Project",
		"description": "Test",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var p db.Project
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID == "" {
		t.Error("expected non-empty project ID")
	}
}

func TestCreateProject_MissingName(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/projects", map[string]string{"description": "no name"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListProjects(t *testing.T) {
	srv, _ := newTestServer(t)
	do(t, srv, http.MethodPost, "/api/projects", map[string]string{"name": "P1"})
	do(t, srv, http.MethodPost, "/api/projects", map[string]string{"name": "P2"})

	w := do(t, srv, http.MethodGet, "/api/projects", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var projects []db.Project
	_ = json.Unmarshal(w.Body.Bytes(), &projects)
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestGetProject_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/projects/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- Tasks ---

func createTestProject(t *testing.T, srv *server.Server) string {
	t.Helper()
	w := do(t, srv, http.MethodPost, "/api/projects", map[string]string{"name": "Test"})
	var p db.Project
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	return p.ID
}

func TestCreateTask(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)
	w := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID,
		"type":       "implement",
		"role":       "worker",
		"priority":   5,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var task db.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	if task.Status != db.TaskStatusBacklog {
		t.Errorf("expected status %s, got %s", db.TaskStatusBacklog, task.Status)
	}
}

func TestCreateTask_MissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/tasks", map[string]string{"type": "implement"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestClaimTask(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	// Create task
	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// Register agent
	aw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "worker-1", "roles": []string{"worker"},
	})
	var regResp map[string]string
	_ = json.Unmarshal(aw.Body.Bytes(), &regResp)
	agentID := regResp["agent_id"]

	// Claim
	cw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/claim", map[string]string{"agent_id": agentID})
	if cw.Code != http.StatusOK {
		t.Fatalf("claim: expected 200, got %d: %s", cw.Code, cw.Body.String())
	}

	// Double claim should fail
	dw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/claim", map[string]string{"agent_id": agentID})
	if dw.Code != http.StatusConflict {
		t.Errorf("double claim: expected 409, got %d", dw.Code)
	}
}

// --- Agents ---

func TestRegisterAgent(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name":  "agent-1",
		"roles": []string{"worker"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["agent_id"] == "" {
		t.Error("expected non-empty agent_id")
	}
}

func TestAgentHeartbeat(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "a1", "roles": []string{"worker"},
	})
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	agentID := resp["agent_id"]

	hw := do(t, srv, http.MethodPost, "/api/agents/"+agentID+"/heartbeat", nil)
	if hw.Code != http.StatusOK {
		t.Errorf("heartbeat: expected 200, got %d", hw.Code)
	}
}

func TestGetNextTask(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	// Register agent
	aw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "w1", "roles": []string{"worker"},
	})
	var regResp map[string]string
	_ = json.Unmarshal(aw.Body.Bytes(), &regResp)
	agentID := regResp["agent_id"]

	// No task yet
	w := do(t, srv, http.MethodGet, "/api/agents/"+agentID+"/tasks/next", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Create task
	do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})

	// Now should return a task wrapped in ClaimTaskResponse.
	w2 := do(t, srv, http.MethodGet, "/api/agents/"+agentID+"/tasks/next", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp struct {
		Task map[string]interface{} `json:"task"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp.Task == nil || resp.Task["id"] == "" {
		t.Error("expected non-empty task in response")
	}
}

// --- DELETE /api/tasks/:id ---

func TestDeleteTask(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	// Create a task.
	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	if tw.Code != http.StatusCreated {
		t.Fatalf("create task: expected 201, got %d", tw.Code)
	}
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// Delete it.
	dw := do(t, srv, http.MethodDelete, "/api/tasks/"+task.ID, nil)
	if dw.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", dw.Code, dw.Body.String())
	}

	// Subsequent GET should 404.
	gw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID, nil)
	if gw.Code != http.StatusNotFound {
		t.Errorf("get after delete: expected 404, got %d", gw.Code)
	}
}

func TestDeleteTask_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodDelete, "/api/tasks/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteTask_RemovedFromList(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	// Create two tasks.
	tw1 := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	tw2 := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "review", "role": "worker",
	})
	var t1, t2 db.Task
	_ = json.Unmarshal(tw1.Body.Bytes(), &t1)
	_ = json.Unmarshal(tw2.Body.Bytes(), &t2)

	// Delete t1.
	do(t, srv, http.MethodDelete, "/api/tasks/"+t1.ID, nil)

	// List should contain only t2.
	lw := do(t, srv, http.MethodGet, "/api/tasks", nil)
	var tasks []db.Task
	_ = json.Unmarshal(lw.Body.Bytes(), &tasks)
	for _, tk := range tasks {
		if tk.ID == t1.ID {
			t.Errorf("deleted task %s still appears in list", t1.ID)
		}
	}
}

// --- Agent poll-status ---

func TestAgentPollStatus(t *testing.T) {
	srv, _ := newTestServer(t)

	// Register an agent.
	aw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "poll-agent", "roles": []string{"worker"},
	})
	var regResp map[string]string
	_ = json.Unmarshal(aw.Body.Bytes(), &regResp)
	agentID := regResp["agent_id"]

	// poll-status endpoint should return 200.
	pw := do(t, srv, http.MethodGet, "/api/agents/"+agentID+"/poll-status", nil)
	if pw.Code != http.StatusOK {
		t.Fatalf("poll-status: expected 200, got %d: %s", pw.Code, pw.Body.String())
	}
	var status map[string]interface{}
	_ = json.Unmarshal(pw.Body.Bytes(), &status)
	if _, ok := status["last_polled_at"]; !ok {
		t.Error("expected last_polled_at in poll-status response")
	}
}

// --- Settings ---

func TestListSettings(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/settings", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSetting(t *testing.T) {
	srv, _ := newTestServer(t)

	// Seed a setting first via PUT.
	w := do(t, srv, http.MethodPut, "/api/settings/test.key", map[string]string{"value": "42"})
	if w.Code != http.StatusOK {
		t.Fatalf("PUT setting: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// GET should return the updated value.
	gw := do(t, srv, http.MethodGet, "/api/settings/test.key", nil)
	if gw.Code != http.StatusOK {
		t.Fatalf("GET setting: expected 200, got %d", gw.Code)
	}
	var s db.Setting
	_ = json.Unmarshal(gw.Body.Bytes(), &s)
	if s.Value != "42" {
		t.Errorf("expected value 42, got %s", s.Value)
	}
}

// --- Agent + Task logs endpoints ---

func TestListAgentLogs_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	// LogDB is nil in test server, so endpoint should return empty array.
	w := do(t, srv, http.MethodGet, "/api/agent-logs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListTaskLogs_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/task-logs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// --- State transitions ---

func TestListStateTransitions_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	w := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/transitions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var transitions []db.StateTransition
	_ = json.Unmarshal(w.Body.Bytes(), &transitions)
	if len(transitions) != 0 {
		t.Errorf("expected 0 transitions, got %d", len(transitions))
	}
}

func TestListStateTransitions_AfterClaim(t *testing.T) {
	srv, database := newTestServer(t)
	ctx := t.Context()
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// Record a transition directly via DB.
	_ = database.TransitionTaskState(ctx, task.ID,
		db.TaskStatusBacklog, db.TaskStatusDeveloping, "agent-1", "claimed")

	w := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/transitions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var transitions []db.StateTransition
	_ = json.Unmarshal(w.Body.Bytes(), &transitions)
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].ToState != db.TaskStatusDeveloping {
		t.Errorf("to_state = %q, want %q", transitions[0].ToState, db.TaskStatusDeveloping)
	}
}

// --- Task reviews ---

func TestListReviews_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	w := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/reviews", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var reviews []db.TaskReview
	_ = json.Unmarshal(w.Body.Bytes(), &reviews)
	if len(reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(reviews))
	}
}

func TestCreateAndListReviews(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
		"status": db.TaskStatusReviewing,
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// Post a review.
	rw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/reviews", map[string]interface{}{
		"author_type": "agent",
		"author_role": "reviewer",
		"author_id":   "agent-rev-1",
		"status":      "changes_requested",
		"body":        "Please fix error handling.",
	})
	if rw.Code != http.StatusCreated {
		t.Fatalf("POST review: expected 201, got %d: %s", rw.Code, rw.Body.String())
	}
	var rev db.TaskReview
	_ = json.Unmarshal(rw.Body.Bytes(), &rev)
	if rev.ID == "" {
		t.Error("expected non-empty review ID")
	}

	// List should return it.
	lw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/reviews", nil)
	if lw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", lw.Code)
	}
	var reviews []db.TaskReview
	_ = json.Unmarshal(lw.Body.Bytes(), &reviews)
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].Status != "changes_requested" {
		t.Errorf("status = %q, want changes_requested", reviews[0].Status)
	}
}

func TestCreateReview_MissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	// Missing status.
	w := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/reviews", map[string]interface{}{
		"body": "some feedback",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing status: expected 400, got %d", w.Code)
	}

	// Missing body.
	w2 := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/reviews", map[string]interface{}{
		"status": "approved",
	})
	if w2.Code != http.StatusBadRequest {
		t.Errorf("missing body: expected 400, got %d", w2.Code)
	}
}

func TestGetReviewByID(t *testing.T) {
	srv, _ := newTestServer(t)
	projectID := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": projectID, "type": "implement", "role": "worker",
	})
	var task db.Task
	_ = json.Unmarshal(tw.Body.Bytes(), &task)

	rw := do(t, srv, http.MethodPost, "/api/tasks/"+task.ID+"/reviews", map[string]interface{}{
		"status": "approved",
		"body":   "LGTM",
	})
	var rev db.TaskReview
	_ = json.Unmarshal(rw.Body.Bytes(), &rev)

	gw := do(t, srv, http.MethodGet, "/api/tasks/"+task.ID+"/reviews/"+rev.ID, nil)
	if gw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", gw.Code, gw.Body.String())
	}
	var got db.TaskReview
	_ = json.Unmarshal(gw.Body.Bytes(), &got)
	if got.Body != "LGTM" {
		t.Errorf("body = %q, want LGTM", got.Body)
	}
}
