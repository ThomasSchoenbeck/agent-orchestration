package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func newScopeProject(t *testing.T, d *db.Database) string {
	t.Helper()
	p := &db.Project{Name: "Scope P"}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return p.ID
}

func TestRequirementCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)

	r := &db.ProjectRequirement{ProjectID: pid, Title: "R1", Body: "must do", Status: "proposed"}
	if err := d.CreateRequirement(ctx, r); err != nil {
		t.Fatalf("CreateRequirement: %v", err)
	}
	if r.ID == "" {
		t.Fatal("CreateRequirement did not assign an id")
	}

	got, err := d.GetRequirement(ctx, r.ID)
	if err != nil {
		t.Fatalf("GetRequirement: %v", err)
	}
	if got.Title != "R1" || got.Status != "proposed" {
		t.Errorf("GetRequirement = %+v", got)
	}

	got.Status = "satisfied"
	if err := d.UpdateRequirement(ctx, got); err != nil {
		t.Fatalf("UpdateRequirement: %v", err)
	}
	if re, _ := d.GetRequirement(ctx, r.ID); re.Status != "satisfied" {
		t.Errorf("update not persisted: %q", re.Status)
	}

	if list, err := d.ListRequirements(ctx, pid); err != nil || len(list) != 1 {
		t.Errorf("ListRequirements = %d (err=%v), want 1", len(list), err)
	}

	counts, err := d.CountLinkedTasksForRequirements(ctx, pid)
	if err != nil {
		t.Fatalf("CountLinkedTasksForRequirements: %v", err)
	}
	if counts[r.ID] != 0 {
		t.Errorf("unlinked requirement count = %d, want 0", counts[r.ID])
	}

	if err := d.DeleteRequirement(ctx, r.ID); err != nil {
		t.Fatalf("DeleteRequirement: %v", err)
	}
	if _, err := d.GetRequirement(ctx, r.ID); err == nil {
		t.Error("GetRequirement after delete should error")
	}
}

func TestFeatureCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	pid := newScopeProject(t, d)

	f := &db.ProjectFeature{ProjectID: pid, Title: "F1", Body: "feature", Status: "planned"}
	if err := d.CreateFeature(ctx, f); err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	if f.ID == "" {
		t.Fatal("CreateFeature did not assign an id")
	}

	got, err := d.GetFeature(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}
	if got.Title != "F1" || got.Status != "planned" {
		t.Errorf("GetFeature = %+v", got)
	}

	got.Status = "done"
	if err := d.UpdateFeature(ctx, got); err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}
	if re, _ := d.GetFeature(ctx, f.ID); re.Status != "done" {
		t.Errorf("update not persisted: %q", re.Status)
	}

	if list, err := d.ListFeatures(ctx, pid); err != nil || len(list) != 1 {
		t.Errorf("ListFeatures = %d (err=%v), want 1", len(list), err)
	}

	counts, err := d.CountLinkedTasksForFeatures(ctx, pid)
	if err != nil {
		t.Fatalf("CountLinkedTasksForFeatures: %v", err)
	}
	if counts[f.ID] != 0 {
		t.Errorf("unlinked feature count = %d, want 0", counts[f.ID])
	}

	if err := d.DeleteFeature(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFeature: %v", err)
	}
	if _, err := d.GetFeature(ctx, f.ID); err == nil {
		t.Error("GetFeature after delete should error")
	}
}
