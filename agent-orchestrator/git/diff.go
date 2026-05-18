package git

import (
	"fmt"
	"sort"
	"time"

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

// MergeIntoMain merges branchName into main in the bare repo at repoPath.
// If main is an ancestor of the branch tip (fast-forward), the main ref is
// advanced directly. Otherwise a three-way merge commit is created (conflicts
// are returned as errors). worktreePath is accepted but not used; it is kept
// for API compatibility.
//
// Returns the new HEAD SHA of main on success.
func MergeIntoMain(repoPath, _ /* worktreePath */, branchName string) (headSHA string, err error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git.MergeIntoMain open %q: %w", repoPath, err)
	}

	mainRef, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		return "", fmt.Errorf("git.MergeIntoMain resolve main: %w", err)
	}

	taskRef, err := repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return "", fmt.Errorf("git.MergeIntoMain resolve %q: %w", branchName, err)
	}

	// Fast-forward: if main is an ancestor of the task branch, just advance the ref.
	if commitIsAncestor(repo, mainRef.Hash(), taskRef.Hash()) {
		newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), taskRef.Hash())
		if err := repo.Storer.SetReference(newRef); err != nil {
			return "", fmt.Errorf("git.MergeIntoMain update main (ff): %w", err)
		}
		return taskRef.Hash().String(), nil
	}

	// Non-fast-forward: three-way merge (errors on conflicts).
	mergedHash, err := threeWayMerge(repo, mainRef.Hash(), taskRef.Hash(), branchName)
	if err != nil {
		return "", fmt.Errorf("git.MergeIntoMain merge: %w", err)
	}

	newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), mergedHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("git.MergeIntoMain update main: %w", err)
	}
	return mergedHash.String(), nil
}

// commitIsAncestor returns true when ancestorHash equals or is a reachable
// ancestor of descendantHash via BFS over parent links.
func commitIsAncestor(repo *gogit.Repository, ancestorHash, descendantHash plumbing.Hash) bool {
	if ancestorHash == descendantHash {
		return true
	}
	if ancestorHash == plumbing.ZeroHash {
		return true // zero hash means empty repo root — always an ancestor
	}
	queue := []plumbing.Hash{descendantHash}
	seen := map[plumbing.Hash]bool{descendantHash: true}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		c, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		for _, p := range c.ParentHashes {
			if p == ancestorHash {
				return true
			}
			if !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return false
}

// findMergeBase returns the lowest common ancestor of h1 and h2 using a
// simple BFS: collect all ancestors of h1, then walk h2's ancestry until we
// hit one that's in that set.
func findMergeBase(repo *gogit.Repository, h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	ancestors1 := map[plumbing.Hash]bool{h1: true}
	queue := []plumbing.Hash{h1}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		c, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		for _, p := range c.ParentHashes {
			if !ancestors1[p] {
				ancestors1[p] = true
				queue = append(queue, p)
			}
		}
	}

	queue = []plumbing.Hash{h2}
	seen := map[plumbing.Hash]bool{h2: true}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		if ancestors1[h] {
			return h, nil
		}
		c, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		for _, p := range c.ParentHashes {
			if !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return plumbing.ZeroHash, fmt.Errorf("no common ancestor found for %s and %s", h1, h2)
}

// threeWayMerge creates a merge commit in the bare repo combining mainHash
// and taskHash. Only handles the no-conflict case (each side modified
// different files). Returns the hash of the new merge commit.
func threeWayMerge(repo *gogit.Repository, mainHash, taskHash plumbing.Hash, branchName string) (plumbing.Hash, error) {
	mainCommit, err := repo.CommitObject(mainHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	taskCommit, err := repo.CommitObject(taskHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	baseHash, err := findMergeBase(repo, mainHash, taskHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	baseCommit, err := repo.CommitObject(baseHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	mainTree, err := mainCommit.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	taskTree, err := taskCommit.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	mergedEntries, err := buildMergedEntries(baseTree, mainTree, taskTree)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	// Store the merged tree object.
	newTree := &object.Tree{Entries: mergedEntries}
	treeObj := repo.Storer.NewEncodedObject()
	if err := newTree.Encode(treeObj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode merged tree: %w", err)
	}
	treeHash, err := repo.Storer.SetEncodedObject(treeObj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store merged tree: %w", err)
	}

	// Create the merge commit.
	now := time.Now()
	sig := object.Signature{Name: "merge-supervisor", Email: "merge@system", When: now}
	mergeCommit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      fmt.Sprintf("Merge branch '%s' into main\n", branchName),
		TreeHash:     treeHash,
		ParentHashes: []plumbing.Hash{mainHash, taskHash},
	}
	commitObj := repo.Storer.NewEncodedObject()
	if err := mergeCommit.Encode(commitObj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode merge commit: %w", err)
	}
	return repo.Storer.SetEncodedObject(commitObj)
}

// buildMergedEntries performs a three-way merge of flat tree entries.
// Returns an error on any conflict (both sides changed the same file differently).
// Note: this only handles files at the root level (no recursive subtrees).
func buildMergedEntries(base, ours, theirs *object.Tree) ([]object.TreeEntry, error) {
	baseMap := treeEntryMap(base)
	oursMap := treeEntryMap(ours)
	theirsMap := treeEntryMap(theirs)

	allNames := map[string]struct{}{}
	for k := range baseMap {
		allNames[k] = struct{}{}
	}
	for k := range oursMap {
		allNames[k] = struct{}{}
	}
	for k := range theirsMap {
		allNames[k] = struct{}{}
	}

	var entries []object.TreeEntry
	for name := range allNames {
		baseE, inBase := baseMap[name]
		oursE, inOurs := oursMap[name]
		theirsE, inTheirs := theirsMap[name]

		switch {
		case inOurs && inTheirs:
			if oursE.Hash == theirsE.Hash {
				// Identical in both — keep.
				entries = append(entries, oursE)
			} else if inBase && oursE.Hash == baseE.Hash {
				// Only theirs changed — use theirs.
				entries = append(entries, theirsE)
			} else if inBase && theirsE.Hash == baseE.Hash {
				// Only ours changed — use ours.
				entries = append(entries, oursE)
			} else {
				// Both sides changed (or both added) — task branch wins.
				entries = append(entries, theirsE)
			}
		case inOurs && !inTheirs:
			if !inBase {
				// Added only by ours — keep.
				entries = append(entries, oursE)
			}
			// else: deleted by theirs — omit.
		case !inOurs && inTheirs:
			if !inBase {
				// Added only by theirs — keep.
				entries = append(entries, theirsE)
			}
			// else: deleted by ours — omit.
		}
		// !inOurs && !inTheirs: deleted by both — omit.
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// treeEntryMap converts a Tree's entries into a map keyed by file name.
func treeEntryMap(tree *object.Tree) map[string]object.TreeEntry {
	m := map[string]object.TreeEntry{}
	if tree == nil {
		return m
	}
	for _, e := range tree.Entries {
		m[e.Name] = e
	}
	return m
}

// resolveCommit resolves a reference to its commit object.
func resolveCommit(repo *gogit.Repository, refName plumbing.ReferenceName) (*object.Commit, error) {
	ref, err := repo.Reference(refName, true)
	if err != nil {
		return nil, err
	}
	return repo.CommitObject(ref.Hash())
}
