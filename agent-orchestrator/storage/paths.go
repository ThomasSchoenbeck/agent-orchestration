// Package storage provides helpers for resolving server-managed filesystem paths.
package storage

import "path/filepath"

const (
	defaultReposDir     = "repos"
	defaultWorktreesDir = "worktrees"
)

// Paths holds the resolved storage root and exposes path helpers.
type Paths struct {
	root         string
	reposDir     string
	worktreesDir string
}

// New returns a Paths rooted at root.  reposDir and worktreesDir name the
// subdirectories for bare repos and agent worktrees respectively; empty strings
// fall back to "repos" and "worktrees".
func New(root, reposDir, worktreesDir string) *Paths {
	if reposDir == "" {
		reposDir = defaultReposDir
	}
	if worktreesDir == "" {
		worktreesDir = defaultWorktreesDir
	}
	return &Paths{root: root, reposDir: reposDir, worktreesDir: worktreesDir}
}

// Dirs returns the repos and worktrees subdirectory names.
func (p *Paths) Dirs() (reposDir, worktreesDir string) {
	return p.reposDir, p.worktreesDir
}

// RepoPath returns the absolute path for the bare git repo of a project.
// Layout: {root}/{reposDir}/{projectID}.git/
func (p *Paths) RepoPath(projectID string) string {
	return filepath.Join(p.root, p.reposDir, projectID+".git")
}

// WorktreePath returns the absolute path for an agent worktree for a task.
// Layout: {root}/{worktreesDir}/{taskID}/
func (p *Paths) WorktreePath(taskID string) string {
	return filepath.Join(p.root, p.worktreesDir, taskID)
}
