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

	// Check status BEFORE staging.
	status, err := wt.Status()
	if err != nil {
		return "", fmt.Errorf("repo.CommitAndPush status: %w", err)
	}

	if !status.IsClean() {
		// Stage and commit new changes.
		if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
			return "", fmt.Errorf("repo.CommitAndPush add: %w", err)
		}
		sig := &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now().UTC(),
		}
		if _, err = wt.Commit(commitMsg, &gogit.CommitOptions{Author: sig}); err != nil {
			return "", fmt.Errorf("repo.CommitAndPush commit: %w", err)
		}
	}

	// Resolve HEAD — unborn branch means nothing to push.
	head, err := repo.Head()
	if err != nil {
		return "", nil
	}

	// Push over HTTP. The worktree's origin was set to the embedded HTTP server
	// URL by the server during worktree provisioning (SetRemoteURL in claim.go),
	// so both colocated and remote agents use go-git's HTTP transport here.
	// This avoids go-git's unreliable local-file push packfile negotiation.
	refSpec := gogitconfig.RefSpec("+" + head.Name().String() + ":" + head.Name().String())
	pushOpts := &gogit.PushOptions{
		RefSpecs: []gogitconfig.RefSpec{refSpec},
		Force:    true,
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

	return head.Hash().String(), nil
}
