package integration_test

import (
	"testing"

	"agent-orchestrator/db"
)

// TestDescriptionEdit_SetsScopeDirty verifies that editing a project's
// description via the API marks its scope as dirty.
func TestDescriptionEdit_SetsScopeDirty(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, _ := makeProject(t, srv.BaseURL, "scope-dirty")

	var p db.Project
	apiJSON(t, "GET", srv.BaseURL, "/api/projects/"+projectID, nil, &p)
	if p.ScopeDirty {
		t.Fatal("new project should not be scope_dirty")
	}

	apiJSON(t, "PUT", srv.BaseURL, "/api/projects/"+projectID,
		map[string]interface{}{"description": "A brand new statement of intent"}, nil)

	var p2 db.Project
	apiJSON(t, "GET", srv.BaseURL, "/api/projects/"+projectID, nil, &p2)
	if !p2.ScopeDirty {
		t.Error("editing description should set scope_dirty")
	}
}

// TestProjectUpdate_AutoQueueRoundtrip verifies the auto-queue fields are
// accepted by the project update API and persisted.
func TestProjectUpdate_AutoQueueRoundtrip(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, _ := makeProject(t, srv.BaseURL, "auto-queue")

	apiJSON(t, "PUT", srv.BaseURL, "/api/projects/"+projectID,
		map[string]interface{}{"auto_queue": true, "status": "active", "max_open_tasks": 5}, nil)

	var p db.Project
	apiJSON(t, "GET", srv.BaseURL, "/api/projects/"+projectID, nil, &p)
	if !p.AutoQueue {
		t.Error("auto_queue should persist as true")
	}
	if p.MaxOpenTasks != 5 {
		t.Errorf("max_open_tasks = %d, want 5", p.MaxOpenTasks)
	}
	if p.Status != "active" {
		t.Errorf("status = %q, want active", p.Status)
	}
}
