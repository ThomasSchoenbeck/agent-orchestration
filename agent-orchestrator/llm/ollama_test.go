package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaChat_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s, want /api/chat", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message":           map[string]interface{}{"content": "hi there"},
			"done_reason":       "stop",
			"prompt_eval_count": 10,
			"eval_count":        5,
			"eval_duration":     2_000_000, // 2ms in ns
		})
	}))
	defer srv.Close()

	p := NewOllamaProviderWithClient("ollama", srv.URL, "gemma", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hi there" {
		t.Errorf("content = %q, want 'hi there'", resp.Content)
	}
	if resp.TokensUsed != 15 || resp.InputTokens != 10 || resp.OutputTokens != 5 {
		t.Errorf("tokens = %+v, want total=15 in=10 out=5", resp)
	}
	if resp.DurationMs != 2 {
		t.Errorf("duration = %d, want 2ms", resp.DurationMs)
	}
}

func TestOllamaChat_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]interface{}{
				"content": "",
				"tool_calls": []map[string]interface{}{
					{"function": map[string]interface{}{
						"name":      "list_tasks",
						"arguments": map[string]interface{}{"project_id": "p1"},
					}},
				},
			},
			"done_reason": "stop",
		})
	}))
	defer srv.Close()

	p := NewOllamaProviderWithClient("ollama", srv.URL, "gemma", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{
		Tools:    []ToolDef{{Name: "list_tasks"}},
		Messages: []Message{{Role: "user", Content: "go"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "list_tasks" {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	if resp.ToolCalls[0].Arguments["project_id"] != "p1" {
		t.Errorf("tool args = %+v", resp.ToolCalls[0].Arguments)
	}
}

func TestOllamaChat_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	p := NewOllamaProviderWithClient("ollama", srv.URL, "gemma", srv.Client())
	if _, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "x"}}}); err == nil {
		t.Error("expected error on HTTP 500")
	}
}
