package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

func TestBuildUserMessage_IncludesProjectID(t *testing.T) {
	e := &Executor{}
	task := &db.Task{
		ID:           "task-1",
		ProjectID:    "proj-9",
		Role:         "orchestrator",
		WorktreePath: "/work/task-1", // set so buildUserMessage doesn't touch e.log
		Payload:      map[string]interface{}{"title": "Re-sync project scope"},
	}
	msg := e.buildUserMessage(task)
	if !strings.Contains(msg, "Project ID: proj-9") {
		t.Errorf("user message must include the project id; got:\n%s", msg)
	}
	if !strings.Contains(msg, "Re-sync project scope") {
		t.Errorf("user message must include the title; got:\n%s", msg)
	}
}

func TestIsPlannerTask(t *testing.T) {
	reg := llm.NewRegistry()
	reg.Set("p", llm.NewOllamaProvider("p", "http://localhost:11434", "m"))
	rtr := router.New(&config.Config{}, reg)
	providers := []*db.Provider{{
		Name: "p", ModelName: "m", Enabled: true,
		Models: []db.ProviderModel{{Name: "m", Roles: []string{"orchestrator", "worker"}}},
	}}
	roles := []*db.RoleDefinition{
		{Name: "orchestrator", Enabled: true, Capabilities: []string{"creates_tasks"}},
		{Name: "worker", Enabled: true},
	}
	if err := rtr.LoadFromData(providers, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}
	e := &Executor{rtr: rtr}

	if !e.IsPlannerTask(&db.Task{Role: "orchestrator"}) {
		t.Error("orchestrator (creates_tasks) should be a planner task")
	}
	if e.IsPlannerTask(&db.Task{Role: "worker"}) {
		t.Error("worker should not be a planner task")
	}
}

func TestPlanningContext_IncludesDescriptionAndScope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/projects/p1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "p1", "description": "Track expenses with login"})
	})
	mux.HandleFunc("/api/agent/projects/p1/requirements", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "r1", "title": "User login", "status": "accepted"}})
	})
	mux.HandleFunc("/api/agent/projects/p1/features", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"id": "f1", "title": "Reports", "status": "planned"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := &Executor{client: NewServerClient(srv.URL)}
	pc := e.planningContext(context.Background(), "p1")
	for _, want := range []string{"Track expenses with login", "User login", "Reports"} {
		if !strings.Contains(pc, want) {
			t.Errorf("planning context missing %q; got:\n%s", want, pc)
		}
	}
}
