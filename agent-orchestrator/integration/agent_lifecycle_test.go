package integration_test

import (
	"testing"

	"agent-orchestrator/db"
)

// registerWithSkills registers an agent with roles + skills and returns its ID.
func registerWithSkills(t *testing.T, baseURL, name string, roles, skills []string) string {
	t.Helper()
	var resp map[string]string
	st := apiJSON(t, "POST", baseURL, "/api/agents/register", map[string]interface{}{
		"name": name, "roles": roles, "skills": skills,
	}, &resp)
	if st != 200 && st != 201 {
		t.Fatalf("register %q: status %d", name, st)
	}
	return resp["agent_id"]
}

func getAgent(t *testing.T, baseURL, id string) db.Agent {
	t.Helper()
	var a db.Agent
	apiJSON(t, "GET", baseURL, "/api/agents/"+id, nil, &a)
	return a
}

func TestRegister_ResetsLiveToStart(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	id := registerWithSkills(t, srv.BaseURL, "reset-agent", []string{"worker"}, []string{"backend"})

	// Override live config.
	apiJSON(t, "PATCH", srv.BaseURL, "/api/agents/"+id,
		map[string]interface{}{"roles": []string{"worker", "reviewer"}, "skills": []string{"frontend"}}, nil)
	if a := getAgent(t, srv.BaseURL, id); len(a.Roles) != 2 {
		t.Fatalf("expected override to 2 live roles, got %v", a.Roles)
	}

	// Re-register (process restart) → live resets to start, desired_state=run.
	registerWithSkills(t, srv.BaseURL, "reset-agent", []string{"worker"}, []string{"backend"})
	a := getAgent(t, srv.BaseURL, id)
	if len(a.Roles) != 1 || a.Roles[0] != "worker" {
		t.Errorf("live roles after restart = %v, want [worker]", a.Roles)
	}
	if len(a.Skills) != 1 || a.Skills[0] != "backend" {
		t.Errorf("live skills after restart = %v, want [backend]", a.Skills)
	}
	if a.DesiredState != "run" {
		t.Errorf("desired_state = %q, want run", a.DesiredState)
	}
}

func TestPatchAgent_UpdatesLiveNotStart(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	id := registerWithSkills(t, srv.BaseURL, "patch-agent", []string{"worker"}, []string{"backend"})

	apiJSON(t, "PATCH", srv.BaseURL, "/api/agents/"+id,
		map[string]interface{}{"roles": []string{"worker", "reviewer"}}, nil)

	a := getAgent(t, srv.BaseURL, id)
	if len(a.Roles) != 2 {
		t.Errorf("live roles = %v, want 2", a.Roles)
	}
	if len(a.StartRoles) != 1 || a.StartRoles[0] != "worker" {
		t.Errorf("start_roles changed to %v, want [worker]", a.StartRoles)
	}
}

func TestGetNextTask_UsesLiveRolesFromRow(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	projectID, slug := makeProject(t, srv.BaseURL, "live-routing")
	seedMainBranch(t, srv.BaseURL, slug)

	id := registerWithSkills(t, srv.BaseURL, "lr-agent", []string{"worker"}, nil)

	// A reviewer-role task in the backlog.
	var task db.Task
	apiJSON(t, "POST", srv.BaseURL, "/api/tasks",
		map[string]interface{}{"project_id": projectID, "role": "reviewer", "priority": 5}, &task)

	// Override live roles to reviewer.
	apiJSON(t, "PATCH", srv.BaseURL, "/api/agents/"+id,
		map[string]interface{}{"roles": []string{"reviewer"}}, nil)

	// Poll with a stale ?roles=worker query param — it must be ignored in favor
	// of the live row, so the reviewer task is offered.
	var resp struct {
		Task *db.Task `json:"task"`
	}
	apiJSON(t, "GET", srv.BaseURL, "/api/agents/"+id+"/tasks/next?roles=worker", nil, &resp)
	if resp.Task == nil {
		t.Fatal("expected the reviewer task to be offered via live roles")
	}
	if resp.Task.Role != "reviewer" {
		t.Errorf("offered task role = %q, want reviewer", resp.Task.Role)
	}
}

func TestStopAgent_SetsDesiredState(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	id := registerWithSkills(t, srv.BaseURL, "stop-agent", []string{"worker"}, nil)

	apiJSON(t, "POST", srv.BaseURL, "/api/agents/"+id+"/stop", nil, nil)

	if a := getAgent(t, srv.BaseURL, id); a.DesiredState != "stop" {
		t.Errorf("desired_state = %q, want stop", a.DesiredState)
	}

	// Heartbeat must carry the stop signal.
	var hb struct {
		DesiredState string `json:"desired_state"`
	}
	apiJSON(t, "POST", srv.BaseURL, "/api/agents/"+id+"/heartbeat", nil, &hb)
	if hb.DesiredState != "stop" {
		t.Errorf("heartbeat desired_state = %q, want stop", hb.DesiredState)
	}
}

func TestResetAgent_LiveEqualsStart(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	id := registerWithSkills(t, srv.BaseURL, "reset2-agent", []string{"worker"}, []string{"backend"})

	apiJSON(t, "PATCH", srv.BaseURL, "/api/agents/"+id,
		map[string]interface{}{"roles": []string{"worker", "reviewer"}, "skills": []string{"frontend"}}, nil)
	apiJSON(t, "POST", srv.BaseURL, "/api/agents/"+id+"/reset", nil, nil)

	a := getAgent(t, srv.BaseURL, id)
	if len(a.Roles) != 1 || a.Roles[0] != "worker" {
		t.Errorf("live roles after reset = %v, want [worker]", a.Roles)
	}
	if len(a.Skills) != 1 || a.Skills[0] != "backend" {
		t.Errorf("live skills after reset = %v, want [backend]", a.Skills)
	}
}

func TestHeartbeatResponse_CarriesLiveConfig(t *testing.T) {
	t.Parallel()
	srv := newGitTestServer(t)
	id := registerWithSkills(t, srv.BaseURL, "hb-agent", []string{"worker"}, []string{"backend"})

	apiJSON(t, "PATCH", srv.BaseURL, "/api/agents/"+id,
		map[string]interface{}{"roles": []string{"worker", "reviewer"}, "skills": []string{"backend", "go"}}, nil)

	var hb struct {
		DesiredState string   `json:"desired_state"`
		Roles        []string `json:"roles"`
		Skills       []string `json:"skills"`
	}
	apiJSON(t, "POST", srv.BaseURL, "/api/agents/"+id+"/heartbeat", nil, &hb)
	if len(hb.Roles) != 2 {
		t.Errorf("heartbeat roles = %v, want 2 (live)", hb.Roles)
	}
	if len(hb.Skills) != 2 {
		t.Errorf("heartbeat skills = %v, want 2 (live)", hb.Skills)
	}
	if hb.DesiredState != "run" {
		t.Errorf("desired_state = %q, want run", hb.DesiredState)
	}
}
