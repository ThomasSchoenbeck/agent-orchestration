package agent

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"
)

// ReconnectConfig controls reconnect backoff behaviour.
type ReconnectConfig struct {
	// InitialDelay is the wait after the first failure (default 1s).
	InitialDelay time.Duration
	// MaxDelay caps the backoff (default 60s).
	MaxDelay time.Duration
	// MaxAttempts is the maximum number of registration attempts (0 = unlimited).
	MaxAttempts int
}

func defaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialDelay: time.Second,
		MaxDelay:     60 * time.Second,
		MaxAttempts:  0, // unlimited
	}
}

// StartWithReconnect calls Start and, if registration fails, retries with
// exponential backoff until the context is cancelled or MaxAttempts is reached.
//
// This is the recommended entry point for production agents: it tolerates
// transient server unavailability at startup and also re-registers automatically
// after a crash-and-restart cycle.
func (a *Agent) StartWithReconnect(ctx context.Context, cfg ReconnectConfig) error {
	if cfg.InitialDelay <= 0 {
		cfg = defaultReconnectConfig()
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 60 * time.Second
	}

	attempt := 0
	delay := cfg.InitialDelay

	for {
		attempt++
		err := a.Start(ctx)
		if err == nil {
			return nil
		}

		if cfg.MaxAttempts > 0 && attempt >= cfg.MaxAttempts {
			return fmt.Errorf("agent %q: registration failed after %d attempts: %w", a.name, attempt, err)
		}

		log.Printf("agent %q: registration attempt %d failed (%v), retrying in %v",
			a.name, attempt, err, delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		// Exponential backoff, capped at MaxDelay.
		delay = time.Duration(math.Min(
			float64(delay)*2,
			float64(cfg.MaxDelay),
		))
	}
}

// WithRetry wraps a function call with exponential backoff retry logic.
// It retries on error up to maxAttempts times (0 = no retry).
// initialDelay is doubled on each failure.
func WithRetry(ctx context.Context, maxAttempts int, initialDelay time.Duration, fn func() error) error {
	if maxAttempts <= 0 {
		return fn()
	}
	delay := initialDelay
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if i == maxAttempts-1 {
			break // don't wait after the last attempt
		}
		log.Printf("[retry] attempt %d/%d failed: %v — retrying in %v", i+1, maxAttempts, lastErr, delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay = time.Duration(math.Min(float64(delay)*2, float64(60*time.Second)))
	}
	return fmt.Errorf("all %d attempts failed: %w", maxAttempts, lastErr)
}

// RetryBackoff computes the exponential backoff duration for a given attempt.
// attempt=0 → initialDelay, attempt=1 → 2*initialDelay, etc., capped at maxDelay.
func RetryBackoff(attempt int, initialDelay, maxDelay time.Duration) time.Duration {
	d := float64(initialDelay) * math.Pow(2, float64(attempt))
	if d > float64(maxDelay) {
		d = float64(maxDelay)
	}
	return time.Duration(d)
}
