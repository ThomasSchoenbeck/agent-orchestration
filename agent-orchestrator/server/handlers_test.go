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
	if task.Status != "planned" {
		t.Errorf("expected status planned, got %s", task.Status)
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

	// Now should return a task
	w2 := do(t, srv, http.MethodGet, "/api/agents/"+agentID+"/tasks/next", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var task db.Task
	_ = json.Unmarshal(w2.Body.Bytes(), &task)
	if task.ID == "" {
		t.Error("expected non-empty task")
	}
}
