package integration_test

import (
	"testing"

	"agent-orchestrator/db"
)

// TestMetaSkills_LiveFromDB verifies GET /api/meta/skills returns only enabled
// skill definitions.
func TestMetaSkills_LiveFromDB(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)

	var backend db.SkillDefinition
	if st := apiJSON(t, "POST", srv.BaseURL, "/api/skills",
		map[string]interface{}{"name": "backend", "label": "Backend"}, &backend); st != 201 {
		t.Fatalf("create backend skill: status %d", st)
	}
	var legacy db.SkillDefinition
	if st := apiJSON(t, "POST", srv.BaseURL, "/api/skills",
		map[string]interface{}{"name": "legacy", "label": "Legacy"}, &legacy); st != 201 {
		t.Fatalf("create legacy skill: status %d", st)
	}

	// Disable the legacy skill.
	legacy.Enabled = false
	apiJSON(t, "PUT", srv.BaseURL, "/api/skills/"+legacy.ID, legacy, nil)

	var items []struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	apiJSON(t, "GET", srv.BaseURL, "/api/meta/skills", nil, &items)

	hasBackend, hasLegacy := false, false
	for _, it := range items {
		if it.Value == "backend" {
			hasBackend = true
		}
		if it.Value == "legacy" {
			hasLegacy = true
		}
	}
	if !hasBackend {
		t.Error("expected enabled 'backend' skill in /api/meta/skills")
	}
	if hasLegacy {
		t.Error("disabled 'legacy' skill must not appear in /api/meta/skills")
	}
}
