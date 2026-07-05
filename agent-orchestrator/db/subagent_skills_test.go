package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestSubagentSkill_CRUDRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	sk := &db.SubagentSkill{
		Name:           "investigate_codebase",
		Label:          "Investigate Codebase",
		Description:    "Read-only exploration",
		PromptTemplate: "Explore: {{instructions}}. Then summarize.",
		ToolAllowlist:  []string{"read_file", "list_files"},
		ContextInclude: []string{"server/**"},
		ContextExclude: []string{"**/*_test.go"},
		MaxRounds:      6,
		MaxTokens:      4000,
		Enabled:        true,
	}
	if err := d.CreateSubagentSkill(ctx, sk); err != nil {
		t.Fatalf("CreateSubagentSkill: %v", err)
	}
	if sk.ID == "" {
		t.Fatal("expected generated ID")
	}

	got, err := d.GetSubagentSkillByName(ctx, "investigate_codebase")
	if err != nil {
		t.Fatalf("GetSubagentSkillByName: %v", err)
	}
	if got.PromptTemplate != "Explore: {{instructions}}. Then summarize." {
		t.Errorf("prompt template = %q", got.PromptTemplate)
	}
	if len(got.ToolAllowlist) != 2 || got.ToolAllowlist[0] != "read_file" {
		t.Errorf("tool allowlist = %v", got.ToolAllowlist)
	}
	if got.MaxRounds != 6 || got.MaxTokens != 4000 {
		t.Errorf("bounds = rounds %d tokens %d", got.MaxRounds, got.MaxTokens)
	}
	if len(got.ContextInclude) != 1 || len(got.ContextExclude) != 1 {
		t.Errorf("context globs = inc %v exc %v", got.ContextInclude, got.ContextExclude)
	}

	got.PromptTemplate = "Updated."
	got.Enabled = false
	if err := d.UpdateSubagentSkill(ctx, got); err != nil {
		t.Fatalf("UpdateSubagentSkill: %v", err)
	}
	again, _ := d.GetSubagentSkill(ctx, got.ID)
	if again.PromptTemplate != "Updated." || again.Enabled {
		t.Errorf("update not persisted: %+v", again)
	}

	if err := d.DeleteSubagentSkill(ctx, got.ID); err != nil {
		t.Fatalf("DeleteSubagentSkill: %v", err)
	}
	if _, err := d.GetSubagentSkill(ctx, got.ID); err == nil {
		t.Error("expected error after delete")
	}
}

func TestCreateSubagentSkill_DefaultsMaxRounds(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	sk := &db.SubagentSkill{Name: "no_rounds", Enabled: true}
	if err := d.CreateSubagentSkill(ctx, sk); err != nil {
		t.Fatalf("CreateSubagentSkill: %v", err)
	}
	got, _ := d.GetSubagentSkillByName(ctx, "no_rounds")
	if got.MaxRounds == 0 {
		t.Error("expected max_rounds to default to a non-zero bound")
	}
}

func TestCreateSubagentSkill_DuplicateNameErrors(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.CreateSubagentSkill(ctx, &db.SubagentSkill{Name: "dup", Enabled: true}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := d.CreateSubagentSkill(ctx, &db.SubagentSkill{Name: "dup", Enabled: true}); err == nil {
		t.Error("expected duplicate-name insert to error")
	}
}

func TestSeedSubagentSkills_Idempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	defaults := db.DefaultSubagentSkills()
	n1, err := d.SeedSubagentSkills(ctx, defaults)
	if err != nil {
		t.Fatalf("seed 1: %v", err)
	}
	if n1 != len(defaults) {
		t.Fatalf("expected to seed %d default subagent skills, got %d", len(defaults), n1)
	}

	got, err := d.GetSubagentSkillByName(ctx, "investigate_codebase")
	if err != nil {
		t.Fatalf("seeded skill not found: %v", err)
	}
	if !got.Enabled {
		t.Error("seeded investigate_codebase should be enabled")
	}

	// No default subagent skill may grant run_subagent — subagents cannot nest.
	for _, s := range defaults {
		for _, tool := range s.ToolAllowlist {
			if tool == "run_subagent" {
				t.Errorf("default subagent skill %q allowlist must not include run_subagent", s.Name)
			}
		}
	}

	n2, err := d.SeedSubagentSkills(ctx, db.DefaultSubagentSkills())
	if err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second seed inserted %d, want 0", n2)
	}
}

func TestDefaultSubagentSkills_InvestigateHasSearchFiles(t *testing.T) {
	var inv *db.SubagentSkill
	for _, s := range db.DefaultSubagentSkills() {
		if s.Name == "investigate_codebase" {
			inv = s
		}
	}
	if inv == nil {
		t.Fatal("expected investigate_codebase default skill")
	}
	var found bool
	for _, tool := range inv.ToolAllowlist {
		if tool == "search_files" {
			found = true
		}
	}
	if !found {
		t.Errorf("investigate_codebase must allow search_files (got %v)", inv.ToolAllowlist)
	}
}

func TestDefaultSubagentSkills_CodeSubtaskHasWriteTools(t *testing.T) {
	var code *db.SubagentSkill
	for _, s := range db.DefaultSubagentSkills() {
		if s.Name == "code_subtask" {
			code = s
		}
	}
	if code == nil {
		t.Fatal("expected a code_subtask default subagent skill")
	}
	want := map[string]bool{"write_file": false, "apply_diff": false, "run_tests": false}
	for _, tool := range code.ToolAllowlist {
		if _, ok := want[tool]; ok {
			want[tool] = true
		}
	}
	for tool, found := range want {
		if !found {
			t.Errorf("code_subtask missing write tool %q", tool)
		}
	}
}

func TestDefaultSubagentSkills_TaskStatusIsReadOnlyRecovery(t *testing.T) {
	var ts *db.SubagentSkill
	for _, s := range db.DefaultSubagentSkills() {
		if s.Name == "task_status" {
			ts = s
		}
	}
	if ts == nil {
		t.Fatal("expected a task_status default subagent skill")
	}
	if !ts.Enabled {
		t.Error("task_status should be enabled")
	}
	// It reconstructs progress from the durable record: must have get_task_progress
	// and read_memory, and must not carry any write tools.
	allow := map[string]bool{}
	for _, tool := range ts.ToolAllowlist {
		allow[tool] = true
	}
	for _, want := range []string{"get_task_progress", "read_memory"} {
		if !allow[want] {
			t.Errorf("task_status must allow %q (got %v)", want, ts.ToolAllowlist)
		}
	}
	for _, forbidden := range []string{"write_file", "apply_diff", "write_memory"} {
		if allow[forbidden] {
			t.Errorf("task_status must be read-only, but allows %q", forbidden)
		}
	}
}

func TestDefaultSubagentSkills_PromptPrepIsToollessOneShot(t *testing.T) {
	var pp *db.SubagentSkill
	for _, s := range db.DefaultSubagentSkills() {
		if s.Name == "prompt_prep" {
			pp = s
		}
	}
	if pp == nil {
		t.Fatal("expected a prompt_prep default subagent skill")
	}
	if !pp.Enabled {
		t.Error("prompt_prep should be enabled")
	}
	if len(pp.ToolAllowlist) != 0 {
		t.Errorf("prompt_prep must carry no tools (one-shot synthesis), got %v", pp.ToolAllowlist)
	}
	if pp.MaxRounds != 1 {
		t.Errorf("prompt_prep should be one-shot (max_rounds 1), got %d", pp.MaxRounds)
	}
}

func TestDefaultSubagentSkills_ReviewSubtaskIsReadOnly(t *testing.T) {
	var rv *db.SubagentSkill
	for _, s := range db.DefaultSubagentSkills() {
		if s.Name == "review_subtask" {
			rv = s
		}
	}
	if rv == nil {
		t.Fatal("expected a review_subtask default subagent skill")
	}
	allow := map[string]bool{}
	for _, tool := range rv.ToolAllowlist {
		allow[tool] = true
	}
	if !allow["read_file"] || !allow["search_files"] {
		t.Errorf("review_subtask must allow read_file + search_files (got %v)", rv.ToolAllowlist)
	}
	for _, forbidden := range []string{"write_file", "apply_diff", "run_tests"} {
		if allow[forbidden] {
			t.Errorf("review_subtask must be read-only, but allows %q", forbidden)
		}
	}
}
