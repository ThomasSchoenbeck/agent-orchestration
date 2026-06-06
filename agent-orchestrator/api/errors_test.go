package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-orchestrator/api"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	api.WriteJSON(rec, http.StatusCreated, map[string]any{"hello": "world", "n": 3})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("body = %v, want hello=world", body)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	api.WriteError(rec, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var resp api.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if resp.Code != api.ErrCodeInvalidInput {
		t.Errorf("code = %q, want %q", resp.Code, api.ErrCodeInvalidInput)
	}
	if resp.Message != "name is required" {
		t.Errorf("message = %q, want %q", resp.Message, "name is required")
	}
}

func TestErrorCodesDistinct(t *testing.T) {
	codes := []string{
		api.ErrCodeNotFound, api.ErrCodeInvalidInput, api.ErrCodeConflict,
		api.ErrCodeInternal, api.ErrCodeUnavailable, api.ErrCodeUnauthorized,
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if c == "" {
			t.Error("error code constant is empty")
		}
		if seen[c] {
			t.Errorf("duplicate error code %q", c)
		}
		seen[c] = true
	}
}
