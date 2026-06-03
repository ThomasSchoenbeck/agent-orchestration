# AGENTS.md — Agent operation & patterns

This document describes how agents work in the orchestrator: the roles and
capabilities that decide *who does what*, the task lifecycle that decides *when*,
the skills that decide *how an agent is specialized*, and the operational
surfaces (CLI, config, UI, managed agents) that run them. It reflects the system
through Features 2–8.

The mental model is small: **an agent is `roles × skills`, gated by
`capabilities`, driving a task through a fixed state machine.** Everything below
elaborates that sentence.

---

## 1. The three orthogonal dimensions

| Dimension | Answers | Count | Lives on |
|---|---|---|---|
| **Role** | function + lifecycle authority | small, stable (7 seeded) | `task.role` (one) + `agent.roles` (1+) |
| **Capability** | what lifecycle actions a role may perform | a handful of flags | `role.capabilities` |
| **Skill / focus** | stack / technology / persona | many, free-form | `agent.skills` (0+) + `task.focus` (0+, optional) |

Roles never grow to encode technology — that is what skills are for. Capabilities
are the only source of lifecycle authority; skills grant none.

### Seeded roles and their capabilities

| Role | Capabilities | Sees project scope? |
|---|---|---|
| `worker` | — | no (scope excluded) |
| `reviewer` | `handles_review` | no |
| `planner` | `creates_tasks` | yes |
| `researcher` | `creates_tasks` | yes |
| `security` | — | yes (whole-project view) |
| `deployer` | `handles_merge`, `handles_deploy` | no |
| `designer` | `creates_tasks` | yes |

Roles, capabilities, prompts, context rules, and tool allowlists are all editable
at runtime on the Roles page; the table above is the seeded default for a fresh
database.

---

## 2. Task lifecycle (the state machine)

```
BACKLOG ──claim──▶ DEVELOPING ──push──▶ AWAITING_REVIEW ──claim──▶ REVIEWING
                                                                      │
                            ┌───────── changes_requested ─────────────┤
                            ▼                                         approved
                     AWAITING_REVISION ◀── reject ──┐                  │
                            │                        │                 ▼
                            └──── re-develop ────────┘            (PR opened)
                                                                       │
                                                                       ▼
                                            AWAITING_MERGE ──claim──▶ MERGING
                                                  │                    │
                                       human approve/reject     deployer decision
                                                  └────────┬───────────┘
                                              approve │           │ reject
                                                       ▼           ▼
                                                   COMPLETED   AWAITING_REVISION
```

`FAILED` is reachable from execution states on unrecoverable error. Queue states
(`BACKLOG`, `AWAITING_REVIEW`, `AWAITING_REVISION`, `AWAITING_MERGE`) hold no
agent; execution states (`DEVELOPING`, `REVIEWING`, `MERGING`) are claimed.

Claiming maps a queue state to its execution counterpart (`BACKLOG`/
`AWAITING_REVISION` → `DEVELOPING`, `AWAITING_REVIEW` → `REVIEWING`,
`AWAITING_MERGE` → `MERGING`). Timed-out execution tasks requeue: `REVIEWING` →
`AWAITING_REVIEW`, others → `BACKLOG`.

---

## 3. Routing — who is offered which task

`GetNextTask` resolves eligibility in Go from the agent's **live** roles/skills:

- **Work** (`BACKLOG`/`AWAITING_REVISION`): `task.role ∈ agent.roles`, and every
  task dependency is `COMPLETED` (dependency gating), and — if `task.focus` is
  non-empty — `task.focus ⊆ agent.skills`. Empty focus is unrestricted.
- **Review** (`AWAITING_REVIEW`): `task.review_role` matches an agent role that
  carries `handles_review`.
- **Merge** (`AWAITING_MERGE`): any agent role carries `handles_merge`.

The poll endpoint reads the agent's roles/skills from its **database row**, not
from the request — so a UI override (Feature 7) takes effect on the next poll.

---

## 4. Worker pattern

A worker claims a `BACKLOG`/`AWAITING_REVISION` task, clones the project repo over
the embedded git server, and executes an LLM + tool loop against a per-task
worktree. On success it commits and pushes its `task/<id>` branch; the push moves
the task to `AWAITING_REVIEW`. The agent then submits for review and posts a
completion comment. Its local workspace is removed on every exit (success,
failure, review, merge, or panic).

A worker never reaches `COMPLETED` directly — completion is only reached through
an explicit merge approval (Section 6).

---

## 5. Review pattern

A reviewer claims an `AWAITING_REVIEW` task (it becomes `REVIEWING`) and submits a
verdict:

- `approved` → the server opens a pull request and moves the task to
  `AWAITING_MERGE` (logs `pr_opened`). The merge is **not** performed here.
- `changes_requested` / `revision_requested` → task → `AWAITING_REVISION`; the
  worker re-develops and resubmits, re-entering review.

Two distinct review gates exist: the **work review** (`reviewer`,
`handles_review`) judges correctness; the **merge review** (`deployer`,
`handles_merge`) judges deploy-safety. Both are real verdicts — neither is
automatic.

---

## 6. PR / merge pattern (Feature 2)

PRs are **never auto-approved.** The approve endpoint is the single place that
touches git, so a merge can only follow an explicit decision.

```
reviewer approves     → PR opened (status: open), task → AWAITING_MERGE
deployer claims        → MERGING (the deployer is reviewing the PR)
  approve  → server merges task/<id> into main, deletes the branch,
             PR → merged, task → COMPLETED   (and mirrors upstream if configured)
  reject   → PR → rejected, task → AWAITING_REVISION  (branch kept)
human (any time)       → may approve/reject via the UI, same effect
merge conflict          → PR rejected with the conflict log, task → AWAITING_REVISION
```

A `deployer` agent (capability `handles_merge`) claims `AWAITING_MERGE`, runs its
own review, and calls approve/reject. If no deployer is online the task simply
waits in `AWAITING_MERGE` for a human decision. There is no background
auto-merger.

Endpoints: `POST /api/tasks/{id}/pull-requests/{prID}/approve|reject`,
`GET /api/tasks/{id}/pull-requests`.

---

## 7. Planner: bootstrap, scope, auto-queue, self-improvement

The planner (`creates_tasks`) owns project scope and the work backlog.

**Scope (Feature 5).** A project's `description` is the human statement of intent;
**requirements** and **features** are its structured, trackable projection. The
planner derives them and the tools persist them:

- `bootstrap_project` — first-time scope authoring (no-op if scope already
  exists).
- `sync_scope` — non-destructive reconcile after the description changes: new
  intent is created, matched items are left untouched (IDs/status/task links
  preserved), and items no longer supported are flagged `needs_review` (never
  deleted). It clears the project's `scope_dirty` flag and posts a diff.

Item status is **derived** from linked-task completion (recomputed on every
`COMPLETED` transition): a requirement becomes `satisfied` / a feature becomes
`done` only when all its linked tasks are `COMPLETED`; partial progress yields
`accepted` / `in_progress`; `needs_review` is never auto-overwritten.

Only roles whose `context_include` permits the scope types receive
`project_requirements` / `project_features` in their context — implementation
roles (worker/reviewer/deployer) stay focused on their own task.

**Auto-queue (Feature 4).** When a project is *armed* (`auto_queue = true`,
`status = active`) and its backlog drains to zero open tasks, the queue
supervisor enqueues exactly **one** planner task:

- `initial` mode when scope is undefined or unmet → define/plan work.
- `improvement` mode when scope is fully satisfied → propose beyond-scope work,
  or declare completion.
- a `sync` task when `scope_dirty` is set.

The planner ends a round one of two ways: it creates new tasks (the cycle
continues) or it calls `complete_project`, which verifies the scope-backed
completion definition (every feature `done`, every requirement `satisfied`, no
non-terminal tasks), sets `status = complete`, and disarms `auto_queue`. Safety
caps (`max_open_tasks`, a plan-round ceiling) bound every activation, so the loop
provably ends. A human re-arms (set `auto_queue`, `status = active`,
`plan_rounds = 0`) to trigger exactly one further improvement round —
self-improvement is always bounded, never an open-ended loop.

`complete_project` requires the `creates_tasks` capability.

---

## 8. Skills / focus (Feature 6)

A **skill** is a configurable entity (managed on the Skills page), not an opaque
tag: it carries a prompt fragment ("soul"), context include/exclude rules, and
optional tools. An agent's effective **persona** is `role ⊕ skills`:

- **System prompt** = role prompt followed by each skill's prompt fragment.
- **Context rules** = union of the role's and skills' include/exclude.
- **Allowed tools** = union of the role's and skills' tools.
- **Capabilities** = from roles only (skills add none).

`ResolveAgentPersona(role, skills)` performs this merge. An agent always applies
its own skill fragments (they define who it is); a task's optional `focus` is
purely a routing filter (Section 3). Starter skills `backend`, `frontend`, `infra`
are seeded.

---

## 9. Agent lifecycle (Feature 7)

Each agent has two config layers:

- **Start params** (`start_roles`, `start_skills`) — captured from CLI/config at
  every registration.
- **Live values** (`roles`, `skills`) — initialised to the start params, mutable
  at runtime from the UI. Routing and persona use the live values.

**The reset rule:** every registration (including a restart) sets start params
from the payload, resets live = start, and sets `desired_state = run`. So any UI
override or prior stop is discarded when the process relaunches — the agent always
comes back as its start params describe. Agents are matched across restarts by
`name`.

Control flows through the heartbeat response `{desired_state, roles, skills}`:

- on `stop`, the agent finishes/aborts its task, calls `SetOffline`, and exits;
- on a live skill change, it recomposes its persona before the next task.

Endpoints: `PATCH /api/agents/{id}` (live roles/skills only — never start),
`POST /api/agents/{id}/stop`, `POST /api/agents/{id}/reset` (live = start, no
restart).

---

## 10. Managed agents (Feature 8)

The server can launch and supervise co-located agent processes from an
**AgentTemplate** (name, roles, skills, replicas, autostart). "Co-located" means
lifecycle-managed on the server's host — not a special git path; managed agents
clone/push over `http://localhost:{port}` like any other agent.

The `AgentSupervisor`:

- **launches** `name-1`, `name-2`, … by spawning the orchestrator binary in
  `agent` mode with the template's params as start flags (Feature 7 then sets each
  child's start params);
- **scales** up (new slots) and down (stops the highest-numbered slots
  gracefully);
- **stops** via `desired_state = stop`, then hard-kills after a grace period;
- **relaunches** a crashed slot while the template is enabled, and marks a
  crash-looping slot `failed` rather than hot-looping;
- **autostarts** enabled templates on server boot and stops all on shutdown;
- caps the total managed instances at `max_managed_agents`.

It only manages processes it spawned — remote agents (registered externally) are
never scaled or stopped by the server. Instances are ordinary `agents` rows
linked back via `template_id` for grouping and cross-restart visibility.

Endpoints: `GET/POST /api/agent-templates`, `PATCH/DELETE
/api/agent-templates/{id}`, and `/scale`, `/start`, `/stop`.

---

## 11. Security & safety

- **Capabilities are the only authority.** Lifecycle actions (review, merge,
  task creation, project completion) are gated by role capabilities, never by
  skills. A reviewer can carry a `frontend` skill to review UI code, but skills
  never grant `handles_review` / `handles_merge` / `creates_tasks`.
- **Merge is human-or-deployer gated.** No actor merges without an explicit
  approval decision; conflict detection is a pre-merge guard, not an autonomous
  merger.
- **Self-improvement is bounded.** Auto-queue only runs while armed and active,
  and disarms on completion; `max_open_tasks` and the plan-round ceiling cap each
  activation. A human re-arm yields exactly one further bounded round.
- **Spawning processes is privileged.** Template create/scale/start endpoints
  launch OS processes with the server's own privileges and are capped by
  `max_managed_agents`. Managed agents execute task code locally — the same trust
  assumption as running an agent by hand on that host. Gate these endpoints behind
  the same admin authorization as other privileged operations.
- **Workspaces are ephemeral.** An agent removes its local checkout on every task
  exit, including on panic.

---

## 12. Operating agents

**Run the server:**

```
agent-orchestrator server --config config.yaml
```

**Run a single agent (its CLI flags become its start params):**

```
agent-orchestrator agent \
  --name worker-1 \
  --roles worker \
  --skills backend,go \
  --server http://localhost:8080 \
  --mode colocated
```

**Run several agents defined in config (`agents.definitions`, each with
`name`, `roles`, `skills`, `mode`):**

```
agent-orchestrator agents --config config.yaml
```

**Or define a template in the UI** (Managed page) and start *N* replicas without
touching a terminal.

Relevant config keys live under `agents:` — `heartbeat_interval_sec`,
`task_poll_interval_sec`, `task_timeout_sec`, `max_managed_agents`, and the
`definitions` list. Roles, skills, and project scope are all managed at runtime
through the UI; nothing here requires a redeploy.
