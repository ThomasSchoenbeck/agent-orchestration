package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestPreparedPrompts_CreateAndListOrdered(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	// Empty task → no rows, not an error.
	got, err := d.ListPreparedPrompts(ctx, "t1")
	if err != nil {
		t.Fatalf("ListPreparedPrompts (empty): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no prompts, got %d", len(got))
	}

	for _, r := range []int{0, 1, 2} {
		if err := d.CreatePreparedPrompt(ctx, &db.PreparedPrompt{
			TaskID: "t1", SessionID: "s1", Round: r, Prompt: "prompt round",
		}); err != nil {
			t.Fatalf("CreatePreparedPrompt round %d: %v", r, err)
		}
	}
	// A different task must not leak in.
	if err := d.CreatePreparedPrompt(ctx, &db.PreparedPrompt{TaskID: "t2", Round: 0, Prompt: "other"}); err != nil {
		t.Fatalf("CreatePreparedPrompt t2: %v", err)
	}

	got, err = d.ListPreparedPrompts(ctx, "t1")
	if err != nil {
		t.Fatalf("ListPreparedPrompts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 prompts for t1, got %d", len(got))
	}
	for i, p := range got {
		if p.Round != i {
			t.Errorf("prompt %d: round = %d, want %d", i, p.Round, i)
		}
		if p.ID == "" {
			t.Errorf("prompt %d: expected generated ID", i)
		}
		if p.CreatedAt.IsZero() {
			t.Errorf("prompt %d: expected created_at set", i)
		}
	}
}
