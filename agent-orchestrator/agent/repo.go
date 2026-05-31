package agent

import (
	"fmt"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// CloneOrOpen clones repoURL into localPath (if the directory does not yet
// exist) and checks out branchName, creating it from HEAD if it doesn't exist
// on the remote. If localPath already contains a git repo it is opened as-is
// and the branch checkout is still attempted so retried tasks land on the
// correct branch.
//
// When the remote repo has no commits yet (ErrEmptyRemoteRepository), the
// function falls back to PlainInit + setting the remote, matching the
// behaviour of the integration test helpers and allowing the agent to push
// the first real commit.
func CloneOrOpen(repoURL, localPath, branchName string) error {
	repo, openErr := gogit.PlainOpen(localPath)
	if openErr != nil {
		// Directory absent or not a git repo — clone fresh.
		var cloneErr error
		repo, cloneErr = gogit.PlainClone(localPath, false, &gogit.CloneOptions{
			URL: repoURL,
		})
		if cloneErr != nil {
			if cloneErr != transport.ErrEmptyRemoteRepository {
				return fmt.Errorf("CloneOrOpen clone %q → %q: %w", repoURL, localPath, cloneErr)
			}
			// Server repo has no commits yet — init locally and point origin at it.
			var initErr error
			repo, initErr = gogit.PlainInit(localPath, false)
			if initErr != nil {
				return fmt.Errorf("CloneOrOpen init %q: %w", localPath, initErr)
			}
			if _, initErr = repo.CreateRemote(&gogitconfig.RemoteConfig{
				Name: "origin",
				URLs: []string{repoURL},
			}); initErr != nil {
				return fmt.Errorf("CloneOrOpen set remote %q: %w", localPath, initErr)
			}
			// Skip the branch checkout — there is no HEAD to branch from.
			return nil
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("CloneOrOpen worktree %q: %w", localPath, err)
	}

	branchRef := plumbing.NewBranchReferenceName(branchName)

	// Try switching to the branch without creating (branch may already exist
	// from a previous run of this task).
	err = wt.Checkout(&gogit.CheckoutOptions{Branch: branchRef})
	if err == nil {
		return nil
	}

	// Branch doesn't exist locally — create it from current HEAD.
	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: branchRef,
		Create: true,
	}); err != nil {
		return fmt.Errorf("CloneOrOpen checkout branch %q: %w", branchName, err)
	}
	return nil
}

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
