package llm

import (
	"context"
	"errors"
	"testing"
)

// flakyProvider fails its first failTimes Chat calls, then succeeds.
type flakyProvider struct {
	name      string
	failTimes int
	calls     int
}

func (p *flakyProvider) Name() string { return p.name }
func (p *flakyProvider) Close() error { return nil }
func (p *flakyProvider) Chat(_ context.Context, _ ChatRequest) (ChatResponse, error) {
	p.calls++
	if p.calls <= p.failTimes {
		return ChatResponse{}, errors.New("transient")
	}
	return ChatResponse{Content: "ok"}, nil
}
func (p *flakyProvider) Embed(_ context.Context, _ EmbedRequest) (EmbedResponse, error) {
	return EmbedResponse{}, nil
}
func (p *flakyProvider) Rerank(_ context.Context, _ RerankRequest) (RerankResponse, error) {
	return RerankResponse{}, nil
}

func TestRetryProvider_RetriesThenSucceeds(t *testing.T) {
	fp := &flakyProvider{name: "p", failTimes: 2}
	rp := NewRetryProvider(fp, 3, 0)
	resp, err := rp.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %q, want ok", resp.Content)
	}
	if fp.calls != 3 {
		t.Errorf("expected 3 attempts (2 fail + 1 ok), got %d", fp.calls)
	}
}

func TestRetryProvider_ExhaustsRetries(t *testing.T) {
	fp := &flakyProvider{name: "p", failTimes: 10}
	rp := NewRetryProvider(fp, 2, 0)
	_, err := rp.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if fp.calls != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 attempts, got %d", fp.calls)
	}
}

func TestWrapResilience_FailsOverToFallback(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register("primary", &errProvider{"primary", errors.New("down")})
	_ = reg.Register("fb", &okProvider{"fb"})
	reg.SetRoles("primary", "m", []string{"worker"})

	reg.WrapResilience(ResilienceConfig{
		MaxRetries:       1,
		Backoff:          0,
		BreakerThreshold: 5,
		FallbackProvider: "fb",
	})

	p, _, err := reg.GetForRole("worker")
	if err != nil {
		t.Fatalf("GetForRole: %v", err)
	}
	resp, err := p.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected failover to succeed, got %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %q, want ok (from fallback)", resp.Content)
	}
}
