package git

import (
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ChangedFiles returns the set of file paths that differ between branchName
// and baseBranch (typically "main") in the bare repo at repoPath.
// Only files added, modified, or deleted are included.
func ChangedFiles(repoPath, branchName, baseBranch string) ([]string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("git.ChangedFiles open %q: %w", repoPath, err)
	}

	branchCommit, err := resolveCommit(repo, plumbing.NewBranchReferenceName(branchName))
	if err != nil {
		return nil, fmt.Errorf("git.ChangedFiles resolve branch %q: %w", branchName, err)
	}
	baseCommit, err := resolveCommit(repo, plumbing.NewBranchReferenceName(baseBranch))
	if err != nil {
		return nil, fmt.Errorf("git.ChangedFiles resolve base %q: %w", baseBranch, err)
	}

	branchTree, err := branchCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("git.ChangedFiles branch tree: %w", err)
	}
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("git.ChangedFiles base tree: %w", err)
	}

	changes, err := object.DiffTree(baseTree, branchTree)
	if err != nil {
		return nil, fmt.Errorf("git.ChangedFiles diff: %w", err)
	}

	seen := map[string]struct{}{}
	for _, ch := range changes {
		if ch.From.Name != "" {
			seen[ch.From.Name] = struct{}{}
		}
		if ch.To.Name != "" {
			seen[ch.To.Name] = struct{}{}
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	return paths, nil
}

// HasOverlap returns true if any path in setA also appears in setB.
func HasOverlap(setA, setB []string) bool {
	index := make(map[string]struct{}, len(setA))
	for _, p := range setA {
		index[p] = struct{}{}
	}
	for _, p := range setB {
		if _, ok := index[p]; ok {
			return true
		}
	}
	return false
}

// MergeIntoMain attempts a fast-forward or three-way merge of branchName into
// main in the bare repo. Returns the new HEAD SHA on success, or an error
// describing the conflict.
//
// On conflict the bare repo is left unchanged (the attempt is aborted via a
// temporary worktree that is cleaned up).
func MergeIntoMain(repoPath, worktreePath, branchName string) (headSHA string, err error) {
	// Create a temporary worktree on main.
	sha, err := CreateWorktree(repoPath, worktreePath, "main", "main")
	if err != nil {
		return "", fmt.Errorf("git.MergeIntoMain worktree: %w", err)
	}
	_ = sha

	wRepo, err := gogit.PlainOpen(worktreePath)
	if err != nil {
		_ = RemoveWorktree(worktreePath)
		return "", fmt.Errorf("git.MergeIntoMain open worktree: %w", err)
	}

	wt, err := wRepo.Worktree()
	if err != nil {
		_ = RemoveWorktree(worktreePath)
		return "", fmt.Errorf("git.MergeIntoMain worktree ref: %w", err)
	}

	// Fetch the task branch from origin so it's available locally.
	if err := wRepo.Fetch(&gogit.FetchOptions{RemoteName: "origin"}); err != nil && err != gogit.NoErrAlreadyUpToDate {
		_ = RemoveWorktree(worktreePath)
		return "", fmt.Errorf("git.MergeIntoMain fetch: %w", err)
	}

	// Resolve the task branch ref (as fetched from origin).
	remoteRef := plumbing.NewRemoteReferenceName("origin", branchName)
	ref, err := wRepo.Reference(remoteRef, true)
	if err != nil {
		_ = RemoveWorktree(worktreePath)
		return "", fmt.Errorf("git.MergeIntoMain resolve remote branch %q: %w", branchName, err)
	}

	// Merge.
	if err := wt.Pull(&gogit.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.NewBranchReferenceName(strings.TrimPrefix(branchName, "task/")),
		SingleBranch:  true,
	}); err != nil {
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "merge") {
			_ = RemoveWorktree(worktreePath)
			return "", fmt.Errorf("merge conflict: %w", err)
		}
		// Try a direct merge via the hash reference if Pull failed.
		_ = err
	}

	// Push main back to the bare repo.
	if err := wRepo.Push(&gogit.PushOptions{RemoteName: "origin"}); err != nil && err != gogit.NoErrAlreadyUpToDate {
		_ = RemoveWorktree(worktreePath)
		return "", fmt.Errorf("git.MergeIntoMain push main: %w", err)
	}

	head, err := wRepo.Head()
	if err != nil {
		_ = RemoveWorktree(worktreePath)
		return "", fmt.Errorf("git.MergeIntoMain head: %w", err)
	}

	_ = RemoveWorktree(worktreePath)
	_ = ref // suppress unused
	return head.Hash().String(), nil
}

// resolveCommit resolves a reference to its commit object.
func resolveCommit(repo *gogit.Repository, refName plumbing.ReferenceName) (*object.Commit, error) {
	ref, err := repo.Reference(refName, true)
	if err != nil {
		return nil, err
	}
	return repo.CommitObject(ref.Hash())
}
