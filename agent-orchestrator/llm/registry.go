package llm

import (
	"fmt"
	"sync"

	"agent-orchestrator/config"
)

// Registry manages available LLM providers indexed by name.
type Registry struct {
	providers map[string]LLMProvider
	mu        sync.RWMutex
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]LLMProvider)}
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

// CloseAll closes all registered providers.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.providers {
		_ = p.Close()
	}
}
