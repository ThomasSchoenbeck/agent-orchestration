package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestTaskMemory_AbsentIsNotError(t *testing.T) {
	d := openTestDB(t)
	got, err := d.GetTaskMemory(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetTaskMemory: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for absent memory, got %+v", got)
	}
}

func TestTaskMemory_UpsertRoundTrip(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	m := &db.TaskMemory{
		TaskID: "task-1",
		Content: db.TaskMemoryContent{
			Summary:       "Implement widget",
			Progress:      []string{"read spec", "wrote parser"},
			Decisions:     []string{"use option A"},
			Findings:      []string{"config lives in cfg.go"},
			OpenQuestions: []string{"which model for review?"},
		},
	}
	if err := d.UpsertTaskMemory(ctx, m); err != nil {
		t.Fatalf("UpsertTaskMemory: %v", err)
	}

	got, err := d.GetTaskMemory(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMemory: %v", err)
	}
	if got == nil {
		t.Fatal("expected memory, got nil")
	}
	if got.Content.Summary != "Implement widget" {
		t.Errorf("summary = %q", got.Content.Summary)
	}
	if len(got.Content.Progress) != 2 || got.Content.Progress[1] != "wrote parser" {
		t.Errorf("progress = %v", got.Content.Progress)
	}
	if len(got.Content.OpenQuestions) != 1 {
		t.Errorf("open questions = %v", got.Content.OpenQuestions)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("updated_at should be set")
	}
}

func TestTaskMemory_UpsertReplacesSingleRow(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.UpsertTaskMemory(ctx, &db.TaskMemory{
		TaskID:  "task-1",
		Content: db.TaskMemoryContent{Summary: "v1", Progress: []string{"a"}},
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := d.UpsertTaskMemory(ctx, &db.TaskMemory{
		TaskID:  "task-1",
		Content: db.TaskMemoryContent{Summary: "v2", Progress: []string{"a", "b"}},
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := d.GetTaskMemory(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskMemory: %v", err)
	}
	if got.Content.Summary != "v2" || len(got.Content.Progress) != 2 {
		t.Errorf("expected replaced content v2/2-entries, got %+v", got.Content)
	}
	// A different task keeps its own memory independent.
	if err := d.UpsertTaskMemory(ctx, &db.TaskMemory{TaskID: "task-2", Content: db.TaskMemoryContent{Summary: "other"}}); err != nil {
		t.Fatalf("task-2 upsert: %v", err)
	}
	other, _ := d.GetTaskMemory(ctx, "task-2")
	if other == nil || other.Content.Summary != "other" {
		t.Errorf("task-2 memory = %+v", other)
	}
}
