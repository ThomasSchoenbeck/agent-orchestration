package llm_test

import (
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// TestRegistrySetModelRoles verifies that each model's declared roles are
// indexed and GetForRole returns the correct model name.
func TestRegistrySetModelRoles(t *testing.T) {
	r := llm.NewRegistry()
	p := llm.NewOpenAIProvider("ollama", "http://localhost:11434", "", "gemma3:4b")
	_ = r.Register("ollama", p)

	models := []db.ProviderModel{
		{Name: "gemma3:4b", Roles: []string{"worker"}},
		{Name: "qwen2.5:14b", Roles: []string{"reviewer", "orchestrator"}},
	}
	r.SetModelRoles("ollama", models)

	cases := []struct {
		role      string
		wantModel string
	}{
		{"worker", "gemma3:4b"},
		{"reviewer", "qwen2.5:14b"},
		{"orchestrator", "qwen2.5:14b"},
	}
	for _, tc := range cases {
		_, model, err := r.GetForRole(tc.role)
		if err != nil {
			t.Errorf("GetForRole(%q): unexpected error: %v", tc.role, err)
			continue
		}
		if model != tc.wantModel {
			t.Errorf("GetForRole(%q): model = %q, want %q", tc.role, model, tc.wantModel)
		}
	}
}

// TestRegistryModelRoleWinsOverProviderRole verifies that a model-level role
// entry takes precedence over a provider-level entry for the same role.
func TestRegistryModelRoleWinsOverProviderRole(t *testing.T) {
	r := llm.NewRegistry()
	p := llm.NewOpenAIProvider("ollama", "http://localhost:11434", "", "gemma3:4b")
	_ = r.Register("ollama", p)

	// Provider-level: "worker" → default model "gemma3:4b"
	r.SetRoles("ollama", "gemma3:4b", []string{"worker"})

	// Model-level: "worker" → "qwen2.5:14b" (should win)
	r.SetModelRoles("ollama", []db.ProviderModel{
		{Name: "qwen2.5:14b", Roles: []string{"worker"}},
	})

	_, model, err := r.GetForRole("worker")
	if err != nil {
		t.Fatalf("GetForRole: %v", err)
	}
	if model != "qwen2.5:14b" {
		t.Errorf("model = %q, want model-level entry %q", model, "qwen2.5:14b")
	}
}
