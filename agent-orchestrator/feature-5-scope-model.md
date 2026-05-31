# Feature 5 — Scope model: description as intent, first-class requirements & features

**Status:** `[ ]` pending
**Related:** Feature 3 (revised) — capabilities, planner bootstrap; Feature 4 — auto-queue completion.

---

## Goal

Make a project's **description** the human-authored statement of intent, and make **requirements** and **features** the structured, trackable projection of that intent. The description can change; the requirements/features are kept in sync by the planner **non-destructively** (existing items and their task links are preserved). Scope is **planner-owned**: only roles that need the project-level view receive requirements/features in their context — implementation roles stay focused on their individual task.

This makes the two entities (which today are a passive human-only tracking board, unread by any agent) actually drive the workflow, and gives auto-queue a real completion signal.

## Who sees scope — per-role, via existing context rules

The router already filters context entries by `type` against each role's `ContextInclude` / `ContextExclude` (`router.BuildWithRules`). We reuse that — **no new capability needed.**

- Surface the project's requirements and features to the context builder as two synthetic context entries with types `project_requirements` and `project_features`.
- A role receives them only if its `ContextInclude` lists those types (or its rules don't exclude them). Implementation roles exclude them.

Seed defaults:

| Role | Sees scope? | Mechanism |
|---|---|---|
| `planner` | yes | `context_include` contains `project_requirements`, `project_features` |
| `researcher` | yes | same |
| `designer` | yes | same |
| `security` | yes | same (whole-project view for chaos-monkey analysis) |
| `worker` | no | scope types not in include / listed in `context_exclude` |
| `reviewer` | no | same — reviews the task's diff, not the roadmap |
| `deployer` | no | same |

Because this is just role config, an operator can change who sees scope at runtime on the Roles page.

## Description as source of intent

`Project.Description` (freeform) is added in Feature 3 Part K (`ALTER TABLE projects ADD COLUMN description`). It is the canonical human intent. Requirements/features are derived from it but then **curated and tracked** — they are not a read-only mirror.

## Planner tools

**`bootstrap_project(project_id)`** — first-time scope authoring. If the project has no requirements/features, read `description` (+ context) and create them. Only callable by roles with `creates_tasks`.

**`sync_scope(project_id)`** — reconcile after the description changes. Non-destructive diff:
- New intent not covered by an existing item → **create** a requirement/feature.
- Existing item still supported by the description → leave untouched (preserve ID, status, task links).
- Existing item no longer supported → mark `status = needs_review` (never auto-delete, never unlink tasks).
- Returns a structured diff (added / unchanged / flagged) and posts it as a project comment for human visibility.

A human resolves `needs_review` items manually (keep, edit, or delete). This keeps traceability intact across description edits — the trap of blind regeneration (wiping items + their task links) is avoided by design.

## Description-change trigger

When the description is edited via the API, set a `scope_dirty` flag on the project. Behaviour:
- If auto-queue is armed and active, the queue supervisor enqueues a `planner` task `{"mode": "sync"}` that calls `sync_scope`, then clears the flag.
- If auto-queue is off, the project view shows a "scope may be out of date — re-sync" prompt with a button that creates the same planner sync task on demand.

No automatic destructive change ever happens without the planner's reconcile + human review of flagged items.

## Auto-advancing item status (the completion signal)

Today status is manual and unused. Make it derived from linked-task progress. Statuses:
- Requirement: `proposed → accepted → satisfied` (+ `needs_review`)
- Feature: `planned → in_progress → done` (+ `needs_review`)

Recompute on every task transition into a terminal state. When a task reaches `COMPLETED`, for each requirement/feature it is linked to:
- all linked tasks `COMPLETED` and at least one exists → `satisfied` / `done`
- some linked tasks still open → `accepted` / `in_progress`
- `needs_review` is never auto-overwritten (human must clear it)

Implement as a helper `RecomputeLinkedScopeStatus(ctx, taskID)` called from the `COMPLETED` transition in `db/tasks.go` (and from the merge-complete path). It reads `task_project_links` for the task and updates each linked item.

## Completion definition (used by Feature 4)

A project is **complete** when: every feature is `done`, every requirement is `satisfied` or explicitly resolved, **and** no tasks are in a non-terminal state. The planner's `complete_project` tool (Feature 4) verifies exactly this before setting `status = complete` and disarming auto-queue. This replaces the vaguer "no open tasks" check with a scope-backed signal, so the loop only ends when the declared scope is actually met.

## Files to touch

- `db/models.go` — `Project.Description` (shared with Feature 3 Part K); confirm `ProjectRequirement` / `ProjectFeature` status vocab
- `db/projects.go` — description in CRUD + scans; `scope_dirty` flag
- `db/migrations.go` — `description`, `scope_dirty` columns on `projects`
- `db/project_requirements.go`, `db/project_features.go` — no schema change; add status-vocab helpers if useful
- `db/task_project_links.go` (or `db/tasks.go`) — `RecomputeLinkedScopeStatus`; call it on `COMPLETED` transitions
- `router/router.go` + `router/context.go` — append `project_requirements` / `project_features` entries to the context set before `BuildWithRules` filters them
- `tools/plan.go` — `bootstrap_project`, `sync_scope` (both require `creates_tasks`)
- `server/handlers.go` / `server/requirements_handler.go` — set `scope_dirty` on description edit; re-sync action; surface `needs_review`
- `workflow/queue_supervisor.go` — enqueue `mode: sync` planner task when `scope_dirty` (Feature 4)
- `main.go` / seed file — seed `context_include` / `context_exclude` per role per the table above
- `ui/src/pages/ProjectDetail.svelte` — description editor; `needs_review` badges; auto-status display; "re-sync scope" button
- `ui/src/pages/Roles.svelte` — expose `context_include` / `context_exclude` editing (so scope visibility is configurable)
- `ui/src/lib/api.js` — description update; re-sync call

## Tests

- `TestContextInjection_ScopeOnlyForIncludedRoles` — build context for a planner (include scope) and a worker (exclude scope); planner's context contains requirements/features, worker's does not
- `TestBootstrapProject_FromDescription` — empty project + description → requirements/features created
- `TestSyncScope_AddsNewWithoutTouchingExisting` — add intent to description; sync creates new items, leaves existing IDs/links unchanged
- `TestSyncScope_FlagsStaleAsNeedsReview` — remove intent; matching item becomes `needs_review`, is not deleted, links intact
- `TestSyncScope_PreservesTaskLinks` — item with linked tasks survives a sync that flags it
- `TestRecomputeScopeStatus_FeatureDoneWhenAllTasksComplete` — feature → `done` only when all linked tasks `COMPLETED`
- `TestRecomputeScopeStatus_InProgressWhenPartial` — partial completion → `in_progress`
- `TestRecomputeScopeStatus_NeedsReviewNotOverwritten` — `needs_review` survives recompute
- `TestCompleteProject_RequiresScopeSatisfied` — `complete_project` rejected unless all features `done` / requirements `satisfied` and no open tasks
- `TestDescriptionEdit_SetsScopeDirty` — editing description sets `scope_dirty`; sync clears it
