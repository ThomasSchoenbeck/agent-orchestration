// Package workflow implements the task lifecycle state machine and scheduler.
package workflow

import "fmt"

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	// Queue states — task is waiting for an agent.
	StatusBacklog          TaskStatus = "BACKLOG"
	StatusAwaitingReview   TaskStatus = "AWAITING_REVIEW"
	StatusAwaitingRevision TaskStatus = "AWAITING_REVISION"
	StatusAwaitingMerge    TaskStatus = "AWAITING_MERGE"

	// Execution states — an agent holds the task.
	StatusDeveloping TaskStatus = "DEVELOPING"
	StatusReviewing  TaskStatus = "REVIEWING"
	StatusMerging    TaskStatus = "MERGING"

	// Terminal states.
	StatusCompleted TaskStatus = "COMPLETED"
	StatusFailed    TaskStatus = "FAILED"
)

// IsQueueState reports whether s is a state where the task is waiting for an agent.
func IsQueueState(s TaskStatus) bool {
	switch s {
	case StatusBacklog, StatusAwaitingReview, StatusAwaitingRevision, StatusAwaitingMerge:
		return true
	}
	return false
}

// IsExecutionState reports whether s is a state where an agent is actively working.
func IsExecutionState(s TaskStatus) bool {
	switch s {
	case StatusDeveloping, StatusReviewing, StatusMerging:
		return true
	}
	return false
}

// IsTerminalState reports whether s is a terminal state (no further transitions).
func IsTerminalState(s TaskStatus) bool {
	return s == StatusCompleted || s == StatusFailed
}

// TaskType identifies the kind of work a task represents.
type TaskType string

const (
	TypePlan      TaskType = "plan"
	TypeImplement TaskType = "implement"
	TypeReview    TaskType = "review"
	TypeTest      TaskType = "test"
)

// Transition describes a valid state change.
type Transition struct {
	From TaskStatus
	To   TaskStatus
}

// validTransitions is the set of allowed state changes.
var validTransitions = []Transition{
	// Dev path
	{StatusBacklog, StatusDeveloping},
	{StatusAwaitingRevision, StatusDeveloping},
	{StatusDeveloping, StatusAwaitingReview},
	{StatusDeveloping, StatusFailed},

	// Review path
	{StatusAwaitingReview, StatusReviewing},
	{StatusReviewing, StatusAwaitingMerge},    // approved
	{StatusReviewing, StatusAwaitingRevision}, // revision requested
	{StatusReviewing, StatusFailed},

	// Merge path
	{StatusAwaitingMerge, StatusMerging},
	{StatusMerging, StatusCompleted},
	{StatusMerging, StatusAwaitingRevision}, // merge conflict → back to dev
	{StatusMerging, StatusFailed},

	// Retry
	{StatusFailed, StatusBacklog},
}

// IsValidTransition returns true when moving from → to is allowed.
func IsValidTransition(from, to TaskStatus) bool {
	for _, t := range validTransitions {
		if t.From == from && t.To == to {
			return true
		}
	}
	return false
}

// ValidateTransition returns an error if the transition is not allowed.
func ValidateTransition(from, to TaskStatus) error {
	if IsValidTransition(from, to) {
		return nil
	}
	return fmt.Errorf("invalid task state transition: %s → %s", from, to)
}

// FollowOnType returns the task type that should be created when a task of
// type src completes with the given result outcome. Returns ("", false) when
// no follow-on task is needed.
func FollowOnType(src TaskType, outcome string) (TaskType, bool) {
	switch src {
	case TypeImplement:
		if outcome == "completed" {
			return TypeReview, true
		}
	case TypeReview:
		if outcome == "approved" {
			return TypeTest, true
		}
	}
	return "", false
}

// RoleForType returns the default role for a given task type.
func RoleForType(t TaskType) string {
	switch t {
	case TypePlan:
		return "orchestrator"
	case TypeImplement:
		return "worker"
	case TypeReview:
		return "reviewer"
	case TypeTest:
		return "worker"
	default:
		return "worker"
	}
}
