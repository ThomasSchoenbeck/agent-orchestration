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
	"github.com/go-git/go-git/v5/plumbing/transport"

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

	var pushedSlug, pushedBranch, pushedSHA string
	postReceive := func(slug, branch, sha string) {
		pushedSlug = slug
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
	if pushedSlug != "srv" {
		t.Errorf("postReceive slug = %q, want srv", pushedSlug)
	}
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

// TestHTTPServer_CloneWithInitialCommit is the primary regression test for
// Bug 7. It verifies that a repo seeded by git.InitialCommit (the production
// path) can be cloned via the embedded HTTP server without a 500 error.
// Before the fix, serveUploadPack called UploadPack on an uninitialised
// session, causing a 500 on every clone attempt.
func TestHTTPServer_CloneWithInitialCommit(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "proj.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := git.InitialCommit(repoPath, "main"); err != nil {
		t.Fatalf("InitialCommit: %v", err)
	}

	h := git.NewHTTPHandler(singleRepoResolver("proj", repoPath), nil)
	srv := httptest.NewServer(h)
	defer srv.Close()

	clonePath := filepath.Join(tempDir(t), "clone")
	_, err := gogit.PlainClone(clonePath, false, &gogit.CloneOptions{
		URL: srv.URL + "/git/proj.git",
	})
	if err != nil {
		t.Fatalf("PlainClone of InitialCommit repo: %v (want nil)", err)
	}
}

// TestHTTPServer_CloneEmptyRepo verifies that a bare repo with no commits at
// all returns ErrEmptyRemoteRepository (not a 500). This confirms the server
// handles the empty case gracefully.
func TestHTTPServer_CloneEmptyRepo(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "empty.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	// Do NOT call InitialCommit — repo has no commits.

	h := git.NewHTTPHandler(singleRepoResolver("empty", repoPath), nil)
	srv := httptest.NewServer(h)
	defer srv.Close()

	clonePath := filepath.Join(tempDir(t), "clone")
	_, err := gogit.PlainClone(clonePath, false, &gogit.CloneOptions{
		URL: srv.URL + "/git/empty.git",
	})
	if err != transport.ErrEmptyRemoteRepository {
		t.Fatalf("expected ErrEmptyRemoteRepository, got: %v", err)
	}
}

// TestHTTPServer_UploadPackStateless verifies that the upload-pack POST
// endpoint works when called without a prior info/refs GET on the same
// server session — i.e. it correctly handles stateless smart HTTP.
func TestHTTPServer_UploadPackStateless(t *testing.T) {
	repoPath := filepath.Join(tempDir(t), "stateless.git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", "a.txt", []byte("hello"), "init", "dev", "dev@localhost"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	h := git.NewHTTPHandler(singleRepoResolver("sl", repoPath), nil)
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Two back-to-back clones hit different server sessions each time.
	// If the stateless init fix is missing, the second request will 500.
	for i := 0; i < 2; i++ {
		clonePath := filepath.Join(tempDir(t), fmt.Sprintf("clone%d", i))
		_, err := gogit.PlainClone(clonePath, false, &gogit.CloneOptions{
			URL: srv.URL + "/git/sl.git",
		})
		if err != nil {
			t.Fatalf("clone %d: %v", i, err)
		}
	}
}
