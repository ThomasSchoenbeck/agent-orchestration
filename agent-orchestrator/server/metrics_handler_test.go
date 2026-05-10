package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// --- GET /api/metrics ---

func TestMetrics_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/metrics", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Required top-level fields.
	for _, field := range []string{"total_tokens", "total_tasks", "success_rate", "avg_duration_ms"} {
		if _, ok := resp[field]; !ok {
			t.Errorf("missing field %q in /api/metrics response", field)
		}
	}
	// Empty DB — tasks and tokens should be zero.
	if tasks, _ := resp["total_tasks"].(float64); tasks != 0 {
		t.Errorf("expected total_tasks=0, got %v", tasks)
	}
}

func TestMetrics_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/metrics", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- GET /api/metrics/tokens ---

func TestMetricsTokens_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/metrics/tokens", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"total_input_tokens", "total_output_tokens", "total_tokens"} {
		if _, ok := resp[field]; !ok {
			t.Errorf("missing field %q in /api/metrics/tokens response", field)
		}
	}
	if v, _ := resp["total_tokens"].(float64); v != 0 {
		t.Errorf("expected total_tokens=0, got %v", v)
	}
	if v, _ := resp["total_input_tokens"].(float64); v != 0 {
		t.Errorf("expected total_input_tokens=0, got %v", v)
	}
}

func TestMetricsTokens_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/metrics/tokens", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestMetricsTokens_Delete_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodDelete, "/api/metrics/tokens", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- GET /api/metrics/costs ---

func TestMetricsCosts_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/metrics/costs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["total_cost"]; !ok {
		t.Error("missing 'total_cost' field in /api/metrics/costs response")
	}
	if v, _ := resp["total_cost"].(float64); v != 0 {
		t.Errorf("expected total_cost=0, got %v", v)
	}
}

func TestMetricsCosts_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodPost, "/api/metrics/costs", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestMetricsCosts_ResponseShape(t *testing.T) {
	srv, _ := newTestServer(t)
	w := do(t, srv, http.MethodGet, "/api/metrics/costs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Verify valid JSON and required shape.
	var resp struct {
		TotalCost float64     `json:"total_cost"`
		ByProject interface{} `json:"by_project"`
		ByAgent   interface{} `json:"by_agent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
}

// TestMetricsCosts_AfterProjectCreation verifies the endpoint stays responsive
// when the database has project data but no metrics.
func TestMetricsCosts_AfterProjectCreation(t *testing.T) {
	srv, _ := newTestServer(t)
	// Create a project to ensure DB is non-trivially populated.
	createTestProject(t, srv)

	w := do(t, srv, http.MethodGet, "/api/metrics/costs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if v, _ := resp["total_cost"].(float64); v != 0 {
		t.Errorf("expected 0 cost with no metrics, got %v", v)
	}
}
