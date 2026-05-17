package tools

import (
	"context"
	"fmt"

	"agent-orchestrator/db"
)

// RegisterCommentTools registers the task_comment tool.
func RegisterCommentTools(reg *Registry, database *db.Database) error {
	return reg.Register(taskCommentTool(database))
}

func taskCommentTool(database *db.Database) Definition {
	return Definition{
		Name:        "task_comment",
		Description: "Post a comment on a task. Use this to ask questions, report errors, request clarification, or reply to a review thread.",
		Parameters: map[string]Param{
			"task_id":   {Type: "string", Description: "ID of the task to comment on"},
			"body":      {Type: "string", Description: "Markdown-formatted comment body"},
			"role":      {Type: "string", Description: "Agent role posting this comment (e.g. worker, reviewer). Optional."},
			"review_id": {Type: "string", Description: "Reply under a review thread. Omit for top-level comments. Optional."},
		},
		Required: []string{"task_id", "body"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			taskID, err := strArg(args, "task_id")
			if err != nil {
				return nil, err
			}
			body, err := strArg(args, "body")
			if err != nil {
				return nil, err
			}
			if body == "" {
				return nil, fmt.Errorf("body must not be empty")
			}
			comment := &db.TaskComment{
				TaskID:     taskID,
				AuthorType: "agent",
				AuthorRole: strArgOpt(args, "role"),
				ReviewID:   strArgOpt(args, "review_id"),
				Body:       body,
			}
			if err := database.CreateComment(ctx, comment); err != nil {
				return nil, fmt.Errorf("failed to post comment: %w", err)
			}
			return map[string]string{"comment_id": comment.ID, "status": "posted"}, nil
		},
	}
}
