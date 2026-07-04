package server_test

import (
	"net/http"
	"testing"
)

// handlePlanProject / handleBootstrapProject error branches not covered by the
// happy-path agent planning tests.
func TestAgentPlanProject_MissingArchitecture(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/plan", map[string]interface{}{
		"work_packages": []map[string]interface{}{{"title": "x", "description": "y"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("plan without architecture: expected 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestAgentPlanProject_UnknownProject(t *testing.T) {
	srv, _ := newAgentTestServer(t, "")
	w := agentPost(t, srv, "/api/agent/projects/missing/plan", map[string]interface{}{
		"architecture":  "x",
		"work_packages": []map[string]interface{}{{"title": "x", "description": "y"}},
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("plan unknown project: expected 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestAgentBootstrapProject_Empty(t *testing.T) {
	srv, database := newAgentTestServer(t, "")
	pid := newProject(t, database)
	w := agentPost(t, srv, "/api/agent/projects/"+pid+"/bootstrap", map[string]interface{}{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("bootstrap empty scope: expected 400, got %d %s", w.Code, w.Body.String())
	}
}
