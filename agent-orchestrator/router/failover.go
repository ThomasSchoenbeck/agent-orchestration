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
