package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/git"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
	"agent-orchestrator/tools"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// resolverMock returns a fixed Chat response (the merge-conflict resolution).
type resolverMock struct{ content string }

func (m *resolverMock) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{Content: m.content, StopReason: "end_turn"}, nil
}
func (m *resolverMock) Embed(_ context.Context, _ llm.EmbedRequest) (llm.EmbedResponse, error) {
	return llm.EmbedResponse{}, nil
}
func (m *resolverMock) Rerank(_ context.Context, _ llm.RerankRequest) (llm.RerankResponse, error) {
	return llm.RerankResponse{}, nil
}
func (m *resolverMock) Name() string { return "mock" }
func (m *resolverMock) Close() error { return nil }

func TestEffectiveRole_MergingUsesReviewRole(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{db.TaskStatusReviewing, "reviewer"},
		{db.TaskStatusMerging, "reviewer"},
		{db.TaskStatusDeveloping, "worker"},
	}
	for _, c := range cases {
		task := &db.Task{Role: "worker", ReviewRole: "reviewer", Status: c.status}
		if got := effectiveRole(task); got != c.want {
			t.Errorf("effectiveRole(status=%s) = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestParseResolvedFiles(t *testing.T) {
	in := "=== FILE: a/b.go ===\npackage main\n\nfunc main() {}\n=== END FILE ===\n" +
		"=== FILE: c.txt ===\nhello\n=== END FILE ==="
	got := parseResolvedFiles(in)
	if got["a/b.go"] != "package main\n\nfunc main() {}" {
		t.Errorf("a/b.go body = %q", got["a/b.go"])
	}
	if got["c.txt"] != "hello" {
		t.Errorf("c.txt body = %q", got["c.txt"])
	}
}

func TestHasConflictMarkers(t *testing.T) {
	if !hasConflictMarkers("<<<<<<< feature\nx\n=======\ny\n>>>>>>> main\n") {
		t.Error("expected markers detected")
	}
	if hasConflictMarkers("clean content\n") {
		t.Error("did not expect markers")
	}
}

// buildResolverExecutor wires an Executor whose reviewer role routes to a mock
// provider returning content.
func buildResolverExecutor(t *testing.T, content string) *Executor {
	t.Helper()
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "mock", Type: "ollama", BaseURL: "http://localhost:11434"}},
		Server:    config.ServerConfig{Port: 8080},
		Database:  config.DatabaseConfig{Path: ":memory:"},
	}
	reg := llm.NewRegistry()
	reg.SetRoles("mock", "mock-v1", []string{"reviewer"})
	if err := reg.Register("mock", &resolverMock{content: content}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	rtr := router.New(cfg, reg)
	return NewExecutor(rtr, tools.NewRegistry(), nil, "agent-merge")
}

// makeConflictedWorktree builds a non-bare repo whose feature branch conflicts
// with main on shared.txt, runs MergeIntoFeature (leaving markers), and returns
// the worktree dir.
func makeConflictedWorktree(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "wt")
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	commit := func(file, content, msg string) plumbing.Hash {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := wt.Add(file); err != nil {
			t.Fatalf("add: %v", err)
		}
		sig := &object.Signature{Name: "dev", Email: "dev@x", When: time.Now()}
		h, err := wt.Commit(msg, &gogit.CommitOptions{Author: sig, Committer: sig})
		if err != nil {
			t.Fatalf("commit: %v", err)
		}
		return h
	}
	co := func(b string) {
		if err := wt.Checkout(&gogit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(b)}); err != nil {
			t.Fatalf("checkout %s: %v", b, err)
		}
	}

	c0 := commit("shared.txt", "base\n", "c0")
	for _, b := range []string{"main", "feature"} {
		if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(b), c0)); err != nil {
			t.Fatalf("set %s: %v", b, err)
		}
	}
	co("main")
	mainTip := commit("shared.txt", "main-version\n", "main edit")
	// The agent resolves against origin/main; mirror main into a remote-tracking
	// ref so CommitMerge can resolve "origin/main".
	if err := repo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewRemoteReferenceName("origin", "main"), mainTip)); err != nil {
		t.Fatalf("set origin/main: %v", err)
	}
	co("feature")
	commit("shared.txt", "feature-version\n", "feature edit")

	st, err := git.MergeIntoFeature(dir, "main", "feature")
	if err != nil {
		t.Fatalf("MergeIntoFeature: %v", err)
	}
	if st.Clean {
		t.Fatalf("expected a conflict to set up the test")
	}
	return dir
}

func TestResolveConflictsWithLLM_Success(t *testing.T) {
	dir := makeConflictedWorktree(t)
	e := buildResolverExecutor(t, "=== FILE: shared.txt ===\nmerged-resolution\n=== END FILE ===")
	task := &db.Task{ID: "t1", Status: db.TaskStatusMerging, ReviewRole: "reviewer", Branch: "feature", WorktreePath: dir}

	if !e.resolveConflictsWithLLM(context.Background(), task, "main", []string{"shared.txt"}) {
		t.Fatal("expected conflicts to be resolved")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "shared.txt"))
	if strings.Contains(string(data), "<<<<<<<") {
		t.Errorf("markers still present: %q", data)
	}
	if string(data) != "merged-resolution" {
		t.Errorf("resolved content = %q, want %q", data, "merged-resolution")
	}
	// A two-parent merge commit must now be feature's HEAD.
	repo, _ := gogit.PlainOpen(dir)
	ref, _ := repo.Reference(plumbing.NewBranchReferenceName("feature"), true)
	c, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if c.NumParents() != 2 {
		t.Errorf("feature HEAD parents = %d, want 2", c.NumParents())
	}
}

func TestResolveConflictsWithLLM_Unresolvable(t *testing.T) {
	dir := makeConflictedWorktree(t)
	// The model parrots back content that still contains conflict markers.
	e := buildResolverExecutor(t,
		"=== FILE: shared.txt ===\n<<<<<<< feature\nfeature-version\n=======\nmain-version\n>>>>>>> main\n=== END FILE ===")
	task := &db.Task{ID: "t2", Status: db.TaskStatusMerging, ReviewRole: "reviewer", Branch: "feature", WorktreePath: dir}

	if e.resolveConflictsWithLLM(context.Background(), task, "main", []string{"shared.txt"}) {
		t.Fatal("expected resolution to fail when markers remain")
	}
}
