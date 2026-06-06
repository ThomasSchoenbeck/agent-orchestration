package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/server"
)

// newAgentTestServer builds a server with the given agent API key (empty = open)
// and a provider that carries a secret API key, for namespace/auth tests.
func newAgentTestServer(t *testing.T, agentKey string) (*server.Server, *db.Database) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := database.CreateProvider(context.Background(), &db.Provider{
		Name: "llama.cpp", Type: "openai_compatible",
		BaseURL: "http://localhost:7777/v1", APIKey: "sk-secret-123", Enabled: true,
	}); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Database: config.DatabaseConfig{Path: path},
		Agents:   config.AgentConfig{APIKey: agentKey},
		Storage:  config.StorageConfig{Root: t.TempDir()},
	}
	return server.New(cfg, database, llm.NewRegistry()), database
}

func agentReq(t *testing.T, srv *server.Server, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestAgentNamespace_OpenWhenNoKey(t *testing.T) {
	srv, _ := newAgentTestServer(t, "")
	w := agentReq(t, srv, http.MethodGet, "/api/agent/internal/providers", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no key configured, got %d", w.Code)
	}
}

func TestAgentNamespace_RequiresKeyWhenSet(t *testing.T) {
	srv, _ := newAgentTestServer(t, "topsecret")

	if w := agentReq(t, srv, http.MethodGet, "/api/agent/internal/providers", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: expected 401, got %d", w.Code)
	}
	if w := agentReq(t, srv, http.MethodGet, "/api/agent/internal/providers", "wrong"); w.Code != http.StatusUnauthorized {
		t.Errorf("bad token: expected 401, got %d", w.Code)
	}
	if w := agentReq(t, srv, http.MethodGet, "/api/agent/internal/providers", "topsecret"); w.Code != http.StatusOK {
		t.Errorf("valid token: expected 200, got %d", w.Code)
	}
}

// TestInternalProviders_IncludesKeys_UIStrips verifies the agent endpoint ships
// provider API keys while the UI endpoint strips them.
func TestInternalProviders_IncludesKeys_UIStrips(t *testing.T) {
	srv, _ := newAgentTestServer(t, "topsecret")

	// Agent endpoint (authed) → includes the secret key.
	w := agentReq(t, srv, http.MethodGet, "/api/agent/internal/providers", "topsecret")
	if w.Code != http.StatusOK {
		t.Fatalf("agent providers: expected 200, got %d", w.Code)
	}
	var agentProvs []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &agentProvs); err != nil {
		t.Fatalf("decode agent providers: %v", err)
	}
	if len(agentProvs) != 1 || agentProvs[0]["api_key"] != "sk-secret-123" {
		t.Errorf("agent endpoint should include api_key, got %v", agentProvs)
	}

	// UI endpoint (no key required) → strips the secret key.
	wUI := agentReq(t, srv, http.MethodGet, "/api/providers", "")
	if wUI.Code != http.StatusOK {
		t.Fatalf("UI providers: expected 200, got %d", wUI.Code)
	}
	var uiProvs []map[string]interface{}
	if err := json.Unmarshal(wUI.Body.Bytes(), &uiProvs); err != nil {
		t.Fatalf("decode UI providers: %v", err)
	}
	if len(uiProvs) != 1 {
		t.Fatalf("expected 1 UI provider, got %d", len(uiProvs))
	}
	if _, present := uiProvs[0]["api_key"]; present {
		t.Errorf("UI endpoint must not expose api_key, got %v", uiProvs[0])
	}
}
