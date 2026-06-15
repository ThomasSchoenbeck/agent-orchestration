package server_test

import (
	"net/http"
	"testing"
)

// handleLLMChat validation paths (the success path routes to a live provider and
// is exercised by the integration suite).

func TestLLMChat_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodGet, "/api/llm/chat", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/llm/chat: expected 405, got %d", w.Code)
	}
}

func TestLLMChat_MissingMessages(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/llm/chat", map[string]interface{}{"role": "worker"}); w.Code != http.StatusBadRequest {
		t.Errorf("no messages: expected 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestLLMChat_MissingRole(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]interface{}{
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	if w := do(t, srv, http.MethodPost, "/api/llm/chat", body); w.Code != http.StatusBadRequest {
		t.Errorf("no role: expected 400, got %d %s", w.Code, w.Body.String())
	}
}
