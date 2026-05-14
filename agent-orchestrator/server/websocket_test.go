package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"agent-orchestrator/server"
)

// TestWSMessage_JSONRoundTrip verifies that WSMessage serialises and
// deserialises correctly, including the optional provider_id field.
func TestWSMessage_JSONRoundTrip(t *testing.T) {
	msg := server.WSMessage{
		Type:       "chat",
		Role:       "worker",
		Content:    "Hello from user",
		ProviderID: "prov-abc",
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got server.WSMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != msg.Type {
		t.Errorf("Type: want %q got %q", msg.Type, got.Type)
	}
	if got.Role != msg.Role {
		t.Errorf("Role: want %q got %q", msg.Role, got.Role)
	}
	if got.Content != msg.Content {
		t.Errorf("Content: want %q got %q", msg.Content, got.Content)
	}
	if got.ProviderID != msg.ProviderID {
		t.Errorf("ProviderID: want %q got %q", msg.ProviderID, got.ProviderID)
	}
}

// TestWSMessage_ProviderID_OmitEmpty verifies that an empty provider_id is
// omitted from the JSON output (omitempty tag).
func TestWSMessage_ProviderID_OmitEmpty(t *testing.T) {
	msg := server.WSMessage{Type: "ping", Role: "user", Content: "hi"}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	_ = json.Unmarshal(b, &raw)
	if _, ok := raw["provider_id"]; ok {
		t.Error("expected provider_id to be omitted when empty")
	}
}

// TestWSChat_MethodNotAllowed verifies that non-GET requests to /ws/chat
// receive a 405 response (the WS upgrade guard).
func TestWSChat_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := do(t, srv, method, "/ws/chat", nil)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /ws/chat: expected 405, got %d", method, w.Code)
		}
	}
}

// TestWSChat_UpgradeRequired verifies that a plain GET to /ws/chat without
// upgrade headers results in a non-200 response (400 Bad Request is typical).
func TestWSChat_UpgradeRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/ws/chat", nil)
	// Without the Upgrade: websocket header the server should refuse the connection.
	if w.Code == http.StatusOK {
		t.Errorf("GET /ws/chat without WS headers: expected non-200, got 200")
	}
}

// TestWSMessage_TypeField verifies that the type field controls message routing.
// We test the known types by checking JSON keys.
func TestWSMessage_KnownTypes(t *testing.T) {
	types := []string{"chat", "ping", "pong", "error"}
	for _, typ := range types {
		msg := server.WSMessage{Type: typ}
		b, _ := json.Marshal(msg)
		var raw map[string]interface{}
		_ = json.Unmarshal(b, &raw)
		if raw["type"] != typ {
			t.Errorf("expected type=%q in JSON, got %v", typ, raw["type"])
		}
	}
}
