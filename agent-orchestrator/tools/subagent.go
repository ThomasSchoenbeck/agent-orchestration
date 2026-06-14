package tools

import (
	"context"
	"fmt"
)

// SubagentToolName is the built-in tool the main agent emits to delegate a
// context-heavy subtask to a subagent. The actual spawn is intercepted by the
// executor loop (which has access to the per-task route and worktree); the
// registry handler here is a defensive no-op that should never run.
const SubagentToolName = "run_subagent"

// RegisterSubagentTool registers the run_subagent tool. The handler is a stub:
// agent/executor.go special-cases run_subagent before reaching the registry, so
// this handler only fires if interception is somehow bypassed.
func RegisterSubagentTool(reg *Registry) error {
	return reg.Register(subagentTool())
}

func subagentTool() Definition {
	return Definition{
		Name: SubagentToolName,
		Description: "Delegate a context-heavy subtask to a subagent. The subagent runs its own " +
			"focused tool loop and returns only a concise summary, so the heavy context never " +
			"enters this conversation. Use it to investigate a large or unfamiliar codebase before " +
			"reading lots of files yourself: call run_subagent with skill=\"investigate_codebase\" " +
			"and a clear summary of what you need to find out.",
		Parameters: map[string]Param{
			"skill":        {Type: "string", Description: "Name of the subagent skill to run (e.g. investigate_codebase)."},
			"instructions": {Type: "string", Description: "A self-contained summary of what the subagent should find out or do."},
		},
		Required: []string{"skill", "instructions"},
		Handler: func(_ context.Context, _ map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("run_subagent must be handled by the executor, not the tool registry")
		},
	}
}
