package git

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// MergeStatus reports the outcome of merging a base branch into a feature
// branch inside an agent's working clone.
type MergeStatus struct {
	// UpToDate is true when the feature branch already contains base — no merge
	// commit was needed.
	UpToDate bool
	// Clean is true when the merge applied with no conflicts (including the
	// up-to-date and fast-forward cases). When Clean is true a new feature HEAD
	// is in MergedSHA.
	Clean bool
	// ConflictPaths lists the root-level files that both sides changed
	// divergently. Populated only when Clean is false. The worktree has been
	// left with conflict markers in these files (plus base's non-conflicting
	// changes applied) so the caller can resolve and commit.
	ConflictPaths []string
	// MergedSHA is the new feature HEAD when a merge (or fast-forward) commit was
	// created. Empty on a conflict.
	MergedSHA string
}

// MergeIntoFeature merges baseRef into the feature branch within the normal
// (non-bare) clone at repoPath. baseRef may be a branch name ("main"), a
// remote-tracking ref ("origin/main"), or a full ref path. The feature branch
// must be the repo's current branch.
//
// On a clean merge it materialises the result into the worktree, creates a
// two-parent merge commit on feature, and returns Clean with MergedSHA. On a
// genuine three-way conflict it writes base's non-conflicting changes plus
// whole-file conflict markers into the worktree and returns the conflicted
// paths without committing — the caller resolves them and calls CommitMerge.
//
// Like the rest of the merge layer, it operates on root-level files only.
func MergeIntoFeature(repoPath, baseRef, feature string) (*MergeStatus, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("git.MergeIntoFeature open %q: %w", repoPath, err)
	}

	baseHash, err := resolveRevisionHash(repo, baseRef)
	if err != nil {
		return nil, fmt.Errorf("git.MergeIntoFeature resolve base %q: %w", baseRef, err)
	}
	featureRefName := plumbing.NewBranchReferenceName(feature)
	featureRef, err := repo.Reference(featureRefName, true)
	if err != nil {
		return nil, fmt.Errorf("git.MergeIntoFeature resolve feature %q: %w", feature, err)
	}
	featureHash := featureRef.Hash()

	// Already up to date: feature already contains base.
	if baseHash == featureHash || commitIsAncestor(repo, baseHash, featureHash) {
		return &MergeStatus{UpToDate: true, Clean: true, MergedSHA: featureHash.String()}, nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("git.MergeIntoFeature worktree: %w", err)
	}

	// Fast-forward: feature has no commits of its own — advance it to base.
	if commitIsAncestor(repo, featureHash, baseHash) {
		if err := wt.Reset(&gogit.ResetOptions{Commit: baseHash, Mode: gogit.HardReset}); err != nil {
			return nil, fmt.Errorf("git.MergeIntoFeature fast-forward reset: %w", err)
		}
		return &MergeStatus{Clean: true, MergedSHA: baseHash.String()}, nil
	}

	// Divergent histories — authoritative conflict set (same definition the
	// server-side merge uses).
	conflicts, err := conflictingPaths(repo, featureHash, baseHash)
	if err != nil {
		return nil, fmt.Errorf("git.MergeIntoFeature conflict check: %w", err)
	}

	mergeBase, err := findMergeBase(repo, featureHash, baseHash)
	if err != nil {
		return nil, fmt.Errorf("git.MergeIntoFeature merge base: %w", err)
	}
	ancestorTree, err := treeForCommit(repo, mergeBase)
	if err != nil {
		return nil, err
	}
	oursTree, err := treeForCommit(repo, featureHash)
	if err != nil {
		return nil, err
	}
	theirsTree, err := treeForCommit(repo, baseHash)
	if err != nil {
		return nil, err
	}

	if err := materializeMerge(repoPath, feature, baseRef, ancestorTree, oursTree, theirsTree, conflicts); err != nil {
		return nil, err
	}

	if len(conflicts) > 0 {
		return &MergeStatus{Clean: false, ConflictPaths: conflicts}, nil
	}

	// Clean three-way merge: stage the materialised result and record a
	// two-parent merge commit on the feature branch.
	sha, err := commitMergeCommit(wt, featureHash, baseHash,
		fmt.Sprintf("Merge %s into %s\n", baseRef, feature))
	if err != nil {
		return nil, err
	}
	return &MergeStatus{Clean: true, MergedSHA: sha}, nil
}

// CommitMerge stages everything in the worktree at repoPath and records a
// two-parent merge commit on the feature branch (parents: current feature HEAD
// and baseRef). Used to finalise a conflict resolution after the caller has
// rewritten the conflicted files. Returns the new feature HEAD SHA.
func CommitMerge(repoPath, feature, baseRef, message, authorName, authorEmail string) (string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git.CommitMerge open %q: %w", repoPath, err)
	}
	featureRef, err := repo.Reference(plumbing.NewBranchReferenceName(feature), true)
	if err != nil {
		return "", fmt.Errorf("git.CommitMerge resolve feature %q: %w", feature, err)
	}
	baseHash, err := resolveRevisionHash(repo, baseRef)
	if err != nil {
		return "", fmt.Errorf("git.CommitMerge resolve base %q: %w", baseRef, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("git.CommitMerge worktree: %w", err)
	}
	if message == "" {
		message = fmt.Sprintf("Merge %s into %s\n", baseRef, feature)
	}
	sig := &object.Signature{Name: authorName, Email: authorEmail, When: time.Now().UTC()}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return "", fmt.Errorf("git.CommitMerge add: %w", err)
	}
	h, err := wt.Commit(message, &gogit.CommitOptions{
		Author:    sig,
		Committer: sig,
		Parents:   []plumbing.Hash{featureRef.Hash(), baseHash},
	})
	if err != nil {
		return "", fmt.Errorf("git.CommitMerge commit: %w", err)
	}
	return h.String(), nil
}

// commitMergeCommit stages the worktree and records a two-parent merge commit
// with the given message, advancing the current branch.
func commitMergeCommit(wt *gogit.Worktree, p1, p2 plumbing.Hash, message string) (string, error) {
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return "", fmt.Errorf("git.MergeIntoFeature add: %w", err)
	}
	sig := &object.Signature{Name: "Agent", Email: "agent@system", When: time.Now().UTC()}
	h, err := wt.Commit(message, &gogit.CommitOptions{
		Author:    sig,
		Committer: sig,
		Parents:   []plumbing.Hash{p1, p2},
	})
	if err != nil {
		return "", fmt.Errorf("git.MergeIntoFeature commit: %w", err)
	}
	return h.String(), nil
}

// materializeMerge writes the three-way merge result into the worktree. Files
// in the conflicts set are written with whole-file conflict markers; every other
// file is resolved to its clean merged content (base's non-conflicting changes
// included, deletions applied).
func materializeMerge(repoPath, feature, baseRef string, ancestor, ours, theirs *object.Tree, conflicts []string) error {
	conflictSet := make(map[string]bool, len(conflicts))
	for _, p := range conflicts {
		conflictSet[p] = true
	}

	ancestorMap := treeEntryMap(ancestor)
	oursMap := treeEntryMap(ours)
	theirsMap := treeEntryMap(theirs)

	names := map[string]struct{}{}
	for k := range ancestorMap {
		names[k] = struct{}{}
	}
	for k := range oursMap {
		names[k] = struct{}{}
	}
	for k := range theirsMap {
		names[k] = struct{}{}
	}

	for name := range names {
		full := filepath.Join(repoPath, filepath.FromSlash(name))
		baseE, inBase := ancestorMap[name]
		oursE, inOurs := oursMap[name]
		theirsE, inTheirs := theirsMap[name]

		if conflictSet[name] {
			oursContent, err := blobString(ours, name)
			if err != nil {
				return err
			}
			theirsContent, err := blobString(theirs, name)
			if err != nil {
				return err
			}
			if err := writeWorktreeFile(full, conflictMarkers(feature, baseRef, oursContent, theirsContent)); err != nil {
				return err
			}
			continue
		}

		switch {
		case inOurs && inTheirs:
			switch {
			case oursE.Hash == theirsE.Hash:
				// Identical on both sides — worktree already correct.
			case inBase && oursE.Hash == baseE.Hash:
				// Only base (theirs) changed — bring its version in.
				content, err := blobString(theirs, name)
				if err != nil {
					return err
				}
				if err := writeWorktreeFile(full, content); err != nil {
					return err
				}
			default:
				// Only feature (ours) changed, or theirs==base — worktree already
				// has ours.
			}
		case inOurs && !inTheirs:
			if inBase {
				// Deleted by base — remove from the worktree.
				if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("git.MergeIntoFeature remove %q: %w", name, err)
				}
			}
			// else: added by feature only — already present.
		case !inOurs && inTheirs:
			if !inBase {
				// Added by base only — bring it in.
				content, err := blobString(theirs, name)
				if err != nil {
					return err
				}
				if err := writeWorktreeFile(full, content); err != nil {
					return err
				}
			}
			// else: deleted by feature — already absent.
		}
	}
	return nil
}

// conflictMarkers wraps the two sides of a conflicted file in standard git
// conflict markers, feature side first.
func conflictMarkers(feature, base, ours, theirs string) string {
	return fmt.Sprintf("<<<<<<< %s\n%s\n=======\n%s\n>>>>>>> %s\n",
		feature, ours, theirs, base)
}

// blobString returns the contents of name in tree as a string ("" when absent).
func blobString(tree *object.Tree, name string) (string, error) {
	if tree == nil {
		return "", nil
	}
	f, err := tree.File(name)
	if err == object.ErrFileNotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("git: read blob %q: %w", name, err)
	}
	s, err := f.Contents()
	if err != nil {
		return "", fmt.Errorf("git: contents %q: %w", name, err)
	}
	return s, nil
}

// writeWorktreeFile writes content to a worktree path, creating parent dirs.
func writeWorktreeFile(full, content string) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("git: mkdir for %q: %w", full, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return fmt.Errorf("git: write %q: %w", full, err)
	}
	return nil
}

// resolveRevisionHash resolves a branch name, remote-tracking ref, full ref, or
// SHA to a commit hash.
func resolveRevisionHash(repo *gogit.Repository, rev string) (plumbing.Hash, error) {
	h, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return *h, nil
}
