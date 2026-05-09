package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenAIProvider implements LLMProvider for OpenAI-compatible APIs
// (OpenAI, LM Studio, company APIs, etc.).
type OpenAIProvider struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewOpenAIProvider creates a new OpenAI-compatible provider.
func NewOpenAIProvider(name, baseURL, apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		name:    name,
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// NewOpenAIProviderWithClient creates a provider with a custom HTTP client (useful for testing).
func NewOpenAIProviderWithClient(name, baseURL, apiKey, model string, client *http.Client) *OpenAIProvider {
	return &OpenAIProvider{name: name, baseURL: baseURL, apiKey: apiKey, model: model, client: client}
}

func (p *OpenAIProvider) Name() string  { return p.name }
func (p *OpenAIProvider) Close() error  { return nil }

// Chat sends a chat completion request.
func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Build OpenAI-format request body.
	body := map[string]interface{}{
		"model":    model,
		"messages": openaiMessages(req.Messages),
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		body["tools"] = openaiTools(req.Tools)
		if req.ToolChoice != "" {
			body["tool_choice"] = req.ToolChoice
		}
	}

	respBody, err := p.post(ctx, "/chat/completions", body)
	if err != nil {
		return ChatResponse{}, err
	}

	return parseOpenAIResponse(respBody)
}

// Embed generates embeddings using the OpenAI embeddings endpoint.
func (p *OpenAIProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	body := map[string]interface{}{
		"model": model,
		"input": req.Input,
	}
	respBody, err := p.post(ctx, "/embeddings", body)
	if err != nil {
		return EmbedResponse{}, err
	}
	return parseOpenAIEmbedResponse(respBody)
}

// Rerank is not natively supported by OpenAI; returns an error.
func (p *OpenAIProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{}, fmt.Errorf("rerank not supported by OpenAI-compatible provider %q", p.name)
}

// --- HTTP helpers ---

func (p *OpenAIProvider) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("provider %q returned status %d: %s", p.name, resp.StatusCode, buf.String())
	}

	return buf.Bytes(), nil
}

// --- OpenAI wire-format helpers ---

func openaiMessages(msgs []Message) []map[string]interface{} {
	out := make([]map[string]interface{}, len(msgs))
	for i, m := range msgs {
		entry := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.ToolCallID != "" {
			entry["tool_call_id"] = m.ToolCallID
		}
		out[i] = entry
	}
	return out
}

func openaiTools(tools []ToolDef) []map[string]interface{} {
	out := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		out[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters": map[string]interface{}{
					"type":       t.InputSchema.Type,
					"properties": t.InputSchema.Properties,
					"required":   t.InputSchema.Required,
				},
			},
		}
	}
	return out
}

func parseOpenAIResponse(data []byte) (ChatResponse, error) {
	var raw struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return ChatResponse{}, fmt.Errorf("parse response: %w", err)
	}
	if len(raw.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("no choices in response")
	}

	choice := raw.Choices[0]
	resp := ChatResponse{
		Content:    choice.Message.Content,
		StopReason: mapOpenAIStopReason(choice.FinishReason),
		TokensUsed: raw.Usage.TotalTokens,
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]interface{}
		_ = json.Unmarshal(tc.Function.Arguments, &args)
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return resp, nil
}

func parseOpenAIEmbedResponse(data []byte) (EmbedResponse, error) {
	var raw struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return EmbedResponse{}, fmt.Errorf("parse embed response: %w", err)
	}
	resp := EmbedResponse{TokensUsed: raw.Usage.TotalTokens}
	for _, d := range raw.Data {
		resp.Embeddings = append(resp.Embeddings, d.Embedding)
	}
	return resp, nil
}

func mapOpenAIStopReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}
