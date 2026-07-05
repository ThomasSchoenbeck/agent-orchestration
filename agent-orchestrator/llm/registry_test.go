package llm_test

import (
	"testing"

	"agent-orchestrator/llm"
)

// TestRegistryProviderRoles verifies that provider-level role preferences are
// indexed and GetForRole returns the provider's default model for the role
// (Phase 5, T5.1 removed model-level role routing).
func TestRegistryProviderRoles(t *testing.T) {
	r := llm.NewRegistry()
	p := llm.NewOpenAIProvider("ollama", "http://localhost:11434", "", "gemma3:4b")
	_ = r.Register("ollama", p)

	r.SetRoles("ollama", "gemma3:4b", []string{"worker", "reviewer"})

	for _, role := range []string{"worker", "reviewer"} {
		_, model, err := r.GetForRole(role)
		if err != nil {
			t.Errorf("GetForRole(%q): unexpected error: %v", role, err)
			continue
		}
		if model != "gemma3:4b" {
			t.Errorf("GetForRole(%q): model = %q, want provider default gemma3:4b", role, model)
		}
	}
}

func TestRegistryGetForRole_UnknownErrors(t *testing.T) {
	r := llm.NewRegistry()
	if _, _, err := r.GetForRole("nobody"); err == nil {
		t.Error("expected error for a role no provider serves")
	}
}
