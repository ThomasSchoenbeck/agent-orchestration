package agent

import (
	"fmt"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// CommitAndPush stages all changes in the worktree at worktreePath, creates a
// commit with the given message and author identity, then pushes the current
// branch to the named remote (usually "origin" for colocated, or the HTTP
// server URL for remote mode).
//
// remoteURL overrides the push destination when non-empty (remote-agent mode).
// token is used for HTTP basic-auth when pushing over the git HTTP server.
func CommitAndPush(worktreePath, commitMsg, authorName, authorEmail, remoteURL, token string) (headSHA string, err error) {
	repo, err := gogit.PlainOpen(worktreePath)
	if err != nil {
		return "", fmt.Errorf("repo.CommitAndPush open %q: %w", worktreePath, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("repo.CommitAndPush worktree: %w", err)
	}

	// Check status BEFORE staging so we can return early cleanly.
	status, err := wt.Status()
	if err != nil {
		return "", fmt.Errorf("repo.CommitAndPush status: %w", err)
	}
	if status.IsClean() {
		// Nothing changed — resolve and return current HEAD if it exists.
		head, err := repo.Head()
		if err != nil {
			// Unborn branch — no commits yet, nothing to push.
			return "", nil
		}
		return head.Hash().String(), nil
	}

	// Stage everything.
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return "", fmt.Errorf("repo.CommitAndPush add: %w", err)
	}

	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now().UTC(),
	}
	hash, err := wt.Commit(commitMsg, &gogit.CommitOptions{Author: sig})
	if err != nil {
		return "", fmt.Errorf("repo.CommitAndPush commit: %w", err)
	}

	// Resolve the local branch so we can push it explicitly.
	// Using an explicit refspec avoids go-git defaulting to "master" and
	// ensures the task branch is created on the remote even for orphan worktrees.
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("repo.CommitAndPush head after commit: %w", err)
	}
	localBranch := head.Name() // e.g. refs/heads/task/<id>
	refSpec := gogitconfig.RefSpec(localBranch.String() + ":" + localBranch.String())

	pushOpts := &gogit.PushOptions{
		RefSpecs: []gogitconfig.RefSpec{refSpec},
	}
	if remoteURL != "" {
		pushOpts.RemoteURL = remoteURL
	}
	if token != "" {
		pushOpts.Auth = &http.BasicAuth{Username: "git", Password: token}
	}
	if err := repo.Push(pushOpts); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("repo.CommitAndPush push: %w", err)
	}

	return hash.String(), nil
}
