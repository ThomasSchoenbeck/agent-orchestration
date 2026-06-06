package git

import (
	"fmt"
	"sort"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ConflictError reports that branch and base diverged on the same files and the
// merge cannot proceed without manual resolution.
type ConflictError struct {
	Paths []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("merge conflict on: %v", e.Paths)
}

// MergeBranch merges branch into base in the bare repo at repoPath. It performs
// a fast-forward when base is an ancestor of branch, otherwise a three-way
// merge — but only after a pre-merge guard rejects branches that modified the
// same file divergently (returning a *ConflictError). Returns the new HEAD SHA
// of base on success.
func MergeBranch(repoPath, base, branch string) (sha string, err error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git.MergeBranch open %q: %w", repoPath, err)
	}

	baseRef, err := repo.Reference(plumbing.NewBranchReferenceName(base), true)
	if err != nil {
		return "", fmt.Errorf("git.MergeBranch resolve base %q: %w", base, err)
	}
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return "", fmt.Errorf("git.MergeBranch resolve branch %q: %w", branch, err)
	}

	// Fast-forward: base is an ancestor of branch — just advance the base ref.
	if commitIsAncestor(repo, baseRef.Hash(), branchRef.Hash()) {
		newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(base), branchRef.Hash())
		if err := repo.Storer.SetReference(newRef); err != nil {
			return "", fmt.Errorf("git.MergeBranch update base (ff): %w", err)
		}
		return branchRef.Hash().String(), nil
	}

	// Pre-merge guard: reject genuine three-way content conflicts.
	if conflicts, derr := conflictingPaths(repo, baseRef.Hash(), branchRef.Hash()); derr != nil {
		return "", fmt.Errorf("git.MergeBranch conflict check: %w", derr)
	} else if len(conflicts) > 0 {
		return "", &ConflictError{Paths: conflicts}
	}

	// No conflicts — create a three-way merge commit.
	mergedHash, err := threeWayMerge(repo, baseRef.Hash(), branchRef.Hash(), branch)
	if err != nil {
		return "", fmt.Errorf("git.MergeBranch merge: %w", err)
	}
	newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(base), mergedHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("git.MergeBranch update base: %w", err)
	}
	return mergedHash.String(), nil
}

// SquashMerge collapses all of branch's changes into a single commit on base
// (one parent = base's current HEAD) with the given message and author. Like
// MergeBranch, it rejects genuine three-way conflicts with a *ConflictError.
// Returns the new HEAD SHA of base.
func SquashMerge(repoPath, base, branch, message, authorName, authorEmail string) (sha string, err error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git.SquashMerge open %q: %w", repoPath, err)
	}
	baseRef, err := repo.Reference(plumbing.NewBranchReferenceName(base), true)
	if err != nil {
		return "", fmt.Errorf("git.SquashMerge resolve base %q: %w", base, err)
	}
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return "", fmt.Errorf("git.SquashMerge resolve branch %q: %w", branch, err)
	}
	branchCommit, err := repo.CommitObject(branchRef.Hash())
	if err != nil {
		return "", fmt.Errorf("git.SquashMerge branch commit: %w", err)
	}

	// Determine the squashed tree.
	var treeHash plumbing.Hash
	if commitIsAncestor(repo, baseRef.Hash(), branchRef.Hash()) {
		// Fast-forward: the branch already contains base, so its tree is the result.
		treeHash = branchCommit.TreeHash
	} else {
		// Divergent: reject real conflicts, otherwise build the three-way merged tree.
		if conflicts, derr := conflictingPaths(repo, baseRef.Hash(), branchRef.Hash()); derr != nil {
			return "", fmt.Errorf("git.SquashMerge conflict check: %w", derr)
		} else if len(conflicts) > 0 {
			return "", &ConflictError{Paths: conflicts}
		}
		mergeBase, mberr := findMergeBase(repo, baseRef.Hash(), branchRef.Hash())
		if mberr != nil {
			return "", fmt.Errorf("git.SquashMerge merge base: %w", mberr)
		}
		mergeBaseTree, _ := treeForCommit(repo, mergeBase)
		baseTree, terr := treeForCommit(repo, baseRef.Hash())
		if terr != nil {
			return "", terr
		}
		branchTree, terr := branchCommit.Tree()
		if terr != nil {
			return "", terr
		}
		entries, berr := buildMergedEntries(mergeBaseTree, baseTree, branchTree)
		if berr != nil {
			return "", berr
		}
		newTree := &object.Tree{Entries: entries}
		obj := repo.Storer.NewEncodedObject()
		if encErr := newTree.Encode(obj); encErr != nil {
			return "", fmt.Errorf("git.SquashMerge encode tree: %w", encErr)
		}
		treeHash, err = repo.Storer.SetEncodedObject(obj)
		if err != nil {
			return "", fmt.Errorf("git.SquashMerge store tree: %w", err)
		}
	}

	if message == "" {
		message = fmt.Sprintf("Squash merge of '%s'\n", branch)
	}
	sig := object.Signature{Name: authorName, Email: authorEmail, When: time.Now()}
	commit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      message,
		TreeHash:     treeHash,
		ParentHashes: []plumbing.Hash{baseRef.Hash()},
	}
	cObj := repo.Storer.NewEncodedObject()
	if encErr := commit.Encode(cObj); encErr != nil {
		return "", fmt.Errorf("git.SquashMerge encode commit: %w", encErr)
	}
	newHash, err := repo.Storer.SetEncodedObject(cObj)
	if err != nil {
		return "", fmt.Errorf("git.SquashMerge store commit: %w", err)
	}
	newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(base), newHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("git.SquashMerge update base: %w", err)
	}
	return newHash.String(), nil
}

// DeleteBranch removes the branch ref from the bare repo at repoPath. Deleting a
// non-existent branch is a no-op.
func DeleteBranch(repoPath, branch string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git.DeleteBranch open %q: %w", repoPath, err)
	}
	refName := plumbing.NewBranchReferenceName(branch)
	if _, err := repo.Reference(refName, true); err != nil {
		return nil // already gone
	}
	if err := repo.Storer.RemoveReference(refName); err != nil {
		return fmt.Errorf("git.DeleteBranch remove %q: %w", branch, err)
	}
	return nil
}

// conflictingPaths returns the set of files that both base and branch modified
// differently relative to their merge base — the precise three-way conflict
// condition. An empty result means the branches can be merged cleanly.
func conflictingPaths(repo *gogit.Repository, baseHash, branchHash plumbing.Hash) ([]string, error) {
	mergeBase, err := findMergeBase(repo, baseHash, branchHash)
	if err != nil {
		// No common ancestor (unrelated histories) — treat every shared,
		// differing path as a conflict rather than silently picking a side.
		return divergingSharedPaths(repo, baseHash, branchHash)
	}

	baseTree, err := treeForCommit(repo, baseHash)
	if err != nil {
		return nil, err
	}
	branchTree, err := treeForCommit(repo, branchHash)
	if err != nil {
		return nil, err
	}
	mergeBaseTree, err := treeForCommit(repo, mergeBase)
	if err != nil {
		return nil, err
	}

	ancestor := treeEntryMap(mergeBaseTree)
	ours := treeEntryMap(baseTree)
	theirs := treeEntryMap(branchTree)

	var conflicts []string
	for name, o := range ours {
		th, ok := theirs[name]
		if !ok {
			continue
		}
		if o.Hash == th.Hash {
			continue // identical on both sides
		}
		anc, inAnc := ancestor[name]
		// Both sides changed it relative to the ancestor (or both added it
		// divergently) — that's a conflict.
		oursChanged := !inAnc || o.Hash != anc.Hash
		theirsChanged := !inAnc || th.Hash != anc.Hash
		if oursChanged && theirsChanged {
			conflicts = append(conflicts, name)
		}
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

// divergingSharedPaths returns shared files whose content differs between the
// two commits (used when there is no common ancestor).
func divergingSharedPaths(repo *gogit.Repository, aHash, bHash plumbing.Hash) ([]string, error) {
	aTree, err := treeForCommit(repo, aHash)
	if err != nil {
		return nil, err
	}
	bTree, err := treeForCommit(repo, bHash)
	if err != nil {
		return nil, err
	}
	a := treeEntryMap(aTree)
	b := treeEntryMap(bTree)
	var conflicts []string
	for name, ae := range a {
		if be, ok := b[name]; ok && ae.Hash != be.Hash {
			conflicts = append(conflicts, name)
		}
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

// treeForCommit resolves a commit hash to its root tree, tolerating the zero
// hash (empty repo) by returning a nil tree.
func treeForCommit(repo *gogit.Repository, h plumbing.Hash) (*object.Tree, error) {
	if h == plumbing.ZeroHash {
		return nil, nil
	}
	c, err := repo.CommitObject(h)
	if err != nil {
		return nil, err
	}
	return c.Tree()
}
