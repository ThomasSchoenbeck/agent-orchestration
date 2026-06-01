package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-orchestrator/db"
)

// writeAgentContext populates the .agent_context/ directory inside a worktree
// with files that give the agent task-specific context:
//
//   - task.md          — task title, description, role, type
//   - project_rules.md — project description + coding_rules (always written)
//   - last_review.md   — most recent review body (only when AWAITING_REVISION)
//   - review_thread.md — chronological comment thread for that review (if any)
func writeAgentContext(ctx context.Context, database *db.Database, task *db.Task, worktreePath string) error {
	ctxDir := filepath.Join(worktreePath, ".agent_context")
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		return fmt.Errorf("mkdir .agent_context: %w", err)
	}

	// task.md
	if err := writeTaskMD(ctxDir, task); err != nil {
		return err
	}

	// project_rules.md
	if err := writeProjectRulesMD(ctx, database, ctxDir, task.ProjectID); err != nil {
		return err
	}

	// scope.md — project requirements/features, but only for roles whose context
	// rules permit project-level scope (Feature 5). Best-effort.
	if err := writeScopeContext(ctx, database, ctxDir, task); err != nil {
		return err
	}

	// last_review.md + review_thread.md — only for revision tasks
	if task.Status == db.TaskStatusDeveloping || task.Status == db.TaskStatusAwaitingRevision {
		if err := writeReviewContext(ctx, database, ctxDir, task.ID); err != nil {
			return err
		}
	}

	// Ensure .agent_context/ is gitignored so provisioning files are never
	// committed as agent work by CommitAndPush.
	gitignorePath := filepath.Join(worktreePath, ".gitignore")
	existing, _ := os.ReadFile(gitignorePath)
	if !strings.Contains(string(existing), ".agent_context/") {
		f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
				_, _ = f.WriteString("\n")
			}
			_, _ = f.WriteString(".agent_context/\n")
			_ = f.Close()
		}
	}

	return nil
}

func writeTaskMD(ctxDir string, task *db.Task) error {
	title, _ := task.Payload["title"].(string)
	description, _ := task.Payload["description"].(string)
	if title == "" {
		title = task.Role
	}
	content := fmt.Sprintf("# Task: %s\n\n**ID:** %s\n**Role:** %s\n**Status:** %s\n\n## Description\n\n%s\n",
		title, task.ID, task.Role, task.Status, description)
	return os.WriteFile(filepath.Join(ctxDir, "task.md"), []byte(content), 0o644)
}

func writeProjectRulesMD(ctx context.Context, database *db.Database, ctxDir, projectID string) error {
	if projectID == "" {
		return nil
	}
	p, err := database.GetProject(ctx, projectID)
	if err != nil {
		return nil // non-fatal: project may not exist yet
	}

	content := fmt.Sprintf("# Project: %s\n\n%s\n", p.Name, p.Description)
	if p.CodingRules != "" {
		content += "\n## Coding Rules\n\n" + p.CodingRules + "\n"
	}
	return os.WriteFile(filepath.Join(ctxDir, "project_rules.md"), []byte(content), 0o644)
}

// writeScopeContext writes scope.md (project requirements + features) into the
// agent context, but only when the task's role is permitted to see project-level
// scope via its context_include / context_exclude rules. Implementation roles
// (worker/reviewer/deployer) exclude the scope types and so get no scope.md.
func writeScopeContext(ctx context.Context, database *db.Database, ctxDir string, task *db.Task) error {
	if task.ProjectID == "" {
		return nil
	}
	role, err := database.GetRoleDefinitionByName(ctx, task.Role)
	if err != nil {
		role = nil // unknown role → default allow
	}
	seesReqs := roleAllowsContextType(role, "project_requirements")
	seesFeats := roleAllowsContextType(role, "project_features")
	if !seesReqs && !seesFeats {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Project Scope\n")
	if seesReqs {
		reqs, _ := database.ListRequirements(ctx, task.ProjectID)
		sb.WriteString("\n## Requirements\n\n")
		if len(reqs) == 0 {
			sb.WriteString("_None defined yet._\n")
		}
		for _, r := range reqs {
			sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", r.Title, r.Status))
			if r.Body != "" {
				sb.WriteString("  " + r.Body + "\n")
			}
		}
	}
	if seesFeats {
		feats, _ := database.ListFeatures(ctx, task.ProjectID)
		sb.WriteString("\n## Features\n\n")
		if len(feats) == 0 {
			sb.WriteString("_None defined yet._\n")
		}
		for _, f := range feats {
			sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", f.Title, f.Status))
			if f.Body != "" {
				sb.WriteString("  " + f.Body + "\n")
			}
		}
	}
	return os.WriteFile(filepath.Join(ctxDir, "scope.md"), []byte(sb.String()), 0o644)
}

// roleAllowsContextType mirrors router.BuildWithRules filtering for a single
// context type: included unless include-list excludes it or exclude-list lists it.
func roleAllowsContextType(role *db.RoleDefinition, typ string) bool {
	if role == nil {
		return true
	}
	if len(role.ContextInclude) > 0 && !containsString(role.ContextInclude, typ) {
		return false
	}
	return !containsString(role.ContextExclude, typ)
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func writeReviewContext(ctx context.Context, database *db.Database, ctxDir, taskID string) error {
	reviews, err := database.ListTaskReviews(ctx, taskID)
	if err != nil || len(reviews) == 0 {
		return nil
	}

	// Most recent review.
	latest := reviews[len(reviews)-1]
	reviewContent := fmt.Sprintf("# Code Review\n\n**Status:** %s\n**Author:** %s\n\n%s\n",
		latest.Status, latest.AuthorRole, latest.Body)
	if err := os.WriteFile(filepath.Join(ctxDir, "last_review.md"), []byte(reviewContent), 0o644); err != nil {
		return err
	}

	// Review thread (comments tied to this review).
	comments, err := database.ListTaskComments(ctx, taskID)
	if err != nil || len(comments) == 0 {
		return nil
	}
	var thread string
	for _, c := range comments {
		if c.ReviewID == latest.ID {
			thread += fmt.Sprintf("**%s** (%s): %s\n\n", c.AuthorRole, c.CreatedAt.Format("2006-01-02 15:04"), c.Body)
		}
	}
	if thread != "" {
		return os.WriteFile(filepath.Join(ctxDir, "review_thread.md"), []byte("# Review Thread\n\n"+thread), 0o644)
	}
	return nil
}
