package server_test

import (
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

func TestSubagentSkills_CRUD(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/subagent-skills", map[string]interface{}{
		"name":            "investigate_codebase",
		"label":           "Investigate Codebase",
		"prompt_template": "Explore: {{instructions}}",
		"tool_allowlist":  []string{"read_file", "list_files"},
		"max_rounds":      6,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create subagent skill: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if id == "" {
		t.Fatal("subagent skill missing id")
	}

	if w := do(t, srv, http.MethodPost, "/api/subagent-skills", map[string]interface{}{"label": "no name"}); w.Code != http.StatusBadRequest {
		t.Errorf("subagent skill without name: expected 400, got %d", w.Code)
	}

	if w := do(t, srv, http.MethodGet, "/api/subagent-skills", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list subagent skills: %d %s", w.Code, w.Body.String())
	}

	if w := do(t, srv, http.MethodPut, "/api/subagent-skills/"+id, map[string]interface{}{"label": "Renamed"}); w.Code != http.StatusOK {
		t.Errorf("update subagent skill: %d %s", w.Code, w.Body.String())
	}

	if w := do(t, srv, http.MethodDelete, "/api/subagent-skills/"+id, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete subagent skill: %d", w.Code)
	}
}

func TestSubagentSkills_Seed(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/subagent-skills/seed", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("seed subagent skills: %d %s", w.Code, w.Body.String())
	}
	want := float64(len(db.DefaultSubagentSkills()))
	if n, _ := decodeMap(t, w.Body.Bytes())["seeded"].(float64); n != want {
		t.Errorf("expected to seed %v, got %v", want, n)
	}
}
