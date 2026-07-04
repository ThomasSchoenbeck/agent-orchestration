package server

import (
	"context"
	"testing"
)

// resolveProvider error branches: an empty registry/router has no provider for
// any role, so both role-based and explicit-id lookups fail.
func TestResolveProvider_Errors(t *testing.T) {
	s, _ := newCtxTestServer(t)
	ctx := context.Background()

	if _, _, err := s.resolveProvider(ctx, "worker", ""); err == nil {
		t.Error("expected error resolving provider with no providers registered")
	}
	// Empty role falls back to "orchestrator", which also has no provider.
	if _, _, err := s.resolveProvider(ctx, "", ""); err == nil {
		t.Error("expected error resolving provider for default role")
	}
	// Unknown explicit provider id falls through to role routing, then errors.
	if _, _, err := s.resolveProvider(ctx, "worker", "missing-id"); err == nil {
		t.Error("expected error resolving unknown provider id")
	}
}
