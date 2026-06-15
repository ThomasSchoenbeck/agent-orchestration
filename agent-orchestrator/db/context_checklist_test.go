package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestContextEntryCreateAndQuery(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)

	for _, content := range []string{"microservices architecture", "uses postgres"} {
		if err := d.CreateContextEntry(ctx, &db.ContextEntry{ProjectID: pid, Type: "note", Content: content}); err != nil {
			t.Fatalf("CreateContextEntry: %v", err)
		}
	}

	// Keyword query (no embedder) should find the matching entry.
	hits, err := d.QueryContext(ctx, pid, "microservices", 10)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	if len(hits) == 0 {
		t.Error("QueryContext found no entries for a known keyword")
	}

	// Prune keeps at most maxItems entries.
	if err := d.PruneProjectContext(ctx, pid, 1); err != nil {
		t.Fatalf("PruneProjectContext: %v", err)
	}
	all, err := d.GetContextEntriesWithEmbeddings(ctx, pid)
	if err != nil {
		t.Fatalf("GetContextEntriesWithEmbeddings: %v", err)
	}
	if len(all) > 1 {
		t.Errorf("after prune to 1, have %d entries", len(all))
	}
}

func TestChecklistTemplateCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	tpl := &db.ChecklistTemplate{Name: "Definition of Done", ItemsJSON: `["tests pass","docs updated"]`}
	if err := d.CreateChecklistTemplate(ctx, tpl); err != nil {
		t.Fatalf("CreateChecklistTemplate: %v", err)
	}
	if tpl.ID == "" {
		t.Fatal("CreateChecklistTemplate did not assign an id")
	}

	got, err := d.GetChecklistTemplate(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("GetChecklistTemplate: %v", err)
	}
	if got.Name != "Definition of Done" {
		t.Errorf("GetChecklistTemplate name = %q", got.Name)
	}

	got.Name = "DoD v2"
	if err := d.UpdateChecklistTemplate(ctx, got); err != nil {
		t.Fatalf("UpdateChecklistTemplate: %v", err)
	}
	if re, _ := d.GetChecklistTemplate(ctx, tpl.ID); re.Name != "DoD v2" {
		t.Errorf("update not persisted: %q", re.Name)
	}

	if list, err := d.ListChecklistTemplates(ctx); err != nil || len(list) != 1 {
		t.Errorf("ListChecklistTemplates = %d (err=%v), want 1", len(list), err)
	}

	if err := d.DeleteChecklistTemplate(ctx, tpl.ID); err != nil {
		t.Fatalf("DeleteChecklistTemplate: %v", err)
	}
	if _, err := d.GetChecklistTemplate(ctx, tpl.ID); err == nil {
		t.Error("GetChecklistTemplate after delete should error")
	}
}

func TestChecklistItemsAndIteration(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)
	taskID := newTaskInProject(t, d, pid)

	item := &db.ChecklistItem{TaskID: taskID, GroupLabel: "attempt 1", Position: 0, Label: "write tests", Status: "pending"}
	if err := d.CreateChecklistItem(ctx, item); err != nil {
		t.Fatalf("CreateChecklistItem: %v", err)
	}

	item.Status = "passed"
	if err := d.UpdateChecklistItem(ctx, item); err != nil {
		t.Fatalf("UpdateChecklistItem: %v", err)
	}
	items, err := d.ListChecklistItems(ctx, taskID)
	if err != nil || len(items) != 1 || items[0].Status != "passed" {
		t.Fatalf("ListChecklistItems = %+v (err=%v)", items, err)
	}

	// CloneChecklistIteration duplicates the latest group with reset status.
	newGroup, err := d.CloneChecklistIteration(ctx, taskID)
	if err != nil {
		t.Fatalf("CloneChecklistIteration: %v", err)
	}
	if newGroup == "" {
		t.Error("CloneChecklistIteration returned an empty group label")
	}
	after, _ := d.ListChecklistItems(ctx, taskID)
	if len(after) != 2 {
		t.Errorf("after clone have %d items, want 2", len(after))
	}

	if err := d.DeleteChecklistItem(ctx, taskID, item.ID); err != nil {
		t.Fatalf("DeleteChecklistItem: %v", err)
	}
}
