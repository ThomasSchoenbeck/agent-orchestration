package agent

import (
	"context"
	"fmt"
	"strings"

	"agent-orchestrator/db"
	"agent-orchestrator/router"
)

// taskStatusSkillName is the built-in subagent that reconstructs a resume brief
// from a task's durable record.
const taskStatusSkillName = "task_status"

// reconstructProgress builds a resume brief for a task that has prior persisted
// checkpoints — i.e. it was worked before (a restart, requeue, or hand-off to
// another agent that never had the worktree). It delegates to the task_status
// subagent so the reconstruction stays out of the main session. Returns
// ("", zero) when there is no prior progress or no client. Falls back to the
// latest checkpoint summary when the task_status subagent is unavailable or
// yields nothing.
func (e *Executor) reconstructProgress(
	ctx context.Context, tlog *AgentLogger, route *router.RouteResult, task *db.Task,
) (string, execStats) {
	var stats execStats
	if e.client == nil {
		return "", stats
	}
	sessions, err := e.client.ListAgentSessions(ctx, task.ID)
	if err != nil || len(sessions) == 0 {
		return "", stats // fresh task → normal cold start
	}

	skill := e.lookupSubagentSkill(ctx, taskStatusSkillName)
	if skill == nil {
		return latestCheckpointSummary(sessions), stats
	}

	instructions := fmt.Sprintf(
		"Reconstruct a concise resume brief for task %s: what has been done, the current state, key "+
			"decisions, and what remains. Call get_task_progress with task_id=%q, then read_memory, then "+
			"inspect branch files as needed.", task.ID, task.ID)

	brief, sstats, rerr := e.runSubagent(ctx, tlog, *route, task.WorktreePath, skill, instructions)
	stats = sstats
	if rerr != nil || strings.TrimSpace(brief) == "" {
		tlog.WarnCtx(ctx, "task_status reconstruction failed (%v); falling back to latest checkpoint", rerr)
		return latestCheckpointSummary(sessions), stats
	}
	tlog.InfoCtx(ctx, "reconstructed resume brief from %d prior session(s)", len(sessions))
	return brief, stats
}

// latestCheckpointSummary returns the most recent non-empty checkpoint summary
// from the task's persisted sessions (given oldest-first), or "".
func latestCheckpointSummary(sessions []*db.AgentSession) string {
	for i := len(sessions) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(sessions[i].Summary); s != "" {
			return s
		}
	}
	return ""
}
