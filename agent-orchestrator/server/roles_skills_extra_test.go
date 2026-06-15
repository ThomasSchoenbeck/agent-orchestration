package server_test

import (
	"net/http"
	"testing"
)

// --- Roles: paths not covered by TestRoles_CRUD ---

func TestRoleCreate_AppliesDefaultTools(t *testing.T) {
	srv, _ := newTestServer(t)
	// Creating a well-known role without allowed_tools should pre-populate the
	// suggested tool set (defaultToolsForRole).
	w := do(t, srv, http.MethodPost, "/api/roles", map[string]interface{}{"name": "worker"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create worker role: %d %s", w.Code, w.Body.String())
	}
	tools, _ := decodeMap(t, w.Body.Bytes())["allowed_tools"].([]interface{})
	found := false
	for _, tl := range tools {
		if tl == "read_file" {
			found = true
		}
	}
	if !found {
		t.Errorf("worker role allowed_tools = %v, want it to include read_file", tools)
	}
}

func TestRolePreviewPrompt_RendersTemplate(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/roles", map[string]interface{}{
		"name": "greeter", "system_prompt": "Hello {{.who}}",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create role: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	w = do(t, srv, http.MethodPost, "/api/roles/"+id+"/preview-prompt", map[string]interface{}{"who": "world"})
	if w.Code != http.StatusOK {
		t.Fatalf("preview-prompt: %d %s", w.Code, w.Body.String())
	}
	if got := decodeMap(t, w.Body.Bytes())["rendered"]; got != "Hello world" {
		t.Errorf("rendered = %q, want %q", got, "Hello world")
	}
}

func TestRolePreviewPrompt_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/roles", map[string]interface{}{"name": "r1"})
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if w := do(t, srv, http.MethodGet, "/api/roles/"+id+"/preview-prompt", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET preview-prompt: expected 405, got %d", w.Code)
	}
}

func TestRole_GetMissingReturns404(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/roles/nope", nil); w.Code != http.StatusNotFound {
		t.Errorf("missing role: expected 404, got %d", w.Code)
	}
}

func TestRoleRename_UpdatesName(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/roles", map[string]interface{}{"name": "alpha"})
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	w = do(t, srv, http.MethodPut, "/api/roles/"+id, map[string]interface{}{"name": "beta"})
	if w.Code != http.StatusOK {
		t.Fatalf("rename role: %d %s", w.Code, w.Body.String())
	}
	if got := decodeMap(t, w.Body.Bytes())["name"]; got != "beta" {
		t.Errorf("renamed role name = %q, want beta", got)
	}
	// The renamed role is retrievable under its id with the new name.
	w = do(t, srv, http.MethodGet, "/api/roles/"+id, nil)
	if decodeMap(t, w.Body.Bytes())["name"] != "beta" {
		t.Error("GET after rename did not reflect the new name")
	}
}

// --- Skills: paths not covered by TestSkills_CRUD ---

func TestSkillSeed_ImportsDefaults(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/skills/seed", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("seed skills: %d %s", w.Code, w.Body.String())
	}
	seeded, _ := decodeMap(t, w.Body.Bytes())["seeded"].(float64)
	if seeded < 1 {
		t.Errorf("seeded = %v, want >= 1", seeded)
	}
	// The seeded skills are now listed.
	if w := do(t, srv, http.MethodGet, "/api/skills", nil); len(decodeList(t, w.Body.Bytes())) < 1 {
		t.Error("skills list should be non-empty after seeding")
	}
}

func TestSkillSeed_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/skills/seed", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/skills/seed: expected 405, got %d", w.Code)
	}
}

func TestSkill_GetMissingReturns404(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/skills/nope", nil); w.Code != http.StatusNotFound {
		t.Errorf("missing skill: expected 404, got %d", w.Code)
	}
}
