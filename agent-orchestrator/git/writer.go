package git

import (
	"fmt"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// CommitFile writes content to filePath on branch in the bare repo at repoPath,
// creating a new commit with message and the given author info.
// Returns the new commit SHA.
func CommitFile(repoPath, branch, filePath string, content []byte, message, authorName, authorEmail string) (string, error) {
	if authorName == "" {
		authorName = "user"
	}
	if authorEmail == "" {
		authorEmail = "user@localhost"
	}

	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git.CommitFile open %q: %w", repoPath, err)
	}

	// Resolve branch to its current HEAD commit (if any).
	// A ZeroHash ref means the branch was created as a placeholder (orphan)
	// with no commits yet — treat it the same as a missing branch (first commit).
	var parentHash plumbing.Hash
	var parentTree *object.Tree
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err == nil && branchRef.Hash() != plumbing.ZeroHash {
		parentCommit, cerr := repo.CommitObject(branchRef.Hash())
		if cerr != nil {
			return "", fmt.Errorf("git.CommitFile resolve parent: %w", cerr)
		}
		parentHash = branchRef.Hash()
		parentTree, err = parentCommit.Tree()
		if err != nil {
			return "", fmt.Errorf("git.CommitFile parent tree: %w", err)
		}
	}
	// parentHash == ZeroHash means first commit; parentTree == nil is fine.

	// Store the new blob.
	blobObj := repo.Storer.NewEncodedObject()
	blobObj.SetType(plumbing.BlobObject)
	w, err := blobObj.Writer()
	if err != nil {
		return "", fmt.Errorf("git.CommitFile blob writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		return "", fmt.Errorf("git.CommitFile write blob: %w", err)
	}
	_ = w.Close()
	blobHash, err := repo.Storer.SetEncodedObject(blobObj)
	if err != nil {
		return "", fmt.Errorf("git.CommitFile store blob: %w", err)
	}

	// Build the new root tree with the file inserted/replaced.
	newTreeHash, err := upsertFileInTree(repo, parentTree, filePath, blobHash)
	if err != nil {
		return "", fmt.Errorf("git.CommitFile build tree: %w", err)
	}

	// Create the commit object.
	now := time.Now()
	sig := object.Signature{Name: authorName, Email: authorEmail, When: now}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   message,
		TreeHash:  newTreeHash,
	}
	if parentHash != plumbing.ZeroHash {
		commit.ParentHashes = []plumbing.Hash{parentHash}
	}

	commitObj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitObj); err != nil {
		return "", fmt.Errorf("git.CommitFile encode commit: %w", err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return "", fmt.Errorf("git.CommitFile store commit: %w", err)
	}

	// Advance the branch ref.
	newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), commitHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("git.CommitFile set ref: %w", err)
	}

	return commitHash.String(), nil
}

// FileChange describes a single file to include in a multi-file commit.
type FileChange struct {
	Path    string
	Content []byte
}

// CommitFiles writes multiple files in a single commit on branch in the bare
// repo at repoPath. Returns the new commit SHA.
func CommitFiles(repoPath, branch string, files []FileChange, message, authorName, authorEmail string) (string, error) {
	if authorName == "" {
		authorName = "user"
	}
	if authorEmail == "" {
		authorEmail = "user@localhost"
	}
	if len(files) == 0 {
		return "", fmt.Errorf("git.CommitFiles: no files provided")
	}

	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("git.CommitFiles open %q: %w", repoPath, err)
	}

	var parentHash plumbing.Hash
	var parentTree *object.Tree
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err == nil && branchRef.Hash() != plumbing.ZeroHash {
		parentCommit, cerr := repo.CommitObject(branchRef.Hash())
		if cerr != nil {
			return "", fmt.Errorf("git.CommitFiles resolve parent: %w", cerr)
		}
		parentHash = branchRef.Hash()
		parentTree, err = parentCommit.Tree()
		if err != nil {
			return "", fmt.Errorf("git.CommitFiles parent tree: %w", err)
		}
	}

	// Build each blob and insert into the tree sequentially.
	currentTree := parentTree
	for _, f := range files {
		blobObj := repo.Storer.NewEncodedObject()
		blobObj.SetType(plumbing.BlobObject)
		w, werr := blobObj.Writer()
		if werr != nil {
			return "", fmt.Errorf("git.CommitFiles blob writer: %w", werr)
		}
		if _, werr = w.Write(f.Content); werr != nil {
			return "", fmt.Errorf("git.CommitFiles write blob %q: %w", f.Path, werr)
		}
		_ = w.Close()
		blobHash, serr := repo.Storer.SetEncodedObject(blobObj)
		if serr != nil {
			return "", fmt.Errorf("git.CommitFiles store blob %q: %w", f.Path, serr)
		}

		newTreeHash, terr := upsertFileInTree(repo, currentTree, f.Path, blobHash)
		if terr != nil {
			return "", fmt.Errorf("git.CommitFiles build tree for %q: %w", f.Path, terr)
		}
		currentTree, err = repo.TreeObject(newTreeHash)
		if err != nil {
			return "", fmt.Errorf("git.CommitFiles load intermediate tree: %w", err)
		}
	}

	// Get the final tree hash.
	var finalTreeHash plumbing.Hash
	if currentTree != nil {
		finalTreeHash = currentTree.Hash
	}

	now := time.Now()
	sig := object.Signature{Name: authorName, Email: authorEmail, When: now}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   message,
		TreeHash:  finalTreeHash,
	}
	if parentHash != plumbing.ZeroHash {
		commit.ParentHashes = []plumbing.Hash{parentHash}
	}

	commitObj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitObj); err != nil {
		return "", fmt.Errorf("git.CommitFiles encode commit: %w", err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return "", fmt.Errorf("git.CommitFiles store commit: %w", err)
	}

	newRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), commitHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return "", fmt.Errorf("git.CommitFiles set ref: %w", err)
	}

	return commitHash.String(), nil
}

// upsertFileInTree inserts or replaces filePath (which may include subdirs) in
// the tree rooted at base, returning the hash of the new root tree object.
func upsertFileInTree(repo *gogit.Repository, base *object.Tree, filePath string, blobHash plumbing.Hash) (plumbing.Hash, error) {
	parts := strings.SplitN(filePath, "/", 2)

	// Load existing entries from base (if any).
	entries := map[string]object.TreeEntry{}
	if base != nil {
		for _, e := range base.Entries {
			entries[e.Name] = e
		}
	}

	if len(parts) == 1 {
		// Leaf file — just set the entry.
		entries[parts[0]] = object.TreeEntry{
			Name: parts[0],
			Mode: filemode.Regular,
			Hash: blobHash,
		}
	} else {
		// Intermediate directory — recurse.
		dirName := parts[0]
		var subBase *object.Tree
		if existing, ok := entries[dirName]; ok {
			if t, err := repo.TreeObject(existing.Hash); err == nil {
				subBase = t
			}
		}
		subHash, err := upsertFileInTree(repo, subBase, parts[1], blobHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries[dirName] = object.TreeEntry{
			Name: dirName,
			Mode: filemode.Dir,
			Hash: subHash,
		}
	}

	// Build sorted entry slice.
	sorted := make([]object.TreeEntry, 0, len(entries))
	for _, e := range entries {
		sorted = append(sorted, e)
	}
	sortTreeEntries(sorted)

	tree := &object.Tree{Entries: sorted}
	treeObj := repo.Storer.NewEncodedObject()
	if err := tree.Encode(treeObj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}
	return repo.Storer.SetEncodedObject(treeObj)
}

func sortTreeEntries(entries []object.TreeEntry) {
	// Git sorts tree entries: directories get a trailing "/" for comparison.
	less := func(i, j int) bool {
		a, b := entries[i], entries[j]
		nameA, nameB := a.Name, b.Name
		if a.Mode == filemode.Dir {
			nameA += "/"
		}
		if b.Mode == filemode.Dir {
			nameB += "/"
		}
		return nameA < nameB
	}
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}
