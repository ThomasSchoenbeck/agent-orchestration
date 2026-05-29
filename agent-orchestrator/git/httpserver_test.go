package git_test

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"agent-orchestrator/git"
)

// singleRepoResolver returns a RepoResolver that maps one slug to a repo path.
// Any other slug returns an error, which fsLoader translates to a 404.
func singleRepoResolver(slug, repoPath string) git.RepoResolver {
	return func(s string) (string, error) {
		if s == slug {
			return repoPath, nil
		}
		return "", fmt.Errorf("unknown slug %q", s)
	}
}

func TestHTTPHandler_CloneRepoWithCommits(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "proj.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", "hello.txt", []byte("world"), "init", "dev", "dev@example.com"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	h := git.NewHTTPHandler(singleRepoResolver("proj", repoPath), nil)
	srv := httptest.NewServer(h)
	defer srv.Close()

	clonePath := filepath.Join(tempDir(t), "clone")
	_, err := gogit.PlainClone(clonePath, false, &gogit.CloneOptions{
		URL:           srv.URL + "/git/proj.git",
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		SingleBranch:  true,
	})
	if err != nil {
		t.Fatalf("PlainClone via HTTP: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(clonePath, "hello.txt"))
	if err != nil {
		t.Fatalf("hello.txt missing in clone: %v", err)
	}
	if string(data) != "world" {
		t.Errorf("hello.txt = %q, want world", data)
	}
}

func TestHTTPHandler_UnknownSlugReturns404(t *testing.T) {
	h := git.NewHTTPHandler(func(slug string) (string, error) {
		return "", fmt.Errorf("not found")
	}, nil)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/git/unknown.git/info/refs?service=git-upload-pack")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHTTPHandler_PushAndReadBack(t *testing.T) {
	// Bare repo on "server".
	serverRepo := filepath.Join(tempDir(t), "server.git")
	if err := git.InitBare(serverRepo); err != nil {
		t.Fatalf("InitBare server: %v", err)
	}
	// Seed a commit so clone has something to base a push on.
	if _, err := git.CommitFile(serverRepo, "main", "seed.txt", []byte("seed"), "seed", "dev", "dev@localhost"); err != nil {
		t.Fatalf("CommitFile seed: %v", err)
	}

	var pushedBranch, pushedSHA string
	postReceive := func(branch, sha string) {
		pushedBranch = branch
		pushedSHA = sha
	}

	h := git.NewHTTPHandler(singleRepoResolver("srv", serverRepo), postReceive)
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Clone the server repo.
	clonePath := filepath.Join(tempDir(t), "clone")
	cloneRepo, err := gogit.PlainClone(clonePath, false, &gogit.CloneOptions{
		URL:           srv.URL + "/git/srv.git",
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		SingleBranch:  true,
	})
	if err != nil {
		t.Fatalf("PlainClone: %v", err)
	}

	// Make a new commit in the clone.
	wt, err := cloneRepo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clonePath, "new.txt"), []byte("pushed"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add("new.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	commit, err := wt.Commit("add new.txt", &gogit.CommitOptions{
		Author: &object.Signature{Name: "dev", Email: "dev@localhost"},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Push back.
	if err := cloneRepo.Push(&gogit.PushOptions{
		RemoteName: "origin",
	}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Post-receive hook should have fired.
	if pushedBranch != "main" {
		t.Errorf("postReceive branch = %q, want main", pushedBranch)
	}
	if pushedSHA != commit.String() {
		t.Errorf("postReceive SHA = %q, want %q", pushedSHA, commit.String())
	}

	// File should be readable from the bare server repo.
	data, err := git.ReadFile(serverRepo, "main", "new.txt")
	if err != nil {
		t.Fatalf("ReadFile new.txt from server: %v", err)
	}
	if string(data) != "pushed" {
		t.Errorf("new.txt = %q, want pushed", data)
	}
}
