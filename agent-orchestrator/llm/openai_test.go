package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewOpenAIProvider_NameAndClose(t *testing.T) {
	p := NewOpenAIProvider("oai", "http://x/v1", "key", "gpt-4o")
	if p.Name() != "oai" {
		t.Errorf("Name = %q, want oai", p.Name())
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMapOpenAIStopReason(t *testing.T) {
	cases := map[string]string{
		"stop":       "end_turn",
		"tool_calls": "tool_use",
		"length":     "max_tokens",
		"other":      "other",
	}
	for in, want := range cases {
		if got := mapOpenAIStopReason(in); got != want {
			t.Errorf("mapOpenAIStopReason(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOpenAIResponse_TextAndTools(t *testing.T) {
	// content + usage + a tool call whose arguments are a JSON-encoded string.
	raw := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"content": "hello",
					"tool_calls": []map[string]interface{}{
						{"id": "call_1", "function": map[string]interface{}{"name": "do_it", "arguments": `{"x":1}`}},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	}
	b, _ := json.Marshal(raw)

	resp, err := parseOpenAIResponse(b)
	if err != nil {
		t.Fatalf("parseOpenAIResponse: %v", err)
	}
	if resp.Content != "hello" || resp.StopReason != "tool_use" {
		t.Errorf("content/stop = %q/%q", resp.Content, resp.StopReason)
	}
	if resp.InputTokens != 10 || resp.OutputTokens != 5 || resp.TokensUsed != 15 {
		t.Errorf("tokens = %d/%d/%d", resp.InputTokens, resp.OutputTokens, resp.TokensUsed)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "do_it" {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	if v, _ := resp.ToolCalls[0].Arguments["x"].(float64); v != 1 {
		t.Errorf("tool arg x = %v, want 1", resp.ToolCalls[0].Arguments["x"])
	}
}

func TestParseOpenAIResponse_NoChoices(t *testing.T) {
	if _, err := parseOpenAIResponse([]byte(`{"choices":[]}`)); err == nil {
		t.Error("expected error for empty choices")
	}
}

func TestOpenAIChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProviderWithClient("oai", srv.URL, "key", "gpt-4o", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "yo"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hi" || resp.StopReason != "end_turn" || resp.TokensUsed != 3 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestOpenAIChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewOpenAIProviderWithClient("oai", srv.URL, "key", "gpt-4o", srv.Client())
	if _, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "x"}}}); err == nil {
		t.Error("expected error on HTTP 500")
	}
}

func TestOpenAIEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}],"usage":{"total_tokens":7}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProviderWithClient("oai", srv.URL, "key", "text-embedding-3-small", srv.Client())
	resp, err := p.Embed(context.Background(), EmbedRequest{Input: []string{"hello"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(resp.Embeddings) != 1 || len(resp.Embeddings[0]) != 3 || resp.TokensUsed != 7 {
		t.Errorf("embed resp = %+v", resp)
	}
}

func TestOpenAIChatStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"He\"}}]}\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"llo\"},\"finish_reason\":\"stop\"}]}\n" +
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2}}\n" +
			"data: [DONE]\n"))
	}))
	defer srv.Close()

	p := NewOpenAIProviderWithClient("oai", srv.URL, "key", "gpt-4o", srv.Client())
	var chunks []string
	resp, err := p.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(c string) {
		chunks = append(chunks, c)
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if resp.Content != "Hello" {
		t.Errorf("streamed content = %q, want Hello", resp.Content)
	}
	if strings.Join(chunks, "") != "Hello" {
		t.Errorf("chunks = %v", chunks)
	}
	if resp.StopReason != "end_turn" || resp.InputTokens != 4 || resp.OutputTokens != 2 {
		t.Errorf("stream stats = %+v", resp)
	}
}

func TestOpenAIRerankUnsupported(t *testing.T) {
	p := NewOpenAIProvider("oai", "http://x/v1", "key", "m")
	if _, err := p.Rerank(context.Background(), RerankRequest{Query: "q"}); err == nil {
		t.Error("Rerank should return an unsupported error")
	}
}
