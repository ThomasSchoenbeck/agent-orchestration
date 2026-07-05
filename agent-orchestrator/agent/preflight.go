package agent

import (
	"context"
	"fmt"
	"strings"

	"agent-orchestrator/db"
)

// requiredCoreSkills returns the mandatory core subagent skills a worker or
// reviewer task must have available before it runs. Both share prompt_prep,
// investigate_codebase, and task_status; the work skill differs by role
// (code_subtask for a worker, review_subtask for a reviewer).
func requiredCoreSkills(task *db.Task) []string {
	work := "code_subtask"
	if isReviewTask(task) {
		work = "review_subtask"
	}
	return []string{promptPrepSkillName, "investigate_codebase", work, taskStatusSkillName}
}

// preflightCoreSkills verifies that every mandatory core subagent skill for a
// worker/reviewer task exists and is enabled, returning a specific error naming
// the missing ones so the task can be failed fast. Orchestrator/planner and
// merge-review tasks keep their current path unchanged and are exempt (they do
// not use the work-subagent machinery).
//
// The subagent-skill registry lives in the server; disabled skills never enter
// the resolved cache, so a disabled required skill resolves as absent and fails
// the preflight. If the registry cannot be read at all the skills resolve absent
// too — a broken registry means a broken platform, and the task fails fast.
func (e *Executor) preflightCoreSkills(ctx context.Context, task *db.Task) error {
	if e.IsPlannerTask(task) || isMergeReviewTask(task) {
		return nil
	}
	var missing []string
	for _, name := range requiredCoreSkills(task) {
		if e.lookupSubagentSkill(ctx, name) == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("preflight: missing or disabled required core subagent skill(s): %s",
			strings.Join(missing, ", "))
	}
	return nil
}
