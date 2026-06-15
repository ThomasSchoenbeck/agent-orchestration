package llm

import (
	"context"
	"testing"
	"time"
)

// Registry List + CloseAll (both previously 0%).
func TestRegistryListAndCloseAll(t *testing.T) {
	r := NewRegistry()
	_ = r.Register("a", &okProvider{name: "a"})
	_ = r.Register("b", &okProvider{name: "b"})

	if names := r.List(); len(names) != 2 {
		t.Errorf("List = %v, want 2 names", names)
	}
	r.CloseAll() // okProvider.Close is a no-op; just exercises the path.
}

// RetryProvider pass-through wrappers (Name/Close/Embed/Rerank).
func TestRetryProviderPassThrough(t *testing.T) {
	rp := NewRetryProvider(&okProvider{name: "p"}, 2, 0)
	if rp.Name() != "p" {
		t.Errorf("Name = %q, want p", rp.Name())
	}
	if err := rp.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := rp.Embed(context.Background(), EmbedRequest{Input: []string{"x"}}); err != nil {
		t.Errorf("Embed: %v", err)
	}
	if _, err := rp.Rerank(context.Background(), RerankRequest{Query: "q"}); err != nil {
		t.Errorf("Rerank: %v", err)
	}
}

// CircuitBreaker pass-through wrappers (Name/Close/Embed/Rerank).
func TestCircuitBreakerPassThrough(t *testing.T) {
	cb := NewCircuitBreaker(&okProvider{name: "p"}, 3, time.Minute)
	if cb.Name() != "p" {
		t.Errorf("Name = %q, want p", cb.Name())
	}
	if err := cb.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := cb.Embed(context.Background(), EmbedRequest{Input: []string{"x"}}); err != nil {
		t.Errorf("Embed: %v", err)
	}
	if _, err := cb.Rerank(context.Background(), RerankRequest{Query: "q"}); err != nil {
		t.Errorf("Rerank: %v", err)
	}
}

// FailoverProvider wrappers (Close/Rerank previously 0%).
func TestFailoverProviderPassThrough(t *testing.T) {
	fp := NewFailoverProvider("fo", &okProvider{name: "primary"}, &okProvider{name: "fallback"})
	if fp.Name() != "fo" {
		t.Errorf("Name = %q, want fo", fp.Name())
	}
	if err := fp.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := fp.Rerank(context.Background(), RerankRequest{Query: "q"}); err != nil {
		t.Errorf("Rerank: %v", err)
	}
}
