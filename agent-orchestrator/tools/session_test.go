package tools_test

import (
	"testing"

	"agent-orchestrator/tools"
)

func TestRegisterSessionTool_Schema(t *testing.T) {
	reg := tools.NewRegistry()
	if err := tools.RegisterSessionTool(reg); err != nil {
		t.Fatalf("RegisterSessionTool: %v", err)
	}
	def, err := reg.Get(tools.SessionToolName)
	if err != nil {
		t.Fatalf("checkpoint_session not registered: %v", err)
	}
	if def.Name != "checkpoint_session" {
		t.Errorf("name = %q", def.Name)
	}
	// reason is optional — no required params.
	if len(def.Required) != 0 {
		t.Errorf("checkpoint_session should have no required params, got %v", def.Required)
	}
}
