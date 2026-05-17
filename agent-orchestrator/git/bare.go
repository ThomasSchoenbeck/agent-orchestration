// Package git wraps go-git for server-side repo management.
package git

import (
	"fmt"
	"os"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
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
