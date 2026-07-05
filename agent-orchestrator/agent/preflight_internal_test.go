package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-orchestrator/db"
)

// executorWithSkills builds a client-less executor whose subagent-skill cache is
// pre-populated with the named (enabled) skills, skipping the HTTP fetch.
func executorWithSkills(names ...string) *Executor {
	e := NewExecutor(nil, nil, nil, "a1")
	for _, n := range names {
		e.subagentSkills = append(e.subagentSkills, &db.SubagentSkill{Name: n, Enabled: true})
	}
	e.subagentSkillsResolved = true
	return e
}

func TestPreflight_WorkerAllPresentProceeds(t *testing.T) {
	e := executorWithSkills("prompt_prep", "investigate_codebase", "code_subtask", "task_status")
	task := &db.Task{ID: "t1", Role: "worker", Status: db.TaskStatusDeveloping}
	if err := e.preflightCoreSkills(context.Background(), task); err != nil {
		t.Errorf("expected preflight to pass, got %v", err)
	}
}

func TestPreflight_WorkerMissingWorkSkillFails(t *testing.T) {
	// code_subtask absent.
	e := executorWithSkills("prompt_prep", "investigate_codebase", "task_status")
	task := &db.Task{ID: "t1", Role: "worker", Status: db.TaskStatusDeveloping}
	err := e.preflightCoreSkills(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "code_subtask") {
		t.Errorf("expected failure naming code_subtask, got %v", err)
	}
}

func TestPreflight_ReviewerRequiresReviewSubtask(t *testing.T) {
	// Full worker set present, but a reviewer task needs review_subtask, not code_subtask.
	e := executorWithSkills("prompt_prep", "investigate_codebase", "code_subtask", "task_status")
	task := &db.Task{ID: "t1", Role: "worker", ReviewRole: "reviewer", Status: db.TaskStatusReviewing}
	err := e.preflightCoreSkills(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "review_subtask") {
		t.Errorf("expected failure naming review_subtask, got %v", err)
	}
}

func TestPreflight_ReviewerAllPresentProceeds(t *testing.T) {
	e := executorWithSkills("prompt_prep", "investigate_codebase", "review_subtask", "task_status")
	task := &db.Task{ID: "t1", Role: "worker", ReviewRole: "reviewer", Status: db.TaskStatusReviewing}
	if err := e.preflightCoreSkills(context.Background(), task); err != nil {
		t.Errorf("expected reviewer preflight to pass, got %v", err)
	}
}

func TestPreflight_MergeReviewTaskExempt(t *testing.T) {
	// No skills at all, but a merge-review task keeps its current path (exempt).
	e := executorWithSkills()
	task := &db.Task{ID: "t1", Role: "reviewer", Status: db.TaskStatusMerging}
	if err := e.preflightCoreSkills(context.Background(), task); err != nil {
		t.Errorf("merge-review task must be exempt from preflight, got %v", err)
	}
}

// A disabled required skill is filtered out of the resolved cache and must be
// treated as missing. Exercised through the real HTTP resolve path.
func TestPreflight_DisabledSkillCountsAsMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/subagent-skills", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]*db.SubagentSkill{
			{Name: "prompt_prep", Enabled: true},
			{Name: "investigate_codebase", Enabled: true},
			{Name: "code_subtask", Enabled: false}, // disabled → must count as missing
			{Name: "task_status", Enabled: true},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := NewExecutor(nil, nil, NewServerClient(srv.URL), "a1")
	task := &db.Task{ID: "t1", Role: "worker", Status: db.TaskStatusDeveloping}
	err := e.preflightCoreSkills(context.Background(), task)
	if err == nil || !strings.Contains(err.Error(), "code_subtask") {
		t.Errorf("disabled required skill should count as missing, got %v", err)
	}
}
