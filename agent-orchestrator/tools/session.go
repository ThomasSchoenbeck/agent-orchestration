package tools

import (
	"context"
	"fmt"
)

// SessionToolName is the built-in tool the agent emits to request a session
// checkpoint (summarize the conversation, persist it, and continue from a
// compacted history). Like run_subagent, the spawn is intercepted by the
// executor loop; this registry handler is a defensive no-op.
const SessionToolName = "checkpoint_session"

// RegisterSessionTool registers the checkpoint_session tool.
func RegisterSessionTool(reg *Registry) error {
	return reg.Register(sessionTool())
}

func sessionTool() Definition {
	return Definition{
		Name: SessionToolName,
		Description: "Checkpoint the current session: summarize the work so far, save it, and continue " +
			"from a compact summary instead of the full history. Call this when the conversation has grown " +
			"large and the remaining work can proceed from a summary of what's been done.",
		Parameters: map[string]Param{
			"reason": {Type: "string", Description: "Short reason for checkpointing now (optional)."},
		},
		Required: nil,
		Handler: func(_ context.Context, _ map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("checkpoint_session must be handled by the executor, not the tool registry")
		},
	}
}
