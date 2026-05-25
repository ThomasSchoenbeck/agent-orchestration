package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-orchestrator/llm"
)

// --- Registry tests ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := llm.NewRegistry()

	p := llm.NewOpenAIProvider("test", "http://localhost", "key", "gpt-4o-mini")
	if err := reg.Register("test", p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := reg.Get("test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "test" {
		t.Errorf("expected name test, got %s", got.Name())
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	reg := llm.NewRegistry()
	p := llm.NewOpenAIProvider("a", "", "", "")
	_ = reg.Register("a", p)
	if err := reg.Register("a", p); err == nil {
		t.Fatal("expected error on duplicate register, got nil")
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	reg := llm.NewRegistry()
	if _, err := reg.Get("nonexistent"); err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}
}

func TestRegistry_SetRoles_GetForRole(t *testing.T) {
	reg := llm.NewRegistry()
	p := llm.NewOpenAIProvider("local", "http://localhost", "key", "llama3")
	reg.Set("local", p)
	reg.SetRoles("local", "llama3", []string{"reviewer", "worker"})

	got, model, err := reg.GetForRole("reviewer")
	if err != nil {
		t.Fatalf("GetForRole reviewer: %v", err)
	}
	if got.Name() != "local" {
		t.Errorf("expected provider local, got %s", got.Name())
	}
	if model != "llama3" {
		t.Errorf("expected model llama3, got %s", model)
	}

	_, _, err = reg.GetForRole("orchestrator")
	if err == nil {
		t.Fatal("expected error for unmapped role, got nil")
	}
}

func TestRegistry_SetRoles_ReplacesPreviousMappings(t *testing.T) {
	reg := llm.NewRegistry()
	p := llm.NewOpenAIProvider("prov", "http://localhost", "", "m")
	reg.Set("prov", p)
	reg.SetRoles("prov", "m", []string{"worker", "reviewer"})

	// Reassign with only "worker".
	reg.SetRoles("prov", "m", []string{"worker"})

	if _, _, err := reg.GetForRole("worker"); err != nil {
		t.Errorf("worker should still be mapped: %v", err)
	}
	if _, _, err := reg.GetForRole("reviewer"); err == nil {
		t.Error("reviewer should no longer be mapped after SetRoles update")
	}
}

func TestRegistry_Remove_ClearsRoleIndex(t *testing.T) {
	reg := llm.NewRegistry()
	p := llm.NewOpenAIProvider("prov", "http://localhost", "", "m")
	reg.Set("prov", p)
	reg.SetRoles("prov", "m", []string{"worker"})

	reg.Remove("prov")

	if _, _, err := reg.GetForRole("worker"); err == nil {
		t.Error("role index should be cleared after Remove")
	}
}

// --- OpenAI provider tests ---

func mockOpenAIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenAIProvider_Chat_Success(t *testing.T) {
	srv := mockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message":       map[string]interface{}{"content": "Hello!", "tool_calls": nil},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{"total_tokens": 42},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	p := llm.NewOpenAIProviderWithClient("test", srv.URL, "test-key", "gpt-4o-mini", srv.Client())
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("expected content Hello!, got %q", resp.Content)
	}
	if resp.TokensUsed != 42 {
		t.Errorf("expected 42 tokens, got %d", resp.TokensUsed)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop reason end_turn, got %s", resp.StopReason)
	}
}

func TestOpenAIProvider_Chat_ToolCall(t *testing.T) {
	srv := mockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id": "call_123",
								"function": map[string]interface{}{
									"name":      "read_file",
									"arguments": `{"file_path": "main.go"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]int{"total_tokens": 20},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	p := llm.NewOpenAIProviderWithClient("test", srv.URL, "", "gpt-4", srv.Client())
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Read main.go"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("expected tool_use, got %s", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "read_file" {
		t.Errorf("expected tool read_file, got %s", resp.ToolCalls[0].Name)
	}
}

func TestOpenAIProvider_Chat_Error(t *testing.T) {
	srv := mockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid api key"}`))
	})

	p := llm.NewOpenAIProviderWithClient("test", srv.URL, "bad-key", "gpt-4", srv.Client())
	_, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

// --- Ollama provider tests ---

func TestOllamaProvider_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"message":          map[string]interface{}{"content": "Ollama response", "role": "assistant"},
			"done_reason":      "stop",
			"prompt_eval_count": 10,
			"eval_count":       5,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := llm.NewOllamaProviderWithClient("ollama-test", srv.URL, "qwen2.5-coder", srv.Client())
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Ollama response" {
		t.Errorf("expected 'Ollama response', got %q", resp.Content)
	}
	if resp.TokensUsed != 15 {
		t.Errorf("expected 15 tokens, got %d", resp.TokensUsed)
	}
}

func TestOllamaProvider_Rerank_Unsupported(t *testing.T) {
	p := llm.NewOllamaProvider("test", "http://localhost", "")
	_, err := p.Rerank(context.Background(), llm.RerankRequest{})
	if err == nil {
		t.Fatal("expected error for unsupported rerank, got nil")
	}
}
