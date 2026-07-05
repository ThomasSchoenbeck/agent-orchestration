package router

import (
	"fmt"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// circuitStated is implemented by providers wrapped in a circuit breaker. The
// resolver uses it to skip a provider whose circuit is open (unavailable).
type circuitStated interface {
	State() llm.CircuitState
}

// ResolveModelRef returns the first available (provider, model) from an ordered
// provider>model priority list (Phase 5, T5.3). A ref is skipped when its
// provider is not registered or its circuit breaker is open. It also returns the
// resolved index so a caller can fail over to the next entry on a mid-run Chat
// error. Failover is on errors/unavailability only — a healthy first entry is
// always chosen.
func (r *Router) ResolveModelRef(refs []db.ModelRef) (llm.LLMProvider, string, int, error) {
	return r.ResolveModelRefFrom(refs, 0)
}

// RouteViaModels builds a RouteResult by resolving an ordered provider>model
// priority list with failover (Phase 5, T5.5), using the metadata (models,
// behavioral flags, provider/model prompt layers) of the provider actually
// chosen. It carries no role fields — callers that route a role overlay those.
// Subagents with their own priority list use this directly.
func (r *Router) RouteViaModels(models []db.ModelRef) (*RouteResult, error) {
	prov, model, idx, err := r.ResolveModelRef(models)
	if err != nil {
		return nil, err
	}
	ref := models[idx]

	r.mu.RLock()
	dbProv := r.providersByName[ref.Provider]
	r.mu.RUnlock()

	textToolCalls, fold, prefix, allow := providerBehavior(dbProv)
	var providerModels []db.ProviderModel
	var providerPrompt, modelPrompt string
	if dbProv != nil {
		providerModels = dbProv.Models
		if v, ok := dbProv.Config["system_prompt"].(string); ok {
			providerPrompt = v
		}
		for _, m := range dbProv.Models {
			if m.Name != model {
				continue
			}
			if m.TextToolCalls {
				textToolCalls = true
			}
			if m.FoldSystemIntoUser {
				fold = true
			}
			if m.SystemPrefix != "" {
				prefix = m.SystemPrefix
			}
			if len(m.ToolAllowlist) > 0 {
				allow = m.ToolAllowlist
			}
			modelPrompt = m.SystemPrompt
			break
		}
	}

	return &RouteResult{
		Provider:           prov,
		Model:              model,
		TextToolCalls:      textToolCalls,
		FoldSystemIntoUser: fold,
		SystemPrefix:       prefix,
		ToolAllowlist:      allow,
		ProviderModels:     providerModels,
		ProviderPrompt:     providerPrompt,
		ModelPrompt:        modelPrompt,
	}, nil
}

// routeViaModelRefs resolves a role's priority list and overlays the role's own
// fields (name, prompt, capabilities, allowlist) onto the result.
func (r *Router) routeViaModelRefs(role *db.RoleDefinition) (*RouteResult, error) {
	res, err := r.RouteViaModels(role.Models)
	if err != nil {
		return nil, err
	}
	res.Role = role.Name
	res.SystemPrompt = role.SystemPrompt
	res.Capabilities = role.Capabilities
	// Role-level allowlist wins over everything.
	if len(role.AllowedTools) > 0 {
		res.ToolAllowlist = role.AllowedTools
	}
	return res, nil
}

// providerBehavior extracts a provider's behavioral defaults from its Config map:
// text-tool-call mode, system folding, system prefix, and provider-level tool
// allowlist. Nil-safe.
func providerBehavior(prov *db.Provider) (textToolCalls, fold bool, prefix string, allow []string) {
	if prov == nil {
		return
	}
	if v, ok := prov.Config["text_tool_calls"]; ok {
		textToolCalls, _ = v.(bool)
	}
	if v, ok := prov.Config["fold_system_into_user"]; ok {
		fold, _ = v.(bool)
	}
	if v, ok := prov.Config["system_prefix"]; ok {
		prefix, _ = v.(string)
	}
	if v, ok := prov.Config["tool_allowlist"]; ok {
		if raw, ok := v.([]interface{}); ok {
			for _, item := range raw {
				if s, ok := item.(string); ok {
					allow = append(allow, s)
				}
			}
		}
	}
	return
}

// ResolveModelRefFrom is ResolveModelRef starting at index start. Callers advance
// past an entry that errored mid-run by passing the failed index + 1; failover is
// sticky (decision #2) — the caller keeps the returned index for the rest of the
// task rather than re-probing earlier entries.
func (r *Router) ResolveModelRefFrom(refs []db.ModelRef, start int) (llm.LLMProvider, string, int, error) {
	if len(refs) == 0 {
		return nil, "", -1, fmt.Errorf("no models configured in priority list")
	}
	if start < 0 {
		start = 0
	}
	var lastErr error
	for i := start; i < len(refs); i++ {
		ref := refs[i]
		if ref.Provider == "" || ref.Model == "" {
			lastErr = fmt.Errorf("model ref %d incomplete (provider=%q model=%q)", i, ref.Provider, ref.Model)
			continue
		}
		prov, err := r.registry.Get(ref.Provider)
		if err != nil {
			lastErr = err
			continue
		}
		if cs, ok := prov.(circuitStated); ok && cs.State() == llm.StateOpen {
			lastErr = fmt.Errorf("provider %q circuit open", ref.Provider)
			continue
		}
		return prov, ref.Model, i, nil
	}
	return nil, "", -1, fmt.Errorf("no available provider in priority list from index %d (last error: %v)", start, lastErr)
}
