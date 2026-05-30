// Package git wraps go-git for server-side repo management.
package git

import (
	"fmt"
	"os"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// InitBare creates a bare git repository at path if one doesn't already exist.
// The repository HEAD is set to point at "main".
func InitBare(path string) error {
	// Idempotent: if a valid bare repo already exists, return nil.
	if _, err := os.Stat(path); err == nil {
		if _, openErr := gogit.PlainOpen(path); openErr == nil {
			return nil
		}
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("git.InitBare mkdir %q: %w", path, err)
	}

	_, err := gogit.PlainInit(path, true /* bare */)
	if err != nil {
		return fmt.Errorf("git.InitBare %q: %w", path, err)
	}

	// Point HEAD at "main" instead of the default "master".
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return fmt.Errorf("git.InitBare open after init %q: %w", path, err)
	}
	headRef := plumbing.NewSymbolicReference(
		plumbing.HEAD,
		plumbing.NewBranchReferenceName("main"),
	)
	if err := repo.Storer.SetReference(headRef); err != nil {
		return fmt.Errorf("git.InitBare set HEAD→main %q: %w", path, err)
	}

	return nil
}

// OpenBare opens an existing bare repository at path.
func OpenBare(path string) (*gogit.Repository, error) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("git.OpenBare %q: %w", path, err)
	}
	return repo, nil
}

// AddRemote adds a named remote to the repository at repoPath.
// If the remote already exists it is silently skipped.
func AddRemote(repoPath, name, url string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git.AddRemote open %q: %w", repoPath, err)
	}

	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err == gogit.ErrRemoteExists {
		return nil
	}
	if err != nil {
		return fmt.Errorf("git.AddRemote create remote %q in %q: %w", name, repoPath, err)
	}
	return nil
}

// EnsureBranchRef creates a branch ref in the bare repo pointing at commitSHA.
// It is a no-op if the ref already exists.
func EnsureBranchRef(repoPath, branchName, commitSHA string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git.EnsureBranchRef open %q: %w", repoPath, err)
	}

	refName := plumbing.NewBranchReferenceName(branchName)
	if _, err := repo.Reference(refName, true); err == nil {
		return nil // already exists
	}

	hash := plumbing.NewHash(commitSHA)
	ref := plumbing.NewHashReference(refName, hash)
	if err := repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("git.EnsureBranchRef set %q: %w", branchName, err)
	}
	return nil
}

// InitialCommit creates an empty root commit on branch (typically "main") in
// the bare repo at repoPath. It is a no-op if the branch already has commits.
func InitialCommit(repoPath, branch string) error {
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return fmt.Errorf("git.InitialCommit open %q: %w", repoPath, err)
	}

	refName := plumbing.NewBranchReferenceName(branch)
	if _, err := repo.Reference(refName, true); err == nil {
		return nil // branch already exists
	}

	// Build an empty tree object.
	enc := repo.Storer.NewEncodedObject()
	enc.SetType(plumbing.TreeObject)
	enc.SetSize(0)
	treeHash, err := repo.Storer.SetEncodedObject(enc)
	if err != nil {
		return fmt.Errorf("git.InitialCommit store tree: %w", err)
	}

	sig := object.Signature{
		Name:  "Agent Orchestrator",
		Email: "noreply@agent-orchestrator",
		When:  time.Now().UTC(),
	}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   "Initial commit",
		TreeHash:  treeHash,
	}
	commitEnc := repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitEnc); err != nil {
		return fmt.Errorf("git.InitialCommit encode commit: %w", err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(commitEnc)
	if err != nil {
		return fmt.Errorf("git.InitialCommit store commit: %w", err)
	}

	ref := plumbing.NewHashReference(refName, commitHash)
	if err := repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("git.InitialCommit set ref %q: %w", branch, err)
	}
	return nil
}
