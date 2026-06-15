package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaNameClose(t *testing.T) {
	p := NewOllamaProvider("o", "http://x", "gemma")
	if p.Name() != "o" {
		t.Errorf("Name = %q, want o", p.Name())
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMapOllamaStopReason(t *testing.T) {
	cases := map[string]string{
		"stop":       "end_turn",
		"tool_calls": "tool_use",
		"length":     "max_tokens",
		"":           "end_turn",
		"weird":      "weird",
	}
	for in, want := range cases {
		if got := mapOllamaStopReason(in); got != want {
			t.Errorf("mapOllamaStopReason(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOllamaEmbedResponse(t *testing.T) {
	resp, err := parseOllamaEmbedResponse([]byte(`{"embeddings":[[0.1,0.2]]}`))
	if err != nil {
		t.Fatalf("parseOllamaEmbedResponse: %v", err)
	}
	if len(resp.Embeddings) != 1 || len(resp.Embeddings[0]) != 2 {
		t.Errorf("embeddings = %+v", resp.Embeddings)
	}
}

func TestOllamaEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %q, want /api/embed", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3]]}`))
	}))
	defer srv.Close()

	p := NewOllamaProviderWithClient("o", srv.URL, "nomic", srv.Client())
	resp, err := p.Embed(context.Background(), EmbedRequest{Input: []string{"hi"}})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(resp.Embeddings) != 1 || len(resp.Embeddings[0]) != 3 {
		t.Errorf("embed resp = %+v", resp)
	}
}

func TestOllamaChatStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %q, want /api/chat", r.URL.Path)
		}
		// Newline-delimited JSON chunks; the final one carries done + counts.
		_, _ = w.Write([]byte(
			"{\"message\":{\"content\":\"He\"}}\n" +
				"{\"message\":{\"content\":\"llo\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":4,\"eval_count\":2,\"eval_duration\":3000000}\n"))
	}))
	defer srv.Close()

	p := NewOllamaProviderWithClient("o", srv.URL, "gemma", srv.Client())
	var chunks []string
	resp, err := p.ChatStream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(c string) {
		chunks = append(chunks, c)
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if resp.Content != "Hello" || strings.Join(chunks, "") != "Hello" {
		t.Errorf("content = %q, chunks = %v", resp.Content, chunks)
	}
	if resp.StopReason != "end_turn" || resp.InputTokens != 4 || resp.OutputTokens != 2 || resp.DurationMs != 3 {
		t.Errorf("stream stats = %+v", resp)
	}
}
