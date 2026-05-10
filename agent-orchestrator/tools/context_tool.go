package tools

import (
	"context"
	"fmt"

	"agent-orchestrator/db"
)

// RegisterContextTools registers context-management tools into reg.
func RegisterContextTools(reg *Registry, database *db.Database) error {
	defs := []Definition{
		saveContextTool(database),
		queryContextTool(database),
	}
	for _, d := range defs {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}

func saveContextTool(database *db.Database) Definition {
	return Definition{
		Name:        "save_context",
		Description: "Save a piece of context (summary, snippet, note, diff, test results) associated with a project or task.",
		Parameters: map[string]Param{
			"project_id": {Type: "string", Description: "Project ID to associate this context with"},
			"task_id":    {Type: "string", Description: "Task ID to associate this context with (optional)"},
			"type":       {Type: "string", Description: "Context type: summary|snippet|note|diff|test_results"},
			"content":    {Type: "string", Description: "The context content to save"},
		},
		Required: []string{"project_id", "type", "content"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			ctxType, err := strArg(args, "type")
			if err != nil {
				return nil, err
			}
			content, err := strArg(args, "content")
			if err != nil {
				return nil, err
			}

			entry := &db.ContextEntry{
				ProjectID: projectID,
				TaskID:    strArgOpt(args, "task_id"),
				Type:      ctxType,
				Content:   content,
			}
			if err := database.CreateContextEntry(ctx, entry); err != nil {
				return nil, fmt.Errorf("save_context: %w", err)
			}
			return map[string]interface{}{
				"success":    true,
				"context_id": entry.ID,
			}, nil
		},
	}
}

func queryContextTool(database *db.Database) Definition {
	return Definition{
		Name:        "query_context",
		Description: "Search stored context for a project by keyword. Returns relevant context entries.",
		Parameters: map[string]Param{
			"project_id": {Type: "string", Description: "Project ID to query context for"},
			"query":      {Type: "string", Description: "Keyword or phrase to search for"},
			"limit":      {Type: "number", Description: "Maximum number of results to return (default 10)"},
		},
		Required: []string{"project_id", "query"},
		Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			projectID, err := strArg(args, "project_id")
			if err != nil {
				return nil, err
			}
			query, err := strArg(args, "query")
			if err != nil {
				return nil, err
			}
			limit := intArgOpt(args, "limit", 10)

			entries, err := database.QueryContext(ctx, projectID, query, limit)
			if err != nil {
				return nil, fmt.Errorf("query_context: %w", err)
			}
			return map[string]interface{}{
				"entries": entries,
				"count":   len(entries),
			}, nil
		},
	}
}
