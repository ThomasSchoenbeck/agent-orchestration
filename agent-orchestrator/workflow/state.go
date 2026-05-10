// Package workflow implements the task lifecycle state machine and scheduler.
package workflow

import "fmt"

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	StatusPlanned     TaskStatus = "planned"
	StatusInProgress  TaskStatus = "in_progress"
	StatusNeedsReview TaskStatus = "needs_review"
	StatusApproved    TaskStatus = "approved"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
)

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
	{StatusPlanned, StatusInProgress},
	{StatusInProgress, StatusCompleted},
	{StatusInProgress, StatusFailed},
	{StatusInProgress, StatusNeedsReview},
	{StatusNeedsReview, StatusApproved},
	{StatusNeedsReview, StatusInProgress}, // re-queued after changes requested
	{StatusApproved, StatusCompleted},
	{StatusFailed, StatusPlanned}, // retry
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
//
//	implement + completed  → review
//	review    + approved   → test
//	review    + changes    → (re-queue existing implement, no new task)
//	test      + completed  → (workflow done, no follow-on)
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
		// "changes" means re-queue the implement task — handled by scheduler, not here.
	}
	return "", false
}

// RoleForType returns the default role for a given task type.
// These can be overridden by config.routing.
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
