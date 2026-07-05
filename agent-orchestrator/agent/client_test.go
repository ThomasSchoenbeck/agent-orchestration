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
		// Fallback for the executor's mandatory core-skill preflight (T3.4): serve
		// the built-in subagent-skill registry when the wrapped mux doesn't register
		// one, so worker/reviewer Run tests model a working platform. A test that
		// registers its own /api/subagent-skills handler still wins.
		if r.URL.Path == "/api/subagent-skills" {
			if mux, ok := h.(*http.ServeMux); ok {
				if _, pattern := mux.Handler(r); pattern == "" {
					_ = json.NewEncoder(w).Encode(db.DefaultSubagentSkills())
					return
				}
			}
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

func TestClient_ListSubagentSkills(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/subagent-skills", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode([]*db.SubagentSkill{
			{Name: "investigate_codebase", Enabled: true, ToolAllowlist: []string{"read_file"}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := agent.NewServerClientWithAuth(srv.URL, "tok", nil)
	skills, err := c.ListSubagentSkills(context.Background())
	if err != nil {
		t.Fatalf("ListSubagentSkills: %v", err)
	}
	if gotPath != "/api/agent/subagent-skills" {
		t.Errorf("path = %q, want /api/agent/subagent-skills", gotPath)
	}
	if len(skills) != 1 || skills[0].Name != "investigate_codebase" {
		t.Errorf("skills = %+v", skills)
	}
}

func TestClient_CreateAgentSession(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/agent-sessions", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(&db.AgentSession{ID: "sess-1", TaskID: "t1", Summary: "ok"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := agent.NewServerClientWithAuth(srv.URL, "tok", nil)
	out, err := c.CreateAgentSession(context.Background(), &db.AgentSession{TaskID: "t1", Summary: "ok"})
	if err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}
	if gotPath != "/api/agent/agent-sessions" {
		t.Errorf("path = %q", gotPath)
	}
	if out == nil || out.ID != "sess-1" {
		t.Errorf("session = %+v", out)
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
