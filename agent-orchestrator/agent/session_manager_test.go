package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"agent-orchestrator/router"
)

func TestSessionManager_RunChild_FoldsStatsAndLinks(t *testing.T) {
	m := newSessionManager("task-1")
	parent := newSession(SessionKindMain, "task-1", "agent-1", router.RouteResult{})
	m.register(parent)

	run := func(kind SessionKind, tokens int, cost float64, summary string) {
		child := newChildSession(parent, kind, router.RouteResult{})
		out, err := m.RunChild(context.Background(), parent, child, time.Second,
			func(_ context.Context, s *Session) (string, error) {
				s.Stats.totalTokens += tokens
				s.Stats.inputTokens += tokens
				s.Stats.cost += cost
				return summary, nil
			})
		if err != nil {
			t.Fatalf("RunChild(%s): %v", kind, err)
		}
		if out != summary {
			t.Errorf("summary = %q, want %q", out, summary)
		}
		if child.Status != SessionStatusDone {
			t.Errorf("child status = %q, want done", child.Status)
		}
	}

	run(SessionKindDiscovery, 100, 0.10, "found things")
	run(SessionKindWork, 250, 0.25, "changed code")

	// Two children folded into the parent's stats.
	if parent.Stats.totalTokens != 350 || parent.Stats.inputTokens != 350 {
		t.Errorf("parent tokens = %d, want 350", parent.Stats.totalTokens)
	}
	if parent.Stats.cost != 0.35 {
		t.Errorf("parent cost = %v, want 0.35", parent.Stats.cost)
	}

	// Parent + 2 children registered; both children linked to the parent.
	if got := len(m.Sessions()); got != 3 {
		t.Fatalf("expected 3 sessions, got %d", got)
	}
	if got := len(m.childrenOf(parent)); got != 2 {
		t.Errorf("expected 2 children of parent, got %d", got)
	}
}

func TestSessionManager_RunChild_ErrorPropagates(t *testing.T) {
	m := newSessionManager("t")
	parent := newSession(SessionKindMain, "t", "a", router.RouteResult{})
	child := newChildSession(parent, SessionKindWork, router.RouteResult{})

	wantErr := errors.New("boom")
	_, err := m.RunChild(context.Background(), parent, child, time.Second,
		func(_ context.Context, s *Session) (string, error) {
			s.Stats.cost += 0.01
			return "", wantErr
		})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected boom, got %v", err)
	}
	if child.Status != SessionStatusFailed {
		t.Errorf("child status = %q, want failed", child.Status)
	}
	// Partial stats still fold in on error.
	if parent.Stats.cost != 0.01 {
		t.Errorf("parent cost = %v, want 0.01", parent.Stats.cost)
	}
}

func TestSessionManager_RunChild_TimeoutFoldsPartialStats(t *testing.T) {
	m := newSessionManager("t")
	parent := newSession(SessionKindMain, "t", "a", router.RouteResult{})
	child := newChildSession(parent, SessionKindDiscovery, router.RouteResult{})

	_, err := m.RunChild(context.Background(), parent, child, 20*time.Millisecond,
		func(ctx context.Context, s *Session) (string, error) {
			s.Stats.totalTokens += 42 // work done before the deadline
			<-ctx.Done()              // cooperatively stop on cancellation
			return "partial", ctx.Err()
		})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if child.Status != SessionStatusTimedOut {
		t.Errorf("child status = %q, want timed_out", child.Status)
	}
	// Partial accounting is preserved (not zeroed), and folded race-free.
	if parent.Stats.totalTokens != 42 {
		t.Errorf("parent tokens = %d, want 42 (partial)", parent.Stats.totalTokens)
	}
}
