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
		title = task.Type
	}
	content := fmt.Sprintf("# Task: %s\n\n**ID:** %s\n**Type:** %s\n**Role:** %s\n**Status:** %s\n\n## Description\n\n%s\n",
		title, task.ID, task.Type, task.Role, task.Status, description)
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
