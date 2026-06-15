package server_test

import (
	"net/http"
	"testing"
)

// --- Project requirements: paths not covered by TestRequirements_CRUD ---

func TestRequirements_LinkedCountAndCrossProject(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)

	w := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/requirements", map[string]interface{}{"title": "Req A"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create requirement: %d %s", w.Code, w.Body.String())
	}
	rid, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	// List attaches a linked_tasks count.
	w = do(t, srv, http.MethodGet, "/api/projects/"+pid+"/requirements", nil)
	list := decodeList(t, w.Body.Bytes())
	if len(list) != 1 {
		t.Fatalf("requirements list = %d, want 1", len(list))
	}
	if _, ok := list[0].(map[string]interface{})["linked_tasks"]; !ok {
		t.Error("requirement list item missing linked_tasks field")
	}

	// Patching the requirement under a different project → 404.
	other := newProject(t, database)
	if w := do(t, srv, http.MethodPatch, "/api/projects/"+other+"/requirements/"+rid, map[string]interface{}{"status": "satisfied"}); w.Code != http.StatusNotFound {
		t.Errorf("patch wrong project: expected 404, got %d", w.Code)
	}
}

// --- Project features: patch/delete not covered by TestFeatures_CRUD ---

func TestFeatures_PatchAndDelete(t *testing.T) {
	srv, database := newTestServer(t)
	pid := newProject(t, database)

	w := do(t, srv, http.MethodPost, "/api/projects/"+pid+"/features", map[string]interface{}{"title": "Feat A"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create feature: %d %s", w.Code, w.Body.String())
	}
	fid, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	w = do(t, srv, http.MethodPatch, "/api/projects/"+pid+"/features/"+fid, map[string]interface{}{"title": "Feat B"})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["title"] != "Feat B" {
		t.Errorf("patch feature: %d", w.Code)
	}

	if w := do(t, srv, http.MethodDelete, "/api/projects/"+pid+"/features/"+fid, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete feature: %d", w.Code)
	}
}

// --- Conversations CRUD (messages are covered by TestConversationMessages) ---

func TestConversations_CRUD(t *testing.T) {
	srv, _ := newTestServer(t)

	w := do(t, srv, http.MethodPost, "/api/conversations", map[string]interface{}{"title": "c1"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create conversation: %d %s", w.Code, w.Body.String())
	}
	id, _ := decodeMap(t, w.Body.Bytes())["id"].(string)

	if w := do(t, srv, http.MethodGet, "/api/conversations", nil); w.Code != http.StatusOK || len(decodeList(t, w.Body.Bytes())) != 1 {
		t.Errorf("list conversations: code=%d", w.Code)
	}

	// Detail returns conversation + messages.
	w = do(t, srv, http.MethodGet, "/api/conversations/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get conversation: %d", w.Code)
	}
	if _, ok := decodeMap(t, w.Body.Bytes())["conversation"]; !ok {
		t.Error("detail response missing conversation key")
	}

	// Update the title.
	w = do(t, srv, http.MethodPut, "/api/conversations/"+id, map[string]interface{}{"title": "c2"})
	if w.Code != http.StatusOK || decodeMap(t, w.Body.Bytes())["title"] != "c2" {
		t.Errorf("update conversation: %d", w.Code)
	}

	// Delete, then detail is 404.
	if w := do(t, srv, http.MethodDelete, "/api/conversations/"+id, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete conversation: %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/conversations/"+id, nil); w.Code != http.StatusNotFound {
		t.Errorf("get deleted conversation: expected 404, got %d", w.Code)
	}
}

func TestChatLog_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/chat-log", nil); w.Code != http.StatusOK {
		t.Errorf("chat-log: expected 200, got %d", w.Code)
	}
}
