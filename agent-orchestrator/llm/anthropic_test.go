package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// anthropicResponse builds a minimal Anthropic Messages API JSON response.
func anthropicTextResponse(text string, inputTokens, outputTokens int) []byte {
	resp := map[string]interface{}{
		"id":   "msg_01test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
		"model":       "claude-3-sonnet-20240229",
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func anthropicToolUseResponse(toolID, toolName string, toolInput map[string]interface{}) []byte {
	resp := map[string]interface{}{
		"id":   "msg_02test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type":  "tool_use",
				"id":    toolID,
				"name":  toolName,
				"input": toolInput,
			},
		},
		"model":       "claude-3-sonnet-20240229",
		"stop_reason": "tool_use",
		"usage": map[string]interface{}{
			"input_tokens":  100,
			"output_tokens": 50,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// --- Provider initialisation ---

func TestNewAnthropicProvider_DefaultBaseURL(t *testing.T) {
	p := NewAnthropicProvider("claude", "", "key", "claude-3-sonnet-20240229")
	if p.baseURL != "https://api.anthropic.com" {
		t.Errorf("expected default baseURL, got %q", p.baseURL)
	}
	if p.Name() != "claude" {
		t.Errorf("expected name %q, got %q", "claude", p.Name())
	}
}

func TestAnthropicProvider_Close(t *testing.T) {
	p := NewAnthropicProvider("claude", "", "key", "model")
	if err := p.Close(); err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
}

// --- Chat ---

func TestAnthropicProvider_Chat_TextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers.
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key header, got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Errorf("expected anthropic-version header, got %q", r.Header.Get("anthropic-version"))
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicTextResponse("Hello, world!", 10, 5))
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "test-key", "claude-3-sonnet-20240229", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() unexpected error: %v", err)
	}
	if resp.Content != "Hello, world!" {
		t.Errorf("expected content %q, got %q", "Hello, world!", resp.Content)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason %q, got %q", "end_turn", resp.StopReason)
	}
	if resp.TokensUsed != 15 {
		t.Errorf("expected 15 tokens, got %d", resp.TokensUsed)
	}
}

func TestAnthropicProvider_Chat_ContextTokensIncludesCache(t *testing.T) {
	body := map[string]interface{}{
		"id": "msg", "type": "message", "role": "assistant",
		"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
		"model":       "claude-3-sonnet-20240229",
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":                10,
			"output_tokens":               5,
			"cache_read_input_tokens":     100,
			"cache_creation_input_tokens": 20,
		},
	}
	raw, _ := json.Marshal(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "key", "claude-3-sonnet-20240229", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", resp.InputTokens)
	}
	// ContextTokens = input + cache_read + cache_creation = 10 + 100 + 20.
	if resp.ContextTokens != 130 {
		t.Errorf("ContextTokens = %d, want 130", resp.ContextTokens)
	}
}

func TestAnthropicProvider_Chat_SystemMessage(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicTextResponse("ok", 5, 3))
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "key", "model", srv.Client())
	p.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hi"},
		},
	})

	if receivedBody["system"] != "You are helpful." {
		t.Errorf("expected system field, got %v", receivedBody["system"])
	}
	msgs, ok := receivedBody["messages"].([]interface{})
	if !ok || len(msgs) != 1 {
		t.Errorf("expected 1 message (system excluded), got %v", receivedBody["messages"])
	}
}

func TestAnthropicProvider_Chat_ToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicToolUseResponse("toolu_01", "read_file", map[string]interface{}{"path": "main.go"}))
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "key", "model", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Read the file"}},
		Tools: []ToolDef{
			{Name: "read_file", Description: "Read a file", InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{"path": {Type: "string", Description: "File path"}},
				Required:   []string{"path"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Chat() unexpected error: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason %q, got %q", "tool_use", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "read_file" {
		t.Errorf("expected tool name %q, got %q", "read_file", tc.Name)
	}
	if tc.Arguments["path"] != "main.go" {
		t.Errorf("expected path argument %q, got %v", "main.go", tc.Arguments["path"])
	}
}

func TestAnthropicProvider_Chat_ToolResult_WrappedAsUserMessage(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicTextResponse("done", 5, 3))
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "key", "model", srv.Client())
	p.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Call the tool"},
			{Role: "tool", Content: "file contents here", ToolCallID: "toolu_01"},
		},
	})

	msgs, _ := receivedBody["messages"].([]interface{})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	// The tool result message should be a user message with content block.
	toolMsg, _ := msgs[1].(map[string]interface{})
	if toolMsg["role"] != "user" {
		t.Errorf("tool result should be wrapped as user message, got role %q", toolMsg["role"])
	}
	contents, _ := toolMsg["content"].([]interface{})
	if len(contents) == 0 {
		t.Fatal("expected content blocks in tool result message")
	}
	block, _ := contents[0].(map[string]interface{})
	if block["type"] != "tool_result" {
		t.Errorf("expected content block type %q, got %q", "tool_result", block["type"])
	}
	if block["tool_use_id"] != "toolu_01" {
		t.Errorf("expected tool_use_id %q, got %v", "toolu_01", block["tool_use_id"])
	}
}

func TestAnthropicProvider_Chat_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "bad-key", "model", srv.Client())
	_, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestAnthropicProvider_Chat_ModelOverride(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicTextResponse("ok", 5, 3))
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "key", "default-model", srv.Client())
	p.Chat(context.Background(), ChatRequest{
		Model:    "claude-3-opus-20240229",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})

	if receivedBody["model"] != "claude-3-opus-20240229" {
		t.Errorf("expected overridden model, got %v", receivedBody["model"])
	}
}

// --- Embed / Rerank ---

func TestAnthropicProvider_Embed_Unsupported(t *testing.T) {
	p := NewAnthropicProvider("claude", "", "key", "model")
	_, err := p.Embed(context.Background(), EmbedRequest{Input: []string{"hello"}})
	if err == nil {
		t.Error("expected error for unsupported Embed()")
	}
}

func TestAnthropicProvider_Rerank_Unsupported(t *testing.T) {
	p := NewAnthropicProvider("claude", "", "key", "model")
	_, err := p.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"d"}})
	if err == nil {
		t.Error("expected error for unsupported Rerank()")
	}
}

// --- Tools format ---

func TestAnthropicProvider_Chat_ToolsFormat(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicTextResponse("ok", 5, 3))
	}))
	defer srv.Close()

	p := NewAnthropicProviderWithClient("claude", srv.URL, "key", "model", srv.Client())
	p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools: []ToolDef{
			{
				Name:        "my_tool",
				Description: "Does something",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"arg1": {Type: "string", Description: "First arg"},
					},
					Required: []string{"arg1"},
				},
			},
		},
	})

	tools, _ := receivedBody["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool, _ := tools[0].(map[string]interface{})
	if tool["name"] != "my_tool" {
		t.Errorf("expected tool name %q, got %v", "my_tool", tool["name"])
	}
	if _, ok := tool["input_schema"]; !ok {
		t.Error("expected input_schema field in tool definition")
	}
}

// --- Stop reason mapping ---

func TestMapAnthropicStopReason(t *testing.T) {
	cases := []struct{ in, want string }{
		{"end_turn", "end_turn"},
		{"tool_use", "tool_use"},
		{"max_tokens", "max_tokens"},
		{"stop_sequence", "end_turn"},
		{"", "end_turn"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		got := mapAnthropicStopReason(c.in)
		if got != c.want {
			t.Errorf("mapAnthropicStopReason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
