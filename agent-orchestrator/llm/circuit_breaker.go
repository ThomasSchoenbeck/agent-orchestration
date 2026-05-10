package llm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// StateClosed is the normal operating state: requests pass through.
	StateClosed CircuitState = iota
	// StateOpen means the circuit has tripped: requests are rejected immediately.
	StateOpen
	// StateHalfOpen allows one probe request to test whether the backend has recovered.
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker wraps an LLMProvider and implements the circuit-breaker pattern.
// After `threshold` consecutive failures the circuit opens and requests are rejected
// for `resetTimeout`. After that window one probe request is allowed (half-open);
// if it succeeds the circuit closes, otherwise it reopens.
type CircuitBreaker struct {
	provider     LLMProvider
	threshold    int
	resetTimeout time.Duration

	mu           sync.Mutex
	state        CircuitState
	failures     int
	openedAt     time.Time
}

// NewCircuitBreaker wraps provider with circuit-breaker logic.
// threshold: consecutive failures before opening.
// resetTimeout: how long to stay open before trying again.
func NewCircuitBreaker(provider LLMProvider, threshold int, resetTimeout time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	return &CircuitBreaker{
		provider:     provider,
		threshold:    threshold,
		resetTimeout: resetTimeout,
		state:        StateClosed,
	}
}

func (cb *CircuitBreaker) Name() string { return cb.provider.Name() }
func (cb *CircuitBreaker) Close() error { return cb.provider.Close() }

// State returns the current circuit state (safe for concurrent use).
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentState()
}

// currentState returns the circuit state, transitioning open→half-open if the
// reset timeout has elapsed. Must be called with cb.mu held.
func (cb *CircuitBreaker) currentState() CircuitState {
	if cb.state == StateOpen && time.Since(cb.openedAt) >= cb.resetTimeout {
		cb.state = StateHalfOpen
	}
	return cb.state
}

func (cb *CircuitBreaker) allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.currentState() {
	case StateOpen:
		return fmt.Errorf("circuit breaker open for provider %q (will retry after %v)",
			cb.provider.Name(), cb.resetTimeout)
	case StateHalfOpen:
		return nil // allow the probe
	default:
		return nil
	}
}

func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = StateClosed
}

func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	if cb.state == StateHalfOpen || cb.failures >= cb.threshold {
		cb.state = StateOpen
		cb.openedAt = time.Now()
	}
}

// Chat implements LLMProvider with circuit-breaker protection.
func (cb *CircuitBreaker) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if err := cb.allow(); err != nil {
		return ChatResponse{}, err
	}
	resp, err := cb.provider.Chat(ctx, req)
	if err != nil {
		cb.onFailure()
		return resp, err
	}
	cb.onSuccess()
	return resp, nil
}

// Embed implements LLMProvider with circuit-breaker protection.
func (cb *CircuitBreaker) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	if err := cb.allow(); err != nil {
		return EmbedResponse{}, err
	}
	resp, err := cb.provider.Embed(ctx, req)
	if err != nil {
		cb.onFailure()
		return resp, err
	}
	cb.onSuccess()
	return resp, nil
}

// Rerank implements LLMProvider with circuit-breaker protection.
func (cb *CircuitBreaker) Rerank(ctx context.Context, req RerankRequest) (RerankResponse, error) {
	if err := cb.allow(); err != nil {
		return RerankResponse{}, err
	}
	resp, err := cb.provider.Rerank(ctx, req)
	if err != nil {
		cb.onFailure()
		return resp, err
	}
	cb.onSuccess()
	return resp, nil
}

// FailoverProvider wraps a primary and an optional fallback provider.
// If the primary call fails (or the circuit is open), the call is retried on the fallback.
type FailoverProvider struct {
	primary  LLMProvider
	fallback LLMProvider // may be nil
	name     string
}

// NewFailoverProvider creates a FailoverProvider.
// fallback may be nil (in which case there is no fallback and errors propagate normally).
func NewFailoverProvider(name string, primary, fallback LLMProvider) *FailoverProvider {
	return &FailoverProvider{name: name, primary: primary, fallback: fallback}
}

func (f *FailoverProvider) Name() string { return f.name }
func (f *FailoverProvider) Close() error {
	err := f.primary.Close()
	if f.fallback != nil {
		if err2 := f.fallback.Close(); err == nil {
			err = err2
		}
	}
	return err
}

func (f *FailoverProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	resp, err := f.primary.Chat(ctx, req)
	if err != nil && f.fallback != nil {
		return f.fallback.Chat(ctx, req)
	}
	return resp, err
}

func (f *FailoverProvider) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	resp, err := f.primary.Embed(ctx, req)
	if err != nil && f.fallback != nil {
		return f.fallback.Embed(ctx, req)
	}
	return resp, err
}

func (f *FailoverProvider) Rerank(ctx context.Context, req RerankRequest) (RerankResponse, error) {
	resp, err := f.primary.Rerank(ctx, req)
	if err != nil && f.fallback != nil {
		return f.fallback.Rerank(ctx, req)
	}
	return resp, err
}
