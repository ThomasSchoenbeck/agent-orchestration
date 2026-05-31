package workflow

import (
	"strings"
	"testing"
)

// --- IsValidTransition ---

func TestIsValidTransition_AllValid(t *testing.T) {
	valid := []struct {
		from TaskStatus
		to   TaskStatus
	}{
		// Dev path
		{StatusBacklog, StatusDeveloping},
		{StatusAwaitingRevision, StatusDeveloping},
		{StatusDeveloping, StatusAwaitingReview},
		{StatusDeveloping, StatusFailed},
		// Review path
		{StatusAwaitingReview, StatusReviewing},
		{StatusReviewing, StatusAwaitingMerge},
		{StatusReviewing, StatusAwaitingRevision},
		{StatusReviewing, StatusFailed},
		// Merge path
		{StatusAwaitingMerge, StatusMerging},
		{StatusMerging, StatusCompleted},
		{StatusMerging, StatusAwaitingRevision},
		{StatusMerging, StatusFailed},
		// Retry
		{StatusFailed, StatusBacklog},
	}
	for _, tc := range valid {
		if !IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected valid transition %s → %s", tc.from, tc.to)
		}
	}
}

func TestIsValidTransition_Invalid(t *testing.T) {
	invalid := []struct {
		from TaskStatus
		to   TaskStatus
	}{
		{StatusBacklog, StatusCompleted},
		{StatusBacklog, StatusFailed},
		{StatusCompleted, StatusBacklog},
		{StatusCompleted, StatusDeveloping},
		{StatusCompleted, StatusFailed},
		{StatusFailed, StatusCompleted},
		{StatusFailed, StatusDeveloping},
		{StatusDeveloping, StatusCompleted},
		{StatusDeveloping, StatusMerging},
	}
	for _, tc := range invalid {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected invalid transition %s → %s", tc.from, tc.to)
		}
	}
}

func TestIsValidTransition_SameStatus(t *testing.T) {
	statuses := []TaskStatus{
		StatusBacklog, StatusDeveloping, StatusAwaitingReview,
		StatusReviewing, StatusAwaitingRevision, StatusAwaitingMerge,
		StatusMerging, StatusCompleted, StatusFailed,
	}
	for _, s := range statuses {
		if IsValidTransition(s, s) {
			t.Errorf("self-transition %s → %s should be invalid", s, s)
		}
	}
}

// --- ValidateTransition ---

func TestValidateTransition_Valid(t *testing.T) {
	if err := ValidateTransition(StatusBacklog, StatusDeveloping); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidateTransition_Invalid_ErrorContainsArrow(t *testing.T) {
	err := ValidateTransition(StatusBacklog, StatusCompleted)
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}
	if !strings.Contains(err.Error(), "→") {
		t.Errorf("expected error to contain arrow, got: %q", err.Error())
	}
}

func TestValidateTransition_Invalid_MentionsStatuses(t *testing.T) {
	err := ValidateTransition(StatusCompleted, StatusFailed)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, string(StatusCompleted)) {
		t.Errorf("expected %q in error, got: %q", StatusCompleted, msg)
	}
	if !strings.Contains(msg, string(StatusFailed)) {
		t.Errorf("expected %q in error, got: %q", StatusFailed, msg)
	}
}

// --- IsQueueState / IsExecutionState / IsTerminalState ---

func TestIsQueueState(t *testing.T) {
	queue := []TaskStatus{StatusBacklog, StatusAwaitingReview, StatusAwaitingRevision, StatusAwaitingMerge}
	for _, s := range queue {
		if !IsQueueState(s) {
			t.Errorf("expected %s to be a queue state", s)
		}
	}
	notQueue := []TaskStatus{StatusDeveloping, StatusReviewing, StatusMerging, StatusCompleted, StatusFailed}
	for _, s := range notQueue {
		if IsQueueState(s) {
			t.Errorf("expected %s NOT to be a queue state", s)
		}
	}
}

func TestIsExecutionState(t *testing.T) {
	exec := []TaskStatus{StatusDeveloping, StatusReviewing, StatusMerging}
	for _, s := range exec {
		if !IsExecutionState(s) {
			t.Errorf("expected %s to be an execution state", s)
		}
	}
	notExec := []TaskStatus{StatusBacklog, StatusAwaitingReview, StatusAwaitingRevision, StatusAwaitingMerge, StatusCompleted, StatusFailed}
	for _, s := range notExec {
		if IsExecutionState(s) {
			t.Errorf("expected %s NOT to be an execution state", s)
		}
	}
}

func TestIsTerminalState(t *testing.T) {
	if !IsTerminalState(StatusCompleted) {
		t.Error("COMPLETED should be terminal")
	}
	if !IsTerminalState(StatusFailed) {
		t.Error("FAILED should be terminal")
	}
	for _, s := range []TaskStatus{StatusBacklog, StatusDeveloping, StatusReviewing} {
		if IsTerminalState(s) {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

