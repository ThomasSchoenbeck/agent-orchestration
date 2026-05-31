package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// TestEmptyRepoCloneSucceeds (IT-2.1) verifies that a go-git PlainClone against
// the embedded git HTTP server succeeds (or returns ErrEmptyRemoteRepository)
// and that the .git/ directory is created locally.
func TestEmptyRepoCloneSucceeds(t *testing.T) {
	t.Parallel()

	srv := newGitTestServer(t)
	_, slug := makeProject(t, srv.BaseURL, "clone-test")

	repoURL := srv.BaseURL + "/git/" + slug + ".git"
	cloneDir := t.TempDir()

	_, err := gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL: repoURL,
	})
	// ErrEmptyRemoteRepository means the server responded correctly — the
	// bare repo exists but has no commits yet. Any other error is a failure.
	if err != nil && err != transport.ErrEmptyRemoteRepository {
		t.Fatalf("PlainClone: unexpected error: %v", err)
	}
	// When the remote is empty, go-git returns early without writing .git/ to
	// disk, so we only check for .git/ on a successful (non-empty) clone.
	if err == nil {
		gitDir := filepath.Join(cloneDir, ".git")
		if _, statErr := os.Stat(gitDir); statErr != nil {
			t.Errorf(".git not found at %s: %v", gitDir, statErr)
		}
	}
}

// TestCloneAfterFirstCommit (IT-2.1 extension) confirms that a clone of a
// repo that has at least one commit succeeds without error.
func TestCloneAfterFirstCommit(t *testing.T) {
	t.Parallel()

	srv := newGitTestServer(t)
	_, slug := makeProject(t, srv.BaseURL, "clone-with-commit")
	repoURL := srv.BaseURL + "/git/" + slug + ".git"

	// Push an initial commit on main.
	seedMainBranch(t, srv.BaseURL, slug)

	// Clone should succeed cleanly.
	cloneDir := t.TempDir()
	_, err := gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL: repoURL,
	})
	if err != nil {
		t.Fatalf("PlainClone after seed: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(cloneDir, ".git", "config")); statErr != nil {
		t.Errorf(".git/config not found after clone")
	}
}
