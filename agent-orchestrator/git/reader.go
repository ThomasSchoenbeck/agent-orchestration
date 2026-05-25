package git

import (
	"bytes"
	"fmt"
	"sort"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TreeNode describes one entry in a directory listing.
type TreeNode struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "blob" or "tree"
	Size int64  `json:"size"`
	Mode string `json:"mode"`
}

// ListBranches returns the names of all local branches in the bare repo at repoPath.
func ListBranches(repoPath string) ([]string, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("git.ListBranches open %q: %w", repoPath, err)
	}

	iter, err := repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("git.ListBranches: %w", err)
	}
	var branches []string
	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})
	sort.Strings(branches)
	return branches, nil
}

// ReadTree returns the non-recursive directory listing under subpath at ref.
// subpath="" returns the root. ref may be a branch name, tag, or commit SHA.
// Returns an empty slice for an empty repo (no commits yet).
func ReadTree(repoPath, ref, subpath string) ([]TreeNode, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("git.ReadTree open %q: %w", repoPath, err)
	}

	commit, err := resolveRef(repo, ref)
	if err == plumbing.ErrReferenceNotFound {
		return nil, fmt.Errorf("ref %q not found", ref)
	}
	if err != nil {
		return nil, fmt.Errorf("git.ReadTree resolve ref %q: %w", ref, err)
	}
	if commit == nil {
		return []TreeNode{}, nil // empty repo
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("git.ReadTree tree: %w", err)
	}

	// Navigate to subpath if requested.
	if subpath != "" && subpath != "/" {
		tree, err = tree.Tree(subpath)
		if err != nil {
			return nil, fmt.Errorf("git.ReadTree subpath %q: %w", subpath, err)
		}
	}

	var nodes []TreeNode
	for _, entry := range tree.Entries {
		entryPath := entry.Name
		if subpath != "" && subpath != "/" {
			entryPath = subpath + "/" + entry.Name
		}

		nodeType := "blob"
		if entry.Mode == filemode.Dir || entry.Mode == filemode.Submodule {
			nodeType = "tree"
		}

		var size int64
		if nodeType == "blob" {
			if blob, berr := repo.BlobObject(entry.Hash); berr == nil {
				size = blob.Size
			}
		}

		nodes = append(nodes, TreeNode{
			Name: entry.Name,
			Path: entryPath,
			Type: nodeType,
			Size: size,
			Mode: entry.Mode.String(),
		})
	}
	return nodes, nil
}

// ReadFile returns the raw bytes of filePath at ref.
// Returns (nil, ErrBinaryFile) when the file contains null bytes.
func ReadFile(repoPath, ref, filePath string) ([]byte, error) {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("git.ReadFile open %q: %w", repoPath, err)
	}

	commit, err := resolveRef(repo, ref)
	if err == plumbing.ErrReferenceNotFound {
		return nil, fmt.Errorf("ref %q not found", ref)
	}
	if err != nil {
		return nil, fmt.Errorf("git.ReadFile resolve ref %q: %w", ref, err)
	}
	if commit == nil {
		return nil, fmt.Errorf("file %q not found at ref %q", filePath, ref)
	}

	file, err := commit.File(filePath)
	if err == object.ErrFileNotFound {
		return nil, fmt.Errorf("file %q not found at ref %q", filePath, ref)
	}
	if err != nil {
		return nil, fmt.Errorf("git.ReadFile %q: %w", filePath, err)
	}

	content, err := file.Contents()
	if err != nil {
		return nil, fmt.Errorf("git.ReadFile read %q: %w", filePath, err)
	}

	data := []byte(content)
	if bytes.ContainsRune(data, 0) {
		return nil, ErrBinaryFile
	}
	return data, nil
}

// ErrBinaryFile is returned by ReadFile when the file contains null bytes.
var ErrBinaryFile = fmt.Errorf("binary file")

// resolveRef resolves a ref (branch name, tag, or commit SHA) to a commit.
// Returns nil commit (no error) if the repo has no commits yet.
func resolveRef(repo *gogit.Repository, ref string) (*object.Commit, error) {
	// Try as a branch ref first.
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(ref), true)
	if err == nil {
		return repo.CommitObject(branchRef.Hash())
	}

	// Try as a tag ref.
	tagRef, err := repo.Reference(plumbing.NewTagReferenceName(ref), true)
	if err == nil {
		return repo.CommitObject(tagRef.Hash())
	}

	// Try as a raw commit SHA.
	hash := plumbing.NewHash(ref)
	if hash != plumbing.ZeroHash {
		c, err := repo.CommitObject(hash)
		if err == nil {
			return c, nil
		}
	}

	// If HEAD itself can't be resolved the repo is empty (unborn branch).
	// Return nil, nil so callers can distinguish "empty repo" from "bad ref".
	if _, headErr := repo.Head(); headErr != nil {
		return nil, nil
	}
	return nil, plumbing.ErrReferenceNotFound
}
