package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-orchestrator/agent"
	"agent-orchestrator/db"
)

// wrapAgentAPI maps incoming /api/agent/<rest> requests back to /api/<rest> so
// test servers that register the original /api/... routes keep working with the
// namespaced client. Mirrors the server's agentRewrite.
func wrapAgentAPI(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/agent/") {
			r.URL.Path = "/api" + strings.TrimPrefix(r.URL.Path, "/api/agent")
		}
		h.ServeHTTP(w, r)
	})
}

func TestClient_NamespacedPathAndBearer(t *testing.T) {
	var gotPath, gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/roles", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]*db.RoleDefinition{{Name: "worker"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := agent.NewServerClientWithAuth(srv.URL, "tok123", nil)
	roles, err := c.ListRoleDefinitions(context.Background())
	if err != nil {
		t.Fatalf("ListRoleDefinitions: %v", err)
	}
	if gotPath != "/api/agent/roles" {
		t.Errorf("path = %q, want /api/agent/roles", gotPath)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
	if len(roles) != 1 || roles[0].Name != "worker" {
		t.Errorf("roles = %+v", roles)
	}
}

func TestClient_ProvidersWithKeys(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/internal/providers", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]*db.Provider{{Name: "llama.cpp", APIKey: "sk-1"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := agent.NewServerClientWithAuth(srv.URL, "tok", nil)
	provs, err := c.ListProvidersWithKeys(context.Background())
	if err != nil {
		t.Fatalf("ListProvidersWithKeys: %v", err)
	}
	if len(provs) != 1 || provs[0].APIKey != "sk-1" {
		t.Errorf("providers = %+v", provs)
	}
}

func TestClient_NoTokenOmitsAuthHeader(t *testing.T) {
	var hadAuth bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/roles", func(w http.ResponseWriter, r *http.Request) {
		hadAuth = r.Header.Get("Authorization") != ""
		_ = json.NewEncoder(w).Encode([]*db.RoleDefinition{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := agent.NewServerClient(srv.URL)
	if _, err := c.ListRoleDefinitions(context.Background()); err != nil {
		t.Fatalf("ListRoleDefinitions: %v", err)
	}
	if hadAuth {
		t.Error("expected no Authorization header when token is empty")
	}
}
