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
		{StatusPlanned, StatusInProgress},
		{StatusInProgress, StatusCompleted},
		{StatusInProgress, StatusFailed},
		{StatusInProgress, StatusNeedsReview},
		{StatusNeedsReview, StatusApproved},
		{StatusNeedsReview, StatusInProgress},
		{StatusApproved, StatusCompleted},
		{StatusFailed, StatusPlanned},
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
		{StatusPlanned, StatusCompleted},
		{StatusPlanned, StatusFailed},
		{StatusPlanned, StatusApproved},
		{StatusPlanned, StatusNeedsReview},
		{StatusCompleted, StatusPlanned},
		{StatusCompleted, StatusInProgress},
		{StatusCompleted, StatusFailed},
		{StatusCompleted, StatusNeedsReview},
		{StatusFailed, StatusCompleted},
		{StatusFailed, StatusInProgress},
		{StatusApproved, StatusPlanned},
		{StatusApproved, StatusInProgress},
		{StatusApproved, StatusFailed},
		{StatusNeedsReview, StatusCompleted},
		{StatusNeedsReview, StatusFailed},
	}
	for _, tc := range invalid {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected invalid transition %s → %s", tc.from, tc.to)
		}
	}
}

func TestIsValidTransition_SameStatus(t *testing.T) {
	statuses := []TaskStatus{
		StatusPlanned, StatusInProgress, StatusNeedsReview,
		StatusApproved, StatusCompleted, StatusFailed,
	}
	for _, s := range statuses {
		if IsValidTransition(s, s) {
			t.Errorf("self-transition %s → %s should be invalid", s, s)
		}
	}
}

// --- ValidateTransition ---

func TestValidateTransition_Valid(t *testing.T) {
	if err := ValidateTransition(StatusPlanned, StatusInProgress); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidateTransition_Invalid_ErrorContainsArrow(t *testing.T) {
	err := ValidateTransition(StatusPlanned, StatusCompleted)
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
		t.Errorf("expected 'completed' in error message, got: %q", msg)
	}
	if !strings.Contains(msg, string(StatusFailed)) {
		t.Errorf("expected 'failed' in error message, got: %q", msg)
	}
}

// --- FollowOnType ---

func TestFollowOnType_ImplementCompleted_ReturnsReview(t *testing.T) {
	typ, ok := FollowOnType(TypeImplement, "completed")
	if !ok {
		t.Fatal("expected follow-on for implement+completed")
	}
	if typ != TypeReview {
		t.Errorf("expected TypeReview, got %q", typ)
	}
}

func TestFollowOnType_ReviewApproved_ReturnsTest(t *testing.T) {
	typ, ok := FollowOnType(TypeReview, "approved")
	if !ok {
		t.Fatal("expected follow-on for review+approved")
	}
	if typ != TypeTest {
		t.Errorf("expected TypeTest, got %q", typ)
	}
}

func TestFollowOnType_ReviewChanges_NoFollowOn(t *testing.T) {
	// "changes" means re-queue existing implement — scheduler handles it, no new task
	_, ok := FollowOnType(TypeReview, "changes")
	if ok {
		t.Error("expected no follow-on task for review+changes")
	}
}

func TestFollowOnType_TestCompleted_NoFollowOn(t *testing.T) {
	_, ok := FollowOnType(TypeTest, "completed")
	if ok {
		t.Error("expected no follow-on for test+completed (workflow done)")
	}
}

func TestFollowOnType_PlanCompleted_NoFollowOn(t *testing.T) {
	_, ok := FollowOnType(TypePlan, "completed")
	if ok {
		t.Error("expected no follow-on for plan+completed")
	}
}

func TestFollowOnType_ImplementFailed_NoFollowOn(t *testing.T) {
	_, ok := FollowOnType(TypeImplement, "failed")
	if ok {
		t.Error("expected no follow-on for implement+failed")
	}
}

func TestFollowOnType_UnknownOutcome_NoFollowOn(t *testing.T) {
	_, ok := FollowOnType(TypeImplement, "unknown_outcome")
	if ok {
		t.Error("expected no follow-on for unknown outcome")
	}
}

// --- RoleForType ---

func TestRoleForType_AllTypes(t *testing.T) {
	cases := []struct {
		taskType TaskType
		want     string
	}{
		{TypePlan, "orchestrator"},
		{TypeImplement, "worker"},
		{TypeReview, "reviewer"},
		{TypeTest, "worker"},
		{"unknown", "worker"},
		{"", "worker"},
	}
	for _, tc := range cases {
		got := RoleForType(tc.taskType)
		if got != tc.want {
			t.Errorf("RoleForType(%q): expected %q, got %q", tc.taskType, tc.want, got)
		}
	}
}
