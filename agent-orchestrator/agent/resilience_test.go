package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- WithRetry ---

func TestWithRetry_SucceedsFirstAttempt(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestWithRetry_SucceedsOnRetry(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errors.New("temporary error")
		}
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
	calls := 0
	sentinel := errors.New("always fails")
	err := WithRetry(context.Background(), 3, time.Millisecond, func() error {
		calls++
		return sentinel
	})
	if err == nil {
		t.Error("expected error when all attempts fail")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetry_ZeroMaxAttempts_CallsOnce(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), 0, time.Millisecond, func() error {
		calls++
		return errors.New("error")
	})
	if err == nil {
		t.Error("expected error from single call")
	}
	if calls != 1 {
		t.Errorf("expected 1 call for maxAttempts=0, got %d", calls)
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err := WithRetry(ctx, 10, 20*time.Millisecond, func() error {
		calls++
		return errors.New("error")
	})
	if err == nil {
		t.Error("expected error when context is cancelled")
	}
	// Should have been called once (the context is cancelled during the first backoff wait).
	if calls > 2 {
		t.Errorf("expected at most 2 calls before cancel, got %d", calls)
	}
}

// --- RetryBackoff ---

func TestRetryBackoff_InitialDelay(t *testing.T) {
	got := RetryBackoff(0, time.Second, time.Minute)
	if got != time.Second {
		t.Errorf("attempt 0: expected %v, got %v", time.Second, got)
	}
}

func TestRetryBackoff_Doubles(t *testing.T) {
	got := RetryBackoff(1, time.Second, time.Minute)
	if got != 2*time.Second {
		t.Errorf("attempt 1: expected %v, got %v", 2*time.Second, got)
	}
}

func TestRetryBackoff_CappedAtMax(t *testing.T) {
	maxDelay := 10 * time.Second
	got := RetryBackoff(100, time.Second, maxDelay)
	if got != maxDelay {
		t.Errorf("expected capped at %v, got %v", maxDelay, got)
	}
}

// --- StartWithReconnect ---

func TestStartWithReconnect_MaxAttemptsExceeded(t *testing.T) {
	// Create an agent that will always fail to register (bad server URL).
	a := NewAgent("test-agent", []string{"worker"}, "http://127.0.0.1:1", nil)

	cfg := ReconnectConfig{
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		MaxAttempts:  2,
	}

	err := a.StartWithReconnect(context.Background(), cfg)
	if err == nil {
		a.Stop()
		t.Fatal("expected error when server is unreachable")
	}
}

func TestStartWithReconnect_ContextCancelled(t *testing.T) {
	a := NewAgent("test-agent", []string{"worker"}, "http://127.0.0.1:1", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	cfg := ReconnectConfig{
		InitialDelay: 5 * time.Millisecond,
		MaxDelay:     20 * time.Millisecond,
		MaxAttempts:  0, // unlimited — context should stop it
	}

	err := a.StartWithReconnect(ctx, cfg)
	if err == nil {
		a.Stop()
		t.Fatal("expected error when context times out")
	}
}
