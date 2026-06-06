package tools

import (
	"context"
	"fmt"
)

// RegisterCommentTools registers the task_comment tool.
func RegisterCommentTools(reg *Registry, backend ToolBackend) error {
	return reg.Register(taskCommentTool(backend))
}

func taskCommentTool(backend ToolBackend) Definition {
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
			commentID, err := backend.PostTaskComment(ctx, taskID, body,
				strArgOpt(args, "role"), strArgOpt(args, "review_id"))
			if err != nil {
				return nil, fmt.Errorf("failed to post comment: %w", err)
			}
			return map[string]string{"comment_id": commentID, "status": "posted"}, nil
		},
	}
}
