package tools_test

import (
	"testing"

	"agent-orchestrator/tools"
)

func TestRegisterSubagentTool_SchemaAndAllowlist(t *testing.T) {
	reg := tools.NewRegistry()
	if err := tools.RegisterSubagentTool(reg); err != nil {
		t.Fatalf("RegisterSubagentTool: %v", err)
	}

	def, err := reg.Get(tools.SubagentToolName)
	if err != nil {
		t.Fatalf("run_subagent not registered: %v", err)
	}
	if def.Name != "run_subagent" {
		t.Errorf("name = %q", def.Name)
	}
	if _, ok := def.Parameters["skill"]; !ok {
		t.Error("missing skill param")
	}
	if _, ok := def.Parameters["instructions"]; !ok {
		t.Error("missing instructions param")
	}
	wantReq := map[string]bool{"skill": true, "instructions": true}
	for _, r := range def.Required {
		delete(wantReq, r)
	}
	if len(wantReq) != 0 {
		t.Errorf("required params missing: %v", wantReq)
	}

	// The tool appears in the catalog so it can be allowlisted into a role.
	var found bool
	for _, d := range reg.List() {
		if d.Name == "run_subagent" {
			found = true
		}
	}
	if !found {
		t.Error("run_subagent not in registry catalog")
	}
}
