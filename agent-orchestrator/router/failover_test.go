package router_test

import (
	"context"
	"testing"

	"agent-orchestrator/config"
	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

// fakeProvider is a minimal LLMProvider that also reports a circuit state, so the
// failover resolver can be exercised without real backends.
type fakeProvider struct {
	name  string
	state llm.CircuitState
}

func (f *fakeProvider) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}
func (f *fakeProvider) Embed(context.Context, llm.EmbedRequest) (llm.EmbedResponse, error) {
	return llm.EmbedResponse{}, nil
}
func (f *fakeProvider) Rerank(context.Context, llm.RerankRequest) (llm.RerankResponse, error) {
	return llm.RerankResponse{}, nil
}
func (f *fakeProvider) Name() string             { return f.name }
func (f *fakeProvider) Close() error             { return nil }
func (f *fakeProvider) State() llm.CircuitState  { return f.state }

func failoverRouter(t *testing.T, provs ...*fakeProvider) *router.Router {
	t.Helper()
	reg := llm.NewRegistry()
	for _, p := range provs {
		if err := reg.Register(p.name, p); err != nil {
			t.Fatalf("register %q: %v", p.name, err)
		}
	}
	return router.New(&config.Config{}, reg)
}

func TestResolveModelRef_HealthyStaysOnFirst(t *testing.T) {
	r := failoverRouter(t, &fakeProvider{name: "a"}, &fakeProvider{name: "b"})
	refs := []db.ModelRef{{Provider: "a", Model: "m1"}, {Provider: "b", Model: "m2"}}
	prov, model, idx, err := r.ResolveModelRef(refs)
	if err != nil {
		t.Fatalf("ResolveModelRef: %v", err)
	}
	if prov.Name() != "a" || model != "m1" || idx != 0 {
		t.Errorf("got provider=%s model=%s idx=%d, want a/m1/0", prov.Name(), model, idx)
	}
}

func TestResolveModelRef_SkipsUnregisteredFirst(t *testing.T) {
	// Provider "a" is not registered → fail over to "b".
	r := failoverRouter(t, &fakeProvider{name: "b"})
	refs := []db.ModelRef{{Provider: "a", Model: "m1"}, {Provider: "b", Model: "m2"}}
	prov, model, idx, err := r.ResolveModelRef(refs)
	if err != nil {
		t.Fatalf("ResolveModelRef: %v", err)
	}
	if prov.Name() != "b" || model != "m2" || idx != 1 {
		t.Errorf("got provider=%s model=%s idx=%d, want b/m2/1", prov.Name(), model, idx)
	}
}

func TestResolveModelRef_SkipsOpenCircuit(t *testing.T) {
	r := failoverRouter(t,
		&fakeProvider{name: "a", state: llm.StateOpen},
		&fakeProvider{name: "b"},
	)
	refs := []db.ModelRef{{Provider: "a", Model: "m1"}, {Provider: "b", Model: "m2"}}
	prov, _, idx, err := r.ResolveModelRef(refs)
	if err != nil {
		t.Fatalf("ResolveModelRef: %v", err)
	}
	if prov.Name() != "b" || idx != 1 {
		t.Errorf("expected failover to b (idx 1), got %s idx %d", prov.Name(), idx)
	}
}

func TestResolveModelRef_SkipsIncompleteRef(t *testing.T) {
	r := failoverRouter(t, &fakeProvider{name: "b"})
	refs := []db.ModelRef{{Provider: "a"}, {Provider: "b", Model: "m2"}} // first missing model
	prov, model, idx, err := r.ResolveModelRef(refs)
	if err != nil {
		t.Fatalf("ResolveModelRef: %v", err)
	}
	if prov.Name() != "b" || model != "m2" || idx != 1 {
		t.Errorf("got %s/%s/%d, want b/m2/1", prov.Name(), model, idx)
	}
}

func TestResolveModelRef_AllDownErrors(t *testing.T) {
	r := failoverRouter(t) // nothing registered
	refs := []db.ModelRef{{Provider: "a", Model: "m1"}, {Provider: "b", Model: "m2"}}
	if _, _, _, err := r.ResolveModelRef(refs); err == nil {
		t.Error("expected an error when no provider is available")
	}
}

func TestResolveModelRef_EmptyListErrors(t *testing.T) {
	r := failoverRouter(t)
	if _, _, _, err := r.ResolveModelRef(nil); err == nil {
		t.Error("expected an error for an empty priority list")
	}
}

func TestResolveModelRefFrom_StickyAdvance(t *testing.T) {
	// Starting at index 1 skips a healthy first entry (sticky failover).
	r := failoverRouter(t, &fakeProvider{name: "a"}, &fakeProvider{name: "b"})
	refs := []db.ModelRef{{Provider: "a", Model: "m1"}, {Provider: "b", Model: "m2"}}
	prov, _, idx, err := r.ResolveModelRefFrom(refs, 1)
	if err != nil {
		t.Fatalf("ResolveModelRefFrom: %v", err)
	}
	if prov.Name() != "b" || idx != 1 {
		t.Errorf("expected b at idx 1, got %s idx %d", prov.Name(), idx)
	}
}

// --- T5.5: RouteByRole via the priority list ---

func TestRouteByRole_ViaPriorityList(t *testing.T) {
	reg := llm.NewRegistry()
	_ = reg.Register("anthropic", &fakeProvider{name: "anthropic"})
	r := router.New(&config.Config{}, reg)

	providers := []*db.Provider{{
		ID: "p1", Name: "anthropic", Enabled: true,
		Models: []db.ProviderModel{{Name: "claude-x"}},
		Config: map[string]interface{}{},
	}}
	roles := []*db.RoleDefinition{{
		ID: "r1", Name: "worker", Enabled: true,
		Models: []db.ModelRef{{Provider: "anthropic", Model: "claude-x"}},
	}}
	if err := r.LoadFromData(providers, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}

	res, err := r.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if res.Provider.Name() != "anthropic" || res.Model != "claude-x" || res.Role != "worker" {
		t.Errorf("got provider=%s model=%s role=%s", res.Provider.Name(), res.Model, res.Role)
	}
}

func TestRouteByRole_PriorityListFailsOver(t *testing.T) {
	reg := llm.NewRegistry()
	_ = reg.Register("up", &fakeProvider{name: "up"}) // "down" intentionally unregistered
	r := router.New(&config.Config{}, reg)

	providers := []*db.Provider{{
		ID: "p2", Name: "up", Enabled: true,
		Models: []db.ProviderModel{{Name: "m2"}},
		Config: map[string]interface{}{},
	}}
	roles := []*db.RoleDefinition{{
		ID: "r1", Name: "worker", Enabled: true,
		Models: []db.ModelRef{{Provider: "down", Model: "m1"}, {Provider: "up", Model: "m2"}},
	}}
	if err := r.LoadFromData(providers, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}

	res, err := r.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if res.Provider.Name() != "up" || res.Model != "m2" {
		t.Errorf("expected failover to up/m2, got %s/%s", res.Provider.Name(), res.Model)
	}
}

func TestRouteByRole_PriorityListPopulatesPromptLayers(t *testing.T) {
	reg := llm.NewRegistry()
	_ = reg.Register("anthropic", &fakeProvider{name: "anthropic"})
	r := router.New(&config.Config{}, reg)

	providers := []*db.Provider{{
		ID: "p1", Name: "anthropic", Enabled: true,
		Models: []db.ProviderModel{{Name: "claude-x", SystemPrompt: "model-layer"}},
		Config: map[string]interface{}{"system_prompt": "provider-layer"},
	}}
	roles := []*db.RoleDefinition{{
		ID: "r1", Name: "worker", Enabled: true, SystemPrompt: "role-layer",
		Models: []db.ModelRef{{Provider: "anthropic", Model: "claude-x"}},
	}}
	if err := r.LoadFromData(providers, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}

	res, err := r.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if res.SystemPrompt != "role-layer" {
		t.Errorf("role layer = %q", res.SystemPrompt)
	}
	if res.ProviderPrompt != "provider-layer" {
		t.Errorf("provider layer = %q", res.ProviderPrompt)
	}
	if res.ModelPrompt != "model-layer" {
		t.Errorf("model layer = %q", res.ModelPrompt)
	}
}

func TestRouteByRole_LegacyPathPopulatesPromptLayers(t *testing.T) {
	reg := llm.NewRegistry()
	_ = reg.Register("anthropic", &fakeProvider{name: "anthropic"})
	r := router.New(&config.Config{}, reg)

	providers := []*db.Provider{{
		ID: "p1", Name: "anthropic", Enabled: true, ModelName: "claude-x",
		Models: []db.ProviderModel{{Name: "claude-x", SystemPrompt: "model-layer"}},
		Config: map[string]interface{}{"system_prompt": "provider-layer"},
	}}
	roles := []*db.RoleDefinition{{
		ID: "r1", Name: "worker", Enabled: true, ProviderID: "p1", SystemPrompt: "role-layer",
		// no Models → legacy routeFromCache path
	}}
	if err := r.LoadFromData(providers, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}

	res, err := r.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if res.ProviderPrompt != "provider-layer" || res.ModelPrompt != "model-layer" {
		t.Errorf("legacy path layers: provider=%q model=%q", res.ProviderPrompt, res.ModelPrompt)
	}
}

func TestRouteByRole_PriorityListTakesPrecedenceOverBinding(t *testing.T) {
	reg := llm.NewRegistry()
	_ = reg.Register("primary", &fakeProvider{name: "primary"})
	_ = reg.Register("legacy", &fakeProvider{name: "legacy"})
	r := router.New(&config.Config{}, reg)

	providers := []*db.Provider{
		{ID: "pl", Name: "legacy", Enabled: true, ModelName: "legacy-m",
			Models: []db.ProviderModel{{Name: "legacy-m"}}, Config: map[string]interface{}{}},
		{ID: "pp", Name: "primary", Enabled: true,
			Models: []db.ProviderModel{{Name: "primary-m"}}, Config: map[string]interface{}{}},
	}
	roles := []*db.RoleDefinition{{
		ID: "r1", Name: "worker", Enabled: true,
		ProviderID: "pl", ModelOverride: "legacy-m", // legacy single binding
		Models:     []db.ModelRef{{Provider: "primary", Model: "primary-m"}}, // priority list wins
	}}
	if err := r.LoadFromData(providers, roles); err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}

	res, err := r.RouteByRole("worker")
	if err != nil {
		t.Fatalf("RouteByRole: %v", err)
	}
	if res.Provider.Name() != "primary" || res.Model != "primary-m" {
		t.Errorf("priority list should take precedence, got %s/%s", res.Provider.Name(), res.Model)
	}
}
