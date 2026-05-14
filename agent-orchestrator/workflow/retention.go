// Package workflow contains background jobs for the orchestrator.
package workflow

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"agent-orchestrator/db"
)

// RetentionJob runs the two-pass log cleanup on a configurable interval.
type RetentionJob struct {
	database    *db.Database
	intervalMin int
}

// NewRetentionJob creates a RetentionJob. intervalMin is the cleanup frequency.
func NewRetentionJob(database *db.Database, intervalMin int) *RetentionJob {
	if intervalMin <= 0 {
		intervalMin = 60
	}
	return &RetentionJob{database: database, intervalMin: intervalMin}
}

// Run starts the retention cleanup loop. Blocks until ctx is cancelled.
func (j *RetentionJob) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(j.intervalMin) * time.Minute)
	defer ticker.Stop()
	log.Printf("retention: cleanup job started (interval=%dm)", j.intervalMin)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.runOnce(ctx)
		}
	}
}

// runOnce performs one cleanup cycle. Errors are logged but do not stop the loop.
func (j *RetentionJob) runOnce(ctx context.Context) {
	if j.database.LogDB == nil {
		return
	}
	logDB := j.database.LogDB.RawDB()

	// Load settings.
	agentDefault := settingInt(j.database, ctx, "log.retention.agent.default_days", 14)
	taskDefault := settingInt(j.database, ctx, "log.retention.task.default_days", 30)
	systemDefault := settingInt(j.database, ctx, "log.retention.system.default_days", 7)

	// Agent event types with their per-type retention (fall back to agentDefault).
	agentEventTypes := []string{
		"agent_registered", "agent_poll_task_found", "agent_claim_attempt",
		"agent_claim_success", "agent_claim_failed", "agent_execute_start",
		"agent_llm_call", "agent_tool_call", "agent_tool_error",
		"agent_context_overflow", "agent_reasoning_step", "agent_retry_backoff",
		"agent_human_approval_req", "agent_execute_complete", "agent_execute_failed",
		"agent_offline",
	}
	taskEventTypes := []string{
		"task_created", "task_updated", "task_queued", "task_claimed",
		"task_started", "task_llm_round", "task_tool_call", "task_result_submitted",
		"task_completed", "task_failed", "task_timed_out", "task_requeued",
	}

	// Pass 1: drop partitions older than the max retention for each category.
	agentMaxDays := maxRetentionDays(j.database, ctx, agentEventTypes, "log.retention.agent.", agentDefault)
	taskMaxDays := maxRetentionDays(j.database, ctx, taskEventTypes, "log.retention.task.", taskDefault)

	agentDropped, err := db.DropOldAgentLogPartitions(ctx, logDB, agentMaxDays)
	if err != nil {
		log.Printf("retention: drop agent partitions: %v", err)
	}
	taskDropped, err := db.DropOldTaskLogPartitions(ctx, logDB, taskMaxDays)
	if err != nil {
		log.Printf("retention: drop task partitions: %v", err)
	}

	// Pass 2: delete short-retention rows from live partitions.
	var agentDeleted, taskDeleted int64
	for _, et := range agentEventTypes {
		days := settingInt(j.database, ctx, "log.retention.agent."+et+"_days", agentDefault)
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		n, err := db.DeleteShortRetentionAgentRows(ctx, logDB, et, cutoff)
		if err != nil {
			log.Printf("retention: delete agent %s rows: %v", et, err)
			continue
		}
		agentDeleted += n
	}
	for _, et := range taskEventTypes {
		days := settingInt(j.database, ctx, "log.retention.task."+et+"_days", taskDefault)
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		n, err := db.DeleteShortRetentionTaskRows(ctx, logDB, et, cutoff)
		if err != nil {
			log.Printf("retention: delete task %s rows: %v", et, err)
			continue
		}
		taskDeleted += n
	}

	// Clean up system logs (general logs table in main DB).
	systemCutoff := time.Now().UTC().Add(-time.Duration(systemDefault) * 24 * time.Hour)
	sysDeleted, err := j.database.DeleteOldLogs(ctx, systemCutoff)
	if err != nil {
		log.Printf("retention: delete system logs: %v", err)
	}

	summary := fmt.Sprintf(
		"retention cleanup: agent_partitions_dropped=%d task_partitions_dropped=%d agent_rows_deleted=%d task_rows_deleted=%d system_rows_deleted=%d",
		agentDropped, taskDropped, agentDeleted, taskDeleted, sysDeleted,
	)
	log.Print(summary)

	// Record summary as a system log entry.
	_ = j.database.CreateLog(ctx, &db.LogEntry{
		Level:   "info",
		Message: summary,
	})
}

// settingInt reads an integer setting from the DB; returns def on any error.
func settingInt(database *db.Database, ctx context.Context, key string, def int) int {
	s, err := database.GetSetting(ctx, key)
	if err != nil {
		return def
	}
	v, err := strconv.Atoi(s.Value)
	if err != nil {
		return def
	}
	return v
}

// maxRetentionDays returns the maximum retention days across all event types
// in a category (used to decide the oldest partition to keep).
func maxRetentionDays(database *db.Database, ctx context.Context, eventTypes []string, keyPrefix string, categoryDefault int) int {
	max := categoryDefault
	for _, et := range eventTypes {
		days := settingInt(database, ctx, keyPrefix+et+"_days", categoryDefault)
		if days > max {
			max = days
		}
	}
	return max
}
