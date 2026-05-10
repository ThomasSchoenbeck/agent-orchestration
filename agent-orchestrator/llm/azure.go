package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const azureAPIVersion = "2024-02-01"

// AzureOpenAIProvider implements LLMProvider for Azure OpenAI Service.
// Azure uses the same JSON wire format as OpenAI but with different URL structure and auth.
//
// URL pattern:
//
//	{base_url}/openai/deployments/{deployment}/chat/completions?api-version={azureAPIVersion}
//
// Auth: "api-key" header (not "Authorization: Bearer").
type AzureOpenAIProvider struct {
	name       string
	baseURL    string
	apiKey     string
	model      string   // default model for requests that don't specify one
	deployment string   // Azure deployment name (may differ from model name)
	client     *http.Client
}

// NewAzureOpenAIProvider creates a new Azure OpenAI provider.
// deployment is the Azure deployment name (e.g. "gpt-4o-deployment").
// If deployment is empty, model is used as the deployment name.
func NewAzureOpenAIProvider(name, baseURL, apiKey, model, deployment string) *AzureOpenAIProvider {
	if deployment == "" {
		deployment = model
	}
	return &AzureOpenAIProvider{
		name:       name,
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		deployment: deployment,
		client:     &http.Client{Timeout: 120 * time.Second},
	}
}

// NewAzureOpenAIProviderWithClient creates a provider with a custom HTTP client (for testing).
func NewAzureOpenAIProviderWithClient(name, baseURL, apiKey, model, deployment string, client *http.Client) *AzureOpenAIProvider {
	if deployment == "" {
		deployment = model
	}
	return &AzureOpenAIProvider{
		name: name, baseURL: baseURL, apiKey: apiKey,
		model: model, deployment: deployment, client: client,
	}
}

func (p *AzureOpenAIProvider) Name() string { return p.name }
func (p *AzureOpenAIProvider) Close() error { return nil }

// Chat sends a chat completion request to Azure OpenAI.
// The JSON payload is identical to OpenAI's format.
func (p *AzureOpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Derive the deployment name: prefer explicit req.Model if it matches a deployment,
	// otherwise use the provider's configured deployment.
	deployment := p.deployment

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

	path := p.chatPath(deployment)
	respBody, err := p.post(ctx, path, body)
	if err != nil {
		return ChatResponse{}, err
	}

	return parseOpenAIResponse(respBody) // Azure response is identical to OpenAI
}

// Embed generates embeddings via Azure OpenAI.
func (p *AzureOpenAIProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	deployment := p.deployment

	body := map[string]interface{}{
		"model": model,
		"input": req.Input,
	}
	path := p.embedPath(deployment)
	respBody, err := p.post(ctx, path, body)
	if err != nil {
		return EmbedResponse{}, err
	}
	return parseOpenAIEmbedResponse(respBody)
}

// Rerank is not natively supported by Azure OpenAI.
func (p *AzureOpenAIProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{}, fmt.Errorf("rerank not supported by Azure OpenAI provider %q", p.name)
}

// --- URL helpers ---

func (p *AzureOpenAIProvider) chatPath(deployment string) string {
	return fmt.Sprintf("/openai/deployments/%s/chat/completions?api-version=%s", deployment, azureAPIVersion)
}

func (p *AzureOpenAIProvider) embedPath(deployment string) string {
	return fmt.Sprintf("/openai/deployments/%s/embeddings?api-version=%s", deployment, azureAPIVersion)
}

// --- HTTP helpers ---

func (p *AzureOpenAIProvider) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Azure uses "api-key" header, not "Authorization: Bearer".
	if p.apiKey != "" {
		req.Header.Set("api-key", p.apiKey)
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
		// Try to extract Azure error message (same format as OpenAI).
		var apiErr struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(buf.Bytes(), &apiErr); jsonErr == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("azure openai error (%s %d): %s", apiErr.Error.Code, resp.StatusCode, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("azure openai provider %q returned status %d: %s", p.name, resp.StatusCode, buf.String())
	}

	return buf.Bytes(), nil
}
