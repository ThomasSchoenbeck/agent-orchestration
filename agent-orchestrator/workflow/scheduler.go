package workflow

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"agent-orchestrator/db"
)

// Scheduler manages the task lifecycle: detects completions, creates follow-on tasks,
// handles retries with exponential backoff, and requeues timed-out tasks.
type Scheduler struct {
	db         *db.Database
	timeoutSec int
	maxRetries int
	interval   time.Duration
	stop       chan struct{}
}

// NewScheduler creates a Scheduler backed by the given database.
func NewScheduler(database *db.Database, timeoutSec, maxRetries int, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{
		db:         database,
		timeoutSec: timeoutSec,
		maxRetries: maxRetries,
		interval:   interval,
		stop:       make(chan struct{}),
	}
}

// Start launches the background scheduling loop.
func (s *Scheduler) Start(ctx context.Context) {
	go s.run(ctx)
}

// Stop signals the background loop to exit.
func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				log.Printf("[scheduler] tick error: %v", err)
			}
		}
	}
}

// Tick performs one scheduling cycle:
//  1. Requeue timed-out tasks.
//  2. Retry eligible failed tasks (exponential backoff).
//  3. Create follow-on tasks for newly completed tasks.
func (s *Scheduler) Tick(ctx context.Context) error {
	requeued, err := s.db.RequeueTimedOutTasks(ctx, s.timeoutSec)
	if err != nil {
		return fmt.Errorf("requeue timed out tasks: %w", err)
	}
	if requeued > 0 {
		log.Printf("[scheduler] requeued %d timed-out tasks", requeued)
	}

	if err := s.retryFailedTasks(ctx); err != nil {
		return fmt.Errorf("retry failed tasks: %w", err)
	}

	return nil
}

// retryFailedTasks promotes FAILED tasks back to BACKLOG if under the retry limit.
// Uses exponential backoff: a task is only retried after 2^(attempts-1) minutes.
func (s *Scheduler) retryFailedTasks(ctx context.Context) error {
	tasks, err := s.db.ListTasks(ctx, db.TaskFilters{Status: db.TaskStatusFailed})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, t := range tasks {
		if t.Attempts >= s.maxRetries {
			continue // permanently failed
		}
		backoffMin := math.Pow(2, float64(t.Attempts-1))
		backoffDur := time.Duration(backoffMin * float64(time.Minute))
		if now.Sub(t.UpdatedAt) < backoffDur {
			continue
		}
		t.Status = db.TaskStatusBacklog
		t.AssignedAgentID = ""
		if err := s.db.UpdateTask(ctx, t); err != nil {
			log.Printf("[scheduler] failed to requeue task %s: %v", t.ID, err)
		} else {
			log.Printf("[scheduler] retrying task %s (attempt %d/%d)", t.ID, t.Attempts, s.maxRetries)
		}
	}
	return nil
}

// HandleReviewResult processes the outcome of a review task.
// "approved" → task moves to AWAITING_MERGE.
// "changes"  → task moves to AWAITING_REVISION (dev picks it up again).
func (s *Scheduler) HandleReviewResult(ctx context.Context, reviewTask *db.Task, outcome string) error {
	switch outcome {
	case "approved":
		reviewTask.Status = db.TaskStatusAwaitingMerge
		return s.db.UpdateTask(ctx, reviewTask)

	case "changes":
		reviewTask.Status = db.TaskStatusAwaitingRevision
		return s.db.UpdateTask(ctx, reviewTask)

	default:
		return fmt.Errorf("unknown review outcome %q; expected 'approved' or 'changes'", outcome)
	}
}

// RetryDelay returns the exponential backoff duration for a given attempt count.
func RetryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return time.Minute
	}
	minutes := math.Pow(2, float64(attempt-1))
	return time.Duration(minutes * float64(time.Minute))
}
