package server_test

import (
	"net/http"
	"testing"
)

// handleAgentTemplateDetail: PATCH / stop / DELETE / 404 / 405.
// (scale + start spawn real agents via a launcher, so they're left to integration.)
func TestAgentTemplateDetail(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/agent-templates", map[string]interface{}{
		"name": "tpl", "roles": []string{"worker"}, "replicas": 1,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create template: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	// PATCH with replicas unchanged → no supervisor scaling, just a DB update.
	w = do(t, srv, http.MethodPatch, "/api/agent-templates/"+id, map[string]interface{}{
		"name": "tpl-renamed", "roles": []string{"worker"}, "replicas": 1,
	})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["name"] != "tpl-renamed" {
		t.Errorf("PATCH template: %d %s", w.Code, w.Body.String())
	}

	// stop action (no running instances → no-op) → 200.
	if w := do(t, srv, http.MethodPost, "/api/agent-templates/"+id+"/stop", nil); w.Code != http.StatusOK {
		t.Errorf("stop template: %d %s", w.Code, w.Body.String())
	}

	// scale requires POST → GET is 405.
	if w := do(t, srv, http.MethodGet, "/api/agent-templates/"+id+"/scale", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET scale: expected 405, got %d", w.Code)
	}

	// GET a non-existent template → 404.
	if w := do(t, srv, http.MethodGet, "/api/agent-templates/nope", nil); w.Code != http.StatusNotFound {
		t.Errorf("GET missing template: expected 404, got %d", w.Code)
	}

	// DELETE (no running instances) → 204.
	if w := do(t, srv, http.MethodDelete, "/api/agent-templates/"+id, nil); w.Code != http.StatusNoContent {
		t.Errorf("DELETE template: %d %s", w.Code, w.Body.String())
	}
}
