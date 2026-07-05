package agent

import (
	"context"
	"fmt"
	"strings"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/logging"
	"agent-orchestrator/router"
)

// promptPrepSkillName is the built-in subagent that synthesizes the system prompt
// for an upcoming LLM round. When absent/disabled, prompt preparation is skipped
// and the loop uses its statically composed system prompt unchanged.
const promptPrepSkillName = "prompt_prep"

// PromptLayers holds the ordered system-prompt layers plus the task description
// that prompt_prep blends into one prompt. Layers are labeled and emitted in a
// stable priority order; empty layers are omitted. Phase 5 (T5.1/T5.4) adds the
// per-level SystemPrompt fields that populate Agent/Provider/Model; until then the
// main loop passes the already-composed base prompt as Role and the task as Task.
type PromptLayers struct {
	Agent    string
	Role     string
	Subagent string
	Provider string
	Model    string
	Task     string
}

// composePromptLayers renders the present layers as a labeled, stably ordered
// block for prompt_prep's composition input. Missing layers are omitted cleanly.
func composePromptLayers(l PromptLayers) string {
	sections := []struct{ label, body string }{
		{"AGENT", l.Agent},
		{"ROLE", l.Role},
		{"SUBAGENT", l.Subagent},
		{"PROVIDER", l.Provider},
		{"MODEL", l.Model},
		{"TASK", l.Task},
	}
	var b strings.Builder
	for _, s := range sections {
		body := strings.TrimSpace(s.body)
		if body == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[%s]\n%s", s.label, body)
	}
	return b.String()
}

// priorRound carries the prompt actually used and the model's result from the
// previous round so prompt_prep can roll the prompt forward.
type priorRound struct {
	prompt string
	result string
}

// promptPrepInstructions builds the {{instructions}} body for prompt_prep: the
// composed layers on the first round, plus the prior prompt + latest result on
// later rounds so the prompt is rolled forward rather than recomposed cold.
func promptPrepInstructions(layers string, prior *priorRound) string {
	var b strings.Builder
	b.WriteString("Layers (priority order):\n")
	b.WriteString(layers)
	if prior != nil && (strings.TrimSpace(prior.prompt) != "" || strings.TrimSpace(prior.result) != "") {
		b.WriteString("\n\n---\n\nPrompt used last round:\n")
		b.WriteString(strings.TrimSpace(prior.prompt))
		b.WriteString("\n\nModel's latest result:\n")
		b.WriteString(strings.TrimSpace(prior.result))
	}
	return b.String()
}

// synthesizePrompt runs prompt_prep as a single one-shot LLM call (the skill
// carries no tools) and returns the synthesized system prompt plus its token/cost
// stats. It does NOT itself call preparePrompt, so there is no recursion.
func (e *Executor) synthesizePrompt(
	ctx context.Context, route router.RouteResult, skill *db.SubagentSkill, instructions string,
) (string, execStats, error) {
	var stats execStats
	stats.model = route.Model

	systemMsg := strings.ReplaceAll(skill.PromptTemplate, subagentInstructionsPlaceholder, instructions)
	if !strings.Contains(skill.PromptTemplate, subagentInstructionsPlaceholder) {
		systemMsg = strings.TrimSpace(systemMsg + "\n\nComposition inputs:\n" + instructions)
	}
	const userMsg = "Produce the system prompt now."
	var messages []llm.Message
	if route.FoldSystemIntoUser || systemMsg == "" {
		messages = []llm.Message{{Role: "user", Content: strings.TrimSpace(systemMsg + "\n\n" + userMsg)}}
	} else {
		messages = []llm.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		}
	}

	resp, err := route.Provider.Chat(ctx, llm.ChatRequest{Model: route.Model, Messages: messages})
	stats.totalTokens += resp.TokensUsed
	stats.inputTokens += resp.InputTokens
	stats.outputTokens += resp.OutputTokens
	stats.cost += logging.CostForCallWithProvider(route.ProviderModels, nil, route.Model, resp.InputTokens, resp.OutputTokens)
	if err != nil {
		return "", stats, fmt.Errorf("prompt_prep synthesize: %w", err)
	}
	return strings.TrimSpace(resp.Content), stats, nil
}

// preparePrompt synthesizes messages[0] (the system prompt) via the prompt_prep
// subagent before an LLM round, folding its token/cost into stats and persisting
// the result for inspection. It is a no-op — returning messages unchanged — when
// prompt_prep is unavailable, when messages[0] is not a system message (fold-into-
// user mode), or when synthesis fails/returns empty. The synthesized prompt is
// written back into messages[0].Content; the rest of the history is preserved.
func (e *Executor) preparePrompt(
	ctx context.Context, tlog *AgentLogger, route router.RouteResult,
	taskID, sessionID string, round int,
	messages []llm.Message, layers PromptLayers, prior *priorRound, stats *execStats,
) []llm.Message {
	skill := e.lookupSubagentSkill(ctx, promptPrepSkillName)
	if skill == nil {
		return messages // feature off: no prompt_prep skill seeded/enabled
	}
	if len(messages) == 0 || messages[0].Role != "system" {
		return messages // fold-into-user mode: nothing to swap
	}

	instructions := promptPrepInstructions(composePromptLayers(layers), prior)
	synth, sstats, err := e.synthesizePrompt(ctx, route, skill, instructions)
	stats.totalTokens += sstats.totalTokens
	stats.inputTokens += sstats.inputTokens
	stats.outputTokens += sstats.outputTokens
	stats.cost += sstats.cost
	if err != nil || synth == "" {
		if err != nil {
			tlog.WarnCtx(ctx, "prompt_prep synthesis failed (round %d); using base prompt: %v", round, err)
		}
		return messages
	}

	messages[0].Content = synth
	e.persistPreparedPrompt(ctx, tlog, taskID, sessionID, round, synth)
	tlog.LogWithMeta(ctx, "info", fmt.Sprintf("prompt_prep synthesized prompt (round %d)", round), map[string]interface{}{
		"source":     "prompt_prep",
		"round":      round,
		"session_id": sessionID,
		"prompt":     synth,
	})
	return messages
}

// persistPreparedPrompt records a synthesized prompt (best-effort; skipped when
// there is no server client).
func (e *Executor) persistPreparedPrompt(
	ctx context.Context, tlog *AgentLogger, taskID, sessionID string, round int, prompt string,
) {
	// No client → nothing to persist to. Empty taskID → a subagent synthesis (not
	// yet a tracked session, T3.2); swap + stats still apply, but skip the row.
	if e.client == nil || taskID == "" {
		return
	}
	if err := e.client.CreatePreparedPrompt(ctx, &db.PreparedPrompt{
		TaskID: taskID, SessionID: sessionID, Round: round, Prompt: prompt,
	}); err != nil {
		tlog.Warn("failed to persist prepared prompt: %v", err)
	}
}
