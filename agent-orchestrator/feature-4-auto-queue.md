# Feature 4 — Auto-queue: bounded autonomous task generation

**Status:** `[ ]` pending
**Related:** Feature 3 (revised) — capabilities & planner bootstrap; Feature 2 (revised) — PR workflow.

---

## Goal

Let a project keep its own backlog full without a human creating each task — but in a **bounded** way that cannot loop forever. Auto-queue runs until the project is deemed complete, then disarms itself. A human can re-arm it to trigger exactly one further round of beyond-scope improvements, which again ends in completion and disarm. Two cooperating mechanisms:

1. **Dependency gating** — release tasks for claim only when their prerequisites are done.
2. **Backlog replenishment** — when a project drains, enqueue a planner task; the planner either proposes more work or declares the project complete (which disarms auto-queue).

## Project state

Add to `db.Project`:

```go
AutoQueue    bool   `json:"auto_queue"`      // armed?
Status       string `json:"status"`          // active | complete
MaxOpenTasks int    `json:"max_open_tasks"`  // 0 = unlimited; safety cap per cycle
PlanRounds   int    `json:"plan_rounds"`     // planning rounds used this activation (safety counter)
```

Migration: `ALTER TABLE projects ADD COLUMN auto_queue INTEGER NOT NULL DEFAULT 0`, `status TEXT NOT NULL DEFAULT 'active'`, `max_open_tasks INTEGER NOT NULL DEFAULT 0`, `plan_rounds INTEGER NOT NULL DEFAULT 0`.

## Mechanism 1 — Dependency gating

Tasks already support dependencies (`task_dependencies`). A task in `BACKLOG` with one or more **uncompleted** dependencies is **not** offered by `GetNextTask`; the moment its last dependency reaches `COMPLETED`, it becomes claimable automatically. Implement as an additional clause in the `BACKLOG` branch of `GetNextTask`:

```sql
AND NOT EXISTS (
  SELECT 1 FROM task_dependencies d
  JOIN tasks dep ON dep.id = d.depends_on_task_id
  WHERE d.task_id = tasks.id AND dep.status != 'COMPLETED'
)
```

This lets the planner lay out the whole dependency graph up front; the system releases work in the correct order on its own. Dependency edges are editable on the task form (Feature 3 Part F).

## Mechanism 2 — Backlog replenishment (the queue supervisor)

A background `QueueSupervisor` (a polling loop started from `main.go`) watches projects where `auto_queue = true AND status = 'active'`. For each, it computes **open work** = count of tasks not in a terminal state (anything not `COMPLETED`/`FAILED`).

- If open work > 0 → do nothing (work is flowing).
- If open work == 0 → the project has drained. Enqueue **one** planner task and increment `plan_rounds`:
  - First activation of a fresh project (`plan_rounds == 0` and no prior completion): planner task payload `{"mode": "initial"}` → "define scope from the description if needed, then plan the work to satisfy the requirements/features."
  - Re-armed after a prior completion: payload `{"mode": "improvement"}` → "the in-scope work is done; survey the project and propose beyond-scope improvements, or declare complete if there is nothing worthwhile."
- Safety caps: if `MaxOpenTasks > 0`, never enqueue when open work would exceed it; if `plan_rounds` exceeds a configured ceiling, stop and log a warning instead of enqueuing (guards against a planner that never declares completion).

Separately, if a project's `scope_dirty` flag is set (the description was edited — see Feature 5), the supervisor enqueues a `planner` task `{"mode": "sync"}` that calls `sync_scope` to reconcile requirements/features, then clears the flag. This runs independently of the drain trigger.

The planner task routes and executes like any other (`role: planner`, claimed by a planner agent with `creates_tasks`).

## Completion and disarm (prevents infinite loop)

The planner decides when there is no more useful work. Add a planner tool `complete_project(project_id, summary)` that:
1. Verifies the scope is satisfied — **every feature `done`, every requirement `satisfied`/resolved, and no tasks in a non-terminal state** (the scope-backed completion definition from Feature 5). Rejected otherwise.
2. Sets `project.Status = 'complete'` and `project.AutoQueue = false`.
3. Records the summary as a context entry and logs `project_completed`.

So a planner ends a replenishment round one of two ways: it creates new tasks (the cycle continues — backlog refills, drains, replenishes), or it calls `complete_project` (the cycle stops and auto-queue disarms). Because the empty-queue trigger only fires while `auto_queue = true AND status = 'active'`, disarming halts the loop.

### Re-arm for a beyond-scope round

A human re-arms via the UI: set `auto_queue = true` and `status = 'active'` (and reset `plan_rounds = 0`). Because the project was previously `complete`, the next enqueued planner task is `{"mode": "improvement"}`. The planner proposes improvement tasks; they flow through implement → review → PR → merge (Features 2/3); the queue drains; the planner is enqueued once more; finding nothing further worthwhile, it calls `complete_project` and auto-queue disarms again. Each human re-arm therefore yields exactly one bounded improvement cycle — never an open-ended loop.

## Optional scheduled trigger

In addition to the empty-queue trigger, a scheduled task may re-arm a project on a cadence (e.g. weekly): set `auto_queue = true`, `status = 'active'`, `mode = improvement`. This is opt-in and orthogonal to the supervisor.

## UI

- Project settings: **Auto-queue** toggle, **Status** badge (`active` / `complete`), `max_open_tasks` and plan-round-ceiling inputs, and a **Re-enable auto-queue** action shown when `status = complete`.
- Project view: a small indicator of the current planning mode (`initial` / `improvement`) and `plan_rounds` used this activation.
- Task form: dependencies multi-select (shared with Feature 3 Part F).

## Files to touch

- `db/models.go` — add `AutoQueue`, `Status`, `MaxOpenTasks`, `PlanRounds` to `Project`
- `db/projects.go` — CRUD + scans for the new fields
- `db/migrations.go` — new project columns
- `db/tasks.go` — dependency-gating clause in `GetNextTask` `BACKLOG` branch
- `workflow/queue_supervisor.go` — new: poll armed/active projects, replenish on drain, respect caps
- `tools/plan.go` — add `complete_project` tool (verifies no open tasks; sets status + disarms)
- `server/handlers.go` — project update accepts `auto_queue` / `status` / caps; re-arm action; enforce `creates_tasks` for `complete_project`
- `main.go` — start `QueueSupervisor`
- `ui/src/pages/Projects.svelte` (or project form) — auto-queue toggle, status badge, re-enable action, caps
- `ui/src/pages/Tasks.svelte` — dependencies multi-select
- `ui/src/lib/api.js` — project auto-queue/status updates; re-arm call

## Tests

- `TestGetNextTask_DependencyGated` — task with an uncompleted dependency is not offered; becomes claimable once the dependency is `COMPLETED`
- `TestQueueSupervisor_ReplenishesOnDrain` — armed/active project with zero open work gets exactly one planner task enqueued
- `TestQueueSupervisor_NoReplenishWhileWorkOpen` — open work > 0 → no enqueue
- `TestQueueSupervisor_RespectsMaxOpenTasks` — cap reached → no enqueue
- `TestQueueSupervisor_RespectsPlanRoundCeiling` — ceiling exceeded → stop + warn, no enqueue
- `TestCompleteProject_DisarmsAutoQueue` — `complete_project` sets `status=complete`, `auto_queue=false`; supervisor stops enqueuing
- `TestCompleteProject_RejectedWithOpenTasks` — `complete_project` errors if non-terminal tasks remain
- `TestReArm_TriggersImprovementMode` — re-armed project's next planner task carries `mode=improvement`
- `TestAutoQueue_Terminates` — drive a full cycle: initial plan → tasks complete → planner declares complete → no further enqueues (loop provably ends)
- `TestCompleteProject_RequiresCreatesTasks` — role without `creates_tasks` cannot call `complete_project`
