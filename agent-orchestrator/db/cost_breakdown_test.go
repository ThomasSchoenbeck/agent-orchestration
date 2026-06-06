package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestCostBreakdown_BySourceAndRole(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	mk := func(m *db.Metric) {
		if err := d.CreateMetric(ctx, m); err != nil {
			t.Fatalf("CreateMetric: %v", err)
		}
	}
	mk(&db.Metric{Source: "agent", AgentRole: "worker", AgentID: "a1", InputTokens: 100, OutputTokens: 20, Cost: 0.01})
	mk(&db.Metric{Source: "agent", AgentRole: "worker", AgentID: "a2", InputTokens: 50, OutputTokens: 10, Cost: 0.02})
	mk(&db.Metric{Source: "chat", ConversationID: "c1", InputTokens: 200, OutputTokens: 40, Cost: 0.05})

	bySource, err := d.CostBreakdown(ctx, "source")
	if err != nil {
		t.Fatalf("CostBreakdown(source): %v", err)
	}
	got := map[string]*db.CostBucket{}
	for _, b := range bySource {
		got[b.Key] = b
	}
	if got["agent"] == nil || got["chat"] == nil {
		t.Fatalf("expected agent + chat buckets, got %+v", bySource)
	}
	if got["agent"].Count != 2 {
		t.Errorf("agent count = %d, want 2", got["agent"].Count)
	}
	if got["chat"].Cost < 0.049 || got["chat"].Cost > 0.051 {
		t.Errorf("chat cost = %f, want ~0.05", got["chat"].Cost)
	}

	byRole, err := d.CostBreakdown(ctx, "agent_role")
	if err != nil {
		t.Fatalf("CostBreakdown(agent_role): %v", err)
	}
	var workerCount int
	for _, b := range byRole {
		if b.Key == "worker" {
			workerCount = b.Count
		}
	}
	if workerCount != 2 {
		t.Errorf("worker-role count = %d, want 2", workerCount)
	}

	// Unknown group_by falls back to source without error.
	if _, err := d.CostBreakdown(ctx, "bogus"); err != nil {
		t.Fatalf("fallback group_by: %v", err)
	}
}
