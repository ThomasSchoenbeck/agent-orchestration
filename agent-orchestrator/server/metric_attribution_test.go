package server_test

import (
	"context"
	"net/http"
	"testing"

	"agent-orchestrator/db"
)

// TestTaskResult_RecordsAgentMetricWithAttribution verifies that submitting a
// task result records a metric tagged source=agent with the task's role, so it
// shows up in the cost breakdown.
func TestTaskResult_RecordsAgentMetricWithAttribution(t *testing.T) {
	srv, database := newTestServer(t)
	pid := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": pid, "role": "worker", "priority": 5,
	})
	if tw.Code != http.StatusCreated {
		t.Fatalf("create task: %d %s", tw.Code, tw.Body.String())
	}
	taskID, _ := decodeMap(t, tw.Body.Bytes())["id"].(string)

	rw := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/result", map[string]interface{}{
		"status": "COMPLETED",
		"result": map[string]interface{}{"output": "done"},
		"metrics": map[string]interface{}{
			"model": "gpt-4o", "input_tokens": 100, "output_tokens": 20, "cost": 0.01,
		},
	})
	if rw.Code != http.StatusOK {
		t.Fatalf("submit result: %d %s", rw.Code, rw.Body.String())
	}

	ctx := context.Background()
	bySource, err := database.CostBreakdown(ctx, "source")
	if err != nil {
		t.Fatalf("CostBreakdown(source): %v", err)
	}
	var agent *db.CostBucket
	for _, b := range bySource {
		if b.Key == "agent" {
			agent = b
		}
	}
	if agent == nil || agent.Count != 1 {
		t.Fatalf("expected 1 agent metric, got %+v", bySource)
	}

	byRole, _ := database.CostBreakdown(ctx, "agent_role")
	hasWorker := false
	for _, b := range byRole {
		if b.Key == "worker" {
			hasWorker = true
		}
	}
	if !hasWorker {
		t.Errorf("expected a 'worker' role bucket, got %+v", byRole)
	}

	// Stage C: the breakdown is exposed via GET /api/metrics/costs?group_by=.
	w := do(t, srv, http.MethodGet, "/api/metrics/costs?group_by=source", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("group_by costs: %d %s", w.Code, w.Body.String())
	}
	list := decodeList(t, w.Body.Bytes())
	foundAgent := false
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok && m["key"] == "agent" {
			foundAgent = true
		}
	}
	if !foundAgent {
		t.Errorf("group_by=source should include an 'agent' bucket, got %v", list)
	}

	// Without group_by, the legacy cost summary shape is preserved.
	if w := do(t, srv, http.MethodGet, "/api/metrics/costs", nil); w.Code != http.StatusOK {
		t.Errorf("legacy costs endpoint: %d", w.Code)
	}
}
