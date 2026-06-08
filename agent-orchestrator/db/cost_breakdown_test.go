package db_test

import (
	"context"
	"testing"
	"time"

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

	bySource, err := d.CostBreakdown(ctx, "source", db.CostFilter{})
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

	byRole, err := d.CostBreakdown(ctx, "agent_role", db.CostFilter{})
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
	if _, err := d.CostBreakdown(ctx, "bogus", db.CostFilter{}); err != nil {
		t.Fatalf("fallback group_by: %v", err)
	}
}

func TestCostBreakdown_Filters(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	mk := func(m *db.Metric) {
		if err := d.CreateMetric(ctx, m); err != nil {
			t.Fatalf("CreateMetric: %v", err)
		}
	}
	old := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	mk(&db.Metric{Source: "agent", AgentRole: "worker", Model: "gpt-4", ProviderID: "p1", Cost: 0.10, CreatedAt: old})
	mk(&db.Metric{Source: "agent", AgentRole: "reviewer", Model: "gpt-4", ProviderID: "p1", Cost: 0.20, CreatedAt: recent})
	mk(&db.Metric{Source: "chat", AgentRole: "", Model: "claude", ProviderID: "p2", Cost: 0.40, CreatedAt: recent})

	sumCost := func(buckets []*db.CostBucket) float64 {
		var s float64
		for _, b := range buckets {
			s += b.Cost
		}
		return s
	}

	// Model filter.
	got, err := d.CostBreakdown(ctx, "source", db.CostFilter{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("filter model: %v", err)
	}
	if c := sumCost(got); c < 0.299 || c > 0.301 {
		t.Errorf("model=gpt-4 total = %f, want ~0.30", c)
	}

	// AgentRole ("task type") filter.
	got, _ = d.CostBreakdown(ctx, "source", db.CostFilter{AgentRole: "reviewer"})
	if c := sumCost(got); c < 0.199 || c > 0.201 {
		t.Errorf("agent_role=reviewer total = %f, want ~0.20", c)
	}

	// Source filter.
	got, _ = d.CostBreakdown(ctx, "model", db.CostFilter{Source: "chat"})
	if c := sumCost(got); c < 0.399 || c > 0.401 {
		t.Errorf("source=chat total = %f, want ~0.40", c)
	}

	// Provider filter.
	got, _ = d.CostBreakdown(ctx, "source", db.CostFilter{ProviderID: "p1"})
	if c := sumCost(got); c < 0.299 || c > 0.301 {
		t.Errorf("provider=p1 total = %f, want ~0.30", c)
	}

	// Date range: From excludes the January row (inclusive To handled at handler layer).
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	got, _ = d.CostBreakdown(ctx, "source", db.CostFilter{From: &from})
	if c := sumCost(got); c < 0.599 || c > 0.601 {
		t.Errorf("from=2026-05-01 total = %f, want ~0.60", c)
	}

	// To bound (exclusive) keeps only the January row.
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	got, _ = d.CostBreakdown(ctx, "source", db.CostFilter{To: &to})
	if c := sumCost(got); c < 0.099 || c > 0.101 {
		t.Errorf("to=2026-02-01 total = %f, want ~0.10", c)
	}

	// Combined filters intersect.
	got, _ = d.CostBreakdown(ctx, "source", db.CostFilter{Model: "gpt-4", AgentRole: "worker"})
	if c := sumCost(got); c < 0.099 || c > 0.101 {
		t.Errorf("model+role total = %f, want ~0.10", c)
	}
}

func TestCostBreakdown_TaskAndChatDimensions(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	mk := func(m *db.Metric) {
		if err := d.CreateMetric(ctx, m); err != nil {
			t.Fatalf("CreateMetric: %v", err)
		}
	}
	mk(&db.Metric{Source: "agent", TaskID: "t1", Cost: 0.10})
	mk(&db.Metric{Source: "agent", TaskID: "t1", Cost: 0.20})
	mk(&db.Metric{Source: "chat", ConversationID: "c1", Cost: 0.05})

	byTask, err := d.CostBreakdown(ctx, "task", db.CostFilter{})
	if err != nil {
		t.Fatalf("CostBreakdown(task): %v", err)
	}
	var t1 *db.CostBucket
	for _, b := range byTask {
		if b.Key == "t1" {
			t1 = b
		}
	}
	if t1 == nil || t1.Count != 2 {
		t.Fatalf("task t1 = %+v, want count 2", t1)
	}

	byChat, err := d.CostBreakdown(ctx, "chat", db.CostFilter{})
	if err != nil {
		t.Fatalf("CostBreakdown(chat): %v", err)
	}
	var found bool
	for _, b := range byChat {
		if b.Key == "c1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected conversation c1 bucket, got %+v", byChat)
	}
}

func TestCostFilterOptions(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	mk := func(m *db.Metric) {
		if err := d.CreateMetric(ctx, m); err != nil {
			t.Fatalf("CreateMetric: %v", err)
		}
	}
	mk(&db.Metric{Source: "agent", AgentRole: "worker", Model: "gpt-4", ProviderID: "p1", Cost: 0.1,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	mk(&db.Metric{Source: "chat", Model: "claude", ProviderID: "p2", Cost: 0.2,
		CreatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)})

	opts, err := d.CostFilterOptions(ctx)
	if err != nil {
		t.Fatalf("CostFilterOptions: %v", err)
	}
	if len(opts.Models) != 2 {
		t.Errorf("models = %v, want 2", opts.Models)
	}
	if len(opts.Providers) != 2 {
		t.Errorf("providers = %v, want 2", opts.Providers)
	}
	// agent_role empty string is excluded, so only "worker" remains.
	if len(opts.AgentRoles) != 1 || opts.AgentRoles[0] != "worker" {
		t.Errorf("agent_roles = %v, want [worker]", opts.AgentRoles)
	}
	if opts.MinDate != "2026-01-01" || opts.MaxDate != "2026-06-01" {
		t.Errorf("date bounds = %s..%s, want 2026-01-01..2026-06-01", opts.MinDate, opts.MaxDate)
	}
}
