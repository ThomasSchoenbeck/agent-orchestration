// Package llm defines the LLM provider abstraction and common types.
package llm

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"` // system | user | assistant | tool
	Content string `json:"content"`
	// ToolCallID is set when Role == "tool" (the result of a tool call).
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ChatRequest represents a completion/chat request.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Tools       []ToolDef `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"` // auto | required | none
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse represents a completion response.
type ChatResponse struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"` // end_turn | tool_use | max_tokens | error
	TokensUsed int        `json:"tokens_used"`
}

// EmbedRequest represents an embedding request.
type EmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// EmbedResponse represents an embedding response.
type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	TokensUsed int         `json:"tokens_used"`
}

// RerankRequest represents a reranking request.
type RerankRequest struct {
	Model string   `json:"model"`
	Query string   `json:"query"`
	Docs  []string `json:"documents"`
	TopK  int      `json:"top_k"`
}

// RerankResponse represents a reranking response.
type RerankResponse struct {
	Results []RerankResult `json:"results"`
}

// RerankResult is a single reranked document.
type RerankResult struct {
	Index int     `json:"index"`
	Score float32 `json:"score"`
}

// ToolDef defines an available tool that the LLM can call.
type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"input_schema"`
}

// InputSchema is the JSON-Schema–like description of a tool's arguments.
type InputSchema struct {
	Type       string              `json:"type"` // "object"
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes one argument of a tool.
type Property struct {
	Type        string `json:"type"` // string | number | boolean | array | object
	Description string `json:"description"`
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}
