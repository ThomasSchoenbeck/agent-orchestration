package agent

import (
	"context"
	"fmt"
	"log"
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
}

// NewExecutor creates an Executor with the given router, tool registry, and
// server client.
func NewExecutor(rtr *router.Router, toolReg *tools.Registry, client *ServerClient, agentID string) *Executor {
	return &Executor{
		rtr:     rtr,
		tools:   toolReg,
		client:  client,
		agentID: agentID,
	}
}

// Run executes a claimed task and submits the result.
func (e *Executor) Run(ctx context.Context, task *db.Task) {
	start := time.Now()
	log.Printf("executor: starting task %s (type=%s role=%s)", task.ID, task.Type, task.Role)

	result, status, tokensUsed, execErr := e.execute(ctx, task)
	if execErr != nil {
		log.Printf("executor: task %s failed: %v", task.ID, execErr)
		status = "failed"
		result = map[string]interface{}{
			"error": execErr.Error(),
		}
	}

	durationMs := int(time.Since(start).Milliseconds())
	metrics := &api.TaskMetrics{
		TokensUsed: tokensUsed,
		DurationMs: durationMs,
	}

	// Reviewer tasks post a review instead of a generic result.
	if task.Role == "reviewer" && execErr == nil {
		reviewStatus, body := extractReviewFromResult(result)
		if err := e.client.PostReview(ctx, task.ID, reviewStatus, body, task.BranchHeadSHA, e.agentID); err != nil {
			log.Printf("executor: PostReview task %s: %v", task.ID, err)
		}
		// Also submit result so metrics are recorded.
		_ = e.client.SubmitTaskResult(ctx, task.ID, result, "completed", metrics)
		log.Printf("executor: reviewer task %s done (review_status=%s tokens=%d duration=%dms)",
			task.ID, reviewStatus, tokensUsed, durationMs)
		return
	}

	if err := e.client.SubmitTaskResult(ctx, task.ID, result, status, metrics); err != nil {
		log.Printf("executor: failed to submit result for task %s: %v", task.ID, err)
	} else {
		log.Printf("executor: task %s done (status=%s tokens=%d duration=%dms)",
			task.ID, status, tokensUsed, durationMs)
	}
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
	// Resolve the provider for this task.
	route, err := e.rtr.RouteByTaskType(task.Type)
	if err != nil {
		log.Printf("executor: task %s RouteByTaskType(%q) failed (%v); trying RouteByRole(%q)",
			task.ID, task.Type, err, task.Role)
		route, err = e.rtr.RouteByRole(task.Role)
		if err != nil {
			return nil, "failed", 0, fmt.Errorf("route task (type=%s role=%s): %w", task.Type, task.Role, err)
		}
	}
	if route.Provider == nil {
		return nil, "failed", 0, fmt.Errorf("no provider for role %q", route.Role)
	}
	log.Printf("executor: task %s resolved to role=%q provider=%q model=%q",
		task.ID, route.Role, route.Provider.Name(), route.Model)

	// Build the initial system + user message.
	systemMsg := e.buildSystemMessage(task, route)
	userMsg := e.buildUserMessage(task)

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}

	toolDefs := e.tools.List()

	// LLM ↔ tool loop.
	for round := 0; round < maxToolRounds; round++ {
		resp, callErr := route.Provider.Chat(ctx, llm.ChatRequest{
			Model:     route.Model,
			Messages:  messages,
			Tools:     toolDefs,
			MaxTokens: 8192,
		})
		if callErr != nil {
			return nil, "failed", totalTokens, fmt.Errorf("llm chat (round %d): %w", round, callErr)
		}
		totalTokens += resp.TokensUsed

		// Append assistant turn.
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: resp.Content,
		})

		// No tool calls — we're done.
		if resp.StopReason == "end_turn" || len(resp.ToolCalls) == 0 {
			return map[string]interface{}{
				"output": resp.Content,
			}, "completed", totalTokens, nil
		}

		// Execute each tool call and build tool-result messages.
		for _, tc := range resp.ToolCalls {
			toolResult, jsonErr := e.tools.ExecuteJSON(ctx, tc.Name, tc.Arguments)
			if jsonErr != nil {
				toolResult = fmt.Sprintf(`{"error":%q}`, jsonErr.Error())
			}
			log.Printf("executor: tool %s called (round %d)", tc.Name, round)
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    toolResult,
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
			}, "completed", totalTokens, nil
		}
	}
	return map[string]interface{}{
		"warning": "max tool rounds exceeded with no assistant output",
	}, "failed", totalTokens, nil
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

	// Generic fallback if no system prompt is defined (should rarely happen with DB-backed roles).
	return fmt.Sprintf(
		"You are an agent executing a task.\nTask ID: %s\nType: %s\nRole: %s\n\nExecution payload:\n%v",
		task.ID, task.Type, task.Role, task.Payload,
	)
}

// buildUserMessage constructs the initial user message from the task payload.
func (e *Executor) buildUserMessage(task *db.Task) string {
	// If there's a description in the payload, use it.
	if task.Payload != nil {
		if desc, ok := task.Payload["description"]; ok {
			if s, ok := desc.(string); ok && s != "" {
				title := ""
				if t, ok := task.Payload["title"]; ok {
					if ts, ok := t.(string); ok {
						title = ts + "\n\n"
					}
				}
				return title + s
			}
		}
	}
	return fmt.Sprintf("Execute task %s (type=%s, role=%s).", task.ID, task.Type, task.Role)
}
