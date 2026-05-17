package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OllamaProvider implements LLMProvider for a local Ollama instance.
type OllamaProvider struct {
	name    string
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(name, baseURL, model string) *OllamaProvider {
	return &OllamaProvider{
		name:    name,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 300 * time.Second}, // local models can be slow
	}
}

// NewOllamaProviderWithClient creates a provider with a custom HTTP client.
func NewOllamaProviderWithClient(name, baseURL, model string, client *http.Client) *OllamaProvider {
	return &OllamaProvider{name: name, baseURL: baseURL, model: model, client: client}
}

func (p *OllamaProvider) Name() string { return p.name }
func (p *OllamaProvider) Close() error { return nil }

// Chat sends a chat request to the Ollama /api/chat endpoint.
func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Ollama uses OpenAI-compatible /api/chat format.
	body := map[string]interface{}{
		"model":    model,
		"messages": openaiMessages(req.Messages),
		"stream":   false,
	}
	if len(req.Tools) > 0 {
		body["tools"] = openaiTools(req.Tools)
	}

	respBytes, err := p.post(ctx, "/api/chat", body)
	if err != nil {
		return ChatResponse{}, err
	}
	return parseOllamaChatResponse(respBytes)
}

// Embed generates embeddings using the Ollama /api/embed endpoint.
func (p *OllamaProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	body := map[string]interface{}{
		"model": model,
		"input": req.Input,
	}
	respBytes, err := p.post(ctx, "/api/embed", body)
	if err != nil {
		return EmbedResponse{}, err
	}
	return parseOllamaEmbedResponse(respBytes)
}

// Rerank is not natively supported by Ollama.
func (p *OllamaProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{}, fmt.Errorf("rerank not supported by Ollama provider %q", p.name)
}

// --- HTTP helper ---

func (p *OllamaProvider) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request to ollama: %w", err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, buf.String())
	}

	return buf.Bytes(), nil
}

// --- Response parsers ---

func parseOllamaChatResponse(data []byte) (ChatResponse, error) {
	// Ollama /api/chat (non-streaming) returns OpenAI-compatible format.
	var raw struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		DoneReason      string `json:"done_reason"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
		EvalDuration    int64  `json:"eval_duration"` // nanoseconds
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return ChatResponse{}, fmt.Errorf("parse ollama response: %w", err)
	}

	resp := ChatResponse{
		Content:      raw.Message.Content,
		StopReason:   mapOllamaStopReason(raw.DoneReason),
		TokensUsed:   raw.PromptEvalCount + raw.EvalCount,
		InputTokens:  raw.PromptEvalCount,
		OutputTokens: raw.EvalCount,
		DurationMs:   int(raw.EvalDuration / 1_000_000),
	}

	for i, tc := range raw.Message.ToolCalls {
		var args map[string]interface{}
		_ = json.Unmarshal(tc.Function.Arguments, &args)
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return resp, nil
}

// ChatStream streams a chat using Ollama's NDJSON streaming format.
func (p *OllamaProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk ChunkHandler) (ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	body := map[string]interface{}{
		"model":    model,
		"messages": openaiMessages(req.Messages),
		"stream":   true,
	}
	if len(req.Tools) > 0 {
		body["tools"] = openaiTools(req.Tools)
	}

	b, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("http request to ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return ChatResponse{}, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, buf.String())
	}

	var (
		fullContent     bytes.Buffer
		promptEvalCount int
		evalCount       int
		evalDuration    int64
		doneReason      string
	)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done            bool   `json:"done"`
			DoneReason      string `json:"done_reason"`
			PromptEvalCount int    `json:"prompt_eval_count"`
			EvalCount       int    `json:"eval_count"`
			EvalDuration    int64  `json:"eval_duration"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if chunk.Message.Content != "" {
			fullContent.WriteString(chunk.Message.Content)
			onChunk(chunk.Message.Content)
		}
		if chunk.Done {
			promptEvalCount = chunk.PromptEvalCount
			evalCount = chunk.EvalCount
			evalDuration = chunk.EvalDuration
			doneReason = chunk.DoneReason
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, fmt.Errorf("stream read: %w", err)
	}

	return ChatResponse{
		Content:      fullContent.String(),
		StopReason:   mapOllamaStopReason(doneReason),
		TokensUsed:   promptEvalCount + evalCount,
		InputTokens:  promptEvalCount,
		OutputTokens: evalCount,
		DurationMs:   int(evalDuration / 1_000_000),
	}, nil
}

func parseOllamaEmbedResponse(data []byte) (EmbedResponse, error) {
	var raw struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return EmbedResponse{}, fmt.Errorf("parse ollama embed response: %w", err)
	}
	return EmbedResponse{Embeddings: raw.Embeddings}, nil
}

func mapOllamaStopReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		if reason == "" {
			return "end_turn"
		}
		return reason
	}
}
