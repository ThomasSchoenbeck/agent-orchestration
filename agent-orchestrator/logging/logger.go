// Package logging provides structured logging and metrics helpers that write
// to both stderr and the orchestrator database.
package logging

import (
	"context"
	"fmt"
	"log"
	"time"

	"agent-orchestrator/db"
)

// Level constants.
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Logger writes structured log entries to the database and to the Go standard
// logger for console output.
type Logger struct {
	db        *db.Database
	agentID   string
	taskID    string
	projectID string
}

// New creates a Logger. Any of agentID/taskID/projectID may be empty.
func New(database *db.Database, agentID, taskID, projectID string) *Logger {
	return &Logger{
		db:        database,
		agentID:   agentID,
		taskID:    taskID,
		projectID: projectID,
	}
}

// WithTask returns a copy of the Logger with the task ID set.
func (l *Logger) WithTask(taskID string) *Logger {
	c := *l
	c.taskID = taskID
	return &c
}

// WithProject returns a copy of the Logger with the project ID set.
func (l *Logger) WithProject(projectID string) *Logger {
	c := *l
	c.projectID = projectID
	return &c
}

func (l *Logger) log(ctx context.Context, level, msg string, meta map[string]interface{}) {
	// Console output.
	prefix := fmt.Sprintf("[%s]", level)
	if l.agentID != "" {
		prefix += fmt.Sprintf("[agent:%s]", l.agentID[:min(8, len(l.agentID))])
	}
	if l.taskID != "" {
		prefix += fmt.Sprintf("[task:%s]", l.taskID[:min(8, len(l.taskID))])
	}
	log.Printf("%s %s", prefix, msg)

	if l.db == nil {
		return
	}

	entry := &db.LogEntry{
		AgentID:   l.agentID,
		TaskID:    l.taskID,
		ProjectID: l.projectID,
		Level:     level,
		Message:   msg,
		Metadata:  meta,
		Timestamp: time.Now().UTC(),
	}
	if err := l.db.CreateLog(ctx, entry); err != nil {
		log.Printf("logger: failed to persist log entry: %v", err)
	}
}

// Debug logs a debug-level message.
func (l *Logger) Debug(ctx context.Context, msg string, meta ...map[string]interface{}) {
	l.log(ctx, LevelDebug, msg, mergedMeta(meta))
}

// Info logs an info-level message.
func (l *Logger) Info(ctx context.Context, msg string, meta ...map[string]interface{}) {
	l.log(ctx, LevelInfo, msg, mergedMeta(meta))
}

// Warn logs a warning-level message.
func (l *Logger) Warn(ctx context.Context, msg string, meta ...map[string]interface{}) {
	l.log(ctx, LevelWarn, msg, mergedMeta(meta))
}

// Error logs an error-level message.
func (l *Logger) Error(ctx context.Context, msg string, meta ...map[string]interface{}) {
	l.log(ctx, LevelError, msg, mergedMeta(meta))
}

func mergedMeta(metas []map[string]interface{}) map[string]interface{} {
	if len(metas) == 0 {
		return nil
	}
	out := make(map[string]interface{})
	for _, m := range metas {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
