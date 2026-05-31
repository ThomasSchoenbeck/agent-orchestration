package logging

import (
	"agent-orchestrator/config"
	"agent-orchestrator/db"
)

// CostForCall computes the USD cost of one LLM call.
// model is the model name (must match a key in cfg.Pricing).
// If no pricing is configured for the model, returns 0.
func CostForCall(cfg *config.Config, model string, inputTokens, outputTokens int) float64 {
	if cfg == nil || cfg.Pricing == nil {
		return 0
	}
	pricing, ok := cfg.Pricing[model]
	if !ok {
		return 0
	}
	return calcCost(inputTokens, outputTokens, pricing.InputPerMillion, pricing.OutputPerMillion)
}

// CostForCallWithProvider computes the USD cost using the provider's per-model
// pricing first, falling back to cfg.Pricing when the provider has no pricing
// for the given model name, and returning 0 when neither source has data.
func CostForCallWithProvider(models []db.ProviderModel, cfg *config.Config, model string, inputTokens, outputTokens int) float64 {
	for _, m := range models {
		if m.Name == model && (m.InputPerMillion > 0 || m.OutputPerMillion > 0) {
			return calcCost(inputTokens, outputTokens, m.InputPerMillion, m.OutputPerMillion)
		}
	}
	return CostForCall(cfg, model, inputTokens, outputTokens)
}

func calcCost(inputTokens, outputTokens int, inputRate, outputRate float64) float64 {
	return float64(inputTokens)/1_000_000*inputRate + float64(outputTokens)/1_000_000*outputRate
}

// TokenMetrics is the response payload for GET /api/metrics/tokens.
type TokenMetrics struct {
	TotalInputTokens  int                    `json:"total_input_tokens"`
	TotalOutputTokens int                    `json:"total_output_tokens"`
	TotalTokens       int                    `json:"total_tokens"`
	ByProject         []ProjectTokenMetrics  `json:"by_project,omitempty"`
	ByAgent           []AgentTokenMetrics    `json:"by_agent,omitempty"`
}

// ProjectTokenMetrics aggregates token counts per project.
type ProjectTokenMetrics struct {
	ProjectID    string `json:"project_id"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
}

// AgentTokenMetrics aggregates token counts per agent.
type AgentTokenMetrics struct {
	AgentID      string `json:"agent_id"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
}

// CostMetrics is the response payload for GET /api/metrics/costs.
type CostMetrics struct {
	TotalCost  float64              `json:"total_cost"`
	ByProject  []ProjectCostMetrics `json:"by_project,omitempty"`
	ByAgent    []AgentCostMetrics   `json:"by_agent,omitempty"`
}

// ProjectCostMetrics aggregates cost per project.
type ProjectCostMetrics struct {
	ProjectID string  `json:"project_id"`
	Cost      float64 `json:"cost"`
	Tasks     int     `json:"tasks"`
}

// AgentCostMetrics aggregates cost per agent.
type AgentCostMetrics struct {
	AgentID string  `json:"agent_id"`
	Cost    float64 `json:"cost"`
	Tasks   int     `json:"tasks"`
}
