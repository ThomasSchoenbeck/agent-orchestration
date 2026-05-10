package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const anthropicAPIVersion = "2023-06-01"

// AnthropicProvider implements LLMProvider for the Anthropic Claude API.
type AnthropicProvider struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(name, baseURL, apiKey, model string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		name:    name,
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// NewAnthropicProviderWithClient creates a provider with a custom HTTP client (useful for testing).
func NewAnthropicProviderWithClient(name, baseURL, apiKey, model string, client *http.Client) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{name: name, baseURL: baseURL, apiKey: apiKey, model: model, client: client}
}

func (p *AnthropicProvider) Name() string { return p.name }
func (p *AnthropicProvider) Close() error { return nil }

// Chat sends a chat/completion request to the Anthropic Messages API.
// Anthropic's format differs from OpenAI in several ways:
//   - system message is a top-level field, not part of messages array
//   - tool calls use content blocks with type "tool_use"
//   - tool results use type "tool_result" inside a user message
func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Separate the system message from the rest.
	systemPrompt, messages := anthropicMessages(req.Messages)

	body := map[string]interface{}{
		"model":      model,
		"messages":   messages,
		"max_tokens": 4096,
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		body["tools"] = anthropicTools(req.Tools)
	}

	respBody, err := p.post(ctx, "/v1/messages", body)
	if err != nil {
		return ChatResponse{}, err
	}

	return parseAnthropicResponse(respBody)
}

// Embed is not natively supported by Anthropic; returns an error.
func (p *AnthropicProvider) Embed(_ context.Context, _ EmbedRequest) (EmbedResponse, error) {
	return EmbedResponse{}, fmt.Errorf("embed not supported by Anthropic provider %q", p.name)
}

// Rerank is not supported by Anthropic; returns an error.
func (p *AnthropicProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{}, fmt.Errorf("rerank not supported by Anthropic provider %q", p.name)
}

// --- HTTP helpers ---

func (p *AnthropicProvider) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

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
		// Try to extract Anthropic error message.
		var apiErr struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(buf.Bytes(), &apiErr); jsonErr == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("anthropic API error (%s %d): %s", apiErr.Error.Type, resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("anthropic provider %q returned status %d: %s", p.name, resp.StatusCode, buf.String())
	}

	return buf.Bytes(), nil
}

// --- Anthropic wire-format helpers ---

// anthropicMessages converts our internal Message slice into Anthropic's format.
// Returns (systemPrompt, messagesArray).
// Anthropic requires:
//   - system messages as a separate top-level field
//   - tool results wrapped as user messages with content blocks
func anthropicMessages(msgs []Message) (string, []map[string]interface{}) {
	var system string
	var out []map[string]interface{}

	for _, m := range msgs {
		switch m.Role {
		case "system":
			system = m.Content

		case "tool":
			// Tool results are user messages with a tool_result content block.
			out = append(out, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     m.Content,
					},
				},
			})

		default:
			out = append(out, map[string]interface{}{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}

	return system, out
}

// anthropicTools converts our ToolDef slice to Anthropic's tool schema format.
func anthropicTools(tools []ToolDef) []map[string]interface{} {
	out := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		props := make(map[string]interface{})
		for name, prop := range t.InputSchema.Properties {
			props[name] = map[string]interface{}{
				"type":        prop.Type,
				"description": prop.Description,
			}
		}
		schema := map[string]interface{}{
			"type":       "object",
			"properties": props,
		}
		if len(t.InputSchema.Required) > 0 {
			schema["required"] = t.InputSchema.Required
		}
		out[i] = map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"input_schema": schema,
		}
	}
	return out
}

// parseAnthropicResponse converts an Anthropic Messages API response to ChatResponse.
// Anthropic content blocks can be "text" or "tool_use".
func parseAnthropicResponse(data []byte) (ChatResponse, error) {
	var raw struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return ChatResponse{}, fmt.Errorf("parse anthropic response: %w", err)
	}

	resp := ChatResponse{
		StopReason: mapAnthropicStopReason(raw.StopReason),
		TokensUsed: raw.Usage.InputTokens + raw.Usage.OutputTokens,
	}

	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			var args map[string]interface{}
			_ = json.Unmarshal(block.Input, &args)
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	return resp, nil
}

func mapAnthropicStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "end_turn"
	case "tool_use":
		return "tool_use"
	case "max_tokens":
		return "max_tokens"
	case "stop_sequence":
		return "end_turn"
	default:
		if reason == "" {
			return "end_turn"
		}
		return reason
	}
}
