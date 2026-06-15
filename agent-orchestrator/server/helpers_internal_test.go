package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

func TestInternalError(t *testing.T) {
	s, _ := newCtxTestServer(t)
	w := httptest.NewRecorder()
	s.internalError(w, errors.New("boom"))
	if w.Code != 500 {
		t.Errorf("internalError code = %d, want 500", w.Code)
	}
}

func TestResolveCredential(t *testing.T) {
	s, d := newCtxTestServer(t)
	ctx := context.Background()

	if s.resolveCredential("") != "" {
		t.Error("empty ref should resolve to empty")
	}
	t.Setenv("MY_TEST_CRED", "secret")
	if s.resolveCredential("MY_TEST_CRED") != "secret" {
		t.Error("env-backed credential not resolved")
	}
	if err := d.SetSetting(ctx, "cred.key", "settingval", ""); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if s.resolveCredential("cred.key") != "settingval" {
		t.Error("setting-backed credential not resolved")
	}
	if s.resolveCredential("does-not-exist") != "" {
		t.Error("unknown ref should resolve to empty")
	}
}

func TestCostFromModel(t *testing.T) {
	s, d := newCtxTestServer(t)
	ctx := context.Background()

	if err := d.CreateProvider(ctx, &db.Provider{
		Name: "p", Type: "openai_compatible", BaseURL: "http://x",
		Models: []db.ProviderModel{{Name: "gpt", InputPerMillion: 1.0, OutputPerMillion: 2.0}},
	}); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	in, out := s.costSplitFromModel(ctx, "gpt", 1_000_000, 1_000_000)
	if in != 1.0 || out != 2.0 {
		t.Errorf("costSplitFromModel = %v/%v, want 1/2", in, out)
	}
	if c := s.costFromModel(ctx, "gpt", 1_000_000, 1_000_000); c != 3.0 {
		t.Errorf("costFromModel = %v, want 3", c)
	}
	// Unknown model ⇒ zero cost.
	if c := s.costFromModel(ctx, "unknown", 1000, 1000); c != 0 {
		t.Errorf("costFromModel(unknown) = %v, want 0", c)
	}
	if id := s.providerIDForModel(ctx, "gpt"); id == "" {
		t.Error("providerIDForModel(gpt) should return a provider id")
	}
	if id := s.providerIDForModel(ctx, "unknown"); id != "" {
		t.Errorf("providerIDForModel(unknown) = %q, want empty", id)
	}
}

func TestRecordChatMetric(t *testing.T) {
	s, _ := newCtxTestServer(t)
	// Exercises recordChatMetric → costSplitFromModel + providerIDForModel + CreateMetric.
	s.recordChatMetric(context.Background(), "gpt", "conv1", "proj1",
		llm.ChatResponse{TokensUsed: 3, InputTokens: 1, OutputTokens: 2})
}
