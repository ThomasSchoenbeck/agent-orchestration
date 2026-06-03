package agent

import (
	"testing"

	"agent-orchestrator/api"
	"agent-orchestrator/config"
)

// TestAgentRecomposesPersonaOnSkillChange verifies that a live skill change
// delivered via the heartbeat updates the agent's skills and flags the executor
// to recompose its persona on the next task (Feature 7 ⟶ Feature 6).
func TestAgentRecomposesPersonaOnSkillChange(t *testing.T) {
	a := NewAgent("persona-test", []string{"worker"}, "http://localhost", &config.Config{})
	a.skills = []string{"backend"}
	a.executor = &Executor{skillNames: []string{"backend"}, skillsResolved: true}

	stop := a.applyControl(&api.HeartbeatResponse{
		DesiredState: "run",
		Roles:        []string{"worker"},
		Skills:       []string{"backend", "go"},
	})
	if stop {
		t.Fatal("run state must not stop the agent")
	}
	if !strSlicesEqual(a.skills, []string{"backend", "go"}) {
		t.Errorf("live skills = %v, want [backend go]", a.skills)
	}
	if !strSlicesEqual(a.executor.skillNames, []string{"backend", "go"}) {
		t.Errorf("executor skillNames = %v, want [backend go]", a.executor.skillNames)
	}
	if a.executor.skillsResolved {
		t.Error("executor should be flagged to re-resolve skill definitions")
	}
}

// TestApplyControl_IgnoresMinimalResponse verifies a non-enriched heartbeat
// (no desired_state) does not wipe the agent's live config.
func TestApplyControl_IgnoresMinimalResponse(t *testing.T) {
	a := NewAgent("min-test", []string{"worker"}, "http://localhost", &config.Config{})
	a.skills = []string{"backend"}

	if a.applyControl(&api.HeartbeatResponse{}) {
		t.Error("empty response must not signal stop")
	}
	if !strSlicesEqual(a.skills, []string{"backend"}) {
		t.Errorf("skills wrongly changed to %v", a.skills)
	}
}
