package server_test

import (
	"net/http"
	"testing"
)

// --- Settings handlers ---

func TestSettings_RoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)

	// Fresh DB: no settings yet.
	if w := do(t, srv, http.MethodGet, "/api/settings", nil); w.Code != http.StatusOK {
		t.Fatalf("list settings: %d %s", w.Code, w.Body.String())
	} else if n := len(decodeList(t, w.Body.Bytes())); n != 0 {
		t.Errorf("fresh settings list = %d, want 0", n)
	}

	// PUT creates/updates a setting and echoes it back.
	w := do(t, srv, http.MethodPut, "/api/settings/feature.flag", map[string]interface{}{
		"value": "on", "description": "a flag",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put setting: %d %s", w.Code, w.Body.String())
	}
	m := decodeMap(t, w.Body.Bytes())
	if m["key"] != "feature.flag" || m["value"] != "on" {
		t.Errorf("put response = %+v, want key=feature.flag value=on", m)
	}

	// GET the detail reflects the stored value.
	w = do(t, srv, http.MethodGet, "/api/settings/feature.flag", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get setting: %d %s", w.Code, w.Body.String())
	}
	if decodeMap(t, w.Body.Bytes())["value"] != "on" {
		t.Error("stored value not returned")
	}

	// The list now contains the one setting.
	if w := do(t, srv, http.MethodGet, "/api/settings", nil); len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Error("settings list should contain 1 entry after PUT")
	}
}

func TestSettings_GetMissingReturns404(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/settings/does-not-exist", nil); w.Code != http.StatusNotFound {
		t.Errorf("missing setting: expected 404, got %d", w.Code)
	}
}

func TestSettings_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/settings", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/settings: expected 405, got %d", w.Code)
	}
}

func TestSettings_EmptyKeyReturns404(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/settings/", nil); w.Code != http.StatusNotFound {
		t.Errorf("empty key: expected 404, got %d", w.Code)
	}
}

// --- Meta handlers ---

func TestMetaTaskRoles(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/meta/task-roles", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("task-roles: %d %s", w.Code, w.Body.String())
	}
	list := decodeList(t, w.Body.Bytes())
	if len(list) == 0 {
		t.Fatal("task-roles list is empty")
	}
	found := false
	for _, it := range list {
		if m, ok := it.(map[string]interface{}); ok && m["value"] == "worker" {
			found = true
		}
	}
	if !found {
		t.Error("task-roles should include the worker role")
	}
}

// Note: /api/meta/tools is covered by TestMetaTools in handlers_crud_test.go.

func TestMetaSkills(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/meta/skills", nil); w.Code != http.StatusOK {
		t.Errorf("meta skills: expected 200, got %d", w.Code)
	}
}

func TestMeta_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/meta/tools", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/meta/tools: expected 405, got %d", w.Code)
	}
}
