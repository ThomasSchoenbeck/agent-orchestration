package agent_test

import (
	"testing"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/db"
)

func TestResilienceFromSettings_ParsesValues(t *testing.T) {
	cfg := agent.ResilienceFromSettings([]*db.Setting{
		{Key: db.SettingKeyMaxRetries, Value: "5"},
		{Key: db.SettingKeyRetryBackoffMs, Value: "250"},
		{Key: db.SettingKeyBreakerThreshold, Value: "7"},
		{Key: db.SettingKeyBreakerResetSec, Value: "15"},
		{Key: db.SettingKeyFallbackProvider, Value: "backup"},
	})
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
	if cfg.Backoff != 250*time.Millisecond {
		t.Errorf("Backoff = %v, want 250ms", cfg.Backoff)
	}
	if cfg.BreakerThreshold != 7 {
		t.Errorf("BreakerThreshold = %d, want 7", cfg.BreakerThreshold)
	}
	if cfg.BreakerReset != 15*time.Second {
		t.Errorf("BreakerReset = %v, want 15s", cfg.BreakerReset)
	}
	if cfg.FallbackProvider != "backup" {
		t.Errorf("FallbackProvider = %q, want backup", cfg.FallbackProvider)
	}
}

func TestResilienceFromSettings_Defaults(t *testing.T) {
	cfg := agent.ResilienceFromSettings(nil)
	if cfg.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.Backoff != time.Second {
		t.Errorf("default Backoff = %v, want 1s", cfg.Backoff)
	}
	if cfg.BreakerThreshold != 5 {
		t.Errorf("default BreakerThreshold = %d, want 5", cfg.BreakerThreshold)
	}
	if cfg.FallbackProvider != "" {
		t.Errorf("default FallbackProvider = %q, want empty", cfg.FallbackProvider)
	}
}
