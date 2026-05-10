package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Provider initialisation ---

func TestNewAzureOpenAIProvider_DeploymentFallsBackToModel(t *testing.T) {
	p := NewAzureOpenAIProvider("azure", "https://my.openai.azure.com", "key", "gpt-4o", "")
	if p.deployment != "gpt-4o" {
		t.Errorf("expected deployment to fall back to model %q, got %q", "gpt-4o", p.deployment)
	}
}

func TestNewAzureOpenAIProvider_ExplicitDeployment(t *testing.T) {
	p := NewAzureOpenAIProvider("azure", "https://my.openai.azure.com", "key", "gpt-4o", "my-deployment")
	if p.deployment != "my-deployment" {
		t.Errorf("expected explicit deployment %q, got %q", "my-deployment", p.deployment)
	}
}

func TestAzureOpenAIProvider_Name(t *testing.T) {
	p := NewAzureOpenAIProvider("azure-prod", "", "k", "m", "d")
	if p.Name() != "azure-prod" {
		t.Errorf("expected name %q, got %q", "azure-prod", p.Name())
	}
}

func TestAzureOpenAIProvider_Close(t *testing.T) {
	p := NewAzureOpenAIProvider("azure", "", "k", "m", "d")
	if err := p.Close(); err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
}

// --- URL construction ---

func TestAzureOpenAIProvider_ChatPath(t *testing.T) {
	p := NewAzureOpenAIProvider("azure", "https://base", "k", "m", "my-deployment")
	path := p.chatPath("my-deployment")
	want := "/openai/deployments/my-deployment/chat/completions?api-version=" + azureAPIVersion
	if path != want {
		t.Errorf("chatPath = %q, want %q", path, want)
	}
}

func TestAzureOpenAIProvider_EmbedPath(t *testing.T) {
	p := NewAzureOpenAIProvider("azure", "https://base", "k", "m", "my-deployment")
	path := p.embedPath("embed-deployment")
	want := "/openai/deployments/embed-deployment/embeddings?api-version=" + azureAPIVersion
	if path != want {
		t.Errorf("embedPath = %q, want %q", path, want)
	}
}

// --- Auth header ---

func TestAzureOpenAIProvider_Chat_UsesApiKeyHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("api-key")
		// Verify it's NOT using Authorization: Bearer.
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Azure provider should not set Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(openaiChatResponse("hello from azure", 10))
	}))
	defer srv.Close()

	p := NewAzureOpenAIProviderWithClient("azure", srv.URL, "my-azure-key", "gpt-4o", "gpt-4o-dep", srv.Client())
	_, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() unexpected error: %v", err)
	}
	if gotKey != "my-azure-key" {
		t.Errorf("expected api-key header %q, got %q", "my-azure-key", gotKey)
	}
}

// --- URL routing in request ---

func TestAzureOpenAIProvider_Chat_CorrectURLPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.Write(openaiChatResponse("ok", 5))
	}))
	defer srv.Close()

	p := NewAzureOpenAIProviderWithClient("azure", srv.URL, "key", "gpt-4o", "my-deployment", srv.Client())
	p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})

	wantPrefix := "/openai/deployments/my-deployment/chat/completions"
	if !strings.HasPrefix(gotPath, wantPrefix) {
		t.Errorf("expected URL path prefix %q, got %q", wantPrefix, gotPath)
	}
	if !strings.Contains(gotPath, "api-version="+azureAPIVersion) {
		t.Errorf("expected api-version query param in URL, got %q", gotPath)
	}
}

// --- Chat response ---

func TestAzureOpenAIProvider_Chat_ParsesResponseCorrectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(openaiChatResponse("Azure response text", 42))
	}))
	defer srv.Close()

	p := NewAzureOpenAIProviderWithClient("azure", srv.URL, "key", "gpt-4o", "dep", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "Azure response text" {
		t.Errorf("expected content %q, got %q", "Azure response text", resp.Content)
	}
	if resp.TokensUsed != 42 {
		t.Errorf("expected 42 tokens, got %d", resp.TokensUsed)
	}
}

// --- Tool calls ---

func TestAzureOpenAIProvider_Chat_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(openaiToolCallResponse("call_abc", "my_tool", `{"key":"value"}`))
	}))
	defer srv.Close()

	p := NewAzureOpenAIProviderWithClient("azure", srv.URL, "key", "gpt-4o", "dep", srv.Client())
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "call tool"}},
		Tools: []ToolDef{
			{Name: "my_tool", Description: "A tool", InputSchema: InputSchema{Type: "object"}},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "my_tool" {
		t.Errorf("expected tool name %q, got %q", "my_tool", resp.ToolCalls[0].Name)
	}
}

// --- Embed ---

func TestAzureOpenAIProvider_Embed(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float32{0.1, 0.2, 0.3}},
			},
			"usage": map[string]interface{}{"total_tokens": 5},
		}
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	p := NewAzureOpenAIProviderWithClient("azure", srv.URL, "key", "text-embedding-ada-002", "embed-dep", srv.Client())
	resp, err := p.Embed(context.Background(), EmbedRequest{Input: []string{"hello"}})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(resp.Embeddings) != 1 {
		t.Errorf("expected 1 embedding, got %d", len(resp.Embeddings))
	}
	if !strings.Contains(gotPath, "/embeddings") {
		t.Errorf("expected embeddings path, got %q", gotPath)
	}
}

// --- Error handling ---

func TestAzureOpenAIProvider_Chat_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"DeploymentNotFound","message":"The deployment 'bad-dep' does not exist."}}`))
	}))
	defer srv.Close()

	p := NewAzureOpenAIProviderWithClient("azure", srv.URL, "key", "gpt-4o", "bad-dep", srv.Client())
	_, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "DeploymentNotFound") {
		t.Errorf("expected error message to contain 'DeploymentNotFound', got: %v", err)
	}
}

func TestAzureOpenAIProvider_Rerank_Unsupported(t *testing.T) {
	p := NewAzureOpenAIProvider("azure", "", "k", "m", "d")
	_, err := p.Rerank(context.Background(), RerankRequest{Query: "q", Docs: []string{"d"}})
	if err == nil {
		t.Error("expected error for unsupported Rerank()")
	}
}

// --- helpers shared with openai tests ---

// openaiChatResponse builds a minimal OpenAI-format JSON chat response.
func openaiChatResponse(content string, tokens int) []byte {
	resp := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message":       map[string]interface{}{"role": "assistant", "content": content},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{"total_tokens": tokens},
	}
	b, _ := json.Marshal(resp)
	return b
}

// openaiToolCallResponse builds an OpenAI-format response with a tool call.
func openaiToolCallResponse(callID, toolName, argsJSON string) []byte {
	resp := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]interface{}{
						{
							"id":   callID,
							"type": "function",
							"function": map[string]interface{}{
								"name":      toolName,
								"arguments": argsJSON,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{"total_tokens": 20},
	}
	b, _ := json.Marshal(resp)
	return b
}
