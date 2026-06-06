package workflow

import (
	"context"
	"fmt"
	"log"
	"time"

	"agent-orchestrator/db"
)

// defaultPlanRoundCeiling bounds how many planner rounds a single activation may
// use before the supervisor stops enqueuing — a guard against a planner that
// never declares completion.
const defaultPlanRoundCeiling = 10

// QueueSupervisor keeps armed/active projects' backlogs full in a bounded way:
// when a project drains (no open work) it enqueues exactly one planner task to
// replenish or declare completion. It self-stops when the project is marked
// complete (auto_queue disarmed) or the plan-round ceiling is reached.
type QueueSupervisor struct {
	database         *db.Database
	intervalSec      int
	planRoundCeiling int
}

// NewQueueSupervisor creates a supervisor with the given DB and polling interval.
func NewQueueSupervisor(database *db.Database, intervalSec int) *QueueSupervisor {
	if intervalSec <= 0 {
		intervalSec = 15
	}
	return &QueueSupervisor{
		database:         database,
		intervalSec:      intervalSec,
		planRoundCeiling: defaultPlanRoundCeiling,
	}
}

// TickOnce runs one supervisor cycle synchronously. For testing only.
func (qs *QueueSupervisor) TickOnce(ctx context.Context) { qs.tick(ctx) }

// Run starts the supervisor loop. It blocks until ctx is cancelled.
func (qs *QueueSupervisor) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(qs.intervalSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			qs.tick(ctx)
		}
	}
}

func (qs *QueueSupervisor) tick(ctx context.Context) {
	projects, err := qs.database.ListAutoQueueProjects(ctx)
	if err != nil {
		log.Printf("queue_supervisor: ListAutoQueueProjects: %v", err)
		return
	}
	for _, p := range projects {
		qs.processProject(ctx, p)
	}
}

func (qs *QueueSupervisor) processProject(ctx context.Context, p *db.Project) {
	// Scope-dirty (Feature 5): reconcile requirements/features against the edited
	// description via a planner sync task, independently of the drain trigger.
	if p.ScopeDirty {
		qs.enqueuePlanner(ctx, p, "sync")
		_ = qs.database.SetScopeDirty(ctx, p.ID, false)
	}

	open, err := qs.database.CountOpenTasks(ctx, p.ID)
	if err != nil {
		log.Printf("queue_supervisor: CountOpenTasks %q: %v", p.ID, err)
		return
	}

	// Safety cap: never add work when open tasks are at or above the cap.
	if p.MaxOpenTasks > 0 && open >= p.MaxOpenTasks {
		return
	}
	// Work is still flowing — nothing to replenish.
	if open > 0 {
		return
	}
	// Plan-round ceiling: stop (and warn) rather than loop forever.
	if p.PlanRounds >= qs.planRoundCeiling {
		log.Printf("queue_supervisor: project %q reached plan-round ceiling (%d) — not enqueuing",
			p.ID, qs.planRoundCeiling)
		return
	}

	qs.enqueuePlanner(ctx, p, qs.planMode(ctx, p))
	_ = qs.database.IncrementPlanRounds(ctx, p.ID)
}

// planMode chooses the planner's mode for a drained project:
//   - "initial"     when scope is undefined or not yet satisfied (define/plan work)
//   - "improvement" when scope exists and is fully satisfied (propose beyond-scope work)
func (qs *QueueSupervisor) planMode(ctx context.Context, p *db.Project) string {
	reqs, _ := qs.database.ListRequirements(ctx, p.ID)
	feats, _ := qs.database.ListFeatures(ctx, p.ID)
	if len(reqs)+len(feats) == 0 {
		return "initial"
	}
	satisfied, _, _ := qs.database.ProjectScopeSatisfied(ctx, p.ID)
	if satisfied {
		return "improvement"
	}
	return "initial"
}

// plannerRoleRef returns the role reference for planner tasks: the enabled role
// definition that carries the creates_tasks capability (matched by id, the same
// form agents register and tasks store). The role's name is irrelevant — in
// setups that call it "orchestrator" rather than "planner" this still resolves
// correctly. Falls back to the literal "planner" name (and warns) when no role
// has the capability, since the task would otherwise be unclaimable.
func (qs *QueueSupervisor) plannerRoleRef(ctx context.Context) string {
	defs, err := qs.database.ListRoleDefinitions(ctx)
	if err == nil {
		for _, d := range defs {
			if !d.Enabled {
				continue
			}
			for _, c := range d.Capabilities {
				if c == "creates_tasks" {
					return d.ID
				}
			}
		}
	}
	log.Printf("queue_supervisor: no enabled role has the creates_tasks capability; " +
		"planner tasks will use role \"planner\" and may be unclaimable")
	return "planner"
}

func (qs *QueueSupervisor) enqueuePlanner(ctx context.Context, p *db.Project, mode string) {
	task := &db.Task{
		ProjectID: p.ID,
		Role:      qs.plannerRoleRef(ctx),
		Status:    db.TaskStatusBacklog,
		Priority:  9,
		Payload: map[string]interface{}{
			"mode":        mode,
			"title":       fmt.Sprintf("Auto-queue planning (%s)", mode),
			"description": plannerInstructions(mode),
		},
	}
	if err := qs.database.CreateTask(ctx, task); err != nil {
		log.Printf("queue_supervisor: enqueue planner for %q: %v", p.ID, err)
	}
}

func plannerInstructions(mode string) string {
	switch mode {
	case "sync":
		return "The project description changed. Call sync_scope to reconcile the requirements and features."
	case "improvement":
		return "The in-scope work is complete. Survey the project and propose beyond-scope improvements, " +
			"or call complete_project if there is nothing worthwhile left to do."
	default: // initial
		return "Define the project scope from the description if needed, then plan the work packages " +
			"required to satisfy the requirements and features."
	}
}
