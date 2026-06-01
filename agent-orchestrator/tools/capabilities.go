package tools

import "context"

// capCtxKey is the context key under which a caller's role capabilities are
// carried into tool execution.
type capCtxKey struct{}

// WithCapabilities returns a context carrying the caller's role capabilities so
// capability-gated tools (e.g. complete_project requires creates_tasks) can
// authorize the call. When a tool runs without capabilities in context the gate
// is open — callers that want enforcement must scope the context.
func WithCapabilities(ctx context.Context, caps []string) context.Context {
	return context.WithValue(ctx, capCtxKey{}, caps)
}

func capabilitiesFromContext(ctx context.Context) ([]string, bool) {
	v := ctx.Value(capCtxKey{})
	if v == nil {
		return nil, false
	}
	caps, ok := v.([]string)
	return caps, ok
}

// contextHasCapability reports whether the context's capability set grants the
// named capability. If no capability set is present (unscoped call), it returns
// true so existing direct callers are unaffected.
func contextHasCapability(ctx context.Context, capability string) bool {
	caps, ok := capabilitiesFromContext(ctx)
	if !ok {
		return true
	}
	for _, c := range caps {
		if c == capability {
			return true
		}
	}
	return false
}
