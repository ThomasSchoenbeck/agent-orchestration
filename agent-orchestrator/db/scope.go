package db

import (
	"context"
	"fmt"
)

// Scope status vocab (Feature 5).
const (
	ReqStatusProposed    = "proposed"
	ReqStatusAccepted    = "accepted"
	ReqStatusSatisfied   = "satisfied"
	FeatStatusPlanned    = "planned"
	FeatStatusInProgress = "in_progress"
	FeatStatusDone       = "done"
	ScopeStatusNeedsReview = "needs_review"
)

// RecomputeLinkedScopeStatus refreshes the status of every requirement/feature
// linked to the given task, deriving it from the completion state of all tasks
// linked to that item:
//
//   - at least one linked task and all COMPLETED → satisfied / done
//   - some linked tasks still open                → accepted / in_progress
//
// A needs_review status is never auto-overwritten (a human must clear it).
// Best-effort: errors are returned but callers typically log and continue.
func (d *Database) RecomputeLinkedScopeStatus(ctx context.Context, taskID string) error {
	links, err := d.ListTaskLinks(ctx, taskID)
	if err != nil {
		return err
	}
	for _, l := range links {
		statuses, err := d.linkedTaskStatuses(ctx, l.Kind, l.TargetID)
		if err != nil {
			return err
		}
		allComplete := len(statuses) > 0
		for _, s := range statuses {
			if s != TaskStatusCompleted {
				allComplete = false
				break
			}
		}
		switch l.Kind {
		case "requirement":
			r, err := d.GetRequirement(ctx, l.TargetID)
			if err != nil {
				continue
			}
			if r.Status == ScopeStatusNeedsReview {
				continue
			}
			next := ReqStatusAccepted
			if allComplete {
				next = ReqStatusSatisfied
			}
			if next != r.Status {
				r.Status = next
				if err := d.UpdateRequirement(ctx, r); err != nil {
					return err
				}
			}
		case "feature":
			f, err := d.GetFeature(ctx, l.TargetID)
			if err != nil {
				continue
			}
			if f.Status == ScopeStatusNeedsReview {
				continue
			}
			next := FeatStatusInProgress
			if allComplete {
				next = FeatStatusDone
			}
			if next != f.Status {
				f.Status = next
				if err := d.UpdateFeature(ctx, f); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// linkedTaskStatuses returns the status of every task linked to a requirement
// or feature.
func (d *Database) linkedTaskStatuses(ctx context.Context, kind, targetID string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT t.status FROM task_project_links tpl
		 JOIN tasks t ON t.id = tpl.task_id
		 WHERE tpl.kind=? AND tpl.target_id=?`,
		kind, targetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ProjectScopeSatisfied reports whether a project's declared scope is met:
// every feature is done, every requirement is satisfied, and no task is in a
// non-terminal state. When not satisfied it returns a human-readable reason.
func (d *Database) ProjectScopeSatisfied(ctx context.Context, projectID string) (bool, string, error) {
	reqs, err := d.ListRequirements(ctx, projectID)
	if err != nil {
		return false, "", err
	}
	for _, r := range reqs {
		if r.Status != ReqStatusSatisfied {
			return false, fmt.Sprintf("requirement %q is %q (want satisfied)", r.Title, r.Status), nil
		}
	}
	feats, err := d.ListFeatures(ctx, projectID)
	if err != nil {
		return false, "", err
	}
	for _, f := range feats {
		if f.Status != FeatStatusDone {
			return false, fmt.Sprintf("feature %q is %q (want done)", f.Title, f.Status), nil
		}
	}

	var openTasks int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE project_id=? AND status NOT IN (?, ?)`,
		projectID, TaskStatusCompleted, TaskStatusFailed,
	).Scan(&openTasks); err != nil {
		return false, "", err
	}
	if openTasks > 0 {
		return false, fmt.Sprintf("%d task(s) still in a non-terminal state", openTasks), nil
	}
	return true, "", nil
}
