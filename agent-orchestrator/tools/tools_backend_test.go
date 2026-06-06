package tools_test

import (
	"context"
	"testing"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/tools"
)

// fakeBackend records calls and returns canned data, implementing tools.ToolBackend.
type fakeBackend struct {
	listFilters   db.TaskFilters
	tasks         []*db.Task
	savedEntry    *db.ContextEntry
	commentArgs   [4]string // taskID, body, role, reviewID
	planProjectID string
	planArch      string
	planPackages  []tools.WorkPackageInput
	completeID    string
}

func (f *fakeBackend) ListTasks(_ context.Context, fl db.TaskFilters) ([]*db.Task, error) {
	f.listFilters = fl
	return f.tasks, nil
}
func (f *fakeBackend) PeekNextTask(context.Context, []string) (*db.Task, error) { return nil, nil }
func (f *fakeBackend) SubmitTaskResult(context.Context, string, map[string]interface{}, string, *api.TaskMetrics) error {
	return nil
}
func (f *fakeBackend) SaveContext(_ context.Context, e *db.ContextEntry) (*db.ContextEntry, error) {
	f.savedEntry = e
	return &db.ContextEntry{ID: "ctx-1", ProjectID: e.ProjectID, Type: e.Type, Content: e.Content}, nil
}
func (f *fakeBackend) QueryContext(context.Context, string, string, int) ([]*db.ContextEntry, error) {
	return []*db.ContextEntry{{ID: "c1"}}, nil
}
func (f *fakeBackend) PostTaskComment(_ context.Context, taskID, body, role, reviewID string) (string, error) {
	f.commentArgs = [4]string{taskID, body, role, reviewID}
	return "cmt-1", nil
}
func (f *fakeBackend) PlanProject(_ context.Context, projectID, arch string, pkgs []tools.WorkPackageInput) (map[string]interface{}, error) {
	f.planProjectID, f.planArch, f.planPackages = projectID, arch, pkgs
	return map[string]interface{}{"success": true, "task_count": len(pkgs)}, nil
}
func (f *fakeBackend) CreateWorkPackage(context.Context, string, tools.WorkPackageInput) (map[string]interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}
func (f *fakeBackend) BootstrapProject(context.Context, string, []tools.ScopeItemInput, []tools.ScopeItemInput) (map[string]interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}
func (f *fakeBackend) SyncScope(context.Context, string, []tools.ScopeItemInput, []tools.ScopeItemInput) (map[string]interface{}, error) {
	return map[string]interface{}{"success": true}, nil
}
func (f *fakeBackend) CompleteProject(_ context.Context, projectID, _ string) (map[string]interface{}, error) {
	f.completeID = projectID
	return map[string]interface{}{"success": true}, nil
}

func registryWith(t *testing.T, b tools.ToolBackend) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	for _, regFn := range []func(*tools.Registry, tools.ToolBackend) error{
		tools.RegisterTaskTools, tools.RegisterPlanTools,
		tools.RegisterContextTools, tools.RegisterCommentTools,
	} {
		if err := regFn(reg, b); err != nil {
			t.Fatalf("register tools: %v", err)
		}
	}
	return reg
}

func TestListTasksTool_PassesFiltersToBackend(t *testing.T) {
	f := &fakeBackend{tasks: []*db.Task{{ID: "t1"}, {ID: "t2"}}}
	reg := registryWith(t, f)
	res, err := reg.Execute(context.Background(), "list_tasks", map[string]interface{}{
		"project_id": "p1", "status": "BACKLOG", "limit": float64(7),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if f.listFilters.ProjectID != "p1" || f.listFilters.Status != "BACKLOG" || f.listFilters.Limit != 7 {
		t.Errorf("filters not passed through: %+v", f.listFilters)
	}
	if m := res.(map[string]interface{}); m["count"] != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
}

func TestSaveContextTool_ReturnsBackendID(t *testing.T) {
	f := &fakeBackend{}
	reg := registryWith(t, f)
	res, err := reg.Execute(context.Background(), "save_context", map[string]interface{}{
		"project_id": "p1", "type": "note", "content": "hello",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if f.savedEntry == nil || f.savedEntry.Content != "hello" {
		t.Errorf("entry not forwarded: %+v", f.savedEntry)
	}
	if m := res.(map[string]interface{}); m["context_id"] != "ctx-1" {
		t.Errorf("context_id = %v, want ctx-1", m["context_id"])
	}
}

func TestTaskCommentTool_ForwardsRoleAndReview(t *testing.T) {
	f := &fakeBackend{}
	reg := registryWith(t, f)
	res, err := reg.Execute(context.Background(), "task_comment", map[string]interface{}{
		"task_id": "t1", "body": "hi", "role": "reviewer", "review_id": "rv1",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if f.commentArgs != [4]string{"t1", "hi", "reviewer", "rv1"} {
		t.Errorf("comment args = %v", f.commentArgs)
	}
	if m := res.(map[string]string); m["comment_id"] != "cmt-1" {
		t.Errorf("comment_id = %v", m["comment_id"])
	}
}

func TestPlanProjectTool_ParsesWorkPackages(t *testing.T) {
	f := &fakeBackend{}
	reg := registryWith(t, f)
	_, err := reg.Execute(context.Background(), "plan_project", map[string]interface{}{
		"project_id":    "p1",
		"architecture":  "layers",
		"work_packages": `[{"title":"a","description":"d","role":"worker","priority":3}]`,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if f.planProjectID != "p1" || f.planArch != "layers" {
		t.Errorf("plan args = %q/%q", f.planProjectID, f.planArch)
	}
	if len(f.planPackages) != 1 || f.planPackages[0].Title != "a" || f.planPackages[0].Priority != 3 {
		t.Errorf("packages not parsed: %+v", f.planPackages)
	}
}

func TestCompleteProjectTool_RequiresCapability(t *testing.T) {
	f := &fakeBackend{}
	reg := registryWith(t, f)
	// Context scoped WITHOUT creates_tasks → gate denies, backend not called.
	// (An unscoped context is intentionally permissive; see contextHasCapability.)
	ctx := tools.WithCapabilities(context.Background(), []string{"reviews_code"})
	_, err := reg.Execute(ctx, "complete_project", map[string]interface{}{"project_id": "p1"})
	if err == nil {
		t.Fatal("expected error without creates_tasks capability")
	}
	if f.completeID != "" {
		t.Errorf("backend should not be called; got completeID=%q", f.completeID)
	}
}
