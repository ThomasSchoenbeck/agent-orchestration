package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"agent-orchestrator/db"
)

// AgentLogger provides structured logging for an agent or executor.
// Every call writes a formatted line to stdout via the standard log package,
// and additionally ships info/warn/error entries to the server via PostLog.
type AgentLogger struct {
	agentID   string
	taskID    string
	projectID string
	client    *ServerClient
}

// newLogger creates a logger scoped to an agent. taskID and projectID may be
// empty and are set later with ForTask / ForProject.
func newLogger(agentID string, client *ServerClient) *AgentLogger {
	return &AgentLogger{agentID: agentID, client: client}
}

// ForTask returns a copy of the logger with taskID set.
func (l *AgentLogger) ForTask(taskID string) *AgentLogger {
	c := *l
	c.taskID = taskID
	return &c
}

// ForProject returns a copy of the logger with projectID set.
func (l *AgentLogger) ForProject(projectID string) *AgentLogger {
	c := *l
	c.projectID = projectID
	return &c
}

// Debug writes a debug-level line to stdout only (not shipped to server).
func (l *AgentLogger) Debug(format string, args ...interface{}) {
	l.write("debug", format, args...)
}

// Info writes an info-level line to stdout and ships it to the server.
func (l *AgentLogger) Info(format string, args ...interface{}) {
	l.write("info", format, args...)
	l.post(context.Background(), "info", fmt.Sprintf(format, args...))
}

// Warn writes a warn-level line to stdout and ships it to the server.
func (l *AgentLogger) Warn(format string, args ...interface{}) {
	l.write("warn", format, args...)
	l.post(context.Background(), "warn", fmt.Sprintf(format, args...))
}

// Error writes an error-level line to stdout and ships it to the server.
func (l *AgentLogger) Error(format string, args ...interface{}) {
	l.write("error", format, args...)
	l.post(context.Background(), "error", fmt.Sprintf(format, args...))
}

// InfoCtx is like Info but uses the provided context (useful for cancellation).
func (l *AgentLogger) InfoCtx(ctx context.Context, format string, args ...interface{}) {
	l.write("info", format, args...)
	l.post(ctx, "info", fmt.Sprintf(format, args...))
}

// WarnCtx is like Warn but uses the provided context.
func (l *AgentLogger) WarnCtx(ctx context.Context, format string, args ...interface{}) {
	l.write("warn", format, args...)
	l.post(ctx, "warn", fmt.Sprintf(format, args...))
}

// ErrorCtx is like Error but uses the provided context.
func (l *AgentLogger) ErrorCtx(ctx context.Context, format string, args ...interface{}) {
	l.write("error", format, args...)
	l.post(ctx, "error", fmt.Sprintf(format, args...))
}

func (l *AgentLogger) write(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	prefix := ""
	if l.agentID != "" {
		prefix += "agent=" + l.agentID + " "
	}
	if l.taskID != "" {
		prefix += "task=" + l.taskID + " "
	}
	log.Printf("[%s] %s%s", level, prefix, msg)
}

// LogWithMeta ships a log entry at the given level with structured metadata
// attached. Use this for LLM prompt/response events where the full content
// should be retrievable from the server (not just the summary message).
func (l *AgentLogger) LogWithMeta(ctx context.Context, level, msg string, meta map[string]interface{}) {
	l.write(level, "%s", msg)
	if l.client == nil {
		return
	}
	entry := db.LogEntry{
		AgentID:   l.agentID,
		TaskID:    l.taskID,
		ProjectID: l.projectID,
		Level:     level,
		Message:   msg,
		Metadata:  meta,
		Timestamp: time.Now(),
	}
	if err := l.client.PostLog(ctx, entry); err != nil {
		log.Printf("[warn] AgentLogger.LogWithMeta failed: %v", err)
	}
}

func (l *AgentLogger) post(ctx context.Context, level, msg string) {
	if l.client == nil {
		return
	}
	entry := db.LogEntry{
		AgentID:   l.agentID,
		TaskID:    l.taskID,
		ProjectID: l.projectID,
		Level:     level,
		Message:   msg,
		Timestamp: time.Now(),
	}
	// Fire-and-forget; log failures to stdout but never block the caller.
	if err := l.client.PostLog(ctx, entry); err != nil {
		log.Printf("[warn] AgentLogger.post failed: %v", err)
	}
}
