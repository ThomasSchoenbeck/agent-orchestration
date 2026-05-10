package logging

import (
	"agent-orchestrator/config"
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
	inputCost := float64(inputTokens) / 1_000_000 * pricing.InputPerMillion
	outputCost := float64(outputTokens) / 1_000_000 * pricing.OutputPerMillion
	return inputCost + outputCost
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
