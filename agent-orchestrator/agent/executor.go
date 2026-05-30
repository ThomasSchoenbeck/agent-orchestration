package agent

import (
	"context"
	"fmt"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
	"agent-orchestrator/tools"
)

// maxToolRounds is the maximum number of tool-call → tool-result cycles
// before the executor gives up and returns what it has.
const maxToolRounds = 10

// Executor runs a task end-to-end: it builds a prompt, calls an LLM, loops
// over tool calls, and finally submits a result to the server.
type Executor struct {
	rtr      *router.Router
	tools    *tools.Registry
	client   *ServerClient
	agentID  string
	log      *AgentLogger
}

// NewExecutor creates an Executor with the given router, tool registry, and
// server client.
func NewExecutor(rtr *router.Router, toolReg *tools.Registry, client *ServerClient, agentID string) *Executor {
	return &Executor{
		rtr:     rtr,
		tools:   toolReg,
		client:  client,
		agentID: agentID,
		log:     newLogger(agentID, client),
	}
}

// CanExecute returns true if at least one of the given roles has a configured,
// registered provider. Used by the poll loop to avoid picking up tasks when the
// model backend is unavailable.
func (e *Executor) CanExecute(roles []string) bool {
	if e.rtr == nil {
		return false
	}
	for _, role := range roles {
		if _, err := e.rtr.RouteByRole(role); err == nil {
			return true
		}
	}
	return false
}

// Run executes a claimed task and submits the result.
func (e *Executor) Run(ctx context.Context, task *db.Task) {
	// Tag logs with both task and project so they appear in project-scoped views.
	tlog := e.log.ForTask(task.ID).ForProject(task.ProjectID)
	start := time.Now()
	tlog.InfoCtx(ctx, "starting task (type=%s role=%s)", task.Type, task.Role)

	// Notify the user which branch this agent is working on.
	branchName := fmt.Sprintf("task/%s", task.ID)
	startComment := fmt.Sprintf("Agent picked up task (role=%s type=%s).\n\nWorking on branch `%s`.",
		task.Role, task.Type, branchName)
	if err := e.client.PostComment(ctx, task.ID, startComment, e.agentID); err != nil {
		tlog.Warn("failed to post start comment: %v", err)
	}

	result, status, tokensUsed, execErr := e.execute(ctx, task)
	if execErr != nil {
		status = db.TaskStatusFailed
		result = map[string]interface{}{"error": execErr.Error()}
		tlog.ErrorCtx(ctx, "task failed: %v", execErr)
	}

	durationMs := int(time.Since(start).Milliseconds())
	metrics := &api.TaskMetrics{TokensUsed: tokensUsed, DurationMs: durationMs}

	// Reviewer tasks post a review instead of a generic result.
	if task.Role == "reviewer" && execErr == nil {
		reviewStatus, body := extractReviewFromResult(result)
		if err := e.client.PostReview(ctx, task.ID, reviewStatus, body, task.BranchHeadSHA, e.agentID); err != nil {
			tlog.WarnCtx(ctx, "PostReview failed: %v", err)
		}
		_ = e.client.SubmitTaskResult(ctx, task.ID, result, db.TaskStatusCompleted, metrics)
		tlog.InfoCtx(ctx, "reviewer task done (review_status=%s tokens=%d duration=%dms)",
			reviewStatus, tokensUsed, durationMs)
		e.postCompletionComment(ctx, task, result, db.TaskStatusCompleted, tokensUsed, durationMs, nil)
		return
	}

	// For non-reviewer tasks: commit work, push branch, then hand off to review.
	if execErr == nil && task.WorktreePath != "" {
		committed := e.commitTaskWork(ctx, task)
		if committed {
			// Work was pushed — transition to AWAITING_REVIEW for a reviewer to pick up.
			if err := e.client.SubmitForReview(ctx, task.ID); err != nil {
				tlog.WarnCtx(ctx, "SubmitForReview failed: %v", err)
			} else {
				tlog.InfoCtx(ctx, "task submitted for review (tokens=%d duration=%dms)", tokensUsed, durationMs)
			}
			e.postCompletionComment(ctx, task, result, db.TaskStatusAwaitingReview, tokensUsed, durationMs, nil)
			return
		}
	}

	// No file changes produced (or no worktree) — mark completed directly.
	if err := e.client.SubmitTaskResult(ctx, task.ID, result, status, metrics); err != nil {
		tlog.ErrorCtx(ctx, "failed to submit result: %v", err)
	} else {
		tlog.InfoCtx(ctx, "task done (status=%s tokens=%d duration=%dms)", status, tokensUsed, durationMs)
	}
	e.postCompletionComment(ctx, task, result, status, tokensUsed, durationMs, execErr)
}

// postCompletionComment posts a human-readable summary comment on the task.
// Failures are silently logged; a missing comment must never fail the task.
func (e *Executor) postCompletionComment(ctx context.Context, task *db.Task, result map[string]interface{}, status string, tokens, durationMs int, execErr error) {
	body := e.buildCompletionComment(task, result, status, tokens, durationMs, execErr)
	if err := e.client.PostComment(ctx, task.ID, body, e.agentID); err != nil {
		e.log.ForTask(task.ID).Warn("failed to post completion comment: %v", err)
	}
}

// buildCompletionComment constructs the comment body for a finished task.
func (e *Executor) buildCompletionComment(task *db.Task, result map[string]interface{}, status string, tokens, durationMs int, execErr error) string {
	if execErr != nil {
		return fmt.Sprintf("**Task failed** (role=%s type=%s duration=%dms)\n\nError: %v",
			task.Role, task.Type, durationMs, execErr)
	}
	header := fmt.Sprintf("**Task %s** (role=%s type=%s duration=%dms tokens=%d)",
		status, task.Role, task.Type, durationMs, tokens)
	if out, ok := result["output"].(string); ok && out != "" {
		if len(out) > 500 {
			out = out[:500] + "…"
		}
		return header + "\n\n" + out
	}
	return header
}

// extractReviewFromResult pulls the review status and body out of the LLM
// result map. The LLM is expected to return:
//
//	{ "review_status": "approved|changes_requested", "review_body": "…" }
//
// Falls back to "changes_requested" with the raw output if keys are absent.
func extractReviewFromResult(result map[string]interface{}) (reviewStatus, body string) {
	if s, ok := result["review_status"].(string); ok && s != "" {
		reviewStatus = s
	} else {
		reviewStatus = "changes_requested"
	}
	if b, ok := result["review_body"].(string); ok && b != "" {
		body = b
	} else if out, ok := result["output"].(string); ok {
		body = out
	}
	return reviewStatus, body
}

// execute performs the LLM+tool loop and returns the final result map, status,
// and total token count. It does NOT write to the server.
func (e *Executor) execute(ctx context.Context, task *db.Task) (
	result map[string]interface{}, status string, totalTokens int, err error,
) {
	tlog := e.log.ForTask(task.ID)

	// Resolve the provider for this task.
	tlog.InfoCtx(ctx, "routing task type=%q role=%q", task.Type, task.Role)
	route, err := e.rtr.RouteByTaskType(task.Type)
	if err != nil {
		tlog.InfoCtx(ctx, "RouteByTaskType(%q) failed — trying RouteByRole(%q): %v", task.Type, task.Role, err)
		route, err = e.rtr.RouteByRole(task.Role)
		if err != nil {
			tlog.ErrorCtx(ctx, "routing failed (type=%s role=%s): %v", task.Type, task.Role, err)
			return nil, db.TaskStatusFailed, 0, fmt.Errorf("route task (type=%s role=%s): %w", task.Type, task.Role, err)
		}
	}
	if route.Provider == nil {
		tlog.ErrorCtx(ctx, "routing returned nil provider for role %q", route.Role)
		return nil, db.TaskStatusFailed, 0, fmt.Errorf("no provider for role %q", route.Role)
	}
	tlog.InfoCtx(ctx, "route resolved provider=%q model=%q role=%q", route.Provider.Name(), route.Model, route.Role)

	// Build the initial system + user message.
	systemMsg := e.buildSystemMessage(task, route)
	userMsg := e.buildUserMessage(task)

	// Log the full prompt so the user can see what was sent to the LLM.
	tlog.LogWithMeta(ctx, "info", "LLM prompt", map[string]interface{}{
		"provider": route.Provider.Name(),
		"model":    route.Model,
		"system":   systemMsg,
		"user":     userMsg,
	})

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}

	toolDefs := e.tools.List()

	// LLM ↔ tool loop.
	for round := 0; round < maxToolRounds; round++ {
		tlog.InfoCtx(ctx, "calling LLM provider=%q model=%q round=%d messages=%d",
			route.Provider.Name(), route.Model, round, len(messages))
		resp, callErr := route.Provider.Chat(ctx, llm.ChatRequest{
			Model:     route.Model,
			Messages:  messages,
			Tools:     toolDefs,
			MaxTokens: 8192,
		})
		if callErr != nil {
			tlog.ErrorCtx(ctx, "LLM call failed (round=%d): %v", round, callErr)
			return nil, db.TaskStatusFailed, totalTokens, fmt.Errorf("llm chat (round %d): %w", round, callErr)
		}
		totalTokens += resp.TokensUsed

		// Log the full response content and any tool calls.
		respMeta := map[string]interface{}{
			"round":       round,
			"stop_reason": resp.StopReason,
			"tokens":      resp.TokensUsed,
			"content":     resp.Content,
		}
		if len(resp.ToolCalls) > 0 {
			calls := make([]map[string]interface{}, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				calls[i] = map[string]interface{}{"name": tc.Name, "arguments": tc.Arguments}
			}
			respMeta["tool_calls"] = calls
		}
		tlog.LogWithMeta(ctx, "info",
			fmt.Sprintf("LLM response round=%d stop=%s tool_calls=%d tokens=%d",
				round, resp.StopReason, len(resp.ToolCalls), resp.TokensUsed),
			respMeta)

		// Append assistant turn.
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: resp.Content,
		})

		// No tool calls — we're done.
		if resp.StopReason == "end_turn" || len(resp.ToolCalls) == 0 {
			tlog.InfoCtx(ctx, "task complete after %d round(s), total_tokens=%d", round+1, totalTokens)
			return map[string]interface{}{
				"output": resp.Content,
			}, db.TaskStatusCompleted, totalTokens, nil
		}

		// Execute each tool call and build tool-result messages.
		for _, tc := range resp.ToolCalls {
			toolResult, jsonErr := e.tools.ExecuteJSON(ctx, tc.Name, tc.Arguments)
			toolResultStr := toolResult
			if jsonErr != nil {
				tlog.Warn("tool %s error: %v", tc.Name, jsonErr)
				toolResultStr = fmt.Sprintf(`{"error":%q}`, jsonErr.Error())
			}
			tlog.LogWithMeta(ctx, "info", fmt.Sprintf("tool call: %s", tc.Name),
				map[string]interface{}{
					"tool":      tc.Name,
					"arguments": tc.Arguments,
					"result":    toolResultStr,
				})
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    toolResultStr,
				ToolCallID: tc.ID,
			})
		}
	}

	// Exceeded maxToolRounds — return what the last assistant message said.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return map[string]interface{}{
				"output":  messages[i].Content,
				"warning": "max tool rounds exceeded",
			}, db.TaskStatusCompleted, totalTokens, nil
		}
	}
	return map[string]interface{}{
		"warning": "max tool rounds exceeded with no assistant output",
	}, db.TaskStatusFailed, totalTokens, nil
}

// commitTaskWork stages, commits, and pushes any changes in the worktree.
// Returns true when a new commit was pushed to the branch (i.e. real work was done).
// Safe to call even when the worktree is clean — returns false without error.
func (e *Executor) commitTaskWork(ctx context.Context, task *db.Task) bool {
	commitMsg := fmt.Sprintf("Agent: task %s", task.ID)
	if task.Payload != nil {
		if title, ok := task.Payload["title"].(string); ok && title != "" {
			commitMsg = "Agent: " + title
		}
	}
	tlog := e.log.ForTask(task.ID).ForProject(task.ProjectID)
	tlog.InfoCtx(ctx, "committing and pushing worktree changes")
	sha, err := CommitAndPush(task.WorktreePath, commitMsg, "Agent", "agent@system", "", "")
	if err != nil {
		tlog.WarnCtx(ctx, "CommitAndPush failed: %v", err)
		return false
	}
	if sha == "" || sha == task.BranchHeadSHA {
		tlog.InfoCtx(ctx, "worktree clean — no new commit")
		return false
	}
	branch := fmt.Sprintf("task/%s", task.ID)
	tlog.InfoCtx(ctx, "pushed branch %q commit=%s", branch, sha[:12])
	body := fmt.Sprintf("Branch `%s` pushed to project repo. Commit: `%s`", branch, sha[:12])
	if err := e.client.PostComment(ctx, task.ID, body, e.agentID); err != nil {
		tlog.Warn("post push comment failed: %v", err)
	}
	return true
}

// buildSystemMessage assembles the system prompt for the task.
// Uses the DB-backed role definition's system prompt, or a generic fallback.
func (e *Executor) buildSystemMessage(task *db.Task, route *router.RouteResult) string {
	// Prefer DB-backed system prompt from the resolved role definition.
	if route.SystemPrompt != "" {
		return route.SystemPrompt
	}

	// Reviewer role gets a structured fallback that instructs the LLM on
	// the expected output format.
	if task.Role == "reviewer" {
		return `You are a code reviewer agent. Review the code changes in the provided worktree or diff.

Respond with a JSON object containing:
- "review_status": one of "approved" (no issues), "changes_requested" (minor issues), or "revision_requested" (blocking issues)
- "review_body": your full review in markdown, including specific inline suggestions using fenced code blocks where applicable

Be constructive. Reference specific files and line numbers where possible.`
	}

	// Generic fallback — tell the agent to use available tools to do real work.
	return fmt.Sprintf(
		"You are a software development agent. Use the available tools (read_file, write_file, list_files, run_tests, etc.) to complete the task.\n\nTask ID: %s\nType: %s\nRole: %s",
		task.ID, task.Type, task.Role,
	)
}

// buildUserMessage constructs the initial user message from the task payload.
func (e *Executor) buildUserMessage(task *db.Task) string {
	msg := ""
	if task.Payload != nil {
		if title, ok := task.Payload["title"].(string); ok && title != "" {
			msg = title
		}
		if desc, ok := task.Payload["description"].(string); ok && desc != "" {
			if msg != "" {
				msg += "\n\n"
			}
			msg += desc
		}
	}
	if msg == "" {
		msg = fmt.Sprintf("Execute task %s (type=%s, role=%s).", task.ID, task.Type, task.Role)
	}
	// Tell the agent where to find its workspace so it can use file tools.
	if task.WorktreePath != "" {
		msg += fmt.Sprintf("\n\nWorkspace directory: %s\nUse this path as repo_path when calling read_file, write_file, list_files, and other file tools.", task.WorktreePath)
	} else {
		e.log.ForTask(task.ID).Warn("task has no WorktreePath — agent will not be able to read or write files")
	}
	return msg
}
