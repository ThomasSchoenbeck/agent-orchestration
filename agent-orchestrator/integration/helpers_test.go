package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/server"
)

// TestMain serialises the very first sqlite3_initialize() call so it happens
// once before any t.Parallel() tests run. Without this, concurrent db.Open()
// calls race on SQLite's one-time global initialiser in modernc.org/sqlite.
func TestMain(m *testing.M) {
	tmp := filepath.Join(os.TempDir(), "integration_sqlite_init.db")
	if d, err := db.Open(tmp); err == nil {
		_ = d.Close()
	}
	_ = os.Remove(tmp)
	os.Exit(m.Run())
}

// gitTestServer bundles the resources owned by a single integration test.
type gitTestServer struct {
	BaseURL     string
	StorageRoot string
	DB          *db.Database
}

// newGitTestServer spins up a full server with a real TCP listener, a
// temporary storage root, and a fresh SQLite database. All resources are
// registered for cleanup via t.Cleanup.
// Tests that call this are skipped under -short to keep unit-test runs fast.
func newGitTestServer(t *testing.T) *gitTestServer {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	storageRoot := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 0,
			Host: "127.0.0.1",
		},
		Database: config.DatabaseConfig{Path: dbPath},
		Agents: config.AgentConfig{
			HeartbeatIntervalSec: 30,
			TaskTimeoutSec:       300,
		},
		Storage: config.StorageConfig{Root: storageRoot},
	}

	reg := llm.NewRegistry()
	srv := server.New(cfg, database, reg)

	ts := httptest.NewServer(srv)
	t.Cleanup(func() { ts.Close() })
	t.Cleanup(func() { _ = database.Close() })

	return &gitTestServer{
		BaseURL:     ts.URL,
		StorageRoot: storageRoot,
		DB:          database,
	}
}

// apiDo sends a JSON request to baseURL+path and returns the raw response.
// The caller is responsible for closing resp.Body.
func apiDo(t *testing.T, method, baseURL, path string, body any) *http.Response {
	t.Helper()
	var reqBytes []byte
	if body != nil {
		var err error
		reqBytes, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("apiDo marshal: %v", err)
		}
	}
	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("apiDo NewRequest: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("apiDo %s %s: %v", method, path, err)
	}
	return resp
}

// apiJSON is apiDo but also JSON-decodes the response body into out (if non-nil).
// Returns the HTTP status code.
func apiJSON(t *testing.T, method, baseURL, path string, body, out any) int {
	t.Helper()
	resp := apiDo(t, method, baseURL, path, body)
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("apiJSON decode (%s %s): %v", method, path, err)
		}
	}
	return resp.StatusCode
}
