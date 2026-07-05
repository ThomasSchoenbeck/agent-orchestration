package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// sessionLoop is the body of a session: it drives the LLM↔tool loop for s,
// updates s.Stats as it goes, and returns a summary (the child's report back to
// its parent). The manager owns lifecycle/timeout/stat-folding around it.
type sessionLoop func(ctx context.Context, s *Session) (summary string, err error)

// SessionManager owns all sessions for a single task and runs child sessions
// synchronously on behalf of a parent. In the multi-session model the parent is
// always blocked inside RunChild while its child runs, so a child never mutates
// the parent's stats concurrently — folding is therefore safe under the manager
// mutex without further coordination.
type SessionManager struct {
	mu       sync.Mutex
	taskID   string
	sessions []*Session // every session for the task, in registration order
}

// newSessionManager creates an empty manager for a task.
func newSessionManager(taskID string) *SessionManager {
	return &SessionManager{taskID: taskID}
}

// register records a session (idempotent by pointer identity).
func (m *SessionManager) register(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.sessions {
		if existing == s {
			return
		}
	}
	m.sessions = append(m.sessions, s)
}

// Sessions returns a snapshot of the task's sessions in registration order.
func (m *SessionManager) Sessions() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, len(m.sessions))
	copy(out, m.sessions)
	return out
}

// childrenOf returns the sessions whose ParentID is parent.ID.
func (m *SessionManager) childrenOf(parent *Session) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Session
	for _, s := range m.sessions {
		if s.ParentID == parent.ID {
			out = append(out, s)
		}
	}
	return out
}

// RunChild runs child's loop synchronously, bounded by timeout, and returns the
// child's summary. It registers the child, marks its terminal status, and folds
// the child's token/cost stats into parent. Stats are folded only after the loop
// goroutine has returned (a happens-before guaranteed by the done channel), so
// there is no data race on the child's stats — even on timeout, where we wait for
// the cancelled loop to unwind before reading them.
func (m *SessionManager) RunChild(
	ctx context.Context, parent, child *Session, timeout time.Duration, loop sessionLoop,
) (string, error) {
	m.register(child)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		summary string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		summary, err := loop(runCtx, child)
		done <- result{summary, err}
	}()

	select {
	case <-runCtx.Done():
		// Cancelled/timed out: wait for the loop to unwind so its stats are safe to
		// read, then fold. Loops honour ctx cancellation each round, so this returns
		// promptly.
		r := <-done
		child.markTimedOut()
		m.foldStats(parent, child)
		if r.err != nil {
			return r.summary, r.err
		}
		return r.summary, fmt.Errorf("session %s (%s) timed out after %s", child.ID, child.Kind, timeout)
	case r := <-done:
		if r.err != nil {
			child.markFailed()
		} else {
			child.markDone()
		}
		m.foldStats(parent, child)
		return r.summary, r.err
	}
}

// foldStats adds child's token/cost totals into parent under the manager mutex.
// Safe because the caller (RunChild) only folds after the child loop has
// returned, and the parent is blocked awaiting this call.
func (m *SessionManager) foldStats(parent, child *Session) {
	if parent == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	parent.Stats.totalTokens += child.Stats.totalTokens
	parent.Stats.inputTokens += child.Stats.inputTokens
	parent.Stats.outputTokens += child.Stats.outputTokens
	parent.Stats.cost += child.Stats.cost
}
