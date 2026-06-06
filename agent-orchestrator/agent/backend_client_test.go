package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

func TestBackendClient_ListTasksAndPlan(t *testing.T) {
	var listQuery, planPath string
	var planBody map[string]interface{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/tasks", func(w http.ResponseWriter, r *http.Request) {
		listQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]*db.Task{{ID: "t1"}})
	})
	mux.HandleFunc("/api/agent/projects/p1/plan", func(w http.ResponseWriter, r *http.Request) {
		planPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&planBody)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := agent.NewServerClient(srv.URL)
	ctx := context.Background()

	tasks, err := c.ListTasks(ctx, db.TaskFilters{ProjectID: "p1", Limit: 5})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Errorf("tasks = %+v", tasks)
	}
	if !strings.Contains(listQuery, "project_id=p1") || !strings.Contains(listQuery, "limit=5") {
		t.Errorf("query missing filters: %q", listQuery)
	}

	res, err := c.PlanProject(ctx, "p1", "arch", []tools.WorkPackageInput{{Title: "a", Priority: 2}})
	if err != nil {
		t.Fatalf("PlanProject: %v", err)
	}
	if planPath != "/api/agent/projects/p1/plan" {
		t.Errorf("plan path = %q", planPath)
	}
	if planBody["architecture"] != "arch" {
		t.Errorf("plan body architecture = %v", planBody["architecture"])
	}
	if res["success"] != true {
		t.Errorf("plan result = %v", res)
	}
}
