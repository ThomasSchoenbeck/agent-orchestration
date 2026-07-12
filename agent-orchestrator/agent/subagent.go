package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/logging"
	"agent-orchestrator/router"
	"agent-orchestrator/tools"
)

// subagentTimeout bounds a single subagent run for isolation. The subagent loop
// runs in a goroutine so a stuck/long subagent cannot block the main loop past
// this deadline.
const subagentTimeout = 5 * time.Minute

// subagentInstructionsPlaceholder is replaced in a skill's prompt_template with
// the main agent's instructions.
const subagentInstructionsPlaceholder = "{{instructions}}"

// injectRepoPath fills repo_path for file tools when the LLM omitted it, and
// strips an accidental worktree prefix from file_path. Shared by the main task
// loop and the subagent loop so both resolve the same worktree identically.
// args must be non-nil.
func injectRepoPath(args map[string]interface{}, worktree string) {
	if worktree == "" {
		return
	}
	if _, ok := args["repo_path"]; !ok {
		args["repo_path"] = worktree
	}
	// Strip workspace prefix from file_path if the model erroneously included it
	// (e.g. "data/worktrees/.../test.txt" → "test.txt").
	if fp, ok := args["file_path"].(string); ok {
		prefix := filepath.ToSlash(worktree)
		cleanFP := strings.TrimPrefix(strings.TrimPrefix(filepath.ToSlash(fp), prefix), "/")
		if cleanFP != fp {
			args["file_path"] = cleanFP
		}
	}
}

// subagentSessionKind maps a subagent skill to the session kind recorded in the
// task's session tree (T3.2): codebase investigation → discovery, task_status →
// task_status, and coding/review (and anything else) → work.
func subagentSessionKind(skillName string) SessionKind {
	switch skillName {
	case "investigate_codebase":
		return SessionKindDiscovery
	case taskStatusSkillName:
		return SessionKindTaskStatus
	default:
		return SessionKindWork
	}
}

// dispatchSubagent handles a run_subagent tool call emitted by the main loop. It
// runs the named subagent skill's nested loop and returns the tool-result JSON
// string to splice back into the main conversation.
//
// Token isolation (C3): the subagent's token/cost is folded into the main task's
// stats, but the subagent's intermediate transcript is NOT added to the main
// message history — only the returned summary is (as the run_subagent tool
// result). That isolation is the whole point of the feature.
func (e *Executor) dispatchSubagent(
	ctx context.Context, tlog *AgentLogger, route router.RouteResult,
	sess *Session, task *db.Task, tc llm.ToolCall, stats *execStats,
) string {
	skillName, _ := tc.Arguments["skill"].(string)
	instructions, _ := tc.Arguments["instructions"].(string)
	if strings.TrimSpace(skillName) == "" {
		return `{"error":"run_subagent requires a non-empty skill"}`
	}
	skill := e.lookupSubagentSkill(ctx, skillName)
	if skill == nil {
		return fmt.Sprintf(`{"error":"unknown or disabled subagent skill %q"}`, skillName)
	}

	tlog.LogWithMeta(ctx, "info", fmt.Sprintf("subagent %s started", skill.Name), map[string]interface{}{
		"source":       "subagent",
		"skill":        skill.Name,
		"instructions": instructions,
	})

	// Track this subagent as a linked child session (T3.2) so it appears in the
	// task's session tree, parented to the spawning session.
	child := newChildSession(sess, subagentSessionKind(skill.Name), route)
	child.Title = skill.Name

	summary, sstats, err := e.runSubagent(ctx, tlog, route, task.WorktreePath, skill, instructions)

	// Fold subagent usage into the task totals (cost views stay accurate) even on
	// error/partial runs.
	stats.totalTokens += sstats.totalTokens
	stats.inputTokens += sstats.inputTokens
	stats.outputTokens += sstats.outputTokens
	stats.cost += sstats.cost

	// Record the child's own usage + terminal status, then persist it as a tree
	// node (best-effort — persistence failure never fails the subagent).
	child.Stats = sstats
	switch {
	case err == nil:
		child.markDone()
	case errors.Is(err, context.DeadlineExceeded):
		child.markTimedOut()
	default:
		child.markFailed()
	}
	e.persistSession(ctx, tlog, child, summary, 0)

	tlog.LogWithMeta(ctx, "info", fmt.Sprintf("subagent %s completed", skill.Name), map[string]interface{}{
		"source":        "subagent",
		"skill":         skill.Name,
		"instructions":  instructions,
		"summary":       summary,
		"input_tokens":  sstats.inputTokens,
		"output_tokens": sstats.outputTokens,
		"cost":          sstats.cost,
	})

	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, fmt.Sprintf("subagent %s failed: %v", skill.Name, err))
	}
	b, _ := json.Marshal(map[string]string{"skill": skill.Name, "summary": summary})
	return string(b)
}

// runSubagent runs a bounded nested LLM↔tool loop for the given subagent skill,
// reusing the spawning agent's resolved provider/model (route) and worktree. It
// returns the subagent's final message as the summary plus its own token/cost
// stats. The loop runs in a goroutine bounded by subagentTimeout for isolation.
func (e *Executor) runSubagent(
	ctx context.Context, tlog *AgentLogger, route router.RouteResult,
	worktree string, skill *db.SubagentSkill, instructions string,
) (string, execStats, error) {
	// T5.5: a subagent that carries its own provider>model priority list routes
	// independently (with failover); otherwise it reuses the spawning session's
	// route. On resolution failure it falls back to the spawning route.
	if len(skill.Models) > 0 && e.rtr != nil {
		if sr, rerr := e.rtr.RouteViaModels(skill.Models); rerr == nil {
			route = *sr
		} else {
			tlog.WarnCtx(ctx, "subagent %s: priority list unresolved (%v); using spawning route", skill.Name, rerr)
		}
	}

	maxRounds := skill.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 8
	}

	// Filtered tool defs: only the skill's allowlist, never run_subagent (so a
	// subagent can never spawn another subagent).
	allowed := make(map[string]bool, len(skill.ToolAllowlist))
	for _, n := range skill.ToolAllowlist {
		if n == tools.SubagentToolName {
			continue
		}
		allowed[n] = true
	}
	var toolDefs []llm.ToolDef
	for _, t := range e.tools.List() {
		if allowed[t.Name] {
			toolDefs = append(toolDefs, t)
		}
	}

	// System prompt = the skill template with the main agent's instructions
	// injected. If the template omits the placeholder, append the instructions.
	systemMsg := strings.ReplaceAll(skill.PromptTemplate, subagentInstructionsPlaceholder, instructions)
	if !strings.Contains(skill.PromptTemplate, subagentInstructionsPlaceholder) {
		systemMsg = strings.TrimSpace(systemMsg + "\n\nRequest:\n" + instructions)
	}
	const userMsg = "Begin now."
	var messages []llm.Message
	if route.FoldSystemIntoUser || systemMsg == "" {
		messages = []llm.Message{{Role: "user", Content: strings.TrimSpace(systemMsg + "\n\n" + userMsg)}}
	} else {
		messages = []llm.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		}
	}

	timeout := subagentTimeout
	if e.subagentTimeoutOverride > 0 {
		timeout = e.subagentTimeoutOverride
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type loopResult struct {
		summary string
		stats   execStats
		err     error
	}
	done := make(chan loopResult, 1)
	go func() {
		summary, stats, err := e.subagentToolLoop(runCtx, tlog, route, messages, toolDefs, allowed, worktree, maxRounds, skill)
		done <- loopResult{summary, stats, err}
	}()

	select {
	case <-runCtx.Done():
		// Wait for the loop goroutine to unwind before reading its result, so its
		// partial token/cost stats are safe to read (no data race) and preserved
		// rather than discarded. The loop observes the cancelled ctx each round and
		// returns promptly.
		r := <-done
		if r.err == nil {
			r.err = fmt.Errorf("subagent %s timed out or was cancelled: %w", skill.Name, runCtx.Err())
		}
		return r.summary, r.stats, r.err
	case r := <-done:
		return r.summary, r.stats, r.err
	}
}

// subagentToolLoop is the focused nested loop: call LLM, run allowlisted tools,
// repeat until the model stops calling tools (its final message is the summary)
// or maxRounds is hit. Deliberately a subset of execute()'s loop — no
// request_input parking, no comments injection, no review extraction.
func (e *Executor) subagentToolLoop(
	ctx context.Context, tlog *AgentLogger, route router.RouteResult,
	messages []llm.Message, toolDefs []llm.ToolDef, allowed map[string]bool,
	worktree string, maxRounds int, skill *db.SubagentSkill,
) (string, execStats, error) {
	var stats execStats
	stats.model = route.Model
	lastAssistant := ""

	// Prompt-prep (Phase 4): synthesize the subagent's system prompt each round,
	// rolling forward from the prior prompt + result. Guarded so prompt_prep never
	// preps itself. The subagent is not yet a tracked session (T3.2), so syntheses
	// are not persisted here (empty taskID) — swap + stats folding still apply.
	baseLayers := PromptLayers{}
	if len(messages) > 0 && messages[0].Role == "system" {
		baseLayers.Subagent = messages[0].Content
	}
	prior := &priorRound{}

	for round := 0; round < maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return lastAssistant, stats, err
		}
		if skill.Name != promptPrepSkillName {
			messages = e.preparePrompt(ctx, tlog, route, "", "", round, messages, baseLayers, prior, &stats)
			if len(messages) > 0 && messages[0].Role == "system" {
				prior.prompt = messages[0].Content
			}
		}
		req := llm.ChatRequest{Model: route.Model, Messages: messages, Tools: toolDefs}
		if len(toolDefs) > 0 {
			req.ToolChoice = "auto"
		}
		resp, err := route.Provider.Chat(ctx, req)
		if err != nil {
			return lastAssistant, stats, fmt.Errorf("subagent %s llm chat (round %d): %w", skill.Name, round, err)
		}
		stats.totalTokens += resp.TokensUsed
		stats.inputTokens += resp.InputTokens
		stats.outputTokens += resp.OutputTokens
		stats.cost += logging.CostForCallWithProvider(route.ProviderModels, nil, route.Model, resp.InputTokens, resp.OutputTokens)
		prior.result = resp.Content

		if route.TextToolCalls && len(resp.ToolCalls) == 0 {
			resp.ToolCalls = parseTextToolCalls(resp.Content)
		}

		messages = append(messages, llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		if strings.TrimSpace(resp.Content) != "" {
			lastAssistant = resp.Content
		}

		// No tool calls — the final assistant message is the summary.
		if len(resp.ToolCalls) == 0 {
			return resp.Content, stats, nil
		}

		var textResults []string
		for _, tc := range resp.ToolCalls {
			if tc.Arguments == nil {
				tc.Arguments = make(map[string]interface{})
			}
			injectRepoPath(tc.Arguments, worktree)

			var toolResultStr string
			if !allowed[tc.Name] {
				// Defense in depth: reject anything outside the allowlist (also
				// blocks run_subagent, enforcing no-nesting).
				toolResultStr = fmt.Sprintf(`{"error":"tool %q not permitted for subagent skill %s"}`, tc.Name, skill.Name)
			} else {
				toolResultStr = validateToolArgs(e.tools, tc.Name, tc.Arguments)
				if toolResultStr == "" {
					r, jerr := e.tools.ExecuteJSON(ctx, tc.Name, tc.Arguments)
					if jerr != nil {
						toolResultStr = fmt.Sprintf(`{"error":%q}`, jerr.Error())
					} else {
						toolResultStr = r
					}
				}
			}

			tlog.LogWithMeta(ctx, "info", fmt.Sprintf("subagent %s tool: %s", skill.Name, tc.Name), map[string]interface{}{
				"source":    "subagent",
				"skill":     skill.Name,
				"tool":      tc.Name,
				"arguments": tc.Arguments,
				"result":    toolResultStr,
			})

			if route.TextToolCalls {
				textResults = append(textResults, fmt.Sprintf("[%s] %s", tc.Name, toolResultStr))
			} else {
				messages = append(messages, llm.Message{Role: "tool", Content: toolResultStr, ToolCallID: tc.ID})
			}
		}
		if route.TextToolCalls {
			messages = append(messages, llm.Message{Role: "user", Content: "Tool results:\n" + strings.Join(textResults, "\n")})
		}
	}

	if lastAssistant == "" {
		return "", stats, fmt.Errorf("subagent %s produced no summary within %d rounds", skill.Name, maxRounds)
	}
	return lastAssistant, stats, nil
}
