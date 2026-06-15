package main

import (
	"testing"
	"time"

	"agent-orchestrator/config"
)

func TestParseRoles(t *testing.T) {
	got := parseRoles(" worker , reviewer ,, ")
	if len(got) != 2 || got[0] != "worker" || got[1] != "reviewer" {
		t.Errorf("parseRoles = %v, want [worker reviewer]", got)
	}
	if r := parseRoles(""); len(r) != 0 {
		t.Errorf("parseRoles(\"\") = %v, want empty", r)
	}
}

func TestDefaultAgentConfig(t *testing.T) {
	c := defaultAgentConfig()
	if c.Server.Port != config.DefaultServerPort || c.Server.Host != config.DefaultServerHost {
		t.Errorf("server defaults wrong: %+v", c.Server)
	}
	if c.Database.Path != config.DefaultDBPath {
		t.Errorf("db default path wrong: %q", c.Database.Path)
	}
	if c.Agents.HeartbeatIntervalSec != config.DefaultHeartbeatIntervalSec {
		t.Errorf("heartbeat default wrong: %d", c.Agents.HeartbeatIntervalSec)
	}
}

func TestReconnectCfg(t *testing.T) {
	cfg := &config.Config{Agents: config.AgentConfig{
		ConnectInitialDelayMs: 500,
		ConnectMaxDelayMs:     30000,
		ConnectMaxRetries:     7,
	}}
	rc := reconnectCfg(cfg)
	if rc.InitialDelay != 500*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 500ms", rc.InitialDelay)
	}
	if rc.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", rc.MaxDelay)
	}
	if rc.MaxAttempts != 7 {
		t.Errorf("MaxAttempts = %d, want 7", rc.MaxAttempts)
	}
}

func TestConfigSkillDefinitions(t *testing.T) {
	out := configSkillDefinitions([]config.SkillConfig{
		{Name: "backend", Label: "Backend", Description: "d", PromptFragment: "pf", AllowedTools: []string{"read_file"}},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(out))
	}
	s := out[0]
	if s.Name != "backend" || s.Label != "Backend" || s.PromptFragment != "pf" || !s.Enabled {
		t.Errorf("skill mapping wrong: %+v", s)
	}
	if len(s.AllowedTools) != 1 || s.AllowedTools[0] != "read_file" {
		t.Errorf("allowed tools not mapped: %+v", s.AllowedTools)
	}
}

func TestConfigSubagentSkills(t *testing.T) {
	out := configSubagentSkills([]config.SubagentSkillConfig{
		{Name: "investigate", Label: "Investigate", PromptTemplate: "tmpl", ToolAllowlist: []string{"read_file", "search_files"}, MaxRounds: 5},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 subagent skill, got %d", len(out))
	}
	s := out[0]
	if s.Name != "investigate" || s.PromptTemplate != "tmpl" || s.MaxRounds != 5 || !s.Enabled {
		t.Errorf("subagent skill mapping wrong: %+v", s)
	}
	if len(s.ToolAllowlist) != 2 {
		t.Errorf("tool allowlist not mapped: %+v", s.ToolAllowlist)
	}
}

func TestPrintUsage(t *testing.T) {
	// Smoke: printUsage writes to stdout and must not panic.
	printUsage()
}
