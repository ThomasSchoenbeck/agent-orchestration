package server

import (
	"net/url"
	"testing"
)

func TestDefaultToolsForRole(t *testing.T) {
	has := func(list []string, s string) bool {
		for _, v := range list {
			if v == s {
				return true
			}
		}
		return false
	}
	if tools := defaultToolsForRole("worker"); !has(tools, "read_file") || !has(tools, "apply_diff") {
		t.Errorf("worker tools = %v", tools)
	}
	if tools := defaultToolsForRole("reviewer"); !has(tools, "read_file") {
		t.Errorf("reviewer tools = %v", tools)
	}
	if tools := defaultToolsForRole("orchestrator"); !has(tools, "plan_project") {
		t.Errorf("orchestrator tools = %v", tools)
	}
	if tools := defaultToolsForRole("unknown-role"); tools != nil {
		t.Errorf("unknown role should get nil (no restriction), got %v", tools)
	}
}

func TestParseCostFilter(t *testing.T) {
	q := url.Values{}
	q.Set("model", "gpt-4o")
	q.Set("agent_role", "worker")
	q.Set("source", "chat")
	q.Set("provider", "p1")
	q.Set("from", "2024-01-01")
	q.Set("to", "2024-12-31")
	f := parseCostFilter(q)
	if f.Model != "gpt-4o" || f.AgentRole != "worker" || f.Source != "chat" || f.ProviderID != "p1" {
		t.Errorf("parseCostFilter scalar fields = %+v", f)
	}
	if f.From == nil || f.To == nil {
		t.Errorf("parseCostFilter dates not parsed: from=%v to=%v", f.From, f.To)
	}

	// Invalid dates are ignored (left nil).
	bad := url.Values{}
	bad.Set("from", "not-a-date")
	bad.Set("to", "also-bad")
	if f := parseCostFilter(bad); f.From != nil || f.To != nil {
		t.Errorf("invalid dates should be ignored, got from=%v to=%v", f.From, f.To)
	}
}
