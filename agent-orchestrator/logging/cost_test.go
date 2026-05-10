package logging

import (
	"math"
	"testing"

	"agent-orchestrator/config"
)

func TestCostForCall_NilConfig(t *testing.T) {
	cost := CostForCall(nil, "gpt-4o", 1000, 500)
	if cost != 0 {
		t.Errorf("expected 0 cost for nil config, got %f", cost)
	}
}

func TestCostForCall_NilPricing(t *testing.T) {
	cfg := &config.Config{}
	cost := CostForCall(cfg, "gpt-4o", 1000, 500)
	if cost != 0 {
		t.Errorf("expected 0 cost for nil pricing map, got %f", cost)
	}
}

func TestCostForCall_UnknownModel(t *testing.T) {
	cfg := &config.Config{
		Pricing: map[string]config.ModelPricing{
			"gpt-4o": {InputPerMillion: 5.0, OutputPerMillion: 15.0},
		},
	}
	cost := CostForCall(cfg, "unknown-model", 1000, 500)
	if cost != 0 {
		t.Errorf("expected 0 cost for unknown model, got %f", cost)
	}
}

func TestCostForCall_KnownModel(t *testing.T) {
	cfg := &config.Config{
		Pricing: map[string]config.ModelPricing{
			// $5 per 1M input, $15 per 1M output
			"gpt-4o": {InputPerMillion: 5.0, OutputPerMillion: 15.0},
		},
	}
	// 1000 input tokens = 1000/1_000_000 * 5.0 = $0.005
	// 500  output tokens = 500/1_000_000 * 15.0 = $0.0075
	// total = $0.0125
	cost := CostForCall(cfg, "gpt-4o", 1000, 500)
	want := 0.005 + 0.0075
	if math.Abs(cost-want) > 1e-9 {
		t.Errorf("expected cost %.6f, got %.6f", want, cost)
	}
}

func TestCostForCall_ZeroTokens(t *testing.T) {
	cfg := &config.Config{
		Pricing: map[string]config.ModelPricing{
			"model": {InputPerMillion: 5.0, OutputPerMillion: 15.0},
		},
	}
	cost := CostForCall(cfg, "model", 0, 0)
	if cost != 0 {
		t.Errorf("expected 0 cost for 0 tokens, got %f", cost)
	}
}

func TestCostForCall_InputOnly(t *testing.T) {
	cfg := &config.Config{
		Pricing: map[string]config.ModelPricing{
			"claude-3-sonnet": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
		},
	}
	// 2_000_000 input tokens = 2 * 3.0 = $6.00
	cost := CostForCall(cfg, "claude-3-sonnet", 2_000_000, 0)
	want := 6.0
	if math.Abs(cost-want) > 1e-9 {
		t.Errorf("expected cost %.2f, got %.6f", want, cost)
	}
}

func TestCostForCall_OutputOnly(t *testing.T) {
	cfg := &config.Config{
		Pricing: map[string]config.ModelPricing{
			"claude-3-sonnet": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
		},
	}
	// 1_000_000 output tokens = 1 * 15.0 = $15.00
	cost := CostForCall(cfg, "claude-3-sonnet", 0, 1_000_000)
	want := 15.0
	if math.Abs(cost-want) > 1e-9 {
		t.Errorf("expected cost %.2f, got %.6f", want, cost)
	}
}

func TestCostForCall_SmallTokenCount(t *testing.T) {
	cfg := &config.Config{
		Pricing: map[string]config.ModelPricing{
			"gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
		},
	}
	// 100 input = 100/1M * 0.15 = 0.000015
	// 50 output = 50/1M * 0.60 = 0.000030
	cost := CostForCall(cfg, "gpt-4o-mini", 100, 50)
	want := 0.000015 + 0.000030
	if math.Abs(cost-want) > 1e-12 {
		t.Errorf("expected cost %.8f, got %.8f", want, cost)
	}
}
