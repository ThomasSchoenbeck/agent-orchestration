package agent

import (
	"encoding/json"

	"github.com/google/uuid"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
	"agent-orchestrator/router"
)

// SessionKind identifies the purpose of a session in the multi-session
// orchestration model (2026-07-04). The main session orchestrates a task; the
// other kinds are synchronous child sessions spawned via run_subagent or
// checkpoints, each reporting a summary back to its parent.
type SessionKind string

const (
	SessionKindMain       SessionKind = "main"
	SessionKindDiscovery  SessionKind = "discovery"
	SessionKindWork       SessionKind = "work"
	SessionKindPromptPrep SessionKind = "prompt_prep"
	SessionKindTaskStatus SessionKind = "task_status"
)

// SessionStatus is a session's lifecycle state.
type SessionStatus string

const (
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusDone     SessionStatus = "done"
	SessionStatusFailed   SessionStatus = "failed"
	SessionStatusTimedOut SessionStatus = "timed_out"
)

// Session is one LLM conversation with a purpose within a task. Sessions form a
// tree: a main session synchronously spawns child sessions (discovery, work,
// prompt_prep, task_status) that report a summary back. Session is a data holder
// — the LLM↔tool loop logic lives in the executor/subagent code that runs it.
type Session struct {
	ID       string
	Kind     SessionKind
	ParentID string // "" for a root/main session
	TaskID   string
	AgentID  string
	Title    string

	// Route is the resolved provider/model this session runs against.
	Route router.RouteResult
	// Messages is the running conversation history.
	Messages []llm.Message

	Status SessionStatus
	// Stats accumulates this session's own token usage and cost (children fold
	// their stats into the parent separately, via the SessionManager).
	Stats execStats
	// costBookmark is the running cost already attributed to prior persisted
	// nodes of this main-session chain, so each persisted row records only its
	// own delta cost across a cross-session continuation.
	costBookmark float64
}

// newSession creates a root session (no parent) in the running state.
func newSession(kind SessionKind, taskID, agentID string, route router.RouteResult) *Session {
	return &Session{
		ID:      uuid.NewString(),
		Kind:    kind,
		TaskID:  taskID,
		AgentID: agentID,
		Route:   route,
		Status:  SessionStatusRunning,
		Stats:   execStats{model: route.Model},
	}
}

// newChildSession creates a child of parent, inheriting its task and agent and
// linking back via ParentID. The child may run against a different route (e.g. a
// subagent with its own model priority list).
func newChildSession(parent *Session, kind SessionKind, route router.RouteResult) *Session {
	c := newSession(kind, parent.TaskID, parent.AgentID, route)
	c.ParentID = parent.ID
	return c
}

// markDone / markFailed / markTimedOut transition a running session to a terminal
// state. They are no-ops once the session is already terminal.
func (s *Session) markDone()     { s.setTerminal(SessionStatusDone) }
func (s *Session) markFailed()   { s.setTerminal(SessionStatusFailed) }
func (s *Session) markTimedOut() { s.setTerminal(SessionStatusTimedOut) }

func (s *Session) setTerminal(status SessionStatus) {
	if s.isTerminal() {
		return
	}
	s.Status = status
}

// isTerminal reports whether the session has reached a terminal state.
func (s *Session) isTerminal() bool {
	return s.Status == SessionStatusDone ||
		s.Status == SessionStatusFailed ||
		s.Status == SessionStatusTimedOut
}

// toAgentSession converts the session into a persistable db.AgentSession row,
// carrying the given summary and tool-loop round. Messages are JSON-encoded; an
// encoding failure falls back to an empty history rather than losing the row.
func (s *Session) toAgentSession(summary string, round int) *db.AgentSession {
	raw, err := json.Marshal(s.Messages)
	if err != nil {
		raw = []byte("[]")
	}
	return &db.AgentSession{
		ID:       s.ID,
		TaskID:   s.TaskID,
		AgentID:  s.AgentID,
		Summary:  summary,
		Messages: string(raw),
		Round:    round,
		Kind:     string(s.Kind),
		ParentID: s.ParentID,
		Status:   string(s.Status),
		Title:    s.Title,
		Cost:     s.Stats.cost,
	}
}
