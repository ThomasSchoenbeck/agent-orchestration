package git

import (
	"fmt"
	"os"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// CreateWorktree creates a git worktree at worktreePath from the bare repo at
// repoPath, checking out branchName. If branchName does not yet exist in the
// bare repo it is created pointing at the current HEAD of baseBranch (e.g.
// "main"). If branchName already exists the existing ref is used.
//
// The returned string is the resolved HEAD SHA after checkout.
func CreateWorktree(repoPath, worktreePath, branchName, baseBranch string) (headSHA string, err error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git.CreateWorktree open bare repo %q: %w", repoPath, err)
	}

	// Resolve or create the branch ref in the bare repo.
	branchRef := plumbing.NewBranchReferenceName(branchName)
	if _, err := repo.Reference(branchRef, true); err != nil {
		// Branch doesn't exist — base it off baseBranch HEAD.
		baseRef, err := repo.Reference(plumbing.NewBranchReferenceName(baseBranch), true)
		if err != nil {
			// baseBranch may not exist yet (empty repo). Use zero hash; the
			// worktree will be on an orphan branch.
			baseRef = plumbing.NewHashReference(branchRef, plumbing.ZeroHash)
		}
		newRef := plumbing.NewHashReference(branchRef, baseRef.Hash())
		if err := repo.Storer.SetReference(newRef); err != nil {
			return "", fmt.Errorf("git.CreateWorktree create branch %q: %w", branchName, err)
		}
	}

	// Resolve HEAD SHA for this branch.
	resolvedRef, err := repo.Reference(branchRef, true)
	if err != nil {
		return "", fmt.Errorf("git.CreateWorktree resolve branch %q: %w", branchName, err)
	}
	sha := resolvedRef.Hash().String()

	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		return "", fmt.Errorf("git.CreateWorktree mkdir %q: %w", worktreePath, err)
	}

	// Empty repo (no commits yet): go-git cannot clone a repo whose only ref
	// points to ZeroHash — it advertises a malformed zero-id ref that the git
	// client rejects. Initialise an empty worktree instead.
	// We point HEAD at the task branch (not the system default "master") and
	// configure "origin" so CommitAndPush can push back to the bare repo.
	if resolvedRef.Hash() == plumbing.ZeroHash {
		wt, initErr := gogit.PlainInit(worktreePath, false)
		if initErr != nil && initErr != gogit.ErrRepositoryAlreadyExists {
			return "", fmt.Errorf("git.CreateWorktree init empty worktree %q: %w", worktreePath, initErr)
		}
		if wt == nil {
			wt, initErr = gogit.PlainOpen(worktreePath)
			if initErr != nil {
				return "", fmt.Errorf("git.CreateWorktree open empty worktree %q: %w", worktreePath, initErr)
			}
		}
		// Point HEAD at the task branch so commits land on the right branch.
		headRef := plumbing.NewSymbolicReference(
			plumbing.HEAD,
			plumbing.NewBranchReferenceName(branchName),
		)
		_ = wt.Storer.SetReference(headRef)
		_, _ = wt.CreateRemote(&gogitconfig.RemoteConfig{
			Name: "origin",
			URLs: []string{repoPath},
		})
		return "", nil
	}

	cloneOpts := &gogit.CloneOptions{
		URL:           repoPath,
		ReferenceName: branchRef,
		SingleBranch:  true,
	}
	_, err = gogit.PlainClone(worktreePath, false, cloneOpts)
	if err != nil && err != gogit.ErrRepositoryAlreadyExists {
		return "", fmt.Errorf("git.CreateWorktree clone into %q: %w", worktreePath, err)
	}

	return sha, nil
}

// RemoveWorktree deletes the worktree directory. Safe to call even if the
// directory does not exist.
func RemoveWorktree(worktreePath string) error {
	if err := os.RemoveAll(worktreePath); err != nil {
		return fmt.Errorf("git.RemoveWorktree %q: %w", worktreePath, err)
	}
	return nil
}
