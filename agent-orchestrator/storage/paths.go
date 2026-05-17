// Package storage provides helpers for resolving server-managed filesystem paths.
package storage

import "path/filepath"

// Paths holds the resolved storage root and exposes path helpers.
type Paths struct {
	root string
}

// New returns a Paths rooted at root (e.g. "./data").
func New(root string) *Paths {
	return &Paths{root: root}
}

// RepoPath returns the absolute path for the bare git repo of a project.
// Layout: {root}/repos/{projectID}.git/
func (p *Paths) RepoPath(projectID string) string {
	return filepath.Join(p.root, "repos", projectID+".git")
}

// WorktreePath returns the absolute path for an agent worktree for a task.
// Layout: {root}/worktrees/{taskID}/
func (p *Paths) WorktreePath(taskID string) string {
	return filepath.Join(p.root, "worktrees", taskID)
}
