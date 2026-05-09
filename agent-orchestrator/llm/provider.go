package llm

import "context"

// LLMProvider is the interface all LLM backends must implement.
type LLMProvider interface {
	// Chat performs a chat/completion request.
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)

	// Embed generates vector embeddings for a list of strings.
	Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error)

	// Rerank reranks documents by relevance to a query.
	Rerank(ctx context.Context, req RerankRequest) (RerankResponse, error)

	// Name returns the unique provider identifier (e.g., "ollama", "openai").
	Name() string

	// Close releases any open resources (connections, etc.).
	Close() error
}
