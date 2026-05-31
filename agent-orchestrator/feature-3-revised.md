# Feature 3 (revised v2) — Capability-driven role system; remove task type; configurable roles

**Status:** `[ ]` pending
**Supersedes:** the original Feature 3 spec.
**Related:** Feature 2 (revised) — PR workflow; Feature 4 — auto-queue. Read all three together.

---

## Design intent

Collapse the overlapping "what kind of work is this" concepts into one runtime-configurable source of truth:

- `Task.Type` — restates the description, adds no routing value → **removed**.
- `RoleDefinition.TaskTypes` — redundant intermediary mapping → **removed from writes/scans**.
- `RoleDefinition` (DB) becomes the single source of truth for roles, including which lifecycle transitions a role may handle, expressed as capability flags.

No role name is hardcoded in server or agent logic; everything reads from the DB. The long-term goal is autonomous operation with human override always available via the UI.

## Capability flags on `RoleDefinition`

Add `Capabilities []string` (`json:"capabilities"`), stored as a JSON array column, following the existing `task_types` / `allowed_tools` pattern in `db/roles.go`.

| Capability | Meaning | Enforced in |
|---|---|---|
| `handles_review` | Role can claim `AWAITING_REVIEW` tasks whose `review_role` matches this role's name | `GetNextTask` (claim routing) |
| `creates_tasks` | Role may create tasks (and write requirements/features) via the API | `POST /api/tasks` + planning tools (authorization) |
| `handles_merge` | Role can claim `AWAITING_MERGE` tasks to **review the PR** and approve/reject it | `GetNextTask` (claim routing) + PR decision endpoints (Feature 2) |
| `handles_deploy` | Role can claim deployment-type tasks (broad `role: deploy` work from `BACKLOG`) | normal `role` match in `GetNextTask` |

`handles_merge` does **not** mean "auto-merge." It grants the right to claim a PR and perform a merge *review*; the merge only happens after an explicit approve decision (see Feature 2). Capabilities are free-form strings — operators may define custom tokens, stored and shown in the UI but with no built-in effect beyond the four above.

## Routing rules

`GetNextTask` resolves capabilities **in Go**, not via SQL `LIKE`. The function already receives `roles []string`; load enabled `RoleDefinition`s (cached), and compute the subsets of the agent's roles that carry `handles_review` and `handles_merge`.

| Task status | Claim condition |
|---|---|
| `BACKLOG` | `task.role IN (agent.roles)` **and all task dependencies COMPLETED** (see Feature 4) |
| `AWAITING_REVISION` | `task.role IN (agent.roles)` |
| `AWAITING_REVIEW` | `task.review_role IN (agent.roles)` **AND** the matched role has `handles_review` |
| `AWAITING_MERGE` | agent has a role with `handles_merge` (the deployer claims it to review the PR) |

In-Go resolution rationale: roles are a tiny, cached set. `capabilities LIKE '%handles_review%'` is fragile (substring false-positives, unindexable) and unnecessary.

## `review_role` on `Task`

Add `ReviewRole string` (`json:"review_role,omitempty"`) to `db.Task`. Migration: `ALTER TABLE tasks ADD COLUMN review_role TEXT NOT NULL DEFAULT ''`. On creation, if empty, resolve to the name of the first enabled `RoleDefinition` with `handles_review`, falling back to `"reviewer"`.

Because `Task.Type` is removed, the automatic review handoff no longer keys off `TaskType`. The review handoff becomes purely state-driven: a worker pushes → `AWAITING_REVIEW` → claimable by an agent whose role matches `task.review_role` and has `handles_review`. Update `workflow/state.go` (`FollowOnType`, `RoleForType`) to drop `TaskType`-based branches; verify and update callers in `workflow/scheduler.go` before deleting.

## Default role set (seeded on first run)

Seven roles, inserted by an explicit `SeedRoleDefinitions` default set (replacing the `cfg.Roles`-derived seeding path in `main.go` for a fresh DB). `SeedRoleDefinitions` only inserts names that do not already exist, so user-created roles are untouched.

| Name | Label | Capabilities | Suggested tools | Notes |
|---|---|---|---|---|
| `worker` | Worker | — | read, write, diff, tests, comment | General implementation; runs tests as part of every task |
| `reviewer` | Reviewer | `handles_review` | read, list, comment | Reviews work; opens PR on approval |
| `planner` | Planner | `creates_tasks` | bootstrap_project, create_work_package, list_tasks, read, comment, web_search | Writes requirements/features, plans, orchestrates, declares completion |
| `researcher` | Researcher | `creates_tasks` | web_search, read, comment, create_work_package | Web research, improvement proposals |
| `security` | Security | — | read, list, run_command, web_search, comment | Autonomous; claims `role: security` from BACKLOG |
| `deployer` | Deployer | `handles_merge`, `handles_deploy` | read, run_command, comment, diff | Reviews PRs, merges on approval, deploys |
| `designer` | Designer | `creates_tasks` | read, write, comment, create_work_package | UI/UX; creates implementation tasks |

`deployer` holds both `handles_merge` (claims `AWAITING_MERGE` to review and decide PRs — never auto-approves) and `handles_deploy`. `security` has no `handles_review`: it is self-directed and claims `role: security` tasks from `BACKLOG`.

## What to build

**Part A — Remove `Task.Type` and `RoleDefinition.TaskTypes`**
`Task.Type`: drop from `CreateTask` insert (`db/tasks.go`), `CreateTaskRequest`, type validation in `handlers.go`, the `type` select / `form.type` / `submit()` payload in `Tasks.svelte`. Remove `GET /api/meta/task-types`, `handleMetaTaskTypes`, the `taskTypes` var in `meta.go`, and `getTaskTypes` in `api.js`. Keep the DB column (nullable / blank default) for backward compatibility.
`RoleDefinition.TaskTypes`: remove from all CRUD writes and scans in `db/roles.go`; remove from the `Roles.svelte` form. Keep the column as dead/nullable.

**Part B — Add `Capabilities` to `RoleDefinition`**
Add `Capabilities []string` to `db/models.go`. Migration: `ALTER TABLE agent_role_definitions ADD COLUMN capabilities TEXT NOT NULL DEFAULT '[]'`. Update `CreateRoleDefinition`, `UpdateRoleDefinition`, and both scan functions in `db/roles.go`. Add a capabilities tag input to `Roles.svelte` with the known tokens as suggestions.

**Part C — Add `ReviewRole` to `Task`; capability-driven `AWAITING_REVIEW` routing**
Add `ReviewRole` to `db.Task` + migration. Resolve default on create. Update `GetNextTask` per the routing table (capability resolved in Go).

**Part D — Capability-gated `AWAITING_MERGE` routing**
Keep `AWAITING_MERGE` claimable in `GetNextTask`, but gate it on the agent holding a role with `handles_merge` (computed in Go), not on `task.role`. This is how the deployer picks up PRs to review. The merge git operation itself is performed by the approve decision endpoint (Feature 2), never by claim alone.

**Part E — `GET /api/meta/task-roles` returns live role definitions**
Replace the hardcoded `taskRoles` in `meta.go` with `db.ListRoleDefinitions` filtered to `enabled = true`. Response shape becomes `[]db.RoleDefinition`. Update `getTaskRoles` in `api.js`.

**Part F — Task creation form (`Tasks.svelte`)**
Remove the `type` select. Populate `role` from `GET /api/meta/task-roles`. Add an optional `review_role` select, filtered to roles with `handles_review`, defaulting to the first match. Add an optional dependencies multi-select (Feature 4) and an optional `focus` multi-select populated from enabled skills (Feature 6). Layout: project · title · role · review_role · focus · dependencies · description · requirements/features · priority. All additions are optional, so the minimal task is still just project · title · role.

**Part G — Role definition form (`Roles.svelte`)**
Add `capabilities` as a tag/chip input (suggested tokens shown). Remove `task_types`.

**Part H — Seed default roles**
Introduce the seven-role default set as the seed source in `main.go` (or a dedicated seed file), idempotent via the existing name check.

**Part I — Rewire follow-on review creation**
Update `workflow/state.go` and `workflow/scheduler.go` so review handoff is state-driven via `review_role`, not `Task.Type`. Remove dead `TaskType` branches.

**Part J — Update planning tools to drop `type`**
`tools/plan.go` (`plan_project`, `create_work_package`) currently set `Type: "implement"`. Replace with `Role` + resolved `ReviewRole`; remove the `task_type` parameter. These tools are what the planner uses to fan out work, so they must match the new task shape.

**Part K — Planner bootstrap from a project description**
Add a freeform `Description string` field to `db.Project` (migration: `ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT ''`), editable in the project create/edit UI. Add a planner tool `bootstrap_project(project_id)` that reads `project.Description` and writes structured entries into the existing `project_requirements` and `project_features` tables. Only roles with `creates_tasks` may call it.

The intended flow: a human creates a project with only a description; the first planner task ("define scope from the description, then plan") calls `bootstrap_project` to author requirements/features, then `plan_project` / `create_work_package` to fan out implementation tasks linked to those features. A one-line description is therefore a sufficient starting point.

**The full scope model is specified in Feature 5**, including: keeping requirements/features in sync with a changing description via a non-destructive `sync_scope` tool; auto-advancing item status from linked-task completion; and per-role scope visibility. Key point for the role system here: scope visibility is controlled by the existing `context_include` / `context_exclude` rules, not by a capability — seed `planner`, `researcher`, `designer`, and `security` with `project_requirements` + `project_features` in `context_include`, and keep them out of `worker` / `reviewer` / `deployer` context so implementation roles stay focused on their task.

**Part L — Document patterns in `AGENTS.md`**
Document: worker pattern; review pattern (`handles_review` roles set as `review_role`); PR/merge pattern (`handles_merge` deployer reviews PRs, approval triggers merge — see Feature 2); planner bootstrap + self-improvement pattern (`creates_tasks`); security chaos-monkey pattern.

## Files to touch

- `db/models.go` — add `Capabilities` to `RoleDefinition`; add `ReviewRole` to `Task`; add `Description` to `Project`; keep `Type` / `TaskTypes` columns dead/nullable
- `db/tasks.go` — remove `type` from `CreateTask`; add `review_role`; resolve default `review_role`; update `GetNextTask` (capability-gated review + merge; dependency gate)
- `db/roles.go` — add `capabilities` to CRUD + both scans; remove `task_types` from writes
- `db/projects.go` — add `description` to CRUD + scans
- `db/migrations.go` — `capabilities` on roles; `review_role` on tasks; `description` on projects
- `api/types.go` — remove `Type` from `CreateTaskRequest`; add `ReviewRole`, `Dependencies`
- `server/meta.go` — live role list; remove `handleMetaTaskTypes`, `taskTypes`, `taskRoles`
- `server/server.go` — remove `/api/meta/task-types` route
- `server/handlers.go` — remove type validation; resolve/store `review_role`; enforce `creates_tasks` on task create
- `server/roles_handler.go` — `defaultToolsForRole` for the new role set; seed capabilities
- `tools/plan.go` — drop `type`; set `role` + `review_role`; add `bootstrap_project` tool
- `workflow/state.go`, `workflow/scheduler.go` — review handoff state-driven via `review_role`
- `ui/src/lib/api.js` — remove `getTaskTypes`; update `getTaskRoles`; add project description + bootstrap wiring
- `ui/src/pages/Tasks.svelte` — remove type select; add `review_role` + dependencies selects
- `ui/src/pages/Roles.svelte` — capabilities tag input; remove `task_types`
- `ui/src/pages/Projects.svelte` (or project form) — description field
- `main.go` / seed file — seven-role default seed set with capabilities

## Tests

- `TestRoleDefinition_CapabilitiesRoundTrip` — capabilities JSON round-trips
- `TestCreateTask_NoTypeRequired` — task creates without `type`
- `TestCreateTask_ReviewRoleResolved` / `TestCreateTask_ExplicitReviewRole`
- `TestGetNextTask_CapabilityBasedReviewRouting` — reviewer claims, security does not
- `TestGetNextTask_ReviewRoleWithoutCapability` — `review_role: worker` (no `handles_review`) → no claim
- `TestGetNextTask_MergeClaimedByHandlesMerge` — deployer (has `handles_merge`) claims `AWAITING_MERGE`; reviewer does not
- `TestGetNextTask_MergeNotClaimedWithoutCapability` — no `handles_merge` agent → task not returned
- `TestMetaTaskRoles_LiveFromDB` / `TestMetaTaskTypes_Removed`
- `TestSeedRoles_NewTaxonomy` / `TestSeedRoles_Idempotent`
- `TestPlanTools_NoType` — `plan_project` / `create_work_package` create tasks with `role` + `review_role`, no `type`
- `TestBootstrapProject_WritesRequirementsAndFeatures` — planner tool reads description, persists requirements + features
- `TestBootstrapProject_RequiresCreatesTasks` — role without `creates_tasks` is rejected
