package server_test

import (
	"net/http"
	"testing"
)

func TestRequirements_CRUD(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	base := "/api/projects/" + pid + "/requirements"

	w := do(t, srv, http.MethodPost, base, map[string]interface{}{"title": "User login", "body": "auth"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create requirement: %d %s", w.Code, w.Body.String())
	}
	rid, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if rid == "" {
		t.Fatal("requirement missing id")
	}
	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"body": "no title"}); w.Code != http.StatusBadRequest {
		t.Errorf("requirement without title: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, base, nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list requirements: %d", w.Code)
	}
	if w := do(t, srv, http.MethodPatch, base+"/"+rid, map[string]interface{}{"status": "satisfied"}); w.Code != http.StatusOK {
		t.Errorf("patch requirement: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodDelete, base+"/"+rid, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete requirement: %d", w.Code)
	}
}

func TestFeatures_CRUD(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)
	base := "/api/projects/" + pid + "/features"

	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"title": "Reports", "body": "x"}); w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, base, map[string]interface{}{"body": "no title"}); w.Code != http.StatusBadRequest {
		t.Errorf("feature without title: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, base, nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list features: %d", w.Code)
	}
}

func TestSkills_CRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/skills", map[string]interface{}{"name": "backend", "label": "Backend"}); w.Code != http.StatusCreated {
		t.Fatalf("create skill: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/skills", map[string]interface{}{"label": "no name"}); w.Code != http.StatusBadRequest {
		t.Errorf("skill without name: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/skills", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list skills: %d", w.Code)
	}
}

func TestAgentTemplates_CRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/agent-templates", map[string]interface{}{"name": "tpl1", "roles": []string{"worker"}})
	if w.Code != http.StatusCreated {
		t.Fatalf("create template: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if id == "" {
		t.Fatal("template missing id")
	}
	if w := do(t, srv, http.MethodPost, "/api/agent-templates", map[string]interface{}{"roles": []string{"worker"}}); w.Code != http.StatusBadRequest {
		t.Errorf("template without name: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/agent-templates", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list templates: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/agent-templates/"+id, nil); w.Code != http.StatusOK {
		t.Errorf("get template detail: %d", w.Code)
	}
}

func TestConversationsAndChatLog(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/conversations", map[string]interface{}{"title": "chat1"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create conversation: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)
	if id == "" {
		t.Fatal("conversation missing id")
	}
	if w := do(t, srv, http.MethodGet, "/api/conversations", nil); w.Code != http.StatusOK {
		t.Errorf("list conversations: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/conversations/"+id, nil); w.Code != http.StatusOK {
		t.Errorf("get conversation detail: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/chat-log", nil); w.Code != http.StatusOK {
		t.Errorf("chat-log: %d", w.Code)
	}
}

func TestMetricsEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/api/metrics", "/api/metrics/tokens", "/api/metrics/costs"} {
		if w := do(t, srv, http.MethodGet, path, nil); w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d %s", path, w.Code, w.Body.String())
		}
	}
}
