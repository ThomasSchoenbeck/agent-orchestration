package tools_test

import (
	"testing"

	"agent-orchestrator/tools"
)

// openContextToolDB returns a Registry with context tools registered and a project ID.
func openContextToolDB(t *testing.T) (*tools.Registry, string) {
	t.Helper()
	d, _, projectID := openPlanDB(t)

	reg := tools.NewRegistry()
	if err := tools.RegisterContextTools(reg, toolBackend(t, d)); err != nil {
		t.Fatalf("RegisterContextTools: %v", err)
	}
	return reg, projectID
}

// contextCount extracts the integer "count" from a query_context result.
// The handler returns a native int, so handle int and float64 defensively.
func contextCount(result map[string]interface{}) int {
	switch v := result["count"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return -1
}

// --- save_context ---

func TestSaveContext_Basic(t *testing.T) {
	reg, projectID := openContextToolDB(t)

	result := callTool(t, reg, "save_context", map[string]interface{}{
		"project_id": projectID,
		"type":       "summary",
		"content":    "The project uses a microservices architecture.",
	})

	if success, _ := result["success"].(bool); !success {
		t.Error("expected success=true")
	}
	contextID, _ := result["context_id"].(string)
	if contextID == "" {
		t.Error("expected non-empty context_id")
	}
}

func TestSaveContext_WithTaskID(t *testing.T) {
	d, _, projectID := openPlanDB(t)
	task := seedTask(t, d, projectID, "planned")

	reg := tools.NewRegistry()
	if err := tools.RegisterContextTools(reg, toolBackend(t, d)); err != nil {
		t.Fatalf("RegisterContextTools: %v", err)
	}

	result := callTool(t, reg, "save_context", map[string]interface{}{
		"project_id": projectID,
		"task_id":    task.ID,
		"type":       "note",
		"content":    "This task handles authentication.",
	})

	if success, _ := result["success"].(bool); !success {
		t.Error("expected success=true")
	}
}

func TestSaveContext_MissingProjectID(t *testing.T) {
	reg, _ := openContextToolDB(t)
	callToolExpectError(t, reg, "save_context", map[string]interface{}{
		"type":    "note",
		"content": "orphan context",
	}, "project_id")
}

func TestSaveContext_MissingContent(t *testing.T) {
	reg, projectID := openContextToolDB(t)
	callToolExpectError(t, reg, "save_context", map[string]interface{}{
		"project_id": projectID,
		"type":       "note",
	}, "content")
}

func TestSaveContext_DifferentTypes(t *testing.T) {
	reg, projectID := openContextToolDB(t)

	for _, ct := range []string{"summary", "snippet", "note", "diff", "test_results"} {
		result := callTool(t, reg, "save_context", map[string]interface{}{
			"project_id": projectID,
			"type":       ct,
			"content":    "content for " + ct,
		})
		if success, _ := result["success"].(bool); !success {
			t.Errorf("save_context(%q): expected success", ct)
		}
	}
}

// --- query_context ---

func TestQueryContext_Match(t *testing.T) {
	reg, projectID := openContextToolDB(t)

	callTool(t, reg, "save_context", map[string]interface{}{
		"project_id": projectID,
		"type":       "note",
		"content":    "We use JWT authentication for all API endpoints.",
	})
	callTool(t, reg, "save_context", map[string]interface{}{
		"project_id": projectID,
		"type":       "note",
		"content":    "Database schema uses UUID primary keys.",
	})

	result := callTool(t, reg, "query_context", map[string]interface{}{
		"project_id": projectID,
		"query":      "authentication",
	})

	if n := contextCount(result); n < 1 {
		t.Errorf("expected at least 1 match for 'authentication', got %d", n)
	}
}

func TestQueryContext_NoMatch(t *testing.T) {
	reg, projectID := openContextToolDB(t)

	callTool(t, reg, "save_context", map[string]interface{}{
		"project_id": projectID,
		"type":       "note",
		"content":    "Database schema details here.",
	})

	result := callTool(t, reg, "query_context", map[string]interface{}{
		"project_id": projectID,
		"query":      "xyzzy_unlikely_term_zzz",
	})

	if n := contextCount(result); n != 0 {
		t.Errorf("expected 0 matches for unknown term, got %d", n)
	}
}

func TestQueryContext_EmptyProject(t *testing.T) {
	reg, projectID := openContextToolDB(t)

	result := callTool(t, reg, "query_context", map[string]interface{}{
		"project_id": projectID,
		"query":      "anything",
	})
	if n := contextCount(result); n != 0 {
		t.Errorf("expected 0 results for empty project, got %d", n)
	}
}

func TestQueryContext_LimitRespected(t *testing.T) {
	reg, projectID := openContextToolDB(t)

	// Save 5 matching entries.
	for i := 0; i < 5; i++ {
		callTool(t, reg, "save_context", map[string]interface{}{
			"project_id": projectID,
			"type":       "note",
			"content":    "microservices architecture design decision",
		})
	}

	result := callTool(t, reg, "query_context", map[string]interface{}{
		"project_id": projectID,
		"query":      "microservices",
		"limit":      float64(2), // intArgOpt accepts float64 from JSON path
	})

	// The handler returns count = len(entries), so count should be ≤ limit.
	if n := contextCount(result); n > 2 {
		t.Errorf("expected at most 2 results with limit=2, got %d", n)
	}
}

func TestQueryContext_MissingProjectID(t *testing.T) {
	reg, _ := openContextToolDB(t)
	callToolExpectError(t, reg, "query_context", map[string]interface{}{
		"query": "test",
	}, "project_id")
}

func TestQueryContext_MissingQuery(t *testing.T) {
	reg, projectID := openContextToolDB(t)
	callToolExpectError(t, reg, "query_context", map[string]interface{}{
		"project_id": projectID,
	}, "query")
}

func TestSaveAndQuery_RoundTrip(t *testing.T) {
	reg, projectID := openContextToolDB(t)

	callTool(t, reg, "save_context", map[string]interface{}{
		"project_id": projectID,
		"type":       "snippet",
		"content":    "func handleWebhook(w http.ResponseWriter, r *http.Request)",
	})

	result := callTool(t, reg, "query_context", map[string]interface{}{
		"project_id": projectID,
		"query":      "handleWebhook",
	})

	if n := contextCount(result); n == 0 {
		t.Error("expected saved snippet to be retrievable by query")
	}
}

func TestQueryContext_CrossProjectIsolation(t *testing.T) {
	d, _, projectA := openPlanDB(t)

	reg := tools.NewRegistry()
	if err := tools.RegisterContextTools(reg, toolBackend(t, d)); err != nil {
		t.Fatalf("RegisterContextTools: %v", err)
	}

	callTool(t, reg, "save_context", map[string]interface{}{
		"project_id": projectA,
		"type":       "note",
		"content":    "exclusive content for project A only",
	})

	// Query using a completely different project ID — should return nothing.
	result := callTool(t, reg, "query_context", map[string]interface{}{
		"project_id": "different-project-id-999",
		"query":      "exclusive",
	})
	if n := contextCount(result); n != 0 {
		t.Errorf("context leak: expected 0 results for different project, got %d", n)
	}
}
