package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/server"
)

func agentPost(t *testing.T, srv *server.Server, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func newProject(t *testing.T, database *db.Database) string {
	t.Helper()
	p := &db.Project{Name: "Test Project"}
	if err := database.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p.ID
}

func TestAgentPlanProject_CreatesTasksAndContext(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)

	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/plan", map[string]interface{}{
		"architecture": "layered services",
		"work_packages": []map[string]interface{}{
			{"title": "build api", "description": "the api", "role": "worker", "priority": 3},
			{"title": "build ui", "description": "the ui"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("plan: expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	tasks, err := database.ListTasks(context.Background(), db.TaskFilters{ProjectID: pid})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks created, got %d", len(tasks))
	}
}

func TestAgentPlanProject_RejectsEmptyWorkPackages(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/plan", map[string]interface{}{
		"architecture": "x", "work_packages": []map[string]interface{}{},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty work_packages, got %d", w.Code)
	}
}

func TestAgentCreateWorkPackage_CreatesTask(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)

	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/work-packages", map[string]interface{}{
		"title": "do a thing", "description": "details here",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("work-packages: expected 201, got %d (%s)", w.Code, w.Body.String())
	}
	tasks, _ := database.ListTasks(context.Background(), db.TaskFilters{ProjectID: pid})
	if len(tasks) != 1 || tasks[0].Role != "worker" {
		t.Errorf("expected 1 worker task, got %+v", tasks)
	}
}

func TestAgentBootstrapProject_CreatesThenSkips(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)

	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/bootstrap", map[string]interface{}{
		"requirements": []map[string]string{{"title": "R1", "body": "must"}},
		"features":     []map[string]string{{"title": "F1", "body": "do"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	reqs, _ := database.ListRequirements(context.Background(), pid)
	feats, _ := database.ListFeatures(context.Background(), pid)
	if len(reqs) != 1 || len(feats) != 1 {
		t.Fatalf("expected 1 req + 1 feat, got %d/%d", len(reqs), len(feats))
	}

	// Second call must skip (scope already exists).
	w2 := agentPost(t, srv, "/api/agent/projects/"+pid+"/bootstrap", map[string]interface{}{
		"requirements": []map[string]string{{"title": "R2", "body": "x"}},
	})
	var resp map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["skipped"] != true {
		t.Errorf("expected skipped=true on second bootstrap, got %v", resp)
	}
}

func TestAgentSyncScope_AddsAndFlags(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)

	// Seed scope with R1.
	if w := agentPost(t, srv, "/api/agent/projects/"+pid+"/bootstrap", map[string]interface{}{
		"requirements": []map[string]string{{"title": "R1", "body": "must"}},
	}); w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d", w.Code)
	}

	// Sync with a desired set that drops R1 and adds R2.
	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/sync-scope", map[string]interface{}{
		"requirements": []map[string]string{{"title": "R2", "body": "new"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("sync-scope: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Added   []string `json:"added"`
		Flagged []string `json:"flagged"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Added) != 1 || resp.Added[0] != "requirement: R2" {
		t.Errorf("expected R2 added, got %v", resp.Added)
	}
	if len(resp.Flagged) != 1 || resp.Flagged[0] != "requirement: R1" {
		t.Errorf("expected R1 flagged, got %v", resp.Flagged)
	}
}

func TestAgentNextTask_Peek(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	task := &db.Task{
		ProjectID: pid, Role: "worker", Status: db.TaskStatusBacklog, Priority: 5,
		Payload: map[string]interface{}{"title": "x"},
	}
	if err := database.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	w := agentReq(t, srv, http.MethodGet, "/api/agent/tasks/next?roles=worker", "")
	if w.Code != http.StatusOK {
		t.Fatalf("next-task peek: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Task *db.Task `json:"task"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Task == nil || resp.Task.ID != task.ID {
		t.Errorf("expected task %s, got %+v", task.ID, resp.Task)
	}
}

func TestAgentCompleteProject_SatisfiedAndBlocked(t *testing.T) {
	srv, database := newAgentTestServer(t, "")

	// Empty project → scope vacuously satisfied → completes.
	pid := newProject(t, database)
	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/complete", map[string]interface{}{"summary": "done"})
	if w.Code != http.StatusOK {
		t.Fatalf("complete (satisfied): expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	p, _ := database.GetProject(context.Background(), pid)
	if p.Status != "complete" || p.AutoQueue {
		t.Errorf("project not marked complete/disarmed: status=%q autoQueue=%v", p.Status, p.AutoQueue)
	}

	// Project with an open (backlog) task → blocked.
	pid2 := newProject(t, database)
	if w := agentPost(t, srv, "/api/agent/projects/"+pid2+"/work-packages", map[string]interface{}{
		"title": "open work", "description": "pending",
	}); w.Code != http.StatusCreated {
		t.Fatalf("seed task: %d", w.Code)
	}
	wBlocked := agentPost(t, srv, "/api/agent/projects/"+pid2+"/complete", map[string]interface{}{})
	if wBlocked.Code != http.StatusConflict {
		t.Errorf("complete (blocked): expected 409, got %d", wBlocked.Code)
	}
}
