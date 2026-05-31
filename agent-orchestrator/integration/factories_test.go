package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-orchestrator/db"

	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// makeProject POSTs to /api/projects and returns (projectID, slug).
// A unique slug is derived from name to satisfy the git HTTP resolver.
func makeProject(t *testing.T, baseURL, name string) (id, slug string) {
	t.Helper()
	slug = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	var p db.Project
	status := apiJSON(t, "POST", baseURL, "/api/projects", map[string]interface{}{
		"name": name,
		"slug": slug,
	}, &p)
	if status != 201 {
		t.Fatalf("makeProject %q: expected 201, got %d", name, status)
	}
	return p.ID, p.Slug
}

// makeTask POSTs to /api/tasks and returns the taskID.
func makeTask(t *testing.T, baseURL, projectID string) string {
	t.Helper()
	var task db.Task
	status := apiJSON(t, "POST", baseURL, "/api/tasks", map[string]interface{}{
		"project_id": projectID,
		"type":       "implement",
		"role":       "worker",
		"priority":   5,
	}, &task)
	if status != 201 {
		t.Fatalf("makeTask: expected 201, got %d", status)
	}
	return task.ID
}

// registerAgent POSTs to /api/agents/register and returns the agentID.
func registerAgent(t *testing.T, baseURL, name string, roles []string) string {
	t.Helper()
	var resp map[string]string
	status := apiJSON(t, "POST", baseURL, "/api/agents/register", map[string]interface{}{
		"name":  name,
		"roles": roles,
	}, &resp)
	if status != 201 {
		t.Fatalf("registerAgent %q: expected 201, got %d", name, status)
	}
	agentID := resp["agent_id"]
	if agentID == "" {
		t.Fatalf("registerAgent %q: empty agent_id", name)
	}
	return agentID
}

// claimTask POSTs /api/tasks/{id}/claim and asserts HTTP 200.
func claimTask(t *testing.T, baseURL, taskID, agentID string) {
	t.Helper()
	status := apiJSON(t, "POST", baseURL, "/api/tasks/"+taskID+"/claim",
		map[string]string{"agent_id": agentID}, nil)
	if status != 200 {
		t.Fatalf("claimTask %q by %q: expected 200, got %d", taskID, agentID, status)
	}
}

// cloneRepo uses go-git PlainClone to clone repoURL into a new t.TempDir().
// For empty remote repositories it falls back to PlainInit + adding the
// remote, so the returned repository always has "origin" configured.
// Returns the go-git Repository and the local clone path.
func cloneRepo(t *testing.T, repoURL string) (*gogit.Repository, string) {
	t.Helper()
	cloneDir := t.TempDir()
	repo, err := gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL: repoURL,
	})
	if err != nil {
		if err == transport.ErrEmptyRemoteRepository {
			// Empty bare repo — init locally and wire up the remote so we can push.
			repo, err = gogit.PlainInit(cloneDir, false)
			if err != nil {
				t.Fatalf("cloneRepo PlainInit after empty remote: %v", err)
			}
			_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{
				Name: "origin",
				URLs: []string{repoURL},
			})
			if err != nil {
				t.Fatalf("cloneRepo CreateRemote: %v", err)
			}
		} else {
			t.Fatalf("cloneRepo %s: %v", repoURL, err)
		}
	}
	return repo, cloneDir
}

// commitAndPush creates (or overwrites) relPath in the clone at clonePath,
// commits it, then pushes the commit to branchName on origin.
// Returns the commit hash string.
func commitAndPush(t *testing.T, clonePath, relPath, content, branch string) string {
	t.Helper()

	repo, err := gogit.PlainOpen(clonePath)
	if err != nil {
		t.Fatalf("commitAndPush PlainOpen: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("commitAndPush Worktree: %v", err)
	}

	// Write the file.
	fullPath := filepath.Join(clonePath, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("commitAndPush MkdirAll: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("commitAndPush WriteFile: %v", err)
	}

	if _, err := wt.Add(relPath); err != nil {
		t.Fatalf("commitAndPush Add: %v", err)
	}

	hash, err := wt.Commit(fmt.Sprintf("test: add %s", relPath), &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Agent",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("commitAndPush Commit: %v", err)
	}

	// Create/update refs/heads/{branch} pointing at the new commit.
	branchRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), hash)
	if err := repo.Storer.SetReference(branchRef); err != nil {
		t.Fatalf("commitAndPush SetReference: %v", err)
	}

	// Force-push the branch to origin.
	refSpec := gogitconfig.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch))
	err = repo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []gogitconfig.RefSpec{refSpec},
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		t.Fatalf("commitAndPush Push branch %q: %v", branch, err)
	}

	return hash.String()
}

// seedMainBranch ensures the bare repo behind repoURL has at least one commit
// on main. Required before any merge-supervisor test, since ChangedFiles needs
// main to exist.
func seedMainBranch(t *testing.T, baseURL, slug string) {
	t.Helper()
	_, clonePath := cloneRepo(t, baseURL+"/git/"+slug+".git")
	commitAndPush(t, clonePath, ".gitkeep", "", "main")
}
