package main

import (
	"testing"

	"agent-orchestrator/config"
)

func TestAgentTLSConfig_Insecure(t *testing.T) {
	c, err := agentTLSConfig("", true)
	if err != nil {
		t.Fatalf("agentTLSConfig: %v", err)
	}
	if c == nil || !c.InsecureSkipVerify {
		t.Errorf("expected InsecureSkipVerify config, got %+v", c)
	}
}

func TestAgentTLSConfig_NoCAUsesSystemRoots(t *testing.T) {
	c, err := agentTLSConfig("", false)
	if err != nil {
		t.Fatalf("agentTLSConfig: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil TLS config (system roots), got %+v", c)
	}
}

func TestAgentTLSConfig_BadCAPath(t *testing.T) {
	if _, err := agentTLSConfig("/no/such/ca.pem", false); err == nil {
		t.Error("expected error for missing CA file")
	}
}

func TestConfigProvidersToDB_MapsModelsAndRoles(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{
			Name:    "llama.cpp",
			Type:    "openai_compatible",
			BaseURL: "http://localhost:7777/v1",
			Roles:   []string{"worker", "reviewer"}, // provider-level (Phase 5, T5.1)
			Models: []config.ProviderModelConfig{
				{Name: "gemma", Default: true},
			},
		}},
	}
	out := configProvidersToDB(cfg)
	if len(out) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(out))
	}
	p := out[0]
	if p.Name != "llama.cpp" || p.ModelName != "gemma" {
		t.Errorf("provider mapping wrong: name=%q model=%q", p.Name, p.ModelName)
	}
	if len(p.Models) != 1 || p.Models[0].Name != "gemma" {
		t.Errorf("models not mapped: %+v", p.Models)
	}
	if len(p.Roles) != 2 { // provider-level roles carried through
		t.Errorf("expected 2 provider roles, got %v", p.Roles)
	}
}
