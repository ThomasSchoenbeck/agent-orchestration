package agent

import (
	"strconv"
	"strings"
	"time"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// ResilienceFromSettings builds an llm.ResilienceConfig from the platform
// settings (UI-configurable). Missing or unparseable values fall back to
// sensible defaults so the agent always has a working configuration.
func ResilienceFromSettings(settings []*db.Setting) llm.ResilienceConfig {
	m := make(map[string]string, len(settings))
	for _, s := range settings {
		m[s.Key] = s.Value
	}
	atoi := func(key string, def int) int {
		if v, ok := m[key]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
		return def
	}
	return llm.ResilienceConfig{
		MaxRetries:       atoi(db.SettingKeyMaxRetries, 3),
		Backoff:          time.Duration(atoi(db.SettingKeyRetryBackoffMs, 1000)) * time.Millisecond,
		BreakerThreshold: atoi(db.SettingKeyBreakerThreshold, 5),
		BreakerReset:     time.Duration(atoi(db.SettingKeyBreakerResetSec, 30)) * time.Second,
		FallbackProvider: strings.TrimSpace(m[db.SettingKeyFallbackProvider]),
	}
}
