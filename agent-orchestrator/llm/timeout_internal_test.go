package llm

import (
	"testing"
	"time"
)

func TestProviderTimeout(t *testing.T) {
	cases := []struct {
		provType    string
		explicitSec int
		want        time.Duration
	}{
		{"openai_compatible", 0, 300 * time.Second}, // local default
		{"ollama", 0, 300 * time.Second},            // local default
		{"anthropic", 0, 120 * time.Second},         // cloud default
		{"azure", 0, 120 * time.Second},             // cloud default
		{"openai_compatible", 600, 600 * time.Second}, // explicit override wins
		{"anthropic", 45, 45 * time.Second},           // explicit override wins
	}
	for _, c := range cases {
		if got := providerTimeout(c.provType, c.explicitSec); got != c.want {
			t.Errorf("providerTimeout(%q, %d) = %v, want %v", c.provType, c.explicitSec, got, c.want)
		}
	}
}

func TestTimeoutSecFromExtra(t *testing.T) {
	if got := timeoutSecFromExtra(nil); got != 0 {
		t.Errorf("nil extra: got %d, want 0", got)
	}
	if got := timeoutSecFromExtra(map[string]interface{}{"request_timeout_sec": float64(240)}); got != 240 {
		t.Errorf("float64: got %d, want 240", got)
	}
	if got := timeoutSecFromExtra(map[string]interface{}{"request_timeout_sec": 90}); got != 90 {
		t.Errorf("int: got %d, want 90", got)
	}
	if got := timeoutSecFromExtra(map[string]interface{}{"other": "x"}); got != 0 {
		t.Errorf("absent: got %d, want 0", got)
	}
}
