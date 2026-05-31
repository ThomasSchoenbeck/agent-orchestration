package logging

import (
	"math"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
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

func TestCostForCallWithProvider_UsesProviderPricing(t *testing.T) {
	models := []db.ProviderModel{
		{Name: "gemma3:4b", InputPerMillion: 0.05, OutputPerMillion: 0.10},
	}
	// 2000 input = 2000/1M * 0.05 = 0.0001
	// 1000 output = 1000/1M * 0.10 = 0.0001
	want := 0.0001 + 0.0001
	got := CostForCallWithProvider(models, nil, "gemma3:4b", 2000, 1000)
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("cost = %.8f, want %.8f", got, want)
	}
}

func TestCostForCallWithProvider_FallsBackToCfgPricing(t *testing.T) {
	models := []db.ProviderModel{
		{Name: "other-model", InputPerMillion: 99, OutputPerMillion: 99},
	}
	cfg := &config.Config{
		Pricing: map[string]config.ModelPricing{
			"gpt-4o": {InputPerMillion: 5.0, OutputPerMillion: 15.0},
		},
	}
	// no provider pricing for "gpt-4o" → fall back to cfg
	want := float64(1000)/1_000_000*5.0 + float64(500)/1_000_000*15.0
	got := CostForCallWithProvider(models, cfg, "gpt-4o", 1000, 500)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("cost = %.8f, want %.8f", got, want)
	}
}

func TestCostForCallWithProvider_ZeroWhenNoPricing(t *testing.T) {
	got := CostForCallWithProvider(nil, nil, "unknown", 1000, 500)
	if got != 0 {
		t.Errorf("expected 0 with no pricing, got %f", got)
	}
}
