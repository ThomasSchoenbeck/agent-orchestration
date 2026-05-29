package git_test

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"agent-orchestrator/git"
)

// startGitServer creates a bare repo pre-seeded with one commit on "main",
// starts an httptest git server serving it, and returns the repo path and
// server URL. The server is automatically shut down at the end of the test.
func startGitServer(t *testing.T, slug string) (repoPath, serverURL string) {
	t.Helper()
	repoPath = filepath.Join(tempDir(t), slug+".git")
	if err := git.InitBare(repoPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if _, err := git.CommitFile(repoPath, "main", "seed.txt", []byte("hello"), "seed", "dev", "dev@localhost"); err != nil {
		t.Fatalf("CommitFile seed: %v", err)
	}

	h := git.NewHTTPHandler(singleRepoResolver(slug, repoPath), nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return repoPath, srv.URL + "/git/" + slug + ".git"
}

// TestFetchRemote verifies that FetchRemote downloads commits from an HTTP
// remote and stores them as remote-tracking refs in the local bare repo.
func TestFetchRemote_PopulatesRemoteTrackingRef(t *testing.T) {
	upstreamPath, url := startGitServer(t, "upstream")

	// Capture the upstream HEAD SHA so we can compare after fetch.
	branches, _ := git.ListBranches(upstreamPath)
	if len(branches) == 0 {
		t.Fatal("upstream has no branches")
	}

	// Set up a fresh local bare repo that points at the upstream HTTP server.
	localPath := filepath.Join(tempDir(t), "local.git")
	if err := git.InitBare(localPath); err != nil {
		t.Fatalf("InitBare local: %v", err)
	}
	if err := git.AddRemote(localPath, "origin", url); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	if err := git.FetchRemote(localPath, "origin", ""); err != nil {
		t.Fatalf("FetchRemote: %v", err)
	}

	// Remote tracking ref refs/remotes/origin/main must now exist in local.
	localRepo, err := gogit.PlainOpen(localPath)
	if err != nil {
		t.Fatalf("PlainOpen local: %v", err)
	}
	trackingRef, err := localRepo.Reference(plumbing.NewRemoteReferenceName("origin", "main"), true)
	if err != nil {
		t.Fatalf("remote tracking ref not found after FetchRemote: %v", err)
	}
	if trackingRef.Hash() == plumbing.ZeroHash {
		t.Error("remote tracking ref points at ZeroHash")
	}
}

// TestFetchRemote_IdempotentWhenUpToDate verifies that fetching an already
// up-to-date remote does not return an error.
func TestFetchRemote_IdempotentWhenUpToDate(t *testing.T) {
	localPath := filepath.Join(tempDir(t), "local.git")
	if err := git.InitBare(localPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	_, url := startGitServer(t, "upstream2")
	if err := git.AddRemote(localPath, "origin", url); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	if err := git.FetchRemote(localPath, "origin", ""); err != nil {
		t.Fatalf("first FetchRemote: %v", err)
	}
	// Second fetch — already up to date, must not error.
	if err := git.FetchRemote(localPath, "origin", ""); err != nil {
		t.Fatalf("second FetchRemote (already up-to-date): %v", err)
	}
}

// TestResetBranchToRemote verifies that after a fetch the local branch ref can
// be advanced to match the remote tracking ref.
func TestResetBranchToRemote_SyncsLocalBranch(t *testing.T) {
	_, url := startGitServer(t, "upstream3")

	localPath := filepath.Join(tempDir(t), "local.git")
	if err := git.InitBare(localPath); err != nil {
		t.Fatalf("InitBare local: %v", err)
	}
	if err := git.AddRemote(localPath, "origin", url); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	if err := git.FetchRemote(localPath, "origin", ""); err != nil {
		t.Fatalf("FetchRemote: %v", err)
	}

	if err := git.ResetBranchToRemote(localPath, "origin", "main"); err != nil {
		t.Fatalf("ResetBranchToRemote: %v", err)
	}

	// Local "main" should now resolve and seed.txt should be readable.
	data, err := git.ReadFile(localPath, "main", "seed.txt")
	if err != nil {
		t.Fatalf("ReadFile after ResetBranchToRemote: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("seed.txt = %q, want hello", data)
	}
}

// TestResetBranchToRemote_ErrorWithoutFetch verifies that calling
// ResetBranchToRemote before fetching returns an error (no tracking ref yet).
func TestResetBranchToRemote_ErrorWithoutFetch(t *testing.T) {
	localPath := filepath.Join(tempDir(t), "local.git")
	if err := git.InitBare(localPath); err != nil {
		t.Fatalf("InitBare: %v", err)
	}
	if err := git.AddRemote(localPath, "origin", "http://localhost:0/git/fake.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	err := git.ResetBranchToRemote(localPath, "origin", "main")
	if err == nil {
		t.Error("expected error when no remote tracking ref exists, got nil")
	}
}

// TestPushRefspec verifies that PushRefspec sends local commits to an HTTP
// remote and that they are readable from the remote repo afterwards.
func TestPushRefspec_SendsCommitsToRemote(t *testing.T) {
	// "Remote" server repo starts empty (no commits).
	remotePath := filepath.Join(tempDir(t), "remote.git")
	if err := git.InitBare(remotePath); err != nil {
		t.Fatalf("InitBare remote: %v", err)
	}
	h := git.NewHTTPHandler(singleRepoResolver("remote", remotePath), nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	remoteURL := srv.URL + "/git/remote.git"

	// Local repo with one commit.
	localPath := filepath.Join(tempDir(t), "local.git")
	if err := git.InitBare(localPath); err != nil {
		t.Fatalf("InitBare local: %v", err)
	}
	if _, err := git.CommitFile(localPath, "main", "work.txt", []byte("done"), "work", "dev", "dev@localhost"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	if err := git.AddRemote(localPath, "origin", remoteURL); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	if err := git.PushRefspec(localPath, "origin", "refs/heads/main:refs/heads/main", ""); err != nil {
		t.Fatalf("PushRefspec: %v", err)
	}

	// work.txt must be readable from the remote bare repo.
	data, err := git.ReadFile(remotePath, "main", "work.txt")
	if err != nil {
		t.Fatalf("ReadFile from remote after push: %v", err)
	}
	if string(data) != "done" {
		t.Errorf("work.txt = %q, want done", data)
	}
}

// TestPushMain is a thin wrapper check — it pushes refs/heads/main and verifies
// the remote receives it, reusing the same server infrastructure.
func TestPushMain_PushesMainBranch(t *testing.T) {
	remotePath := filepath.Join(tempDir(t), "remote2.git")
	if err := git.InitBare(remotePath); err != nil {
		t.Fatalf("InitBare remote: %v", err)
	}
	h := git.NewHTTPHandler(singleRepoResolver("remote2", remotePath), nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	localPath := filepath.Join(tempDir(t), "local2.git")
	if err := git.InitBare(localPath); err != nil {
		t.Fatalf("InitBare local: %v", err)
	}
	if _, err := git.CommitFile(localPath, "main", "main.txt", []byte("main content"), "init", "dev", "dev@localhost"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	if err := git.AddRemote(localPath, "origin", srv.URL+"/git/remote2.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	if err := git.PushMain(localPath, "origin", ""); err != nil {
		t.Fatalf("PushMain: %v", err)
	}

	data, err := git.ReadFile(remotePath, "main", "main.txt")
	if err != nil {
		t.Fatalf("ReadFile from remote after PushMain: %v", err)
	}
	if string(data) != "main content" {
		t.Errorf("main.txt = %q, want main content", data)
	}
}

// TestPushRefspec_IdempotentWhenUpToDate verifies no error when pushing an
// already-pushed ref.
func TestPushRefspec_IdempotentWhenUpToDate(t *testing.T) {
	remotePath := filepath.Join(tempDir(t), "remote3.git")
	if err := git.InitBare(remotePath); err != nil {
		t.Fatalf("InitBare remote: %v", err)
	}
	h := git.NewHTTPHandler(singleRepoResolver("remote3", remotePath), nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	localPath := filepath.Join(tempDir(t), "local3.git")
	if err := git.InitBare(localPath); err != nil {
		t.Fatalf("InitBare local: %v", err)
	}
	if _, err := git.CommitFile(localPath, "main", "f.txt", []byte("x"), "c", "dev", "dev@localhost"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	if err := git.AddRemote(localPath, "origin", srv.URL+"/git/remote3.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	if err := git.PushRefspec(localPath, "origin", "refs/heads/main:refs/heads/main", ""); err != nil {
		t.Fatalf("first PushRefspec: %v", err)
	}
	// Second push — already up to date, must not error.
	if err := git.PushRefspec(localPath, "origin", "refs/heads/main:refs/heads/main", ""); err != nil {
		t.Fatalf("second PushRefspec (already up-to-date): %v", err)
	}
}
