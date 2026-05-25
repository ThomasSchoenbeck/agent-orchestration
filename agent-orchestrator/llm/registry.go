package llm

import (
	"fmt"
	"sync"

	"agent-orchestrator/config"
)

// Registry manages available LLM providers indexed by name.
// It also maintains a role index so providers can be looked up by the agent
// roles they declare support for (e.g. "reviewer", "worker").
type Registry struct {
	providers  map[string]LLMProvider
	roleIndex  map[string]string // role → provider name
	modelIndex map[string]string // provider name → default model name
	mu         sync.RWMutex
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers:  make(map[string]LLMProvider),
		roleIndex:  make(map[string]string),
		modelIndex: make(map[string]string),
	}
}

// Register adds a provider to the registry.
// Returns an error if a provider with that name is already registered.
func (r *Registry) Register(name string, p LLMProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[name]; ok {
		return fmt.Errorf("provider %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get retrieves a provider by name.
func (r *Registry) Get(name string) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return p, nil
}

// List returns all registered provider names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// InitFromConfig initialises providers from the configuration file.
func (r *Registry) InitFromConfig(cfg *config.Config) error {
	for _, pcfg := range cfg.Providers {
		var provider LLMProvider
		switch pcfg.Type {
		case "openai_compatible":
			provider = NewOpenAIProvider(pcfg.Name, pcfg.BaseURL, pcfg.APIKey, pcfg.Model)
		case "ollama":
			provider = NewOllamaProvider(pcfg.Name, pcfg.BaseURL, pcfg.Model)
		case "anthropic":
			provider = NewAnthropicProvider(pcfg.Name, pcfg.BaseURL, pcfg.APIKey, pcfg.Model)
		case "azure":
			provider = NewAzureOpenAIProvider(pcfg.Name, pcfg.BaseURL, pcfg.APIKey, pcfg.Model, pcfg.Deployment)
		default:
			// Skip unknown providers with a warning; they'll be added in later phases.
			continue
		}
		if err := r.Register(pcfg.Name, provider); err != nil {
			return fmt.Errorf("failed to register provider %q: %w", pcfg.Name, err)
		}
	}
	return nil
}

// Set adds or replaces a named provider in the registry.
func (r *Registry) Set(name string, p LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// Remove removes a named provider and clears any role/model index entries for it.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
	delete(r.modelIndex, name)
	for role, pName := range r.roleIndex {
		if pName == name {
			delete(r.roleIndex, role)
		}
	}
}

// SetRoles associates a set of roles with a named provider and records its
// default model. Replaces any previous role mappings for that provider.
// Call this whenever a provider is added or updated with role preferences.
func (r *Registry) SetRoles(name, defaultModel string, roles []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Clear old role mappings for this provider.
	for role, pName := range r.roleIndex {
		if pName == name {
			delete(r.roleIndex, role)
		}
	}
	r.modelIndex[name] = defaultModel
	for _, role := range roles {
		if role != "" {
			r.roleIndex[role] = name
		}
	}
}

// GetForRole returns the provider and its default model for the given role.
// Returns an error if no provider declares support for that role.
func (r *Registry) GetForRole(role string) (LLMProvider, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.roleIndex[role]
	if !ok {
		return nil, "", fmt.Errorf("no provider registered for role %q", role)
	}
	p, ok := r.providers[name]
	if !ok {
		return nil, "", fmt.Errorf("provider %q declared for role %q but not in registry", name, role)
	}
	return p, r.modelIndex[name], nil
}

// NewFromSpec constructs an LLMProvider from raw config values without
// requiring a database import. extra is provider-specific config
// (e.g. {"deployment": "my-dep"} for Azure).
func NewFromSpec(name, provType, baseURL, apiKey, model string, extra map[string]interface{}) (LLMProvider, error) {
	switch provType {
	case "openai_compatible":
		return NewOpenAIProvider(name, baseURL, apiKey, model), nil
	case "ollama":
		return NewOllamaProvider(name, baseURL, model), nil
	case "anthropic":
		return NewAnthropicProvider(name, baseURL, apiKey, model), nil
	case "azure":
		deployment, _ := extra["deployment"].(string)
		return NewAzureOpenAIProvider(name, baseURL, apiKey, model, deployment), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q", provType)
	}
}

// CloseAll closes all registered providers.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.providers {
		_ = p.Close()
	}
}
