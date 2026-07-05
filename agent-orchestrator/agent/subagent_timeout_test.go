package agent

import (
	"context"
	"testing"
	"time"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

// blockingProvider returns one scripted response, then blocks until the context
// is cancelled — used to drive a subagent into its timeout after it has already
// accumulated some token/cost stats.
type blockingProvider struct {
	calls int
	first llm.ChatResponse
}

func (b *blockingProvider) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	b.calls++
	if b.calls == 1 {
		return b.first, nil
	}
	<-ctx.Done()
	return llm.ChatResponse{}, ctx.Err()
}
func (b *blockingProvider) Embed(_ context.Context, _ llm.EmbedRequest) (llm.EmbedResponse, error) {
	return llm.EmbedResponse{}, nil
}
func (b *blockingProvider) Rerank(_ context.Context, _ llm.RerankRequest) (llm.RerankResponse, error) {
	return llm.RerankResponse{}, nil
}
func (b *blockingProvider) Name() string { return "blocking" }
func (b *blockingProvider) Close() error { return nil }

// TestRunSubagent_TimeoutPreservesPartialStats verifies the T3.3 fix: when a
// subagent times out, the token/cost accumulated before the deadline is folded
// back (not discarded), and the read is race-free (see -race).
func TestRunSubagent_TimeoutPreservesPartialStats(t *testing.T) {
	var readCalls int
	reg := subagentTestRegistry(t, &readCalls)
	prov := &blockingProvider{first: llm.ChatResponse{
		ToolCalls:   []llm.ToolCall{{ID: "c1", Name: "read_file", Arguments: map[string]interface{}{"file_path": "a.go"}}},
		StopReason:  "tool_use",
		InputTokens: 50, OutputTokens: 10, TokensUsed: 60,
	}}
	e := NewExecutor(nil, reg, nil, "a1")
	e.subagentTimeoutOverride = 50 * time.Millisecond
	route := router.RouteResult{Provider: prov, Model: "m", Role: "worker"}
	skill := &db.SubagentSkill{Name: "investigate_codebase", Enabled: true, ToolAllowlist: []string{"read_file"}, MaxRounds: 5}

	_, stats, err := e.runSubagent(context.Background(), e.log.ForTask("t1"), route, "/work/t1", skill, "go")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if stats.inputTokens != 50 || stats.outputTokens != 10 {
		t.Errorf("expected partial stats preserved (in=50 out=10), got in=%d out=%d", stats.inputTokens, stats.outputTokens)
	}
}
