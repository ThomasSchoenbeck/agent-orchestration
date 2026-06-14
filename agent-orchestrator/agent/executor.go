package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
	"agent-orchestrator/git"
	"agent-orchestrator/llm"
	"agent-orchestrator/logging"
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
	// Feature 6: the agent's skill tags and the resolved skill definitions
	// (fetched lazily from the server and cached) used to compose the persona.
	skillNames     []string
	skillDefs      []*db.SkillDefinition
	skillsResolved bool
	// Subagents feature: enabled subagent skills (spawnable units of work),
	// fetched lazily from the server and cached. Looked up by name when the
	// main loop emits a run_subagent tool call.
	subagentSkills         []*db.SubagentSkill
	subagentSkillsResolved bool
}

// resolveSubagentSkills fetches the enabled subagent skills from the server once
// and caches them. No-op when there is no client.
func (e *Executor) resolveSubagentSkills(ctx context.Context) {
	if e.subagentSkillsResolved || e.client == nil {
		e.subagentSkillsResolved = true
		return
	}
	all, err := e.client.ListSubagentSkills(ctx)
	if err != nil {
		e.log.Warn("could not fetch subagent skills: %v", err)
		e.subagentSkillsResolved = true
		return
	}
	for _, s := range all {
		if s.Enabled {
			e.subagentSkills = append(e.subagentSkills, s)
		}
	}
	e.subagentSkillsResolved = true
}

// lookupSubagentSkill returns the enabled subagent skill with the given name,
// resolving the cache on first use. Returns nil when not found/disabled.
func (e *Executor) lookupSubagentSkill(ctx context.Context, name string) *db.SubagentSkill {
	e.resolveSubagentSkills(ctx)
	for _, s := range e.subagentSkills {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// resolveSkillDefs fetches the agent's skill definitions from the server once
// and caches them. No-op when the agent has no skills.
func (e *Executor) resolveSkillDefs(ctx context.Context) {
	if e.skillsResolved || len(e.skillNames) == 0 || e.client == nil {
		e.skillsResolved = true
		return
	}
	all, err := e.client.ListSkills(ctx)
	if err != nil {
		e.log.Warn("could not fetch skill definitions: %v", err)
		e.skillsResolved = true
		return
	}
	want := make(map[string]bool, len(e.skillNames))
	for _, n := range e.skillNames {
		want[n] = true
	}
	for _, s := range all {
		if s.Enabled && want[s.Name] {
			e.skillDefs = append(e.skillDefs, s)
		}
	}
	e.skillsResolved = true
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

// contextWindowFor returns the configured max context window for modelName among
// the route's provider models, or 0 when unknown.
func contextWindowFor(models []db.ProviderModel, modelName string) int {
	for _, m := range models {
		if m.Name == modelName {
			return m.ContextWindow
		}
	}
	return 0
}

// roleName resolves a role ref (id or name) to its human-readable role name for
// log output, falling back to the ref when the router can't resolve it.
func (e *Executor) roleName(ref string) string {
	if e.rtr == nil {
		return ref
	}
	return e.rtr.RoleName(ref)
}

// Run executes a claimed task and submits the result.
func (e *Executor) Run(ctx context.Context, task *db.Task) {
	// Tag logs with both task and project so they appear in project-scoped views.
	tlog := e.log.ForTask(task.ID).ForProject(task.ProjectID)
	start := time.Now()
	tlog.InfoCtx(ctx, "starting task (role=%s)", e.roleName(task.Role))

	// Notify the user which branch this agent is working on. The branch is
	// provided by the claim response (human-readable); fall back to task/<id>.
	branchName := task.Branch
	if branchName == "" {
		branchName = fmt.Sprintf("task/%s", task.ID)
	}
	startComment := fmt.Sprintf("Agent picked up task (role=%s).\n\nWorking on branch `%s`.",
		e.roleName(task.Role), branchName)
	if err := e.client.PostComment(ctx, task.ID, startComment, e.agentID); err != nil {
		tlog.Warn("failed to post start comment: %v", err)
	}

	result, runStatus, stats, execErr := e.execute(ctx, task)
	if execErr != nil {
		result = map[string]interface{}{"error": execErr.Error()}
		tlog.ErrorCtx(ctx, "task failed: %v", execErr)
	}

	durationMs := int(time.Since(start).Milliseconds())
	metrics := &api.TaskMetrics{
		TokensUsed:   stats.totalTokens,
		InputTokens:  stats.inputTokens,
		OutputTokens: stats.outputTokens,
		Cost:         stats.cost,
		DurationMs:   durationMs,
		Model:        stats.model,
	}

	// Agent asked for help: park the task in AWAITING_INPUT (the question was
	// already posted as a comment). It resumes when answered and re-queued.
	if execErr == nil && runStatus == db.TaskStatusAwaitingInput {
		if err := e.client.SubmitTaskResult(ctx, task.ID, result, db.TaskStatusAwaitingInput, metrics); err != nil {
			tlog.ErrorCtx(ctx, "failed to submit awaiting-input result: %v", err)
		}
		tlog.InfoCtx(ctx, "task parked awaiting input (tokens=%d duration=%dms)", stats.totalTokens, durationMs)
		return
	}

	// Review tasks (claimed from AWAITING_REVIEW, now in REVIEWING) post a review
	// verdict instead of a generic result. The legacy role=="reviewer" signal is
	// retained for backward compatibility.
	if isReviewTask(task) && execErr == nil {
		reviewStatus, body := extractReviewFromResult(result)
		if err := e.client.PostReview(ctx, task.ID, reviewStatus, body, task.BranchHeadSHA, e.agentID); err != nil {
			tlog.WarnCtx(ctx, "PostReview failed: %v", err)
		}
		_ = e.client.SubmitTaskResult(ctx, task.ID, result, db.TaskStatusCompleted, metrics)
		tlog.InfoCtx(ctx, "reviewer task done (review_status=%s tokens=%d duration=%dms)",
			reviewStatus, stats.totalTokens, durationMs)
		e.postCompletionComment(ctx, task, result, db.TaskStatusCompleted, stats.totalTokens, durationMs, nil)
		return
	}

	// Merge-review tasks (claimed from AWAITING_MERGE, now in MERGING) bring the
	// feature branch up to date with main (resolving conflicts with the LLM when
	// needed) and then record a PR decision. The approve/reject endpoint drives
	// the task's final state, so we do not submit a result here. runMergePhase
	// posts its own comments.
	if isMergeReviewTask(task) && execErr == nil {
		e.runMergePhase(ctx, task, result)
		tlog.InfoCtx(ctx, "merge-review task done (tokens=%d duration=%dms)", stats.totalTokens, durationMs)
		return
	}

	// For non-reviewer tasks with execution errors: fail immediately.
	if execErr != nil {
		if err := e.client.SubmitTaskResult(ctx, task.ID, result, db.TaskStatusFailed, metrics); err != nil {
			tlog.ErrorCtx(ctx, "failed to submit failed result: %v", err)
		}
		e.postCompletionComment(ctx, task, result, db.TaskStatusFailed, stats.totalTokens, durationMs, execErr)
		return
	}

	// Planner/orchestrator (creates_tasks) tasks mutate project state through
	// tools and produce no code — complete directly, never a branch or a review.
	if e.IsPlannerTask(task) {
		if err := e.client.SubmitTaskResult(ctx, task.ID, result, db.TaskStatusCompleted, metrics); err != nil {
			tlog.ErrorCtx(ctx, "failed to submit planner result: %v", err)
		} else {
			tlog.InfoCtx(ctx, "planner task completed (tokens=%d duration=%dms)", stats.totalTokens, durationMs)
		}
		e.postCompletionComment(ctx, task, result, db.TaskStatusCompleted, stats.totalTokens, durationMs, nil)
		return
	}

	// Commit and push work when the agent has a worktree.
	if task.WorktreePath != "" {
		if _, pushErr := e.commitTaskWork(ctx, task); pushErr != nil {
			// Push failed — surface as FAILED so the task is not silently completed.
			tlog.ErrorCtx(ctx, "push failed, marking task failed: %v", pushErr)
			failResult := map[string]interface{}{"error": fmt.Sprintf("push failed: %v", pushErr)}
			if err := e.client.SubmitTaskResult(ctx, task.ID, failResult, db.TaskStatusFailed, metrics); err != nil {
				tlog.ErrorCtx(ctx, "failed to submit push-failure result: %v", err)
			}
			e.postCompletionComment(ctx, task, failResult, db.TaskStatusFailed, stats.totalTokens, durationMs, pushErr)
			return
		}
	}

	// All non-reviewer tasks go to AWAITING_REVIEW regardless of whether files
	// were changed. COMPLETED is only reached via explicit reviewer approval.
	if err := e.client.SubmitForReview(ctx, task.ID, metrics); err != nil {
		tlog.WarnCtx(ctx, "SubmitForReview failed: %v", err)
	} else {
		tlog.InfoCtx(ctx, "task submitted for review (tokens=%d duration=%dms)", stats.totalTokens, durationMs)
	}
	e.postCompletionComment(ctx, task, result, db.TaskStatusAwaitingReview, stats.totalTokens, durationMs, nil)
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
		return fmt.Sprintf("**Task failed** (role=%s duration=%dms)\n\nError: %v",
			task.Role, durationMs, execErr)
	}
	header := fmt.Sprintf("**Task %s** (role=%s duration=%dms tokens=%d)",
		status, task.Role, durationMs, tokens)
	if out, ok := result["output"].(string); ok && out != "" {
		if len(out) > 500 {
			out = out[:500] + "…"
		}
		return header + "\n\n" + out
	}
	return header
}

// isReviewTask reports whether the executor is performing a review. A review
// task is one claimed from AWAITING_REVIEW (now in REVIEWING); the legacy
// role=="reviewer" form is retained for backward compatibility.
func isReviewTask(task *db.Task) bool {
	return task.Status == db.TaskStatusReviewing || task.Role == "reviewer"
}

// isMergeReviewTask reports whether the executor is performing a merge review —
// a task claimed from AWAITING_MERGE by a deployer (handles_merge), now in
// MERGING.
func isMergeReviewTask(task *db.Task) bool {
	return task.Status == db.TaskStatusMerging
}

// maxConflictResolveAttempts bounds how many times the LLM is asked to resolve
// the remaining merge conflicts before the task falls back to revision.
const maxConflictResolveAttempts = 2

// maxBranchUpdateAttempts bounds how many times the merge phase re-fetches and
// re-merges main into the feature branch. A second pass catches main advancing
// while the (slow) LLM conflict resolution ran; the loop converges to UpToDate
// once main stops moving, and gives up (→ revision) if it cannot.
const maxBranchUpdateAttempts = 3

// runMergePhase performs the final merge phase for a task in MERGING. A negative
// review verdict rejects immediately. Otherwise it brings the feature branch up
// to date with the PR base (main), resolving conflicts with the LLM when needed,
// pushes the updated branch, and approves the PR so the server-side merge lands
// cleanly. An unresolved conflict (or any git failure) routes the task back to
// AWAITING_REVISION with an explanatory comment.
func (e *Executor) runMergePhase(ctx context.Context, task *db.Task, result map[string]interface{}) {
	tlog := e.log.ForTask(task.ID).ForProject(task.ProjectID)
	verdict, body := extractReviewFromResult(result)

	if verdict != "approved" {
		e.decidePR(ctx, task, "reject", body)
		return
	}

	// Without a worktree we cannot update the branch — fall back to a plain
	// approval and let the server-side merge decide.
	if task.WorktreePath == "" {
		e.decidePR(ctx, task, "approve", body)
		return
	}

	// Bring the feature branch up to date with main, re-fetching after each
	// conflict resolution so we catch main advancing while the (slow) LLM
	// resolution ran. The loop converges once main stops moving (UpToDate).
	for attempt := 0; ; attempt++ {
		status, base, err := e.updateFeatureBranch(ctx, task)
		if err != nil {
			tlog.WarnCtx(ctx, "branch update failed: %v", err)
			e.commentAndReject(ctx, task, fmt.Sprintf("Branch update failed: %v", err))
			return
		}
		if status.Clean {
			break
		}
		if attempt >= maxBranchUpdateAttempts-1 {
			tlog.WarnCtx(ctx, "branch still conflicting after %d attempts", maxBranchUpdateAttempts)
			e.commentAndReject(ctx, task, conflictComment(status.ConflictPaths))
			return
		}
		if !e.resolveConflictsWithLLM(ctx, task, base, status.ConflictPaths) {
			e.commentAndReject(ctx, task, conflictComment(status.ConflictPaths))
			return
		}
		// Loop: re-fetch in case main moved during resolution.
	}

	// Branch is up to date — push it so the server's approve merges cleanly,
	// then approve.
	if err := PushBranch(task.WorktreePath, task.Branch, "", ""); err != nil {
		tlog.WarnCtx(ctx, "push updated feature branch failed: %v", err)
		e.commentAndReject(ctx, task, fmt.Sprintf("Failed to push merged branch: %v", err))
		return
	}
	e.decidePR(ctx, task, "approve", body)
}

// updateFeatureBranch fetches origin and merges the PR base (defaulting to main)
// into the feature branch in the agent's clone. Returns the merge status and the
// resolved base branch name.
func (e *Executor) updateFeatureBranch(ctx context.Context, task *db.Task) (*git.MergeStatus, string, error) {
	base := "main"
	if pr := e.openPRFor(ctx, task); pr != nil && pr.Base != "" {
		base = pr.Base
	}
	if err := FetchOrigin(task.WorktreePath, "", ""); err != nil {
		return nil, base, err
	}
	status, err := git.MergeIntoFeature(task.WorktreePath, "origin/"+base, task.Branch)
	return status, base, err
}

// decidePR submits an approve/reject decision on the task's open PR.
func (e *Executor) decidePR(ctx context.Context, task *db.Task, decision, body string) {
	tlog := e.log.ForTask(task.ID).ForProject(task.ProjectID)
	pr := e.openPRFor(ctx, task)
	if pr == nil {
		tlog.WarnCtx(ctx, "no open PR found for merge-review task")
		return
	}
	if err := e.client.SubmitPRDecision(ctx, task.ID, pr.ID, decision, body, e.agentID); err != nil {
		tlog.WarnCtx(ctx, "SubmitPRDecision (%s) failed: %v", decision, err)
		return
	}
	tlog.InfoCtx(ctx, "submitted merge decision %q on PR %s", decision, pr.ID)
}

// commentAndReject posts an explanatory comment then rejects the PR, which
// routes the task back to AWAITING_REVISION.
func (e *Executor) commentAndReject(ctx context.Context, task *db.Task, msg string) {
	if err := e.client.PostComment(ctx, task.ID, msg, e.agentID); err != nil {
		e.log.ForTask(task.ID).WarnCtx(ctx, "post conflict comment failed: %v", err)
	}
	e.decidePR(ctx, task, "reject", msg)
}

// openPRFor returns the task's open pull request, or nil when there is none.
func (e *Executor) openPRFor(ctx context.Context, task *db.Task) *db.PullRequest {
	prs, err := e.client.ListPRs(ctx, task.ID)
	if err != nil {
		return nil
	}
	for _, p := range prs {
		if p.Status == "open" {
			return p
		}
	}
	return nil
}

// conflictComment formats the revision comment listing unresolved conflicts.
func conflictComment(paths []string) string {
	return "Merge into main hit conflicts the agent could not resolve in:\n- " +
		strings.Join(paths, "\n- ") + "\n\nReturning the task for revision."
}

const conflictResolveSystemPrompt = `You are resolving git merge conflicts. Each file below contains conflict markers:
<<<<<<< feature
(the feature branch version)
=======
(the main branch version)
>>>>>>> main

For every file, produce the fully resolved file content with ALL conflict markers
removed, combining both sides so the code is correct and complete. Do not leave
any <<<<<<<, =======, or >>>>>>> markers.

Respond ONLY with each resolved file in exactly this format, and nothing else:
=== FILE: <path> ===
<full resolved file content>
=== END FILE ===`

// resolveConflictsWithLLM asks the LLM to rewrite the conflicted files (which
// currently hold conflict markers) until no markers remain, up to a bounded
// number of attempts. On success it records the two-parent merge commit on the
// feature branch and returns true. Returns false when conflicts remain, the LLM
// is unavailable, or the commit fails.
func (e *Executor) resolveConflictsWithLLM(ctx context.Context, task *db.Task, base string, paths []string) bool {
	tlog := e.log.ForTask(task.ID).ForProject(task.ProjectID)
	route, err := e.rtr.RouteByRole(effectiveRole(task))
	if err != nil || route == nil || route.Provider == nil {
		tlog.WarnCtx(ctx, "no provider to resolve conflicts: %v", err)
		return false
	}

	remaining := paths
	for attempt := 0; attempt < maxConflictResolveAttempts && len(remaining) > 0; attempt++ {
		prompt := e.buildConflictPrompt(task.WorktreePath, remaining)
		if prompt == "" {
			break
		}
		resp, callErr := route.Provider.Chat(ctx, llm.ChatRequest{
			Model: route.Model,
			Messages: []llm.Message{
				{Role: "system", Content: conflictResolveSystemPrompt},
				{Role: "user", Content: prompt},
			},
		})
		if callErr != nil {
			tlog.WarnCtx(ctx, "conflict-resolve LLM call failed: %v", callErr)
			return false
		}
		for path, content := range parseResolvedFiles(resp.Content) {
			full := filepath.Join(task.WorktreePath, filepath.FromSlash(path))
			if werr := os.WriteFile(full, []byte(content), 0o644); werr != nil {
				tlog.WarnCtx(ctx, "write resolved %q failed: %v", path, werr)
				return false
			}
		}
		remaining = filesWithConflictMarkers(task.WorktreePath, paths)
	}

	if len(remaining) > 0 {
		tlog.WarnCtx(ctx, "conflicts unresolved after %d attempts: %v", maxConflictResolveAttempts, remaining)
		return false
	}

	sha, err := git.CommitMerge(task.WorktreePath, task.Branch, "origin/"+base,
		fmt.Sprintf("Merge %s into %s (conflicts resolved by agent)\n", base, task.Branch),
		"Agent", "agent@system")
	if err != nil {
		tlog.WarnCtx(ctx, "commit of resolved merge failed: %v", err)
		return false
	}
	tlog.InfoCtx(ctx, "resolved merge conflicts and committed %s", sha)
	return true
}

// buildConflictPrompt reads the conflicted files from the worktree and renders
// the user prompt for the resolver. Files that no longer hold markers (or are
// unreadable) are skipped. Returns "" when nothing remains to resolve.
func (e *Executor) buildConflictPrompt(worktreePath string, paths []string) string {
	var sb strings.Builder
	included := 0
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(worktreePath, filepath.FromSlash(p)))
		if err != nil || !hasConflictMarkers(string(data)) {
			continue
		}
		sb.WriteString("=== FILE: ")
		sb.WriteString(p)
		sb.WriteString(" ===\n")
		sb.Write(data)
		sb.WriteString("\n=== END FILE ===\n\n")
		included++
	}
	if included == 0 {
		return ""
	}
	return strings.TrimSpace(sb.String())
}

// parseResolvedFiles extracts path → content pairs from the resolver response
// in the "=== FILE: <path> ===\n...\n=== END FILE ===" format.
func parseResolvedFiles(content string) map[string]string {
	out := map[string]string{}
	matches := resolvedFileRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		path := strings.TrimSpace(m[1])
		body := strings.Trim(m[2], "\n")
		if path != "" {
			out[path] = body
		}
	}
	return out
}

var resolvedFileRe = regexp.MustCompile(`(?s)=== FILE: (.*?) ===\n(.*?)\n=== END FILE ===`)

// filesWithConflictMarkers returns the subset of paths whose worktree contents
// still contain conflict markers (or could not be read).
func filesWithConflictMarkers(worktreePath string, paths []string) []string {
	var out []string
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(worktreePath, filepath.FromSlash(p)))
		if err != nil || hasConflictMarkers(string(data)) {
			out = append(out, p)
		}
	}
	return out
}

// hasConflictMarkers reports whether s contains a git conflict start marker.
func hasConflictMarkers(s string) bool {
	return strings.Contains(s, "<<<<<<< ") ||
		strings.Contains(s, "\n=======\n") && strings.Contains(s, ">>>>>>> ")
}

// effectiveRole is the role whose configuration (model, prompt, tools) drives
// execution. For a review or a merge the agent acts under task.ReviewRole rather
// than the task's own (implementation) role — the reviewer persona owns both the
// REVIEWING and MERGING phases (handles_review + handles_merge).
func effectiveRole(task *db.Task) string {
	if (task.Status == db.TaskStatusReviewing || task.Status == db.TaskStatusMerging) && task.ReviewRole != "" {
		return task.ReviewRole
	}
	return task.Role
}

// IsPlannerTask reports whether the task's effective role carries the
// creates_tasks capability. Such a task plans/reconciles project state via tools
// and produces no code, so it needs no worktree, no branch, and no review — it
// completes directly.
func (e *Executor) IsPlannerTask(task *db.Task) bool {
	if e.rtr == nil {
		return false
	}
	route, err := e.rtr.RouteByRole(effectiveRole(task))
	if err != nil || route == nil {
		return false
	}
	for _, c := range route.Capabilities {
		if c == "creates_tasks" {
			return true
		}
	}
	return false
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

// execStats carries token usage and cost data out of execute().
type execStats struct {
	totalTokens  int
	inputTokens  int
	outputTokens int
	cost         float64
	model        string
}

// execute performs the LLM+tool loop and returns the final result map, status,
// token usage stats, and any error. It does NOT write to the server.
func (e *Executor) execute(ctx context.Context, task *db.Task) (
	result map[string]interface{}, status string, stats execStats, err error,
) {
	tlog := e.log.ForTask(task.ID)

	// Resolve the provider for this task. For a review the reviewing agent's role
	// is task.ReviewRole, not the task's own implementation role.
	effRole := effectiveRole(task)
	tlog.InfoCtx(ctx, "routing task role=%q", e.roleName(effRole))
	route, err := e.rtr.RouteByRole(effRole)
	if err != nil {
		tlog.ErrorCtx(ctx, "routing failed (role=%s): %v", e.roleName(effRole), err)
		return nil, db.TaskStatusFailed, stats, fmt.Errorf("route task (role=%s): %w", effRole, err)
	}
	if route.Provider == nil {
		tlog.ErrorCtx(ctx, "routing returned nil provider for role %q", route.Role)
		return nil, db.TaskStatusFailed, stats, fmt.Errorf("no provider for role %q", route.Role)
	}
	tlog.InfoCtx(ctx, "route resolved provider=%q model=%q role=%q", route.Provider.Name(), route.Model, route.Role)
	stats.model = route.Model

	// Feature 6: compose the agent's persona (role ⊕ skills).
	e.resolveSkillDefs(ctx)

	// Build the initial system + user message. Skill prompt fragments ("souls")
	// are appended after the role prompt in stable order.
	systemMsg := e.buildSystemMessage(task, route)
	for _, sd := range e.skillDefs {
		if frag := strings.TrimSpace(sd.PromptFragment); frag != "" {
			systemMsg += "\n\n" + frag
		}
	}
	// Subagents: when run_subagent is available to this role, tell the model when
	// to delegate context-heavy exploration. In text mode the tool docs already
	// describe it, so only add the guidance for normal (API) tool-call mode.
	if !route.TextToolCalls && systemMsg != "" {
		al := e.resolvedToolAllowlist(route)
		hasSubagent := len(al) == 0 // empty allowlist ⇒ all tools available
		for _, n := range al {
			if n == tools.SubagentToolName {
				hasSubagent = true
				break
			}
		}
		if hasSubagent {
			systemMsg += "\n\n" + subagentPromptFragment
		}
	}
	if route.SystemPrefix != "" && systemMsg != "" {
		systemMsg = route.SystemPrefix + "\n" + systemMsg
	}
	userMsg := e.buildUserMessage(task)
	// Planner/orchestrator tasks need the project's description and current scope
	// to reconcile/plan meaningfully — inject it ahead of the task instructions.
	if e.IsPlannerTask(task) {
		if pc := e.planningContext(ctx, task.ProjectID); pc != "" {
			userMsg = pc + "\n\n" + userMsg
		}
	}
	// Inject prior task comments (e.g. an answer to a previous request_input) so a
	// resumed task sees the human/orchestrator reply.
	if cc := e.commentsContext(ctx, task.ID); cc != "" {
		userMsg = cc + "\n\n" + userMsg
	}

	// Some models (e.g. Gemma via llama.cpp) have no system role in their chat
	// template; injecting one breaks tool-call argument generation. Fold the
	// system content into the first user message instead.
	var messages []llm.Message
	if route.FoldSystemIntoUser || systemMsg == "" {
		content := userMsg
		if systemMsg != "" {
			content = systemMsg + "\n\n" + userMsg
		}
		messages = []llm.Message{{Role: "user", Content: content}}
	} else {
		messages = []llm.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		}
	}

	// In text tool call mode we don't send tool definitions to the LLM —
	// the model outputs JSON blocks that we parse ourselves.
	var toolDefs []llm.ToolDef
	if !route.TextToolCalls {
		all := e.tools.List()
		// Use the role's configured allowlist; fall back to built-in defaults for
		// known roles so the platform works out of the box without any config.
		// Clearing the field in the UI restores the defaults, not "all tools".
		// Feature 6: union in any tools the agent's skills add (role ⊕ skills).
		allowlist := e.resolvedToolAllowlist(route)
		if len(allowlist) > 0 {
			allowed := make(map[string]bool, len(allowlist))
			for _, name := range allowlist {
				allowed[name] = true
			}
			for _, t := range all {
				if allowed[t.Name] {
					toolDefs = append(toolDefs, t)
				}
			}
		} else {
			toolDefs = all
		}
	}

	// Log the full request so the UI shows exactly what is sent to the LLM.
	promptMeta := map[string]interface{}{
		"provider":      route.Provider.Name(),
		"model":         route.Model,
		"system":        systemMsg,
		"system_prefix": route.SystemPrefix,
		"user":          userMsg,
		"tool_count":    len(toolDefs),
		"tool_choice":   map[bool]string{true: "auto", false: "none"}[len(toolDefs) > 0],
	}
	if len(toolDefs) > 0 {
		names := make([]string, len(toolDefs))
		for i, t := range toolDefs {
			names[i] = t.Name
		}
		promptMeta["tools"] = names
	}
	tlog.LogWithMeta(ctx, "info", "LLM prompt", promptMeta)

	// consecutiveErrorRounds counts rounds where every tool call returned an
	// error. After 3 such rounds we abort — the model is stuck.
	consecutiveErrorRounds := 0
	const maxConsecutiveErrorRounds = 3

	// checkpointRequested is set when the model calls the checkpoint_session tool;
	// it triggers the same compaction path as the automatic context-pressure check.
	checkpointRequested := false

	// LLM ↔ tool loop.
	for round := 0; round < maxToolRounds; round++ {
		tlog.InfoCtx(ctx, "calling LLM provider=%q model=%q round=%d messages=%d",
			route.Provider.Name(), route.Model, round, len(messages))
		req := llm.ChatRequest{
			Model:    route.Model,
			Messages: messages,
			Tools:    toolDefs,
		}
		if len(toolDefs) > 0 {
			req.ToolChoice = "auto"
		}
		resp, callErr := route.Provider.Chat(ctx, req)
		if callErr != nil {
			tlog.ErrorCtx(ctx, "LLM call failed (round=%d): %v", round, callErr)
			return nil, db.TaskStatusFailed, stats, fmt.Errorf("llm chat (round %d): %w", round, callErr)
		}
		stats.totalTokens += resp.TokensUsed
		stats.inputTokens += resp.InputTokens
		stats.outputTokens += resp.OutputTokens
		stats.cost += logging.CostForCallWithProvider(route.ProviderModels, nil, route.Model, resp.InputTokens, resp.OutputTokens)

		// In text mode, parse tool calls out of the response content.
		if route.TextToolCalls && len(resp.ToolCalls) == 0 {
			resp.ToolCalls = parseTextToolCalls(resp.Content)
		}

		// Per-message context size: used (prompt incl. cached) and the model's max.
		contextUsed := resp.ContextTokens
		if contextUsed == 0 {
			contextUsed = resp.InputTokens
		}
		contextMax := contextWindowFor(route.ProviderModels, route.Model)
		ctxStr := fmt.Sprintf("%d", contextUsed)
		if contextMax > 0 {
			ctxStr = fmt.Sprintf("%d/%d", contextUsed, contextMax)
		}

		// Log the full response.
		respMeta := map[string]interface{}{
			"round":          round,
			"stop_reason":    resp.StopReason,
			"tokens":         resp.TokensUsed,
			"input_tokens":   resp.InputTokens,
			"output_tokens":  resp.OutputTokens,
			"context_tokens": contextUsed,
			"context_max":    contextMax,
			"content":        resp.Content,
		}
		if len(resp.ToolCalls) > 0 {
			calls := make([]map[string]interface{}, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				calls[i] = map[string]interface{}{"name": tc.Name, "arguments": tc.Arguments}
			}
			respMeta["tool_calls"] = calls
		}
		tlog.LogWithMeta(ctx, "info",
			fmt.Sprintf("LLM response round=%d stop=%s tool_calls=%d · in %d / out %d · ctx %s",
				round, resp.StopReason, len(resp.ToolCalls), resp.InputTokens, resp.OutputTokens, ctxStr),
			respMeta)

		// Append assistant turn — include ToolCalls so the model can correlate
		// tool results with what it requested in previous rounds.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// No tool calls — we're done.
		if len(resp.ToolCalls) == 0 {
			tlog.InfoCtx(ctx, "task complete after %d round(s), total_tokens=%d", round+1, stats.totalTokens)
			return map[string]interface{}{
				"output": resp.Content,
			}, db.TaskStatusCompleted, stats, nil
		}

		// request_input is terminal: the agent is asking a human/orchestrator for
		// help. Post the question as a comment and park the task (not failed) so it
		// can be answered and re-queued. Handled before normal tool execution.
		for _, tc := range resp.ToolCalls {
			if tc.Name == "request_input" {
				question, _ := tc.Arguments["question"].(string)
				if strings.TrimSpace(question) == "" {
					question = "The agent requested input but did not specify a question."
				}
				body := "**Agent needs input**\n\n" + question +
					"\n\n_Reply with a comment, then re-queue the task to resume._"
				if cErr := e.client.PostComment(ctx, task.ID, body, e.agentID); cErr != nil {
					tlog.Warn("failed to post request_input comment: %v", cErr)
				}
				tlog.InfoCtx(ctx, "agent requested input; parking task as AWAITING_INPUT")
				return map[string]interface{}{"question": question}, db.TaskStatusAwaitingInput, stats, nil
			}
		}

		// Execute each tool call and collect results.
		roundHadSuccess := false
		var textResults []string // used in text mode
		for _, tc := range resp.ToolCalls {
			// Inject repo_path for file tools when the LLM omitted it (and strip an
			// accidental worktree prefix from file_path). Shared with the subagent loop.
			if task.WorktreePath != "" {
				if tc.Arguments == nil {
					tc.Arguments = make(map[string]interface{})
				}
				injectRepoPath(tc.Arguments, task.WorktreePath)
			}
			// run_subagent: spawn a bounded nested loop and splice back only its
			// summary. Token/cost folds into the task stats; the subagent transcript
			// never enters this conversation.
			var toolResultStr string
			if tc.Name == tools.SubagentToolName {
				toolResultStr = e.dispatchSubagent(ctx, tlog, *route, task, tc, &stats)
			} else if tc.Name == tools.SessionToolName {
				// Defer the actual checkpoint to the end of the round (after all
				// tool results are in), then compact the history.
				checkpointRequested = true
				reason, _ := tc.Arguments["reason"].(string)
				toolResultStr = fmt.Sprintf(`{"status":"checkpoint_scheduled","reason":%q}`, reason)
			} else {
				// Validate required arguments; give the model a specific error.
				toolResultStr = validateToolArgs(e.tools, tc.Name, tc.Arguments)
				if toolResultStr == "" {
					result, jsonErr := e.tools.ExecuteJSON(ctx, tc.Name, tc.Arguments)
					if jsonErr != nil {
						tlog.Warn("tool %s error: %v", tc.Name, jsonErr)
						toolResultStr = fmt.Sprintf(`{"error":%q}`, jsonErr.Error())
					} else {
						toolResultStr = result
					}
				}
			}
			if !strings.HasPrefix(toolResultStr, `{"error"`) {
				roundHadSuccess = true
			}
			tlog.LogWithMeta(ctx, "info", fmt.Sprintf("tool call: %s", tc.Name),
				map[string]interface{}{
					"tool":      tc.Name,
					"arguments": tc.Arguments,
					"result":    toolResultStr,
				})
			if route.TextToolCalls {
				textResults = append(textResults, fmt.Sprintf("[%s] %s", tc.Name, toolResultStr))
			} else {
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    toolResultStr,
					ToolCallID: tc.ID,
				})
			}
		}

		// In text mode, bundle all tool results into one user message.
		if route.TextToolCalls {
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: "Tool results:\n" + strings.Join(textResults, "\n"),
			})
		}

		// Abort if the model has been stuck producing only failing tool calls.
		if !roundHadSuccess {
			consecutiveErrorRounds++
			if consecutiveErrorRounds >= maxConsecutiveErrorRounds {
				tlog.WarnCtx(ctx, "aborting after %d consecutive rounds with all tool calls failing", consecutiveErrorRounds)
				return map[string]interface{}{
					"warning": fmt.Sprintf("aborted after %d consecutive all-error tool rounds", consecutiveErrorRounds),
				}, db.TaskStatusFailed, stats, fmt.Errorf("model stuck: %d consecutive rounds of failing tool calls", consecutiveErrorRounds)
			}
		} else {
			consecutiveErrorRounds = 0
		}

		// Session checkpoint: compact-and-continue on an explicit request
		// (checkpoint_session tool) or when context usage crosses the threshold.
		// Replaces the in-memory history with [system, summary]; the full
		// pre-checkpoint messages are persisted as an AgentSession.
		if checkpointRequested || shouldCheckpoint(contextUsed, contextMax) {
			reason := "context_pressure"
			if checkpointRequested {
				reason = "requested"
			}
			messages = e.doCheckpoint(ctx, tlog, *route, task, messages, systemMsg, round, &stats, reason)
			checkpointRequested = false
		}
	}

	// Exceeded maxToolRounds — return what the last assistant message said.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return map[string]interface{}{
				"output":  messages[i].Content,
				"warning": "max tool rounds exceeded",
			}, db.TaskStatusCompleted, stats, nil
		}
	}
	return map[string]interface{}{
		"warning": "max tool rounds exceeded with no assistant output",
	}, db.TaskStatusFailed, stats, nil
}

// textToolCallRe extracts fenced code blocks (```json ... ``` or ``` ... ```)
// so we can parse tool calls the model emits as text rather than native calls.
var textToolCallRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?([\\s\\S]*?)\\n?```")

// parseTextToolCalls scans content for JSON blocks containing a "tool_call" key
// and returns them as ToolCall values.
func parseTextToolCalls(content string) []llm.ToolCall {
	matches := textToolCallRe.FindAllStringSubmatch(content, -1)
	var calls []llm.ToolCall
	for i, m := range matches {
		if len(m) < 2 {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &obj); err != nil {
			continue
		}
		tcRaw, ok := obj["tool_call"]
		if !ok {
			continue
		}
		tcMap, ok := tcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := tcMap["name"].(string)
		if name == "" {
			continue
		}
		args, _ := tcMap["arguments"].(map[string]interface{})
		calls = append(calls, llm.ToolCall{
			ID:        fmt.Sprintf("text_call_%d", i),
			Name:      name,
			Arguments: args,
		})
	}
	return calls
}

// defaultToolsForRole returns the built-in tool set for well-known roles.
// These act as the runtime fallback when no explicit allowlist is configured —
// the platform works correctly out of the box without any setup.
// Returning nil means send all tools (for unknown/custom roles).
// subagentPromptFragment is appended to the system prompt when run_subagent is
// available, telling the model when to delegate context-heavy work.
const subagentPromptFragment = "Delegating subtasks: before reading large amounts of code yourself, " +
	"delegate focused exploration to a subagent. Call run_subagent with skill=\"investigate_codebase\" " +
	"and a clear, self-contained summary of what you need to find out. You will get back a concise " +
	"digest instead of the raw files, keeping your own context small. Use this whenever a task starts " +
	"on an unfamiliar or freshly checked-out codebase.\n\n" +
	"If the conversation grows very long and the remaining work can proceed from a summary of what's " +
	"been done, call checkpoint_session to compact the history and continue."

func defaultToolsForRole(role string) []string {
	switch role {
	case "worker":
		return []string{"read_file", "write_file", "list_files", "apply_diff", "run_tests", "task_comment", "request_input", "run_subagent", "checkpoint_session"}
	case "reviewer":
		return []string{"read_file", "list_files", "task_comment", "request_input", "run_subagent", "checkpoint_session"}
	case "orchestrator":
		return []string{"list_tasks", "create_work_package", "plan_project", "bootstrap_project", "sync_scope", "complete_project", "query_context", "save_context", "task_comment", "request_input", "run_subagent", "checkpoint_session"}
	default:
		return nil // unknown role: send all tools
	}
}

// resolvedToolAllowlist returns the effective tool allowlist for a route: the
// role's configured allowlist (or built-in defaults) unioned with any tools the
// agent's skills add. An empty result means "all tools" (unknown role).
func (e *Executor) resolvedToolAllowlist(route *router.RouteResult) []string {
	baseTools := route.ToolAllowlist
	if len(baseTools) == 0 {
		baseTools = defaultToolsForRole(route.Role)
	}
	roleDef := &db.RoleDefinition{Name: route.Role, AllowedTools: baseTools}
	return router.ResolveAgentPersona(roleDef, e.skillDefs).AllowedTools
}

// validateToolArgs checks that all required arguments for the named tool are
// present in args. Returns an instructional error JSON string if any are
// missing, or "" if the call is valid and should proceed.
func validateToolArgs(reg *tools.Registry, toolName string, args map[string]interface{}) string {
	def, err := reg.Get(toolName)
	if err != nil {
		// Unknown tool — let ExecuteJSON handle it.
		return ""
	}
	var missing []string
	for _, req := range def.Required {
		if _, ok := args[req]; !ok {
			missing = append(missing, req)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	// Build a detailed hint that lists every parameter with its type and description.
	var paramDocs []string
	for name, p := range def.Parameters {
		paramDocs = append(paramDocs, fmt.Sprintf("%s (%s): %s", name, p.Type, p.Description))
	}
	hint := fmt.Sprintf(
		"missing required arguments: %s. %s accepts: %s",
		strings.Join(missing, ", "),
		toolName,
		strings.Join(paramDocs, "; "),
	)
	b, _ := json.Marshal(map[string]string{"error": hint})
	return string(b)
}

// commitTaskWork stages, commits, and pushes any changes in the worktree.
// Returns (true, nil) when a new commit was pushed, (false, nil) when the
// worktree is clean, and (false, err) when the push itself failed.
func (e *Executor) commitTaskWork(ctx context.Context, task *db.Task) (bool, error) {
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
		return false, err
	}
	if sha == "" || sha == task.BranchHeadSHA {
		tlog.InfoCtx(ctx, "worktree clean — no new commit")
		return false, nil
	}
	branch := task.Branch
	if branch == "" {
		branch = fmt.Sprintf("task/%s", task.ID)
	}
	tlog.InfoCtx(ctx, "pushed branch %q commit=%s", branch, sha[:12])
	body := fmt.Sprintf("Branch `%s` pushed to project repo. Commit: `%s`", branch, sha[:12])
	if err := e.client.PostComment(ctx, task.ID, body, e.agentID); err != nil {
		tlog.Warn("post push comment failed: %v", err)
	}
	return true, nil
}

// planningContext fetches the project description + current requirements and
// features over HTTP and formats them for a planner task's prompt. Returns an
// empty string when nothing is available (errors are non-fatal — planning can
// still proceed without the extra context).
func (e *Executor) planningContext(ctx context.Context, projectID string) string {
	if e.client == nil || projectID == "" {
		return ""
	}
	var sb strings.Builder
	if p, err := e.client.GetProject(ctx, projectID); err == nil && p != nil {
		if d := strings.TrimSpace(p.Description); d != "" {
			sb.WriteString("Project description:\n")
			sb.WriteString(d)
			sb.WriteString("\n\n")
		}
	}
	if reqs, err := e.client.ListRequirements(ctx, projectID); err == nil && len(reqs) > 0 {
		sb.WriteString("Current requirements:\n")
		for _, r := range reqs {
			sb.WriteString("- " + r.Title + " (" + r.Status + ")\n")
		}
		sb.WriteString("\n")
	}
	if feats, err := e.client.ListFeatures(ctx, projectID); err == nil && len(feats) > 0 {
		sb.WriteString("Current features:\n")
		for _, f := range feats {
			sb.WriteString("- " + f.Title + " (" + f.Status + ")\n")
		}
		sb.WriteString("\n")
	}
	if roles, err := e.client.ListRoles(ctx); err == nil && len(roles) > 0 {
		var names []string
		for _, r := range roles {
			if r.Enabled {
				names = append(names, r.Name)
			}
		}
		if len(names) > 0 {
			sb.WriteString("Available agent roles for work packages (use one of these exact names for the \"role\" field): ")
			sb.WriteString(strings.Join(names, ", "))
			sb.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// commentsContext fetches a task's comments and formats them for the prompt so a
// resumed task (e.g. after request_input) sees prior questions and answers.
// Returns "" when there are none or on error (non-fatal).
func (e *Executor) commentsContext(ctx context.Context, taskID string) string {
	if e.client == nil || taskID == "" {
		return ""
	}
	comments, err := e.client.ListTaskComments(ctx, taskID)
	if err != nil || len(comments) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Conversation on this task (oldest first):\n")
	for _, c := range comments {
		who := c.AuthorType
		if c.AuthorName != "" {
			who = c.AuthorName
		} else if c.AuthorRole != "" {
			who = c.AuthorRole
		}
		sb.WriteString("- [" + who + "] " + strings.TrimSpace(c.Body) + "\n")
	}
	return strings.TrimSpace(sb.String())
}

// buildSystemMessage assembles the system prompt for the task.
// Uses the DB-backed role definition's system prompt, or a generic fallback.
func (e *Executor) buildSystemMessage(task *db.Task, route *router.RouteResult) string {
	// Prefer DB-backed system prompt from the resolved role definition.
	if route.SystemPrompt != "" {
		return route.SystemPrompt
	}

	// Review tasks get a structured fallback that instructs the LLM on
	// the expected output format.
	if isReviewTask(task) {
		return `You are a code reviewer agent. Review the code changes in the provided worktree or diff.

Respond with a JSON object containing:
- "review_status": one of "approved" (no issues), "changes_requested" (minor issues), or "revision_requested" (blocking issues)
- "review_body": your full review in markdown, including specific inline suggestions using fenced code blocks where applicable

Be constructive. Reference specific files and line numbers where possible.`
	}

	// Generic fallback.
	if route.TextToolCalls {
		// Text mode: tools are not sent via the API, so document them in the prompt.
		toolDocs := e.buildToolDocs()
		return fmt.Sprintf(
			"You are a software development agent. Complete the task by calling tools using JSON code blocks.\n\n"+
				"%s\n\nTask ID: %s\nRole: %s",
			toolDocs, task.ID, task.Role,
		)
	}
	// Normal mode: tools are sent via the API.
	return "You are a precise software development agent. " +
		"Perform actions exclusively by invoking the provided tools. " +
		"Always supply correct JSON arguments that strictly match the required schema. " +
		"Do not respond in chat text when a tool can complete the task."
}

// buildToolDocs generates human-readable tool documentation for text-mode prompts
// (where tools are not sent via the API and the model must emit JSON code blocks).
func (e *Executor) buildToolDocs() string {
	defs := e.tools.List()
	if len(defs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## How to call tools\n")
	sb.WriteString("Output a fenced JSON code block for each tool call:\n")
	sb.WriteString("```json\n{\"tool_call\": {\"name\": \"write_file\", \"arguments\": {\"file_path\": \"hello.txt\", \"content\": \"hello world!\"}}}\n```\n")
	sb.WriteString("You will receive the tool result, then continue.\n\n")
	sb.WriteString("## Available Tools\n")
	for _, t := range defs {
		sb.WriteString("\n### ")
		sb.WriteString(t.Name)
		sb.WriteString("\n")
		sb.WriteString(t.Description)
		sb.WriteString("\n")
		// Required args first.
		requiredSet := make(map[string]bool, len(t.InputSchema.Required))
		for _, r := range t.InputSchema.Required {
			requiredSet[r] = true
		}
		for _, name := range t.InputSchema.Required {
			if p, ok := t.InputSchema.Properties[name]; ok {
				sb.WriteString(fmt.Sprintf("- %s (%s, REQUIRED): %s\n", name, p.Type, p.Description))
			}
		}
		// Optional args.
		for name, p := range t.InputSchema.Properties {
			if requiredSet[name] || name == "repo_path" {
				continue // skip required (already listed) and auto-injected repo_path
			}
			sb.WriteString(fmt.Sprintf("- %s (%s, optional): %s\n", name, p.Type, p.Description))
		}
	}
	return sb.String()
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
		msg = fmt.Sprintf("Execute task %s (role=%s).", task.ID, task.Role)
	}
	// Give the agent the project id so project-scoped tools (plan_project,
	// sync_scope, list_tasks, …) target the right project. Without this the model
	// guesses — and the worktree directory it sees is the *task* id, which it
	// would otherwise (wrongly) pass as the project id.
	if task.ProjectID != "" {
		msg += fmt.Sprintf("\n\nProject ID: %s", task.ProjectID)
	}
	// Tell the agent where its workspace is. Use forward slashes so the model
	// never sees backslashes mid-sentence, which can break JSON argument generation.
	if task.WorktreePath != "" {
		msg += fmt.Sprintf("\n\nWorkspace directory: %s", filepath.ToSlash(task.WorktreePath))
	} else {
		e.log.ForTask(task.ID).Warn("task has no WorktreePath — agent will not be able to read or write files")
	}
	msg += "\n\nPlease respond with a structured tool call."
	return msg
}
