package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/logging"
	"agent-orchestrator/router"
)

// checkpointContextFraction is the share of the model's context window at which
// the executor auto-checkpoints (summarize, persist, compact, continue). High by
// design so short tasks never trigger it.
const checkpointContextFraction = 0.80

// checkpointSummaryPrompt asks the model to distill the conversation so the loop
// can continue from a compacted history.
const checkpointSummaryPrompt = "Summarize the conversation so far so work can continue without the full " +
	"history. Include: what has been done, key results and decisions, the current state of the task, and what " +
	"still remains. Be concise but complete. Reply with only the summary."

// shouldCheckpoint reports whether the current context usage crosses the
// auto-checkpoint threshold. Returns false when the window size is unknown.
func shouldCheckpoint(contextUsed, contextMax int) bool {
	return contextMax > 0 && float64(contextUsed) >= checkpointContextFraction*float64(contextMax)
}

// checkpointSummarize runs one LLM call to summarize the conversation. It returns
// the summary and its own token/cost stats; it does not mutate messages.
func (e *Executor) checkpointSummarize(ctx context.Context, route router.RouteResult, messages []llm.Message) (string, execStats, error) {
	var stats execStats
	stats.model = route.Model
	convo := append(append([]llm.Message{}, messages...), llm.Message{Role: "user", Content: checkpointSummaryPrompt})
	resp, err := route.Provider.Chat(ctx, llm.ChatRequest{Model: route.Model, Messages: convo})
	if err != nil {
		return "", stats, fmt.Errorf("checkpoint summarize: %w", err)
	}
	stats.totalTokens += resp.TokensUsed
	stats.inputTokens += resp.InputTokens
	stats.outputTokens += resp.OutputTokens
	stats.cost += logging.CostForCallWithProvider(route.ProviderModels, nil, route.Model, resp.InputTokens, resp.OutputTokens)
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", stats, fmt.Errorf("checkpoint summarize: empty summary")
	}
	return summary, stats, nil
}

// compactMessages rebuilds the message list as a fresh ("new") session seeded by
// the summary, preserving the system prompt.
func compactMessages(systemMsg, summary string, fold bool) []llm.Message {
	cont := "Continuing from a checkpoint. Summary of the work so far:\n\n" + summary
	if fold || systemMsg == "" {
		content := cont
		if systemMsg != "" {
			content = systemMsg + "\n\n" + cont
		}
		return []llm.Message{{Role: "user", Content: content}}
	}
	return []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: cont},
	}
}

// persistCheckpoint stores the pre-checkpoint messages + summary as an
// AgentSession via the server client (best-effort).
func (e *Executor) persistCheckpoint(ctx context.Context, tlog *AgentLogger, taskID string, preMessages []llm.Message, summary string, round int) {
	if e.client == nil {
		return
	}
	raw, _ := json.Marshal(preMessages)
	if _, err := e.client.CreateAgentSession(ctx, &db.AgentSession{
		TaskID: taskID, AgentID: e.agentID, Summary: summary, Messages: string(raw), Round: round,
	}); err != nil {
		tlog.Warn("failed to persist checkpoint session: %v", err)
	}
}

// doCheckpoint summarizes, persists, and compacts the conversation, folding the
// summarization call's token/cost into the task stats. On error it leaves the
// messages unchanged so the loop can continue uncompacted.
func (e *Executor) doCheckpoint(
	ctx context.Context, tlog *AgentLogger, route router.RouteResult, task *db.Task,
	messages []llm.Message, systemMsg string, round int, stats *execStats, reason string,
) []llm.Message {
	summary, cstats, err := e.checkpointSummarize(ctx, route, messages)
	stats.totalTokens += cstats.totalTokens
	stats.inputTokens += cstats.inputTokens
	stats.outputTokens += cstats.outputTokens
	stats.cost += cstats.cost
	if err != nil {
		tlog.WarnCtx(ctx, "checkpoint failed (%s); continuing without compaction: %v", reason, err)
		return messages
	}
	e.persistCheckpoint(ctx, tlog, task.ID, messages, summary, round)
	tlog.LogWithMeta(ctx, "info", fmt.Sprintf("session checkpoint (%s)", reason), map[string]interface{}{
		"source":  "checkpoint",
		"reason":  reason,
		"round":   round,
		"summary": summary,
	})
	return compactMessages(systemMsg, summary, route.FoldSystemIntoUser)
}
