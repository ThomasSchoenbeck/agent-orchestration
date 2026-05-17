package git

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// authForToken returns BasicAuth from a plain token (e.g. a GitHub PAT).
// Pass an empty token for unauthenticated access.
func authForToken(token string) *http.BasicAuth {
	if token == "" {
		return nil
	}
	return &http.BasicAuth{Username: "git", Password: token}
}

// FetchRemote fetches all refs from the named remote into the bare repo at
// repoPath. token may be empty for public repos.
func FetchRemote(repoPath, remoteName, token string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git.FetchRemote open %q: %w", repoPath, err)
	}

	err = repo.Fetch(&gogit.FetchOptions{
		RemoteName: remoteName,
		Auth:       authForToken(token),
		Tags:       gogit.AllTags,
		Force:      true,
	})
	if err == gogit.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		return fmt.Errorf("git.FetchRemote %q from %q: %w", remoteName, repoPath, err)
	}
	return nil
}

// PushRefspec pushes a single refspec (e.g. "refs/heads/main:refs/heads/main")
// from the bare repo at repoPath to the named remote. token may be empty.
func PushRefspec(repoPath, remoteName, refspec, token string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git.PushRefspec open %q: %w", repoPath, err)
	}

	err = repo.Push(&gogit.PushOptions{
		RemoteName: remoteName,
		RefSpecs:   []config.RefSpec{config.RefSpec(refspec)},
		Auth:       authForToken(token),
		Force:      false,
	})
	if err == gogit.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		return fmt.Errorf("git.PushRefspec %q → %q: %w", refspec, remoteName, err)
	}
	return nil
}

// PushMain pushes refs/heads/main to the named remote.
func PushMain(repoPath, remoteName, token string) error {
	return PushRefspec(repoPath, remoteName,
		"refs/heads/main:refs/heads/main", token)
}

// ResetBranchToRemote overwrites a local branch ref to match the fetched
// remote tracking ref (e.g. after an initial pull).
// remoteName is e.g. "upstream", branch is e.g. "main".
func ResetBranchToRemote(repoPath, remoteName, branch string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git.ResetBranchToRemote open %q: %w", repoPath, err)
	}

	// Remote tracking ref: refs/remotes/{remote}/{branch}
	remoteRef := plumbing.NewRemoteReferenceName(remoteName, branch)
	ref, err := repo.Reference(remoteRef, true)
	if err != nil {
		return fmt.Errorf("git.ResetBranchToRemote resolve %q: %w", remoteRef, err)
	}

	localRef := plumbing.NewHashReference(
		plumbing.NewBranchReferenceName(branch),
		ref.Hash(),
	)
	if err := repo.Storer.SetReference(localRef); err != nil {
		return fmt.Errorf("git.ResetBranchToRemote set %q: %w", branch, err)
	}
	return nil
}
