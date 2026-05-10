package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// errProvider always returns an error from Chat.
type errProvider struct {
	name string
	err  error
}

func (p *errProvider) Name() string  { return p.name }
func (p *errProvider) Close() error  { return nil }
func (p *errProvider) Chat(_ context.Context, _ ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, p.err
}
func (p *errProvider) Embed(_ context.Context, _ EmbedRequest) (EmbedResponse, error) {
	return EmbedResponse{}, p.err
}
func (p *errProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{}, p.err
}

// okProvider always returns a successful response.
type okProvider struct{ name string }

func (p *okProvider) Name() string  { return p.name }
func (p *okProvider) Close() error  { return nil }
func (p *okProvider) Chat(_ context.Context, _ ChatRequest) (ChatResponse, error) {
	return ChatResponse{Content: "ok", StopReason: "end_turn"}, nil
}
func (p *okProvider) Embed(_ context.Context, _ EmbedRequest) (EmbedResponse, error) {
	return EmbedResponse{Embeddings: [][]float32{{0.1}}}, nil
}
func (p *okProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{Results: []RerankResult{{Index: 0, Score: 1.0}}}, nil
}

// --- CircuitBreaker ---

func TestCircuitBreaker_ClosedInitially(t *testing.T) {
	cb := NewCircuitBreaker(&okProvider{"ok"}, 3, time.Minute)
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed initially, got %s", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	sentinel := errors.New("backend error")
	cb := NewCircuitBreaker(&errProvider{"fail", sentinel}, 3, time.Minute)

	for i := 0; i < 3; i++ {
		_, err := cb.Chat(context.Background(), ChatRequest{})
		if err == nil {
			t.Fatal("expected error from failing provider")
		}
	}

	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen after threshold failures, got %s", cb.State())
	}
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	sentinel := errors.New("backend error")
	cb := NewCircuitBreaker(&errProvider{"fail", sentinel}, 1, time.Minute)

	// Trip the circuit.
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck

	// Next call should be rejected by the circuit breaker itself.
	_, err := cb.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected rejection when circuit is open")
	}
	if !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("expected 'circuit breaker open' in error, got: %v", err)
	}
}

func TestCircuitBreaker_TransitionsHalfOpenAfterTimeout(t *testing.T) {
	sentinel := errors.New("backend error")
	cb := NewCircuitBreaker(&errProvider{"fail", sentinel}, 1, 10*time.Millisecond)

	// Trip the circuit.
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck

	// Wait for reset timeout.
	time.Sleep(20 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected StateHalfOpen after timeout, got %s", cb.State())
	}
}

func TestCircuitBreaker_ClosesOnSuccessFromHalfOpen(t *testing.T) {
	// Start in open state, wait for half-open, then succeed.
	sentinel := errors.New("backend error")
	cb := NewCircuitBreaker(&errProvider{"fail", sentinel}, 1, 10*time.Millisecond)

	// Trip.
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck
	time.Sleep(20 * time.Millisecond) // → half-open

	// Replace the failing provider with a succeeding one.
	cb.provider = &okProvider{"ok"}

	_, err := cb.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected success in half-open, got: %v", err)
	}
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after successful probe, got %s", cb.State())
	}
}

func TestCircuitBreaker_ReopensOnFailureFromHalfOpen(t *testing.T) {
	sentinel := errors.New("backend error")
	cb := NewCircuitBreaker(&errProvider{"fail", sentinel}, 1, 10*time.Millisecond)

	// Trip.
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck
	time.Sleep(20 * time.Millisecond) // → half-open

	// Probe fails → should reopen.
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck

	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen after failed probe, got %s", cb.State())
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	sentinel := errors.New("backend error")
	// Alternate provider: fail twice, then succeed.
	var calls int
	alternate := &toggleProvider{
		failUntil: 2,
		err:       sentinel,
		calls:     &calls,
	}
	cb := NewCircuitBreaker(alternate, 5, time.Minute)

	// Two failures.
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck

	// Success resets failure count.
	cb.Chat(context.Background(), ChatRequest{}) //nolint:errcheck

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after success, got %s", cb.State())
	}
	if cb.failures != 0 {
		t.Errorf("expected 0 failures after reset, got %d", cb.failures)
	}
}

func TestCircuitBreaker_Embed(t *testing.T) {
	sentinel := errors.New("embed error")
	cb := NewCircuitBreaker(&errProvider{"fail", sentinel}, 1, time.Minute)
	cb.Embed(context.Background(), EmbedRequest{}) //nolint:errcheck // trip
	_, err := cb.Embed(context.Background(), EmbedRequest{})
	if err == nil || !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("expected circuit breaker rejection for Embed, got: %v", err)
	}
}

func TestCircuitBreaker_Rerank(t *testing.T) {
	sentinel := errors.New("rerank error")
	cb := NewCircuitBreaker(&errProvider{"fail", sentinel}, 1, time.Minute)
	cb.Rerank(context.Background(), RerankRequest{}) //nolint:errcheck // trip
	_, err := cb.Rerank(context.Background(), RerankRequest{})
	if err == nil || !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("expected circuit breaker rejection for Rerank, got: %v", err)
	}
}

func TestCircuitBreaker_StateString(t *testing.T) {
	if StateClosed.String() != "closed" {
		t.Errorf("unexpected string for StateClosed: %q", StateClosed.String())
	}
	if StateOpen.String() != "open" {
		t.Errorf("unexpected string for StateOpen: %q", StateOpen.String())
	}
	if StateHalfOpen.String() != "half-open" {
		t.Errorf("unexpected string for StateHalfOpen: %q", StateHalfOpen.String())
	}
}

// --- FailoverProvider ---

func TestFailoverProvider_PrimarySucceeds(t *testing.T) {
	primary := &okProvider{"primary"}
	fallback := &errProvider{"fallback", errors.New("fallback error")}
	fp := NewFailoverProvider("failover", primary, fallback)

	resp, err := fp.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected success from primary, got: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content %q, got %q", "ok", resp.Content)
	}
}

func TestFailoverProvider_FallsBackOnPrimaryError(t *testing.T) {
	primary := &errProvider{"primary", errors.New("primary down")}
	fallback := &okProvider{"fallback"}
	fp := NewFailoverProvider("failover", primary, fallback)

	resp, err := fp.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected success from fallback, got: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content %q from fallback, got %q", "ok", resp.Content)
	}
}

func TestFailoverProvider_ErrorWhenNoFallback(t *testing.T) {
	primary := &errProvider{"primary", errors.New("primary down")}
	fp := NewFailoverProvider("failover", primary, nil)

	_, err := fp.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Error("expected error when primary fails and no fallback")
	}
}

func TestFailoverProvider_Name(t *testing.T) {
	fp := NewFailoverProvider("my-failover", &okProvider{"p"}, nil)
	if fp.Name() != "my-failover" {
		t.Errorf("expected name %q, got %q", "my-failover", fp.Name())
	}
}

func TestFailoverProvider_Embed_Fallback(t *testing.T) {
	primary := &errProvider{"primary", errors.New("embed error")}
	fallback := &okProvider{"fallback"}
	fp := NewFailoverProvider("failover", primary, fallback)

	resp, err := fp.Embed(context.Background(), EmbedRequest{Input: []string{"hi"}})
	if err != nil {
		t.Fatalf("expected fallback to succeed for Embed: %v", err)
	}
	if len(resp.Embeddings) == 0 {
		t.Error("expected non-empty embeddings from fallback")
	}
}

// --- helpers ---

type toggleProvider struct {
	failUntil int
	err       error
	calls     *int
}

func (p *toggleProvider) Name() string { return "toggle" }
func (p *toggleProvider) Close() error { return nil }
func (p *toggleProvider) Chat(_ context.Context, _ ChatRequest) (ChatResponse, error) {
	*p.calls++
	if *p.calls <= p.failUntil {
		return ChatResponse{}, p.err
	}
	return ChatResponse{Content: "ok"}, nil
}
func (p *toggleProvider) Embed(_ context.Context, _ EmbedRequest) (EmbedResponse, error) {
	return EmbedResponse{}, nil
}
func (p *toggleProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{}, nil
}
