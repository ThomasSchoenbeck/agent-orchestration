package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

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

// shouldCheckpoint reports whether the current context usage crosses the default
// auto-checkpoint threshold. Returns false when the window size is unknown.
func shouldCheckpoint(contextUsed, contextMax int) bool {
	return contextMax > 0 && float64(contextUsed) >= checkpointContextFraction*float64(contextMax)
}

// shouldCheckpointNow is the executor-scoped variant that honours the configured
// context-threshold override (T7.1), falling back to the package default.
func (e *Executor) shouldCheckpointNow(contextUsed, contextMax int) bool {
	return contextMax > 0 && float64(contextUsed) >= e.checkpointFraction()*float64(contextMax)
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

// persistSession stores the session as an AgentSession row — a node in the
// task's session tree — carrying the given summary and tool-loop round
// (best-effort). The row's cost is the delta since this session-chain's last
// persist, so tree views show per-session cost rather than a running total.
func (e *Executor) persistSession(ctx context.Context, tlog *AgentLogger, sess *Session, summary string, round int) {
	if e.client == nil {
		return
	}
	row := sess.toAgentSession(summary, round)
	row.Cost = sess.Stats.cost - sess.costBookmark
	sess.costBookmark = sess.Stats.cost
	if _, err := e.client.CreateAgentSession(ctx, row); err != nil {
		tlog.Warn("failed to persist session: %v", err)
	}
}

// doCheckpoint summarizes the current session, persists it as a completed node,
// and continues in a NEW main session seeded from the summary plus the task's
// worktree memory (cross-session continuation). The new session is linked to the
// one just persisted via ParentID, so successive checkpoints form a chain. The
// summarization call's token/cost folds into the session (task) stats. On a
// summarize error it leaves the messages unchanged so the loop continues
// uncompacted in the same session.
func (e *Executor) doCheckpoint(
	ctx context.Context, tlog *AgentLogger, route router.RouteResult, task *db.Task,
	sess *Session, messages []llm.Message, systemMsg string, round int, reason string,
) []llm.Message {
	stats := &sess.Stats
	summary, cstats, err := e.checkpointSummarize(ctx, route, messages)
	stats.totalTokens += cstats.totalTokens
	stats.inputTokens += cstats.inputTokens
	stats.outputTokens += cstats.outputTokens
	stats.cost += cstats.cost
	if err != nil {
		tlog.WarnCtx(ctx, "checkpoint failed (%s); continuing without compaction: %v", reason, err)
		return messages
	}

	// Persist the session just completed, then roll over to a fresh main session
	// linked to it.
	sess.Messages = messages
	sess.Status = SessionStatusDone
	e.persistSession(ctx, tlog, sess, summary, round)

	prevID := sess.ID
	sess.ID = uuid.NewString()
	sess.ParentID = prevID
	sess.Status = SessionStatusRunning

	// Seed the new session from the summary + the task's worktree memory so work
	// continues across the context-window boundary in a fresh session.
	memory := readWorktreeMemory(task.WorktreePath)
	seeded := seedContinuation(systemMsg, summary, memory, route.FoldSystemIntoUser)
	sess.Messages = seeded

	tlog.LogWithMeta(ctx, "info", fmt.Sprintf("session checkpoint (%s)", reason), map[string]interface{}{
		"source":          "checkpoint",
		"reason":          reason,
		"round":           round,
		"summary":         summary,
		"prev_session_id": prevID,
		"new_session_id":  sess.ID,
	})
	return seeded
}

// seedContinuation builds the opening history for a continuation session from the
// prior session's summary and, when present, the task's worktree memory.
func seedContinuation(systemMsg, summary, memory string, fold bool) []llm.Message {
	if strings.TrimSpace(memory) != "" {
		summary = summary + "\n\nTask memory:\n\n" + memory
	}
	return compactMessages(systemMsg, summary, fold)
}

// readWorktreeMemory returns the human-readable task memory (.agent_context/
// memory.md) from the worktree, or "" when absent. Best-effort.
func readWorktreeMemory(worktree string) string {
	if worktree == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(worktree, memoryContextDir, "memory.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

// memoryContextDir is the worktree subdirectory holding the agent's task memory
// scratchpad (mirrors tools' .agent_context/); kept gitignored, never committed.
const memoryContextDir = ".agent_context"
