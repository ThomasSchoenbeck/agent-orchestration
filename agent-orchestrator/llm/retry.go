package llm

import (
	"context"
	"time"
)

// RetryProvider wraps an LLMProvider and retries a failed call up to maxRetries
// additional times, sleeping backoff*attempt between attempts (linear backoff).
// A retry is abandoned early if the context is cancelled. maxRetries <= 0 means
// no retries (single attempt).
type RetryProvider struct {
	provider   LLMProvider
	maxRetries int
	backoff    time.Duration
}

// NewRetryProvider wraps provider with retry logic.
func NewRetryProvider(provider LLMProvider, maxRetries int, backoff time.Duration) *RetryProvider {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if backoff < 0 {
		backoff = 0
	}
	return &RetryProvider{provider: provider, maxRetries: maxRetries, backoff: backoff}
}

func (r *RetryProvider) Name() string  { return r.provider.Name() }
func (r *RetryProvider) Close() error  { return r.provider.Close() }

// sleepBackoff waits backoff*attempt, returning false if the context is cancelled.
func (r *RetryProvider) sleepBackoff(ctx context.Context, attempt int) bool {
	if r.backoff <= 0 {
		return true
	}
	select {
	case <-time.After(r.backoff * time.Duration(attempt)):
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *RetryProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 && !r.sleepBackoff(ctx, attempt) {
			return ChatResponse{}, ctx.Err()
		}
		resp, err := r.provider.Chat(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return ChatResponse{}, lastErr
}

func (r *RetryProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 && !r.sleepBackoff(ctx, attempt) {
			return EmbedResponse{}, ctx.Err()
		}
		resp, err := r.provider.Embed(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return EmbedResponse{}, lastErr
}

func (r *RetryProvider) Rerank(ctx context.Context, req RerankRequest) (RerankResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 && !r.sleepBackoff(ctx, attempt) {
			return RerankResponse{}, ctx.Err()
		}
		resp, err := r.provider.Rerank(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return RerankResponse{}, lastErr
}
