# Implementation Roadmap

Master plan folding the revised feature specs and remaining bugs into a dependency-ordered build sequence. This supersedes the planning portions of `tasks_2026-05-31.md`. Each work item links to its detailed spec.

## Progress

Checkbox legend: `[x]` done (implemented + tests green) — **no need to re-read its spec file**; `[ ]` not started.

- [x] **Phase 0** — Bug 8, Bug 10, Bug 11
- [x] **Phase 1** — Feature 3 (full) + Bug 9 B/C/D + full `Task.Type`/`task_types` removal
- [ ] **Phase 2** — Feature 2 (PRs) → Feature 5 (scope) → Feature 4 (auto-queue)
- [ ] **Phase 3** — Feature 6 (skills) → Feature 7 (lifecycle) → Feature 8 (managed agents)
- [ ] **Phase 4** — `AGENTS.md` documentation

Per-spec status:

| Spec | File | Status |
|---|---|---|
| Feature 2 (rev) | `feature-2-revised.md` | `[ ]` pending |
| Feature 3 (rev) | `feature-3-revised.md` | `[x]` done |
| Feature 4 | `feature-4-auto-queue.md` | `[ ]` pending |
| Feature 5 | `feature-5-scope-model.md` | `[ ]` pending |
| Feature 6 | `feature-6-skills-focus.md` | `[ ]` pending |
| Feature 7 | `feature-7-agent-lifecycle.md` | `[ ]` pending |
| Feature 8 | `feature-8-managed-agents.md` | `[ ]` pending |

## Status of the original task file

**Done (`[x]`):** Bugs 1–7, Feature 1.

**Superseded by revised specs:**
- Original **Feature 2** (PR workflow) → `feature-2-revised.md` (deployer reviews PRs, no auto-approve; merge only on explicit approval).
- Original **Feature 3** (capability roles) → `feature-3-revised.md` (capabilities resolved in Go, planner bootstrap, no `Task.Type`).
- **Bug 9 Part A** (review claim routing) → absorbed into Feature 3 Part C/D (capability-driven routing). Bug 9 Parts **B, C, D** remain to be done (executor submits review; timeout requeue to `AWAITING_REVIEW`; `AWAITING_REVISION → AWAITING_REVIEW` re-entry).

**Originally pending, now done:** Bug 8 `[x]`, Bug 9 B/C/D `[x]`, Bug 10 `[x]`, Bug 11 `[x]`.

## New feature specs

| Spec | File | Summary |
|---|---|---|
| Feature 2 (rev) | `feature-2-revised.md` | PR workflow; reviewer opens PR, deployer reviews & merges on explicit approval |
| Feature 3 (rev) | `feature-3-revised.md` | Capability-driven roles, remove task type, planner bootstrap, live role list |
| Feature 4 | `feature-4-auto-queue.md` | Bounded auto-queue: dependency gating + replenishment, self-stops on completion |
| Feature 5 | `feature-5-scope-model.md` | Description → requirements/features (planner-owned, role-gated context), completion signal |
| Feature 6 | `feature-6-skills-focus.md` | Configurable skill/focus dimension, orthogonal to roles |
| Feature 7 | `feature-7-agent-lifecycle.md` | Durable start config + runtime UI overrides + UI stop |
| Feature 8 | `feature-8-managed-agents.md` | Server-spawned co-located agents from a template (start N instances) |

## Dependency graph

```
                         ┌─────────────────────────────┐
                         │ Feature 3 (roles, no type,   │   ← the gate; almost everything depends on it
                         │ capabilities, bootstrap)     │
                         └───────────────┬─────────────┘
            ┌──────────────────────┬─────┴───────────────┬───────────────────────┐
            ▼                      ▼                     ▼                         ▼
   Track A: autonomy loop    Track B: agents      Feature 5 (scope)        meta/UI dropdowns
   ┌──────────────────┐      ┌──────────────┐     depends on F3            (live roles/skills)
   │ Bug 9 B/C/D      │      │ Feature 6     │            │
   │ (review submit / │      │ (skills)      │            ▼
   │  requeue / re-   │      └──────┬───────┘     Feature 4 (auto-queue)
   │  entry)          │             ▼             depends on F3 + F5 + F2
   └────────┬─────────┘      ┌──────────────┐
            ▼                │ Feature 7     │
   ┌──────────────────┐     │ (lifecycle)   │
   │ Feature 2 (PRs)  │     └──────┬───────┘
   │ dep: F3 + Bug9   │            ▼
   └──────────────────┘     ┌──────────────┐
                            │ Feature 8     │
                            │ (managed)     │
                            └──────────────┘

Independent cleanup (any time): Bug 8 → Bug 10 ; Bug 11
```

Key edges: F6 → F7 → F8 (skills field needed before lifecycle; lifecycle start/stop needed before the server spawns instances). F2 needs Bug 9's review submission to drive `AWAITING_REVIEW → AWAITING_MERGE`. F4 needs F2 (tasks reach `COMPLETED` via merge) and F5 (scope-backed completion signal). Bug 10 needs Bug 8 (agent identity in logs).

## Phased build order

### Phase 0 — Cleanup that the loop depends on (parallel, independent) ✅
- [x] **Bug 8** — agent name in `TaskLog` / `TaskComment` (stored in metadata + `author_name` column).
- [x] **Bug 10** — status-history timeline derived from status-change event logs.
- [x] **Bug 11** — provider form models table (+ "default (overridden per model)" labels).

### Phase 1 — Role/capability core *(the gate)* ✅
- [x] **Feature 3** in full: removed `Task.Type` + `TaskTypes` (columns + struct + type-routing subsystem); added `Capabilities`; `review_role` + capability-gated `AWAITING_REVIEW`/`AWAITING_MERGE` routing (absorbs Bug 9 Part A); live `/api/meta/task-roles`; planner `bootstrap_project` + `Project.Description`; updated `tools/plan.go`; seeded seven roles; UI (`Tasks.svelte`, `ProjectDetail.svelte`, `Roles.svelte`).
- [x] **Bug 9 B/C/D**: executor submits review verdict (by `REVIEWING` status + effective role); timeout requeue `REVIEWING → AWAITING_REVIEW`; `AWAITING_REVISION → AWAITING_REVIEW` re-entry (via existing develop path).

### Phase 2 — Autonomy loop (Track A)
1. [ ] **Feature 2** — PR workflow: reviewer opens PR on approval; deployer (`handles_merge`) claims `AWAITING_MERGE`, reviews, approves/rejects; approval merges + deletes branch + completes; workdir cleanup. *(dep: Phase 1)*
2. [ ] **Feature 5** — scope model: first-class requirements/features, `sync_scope`, auto-status, scope-backed completion; role-gated scope context via `context_include`. *(dep: Phase 1)*
3. [ ] **Feature 4** — bounded auto-queue: dependency gating in `GetNextTask`, `QueueSupervisor` replenishment, `complete_project` disarm, human re-arm for improvement rounds. *(dep: Phase 1 + F5 + F2)*

### Phase 3 — Agent specialization & management (Track B, parallelizable with Phase 2 after Phase 1)
1. [ ] **Feature 6** — skills/focus: `SkillDefinition` entity, `Agent.Skills`, `Task.Focus`, `ResolveAgentPersona` composition, soft routing match, Skills page. *(dep: Phase 1)*
2. [ ] **Feature 7** — agent lifecycle: start vs live config, routing reads live row, enriched heartbeat (control + persona), UI override/stop/reset. *(dep: F6)*
3. [ ] **Feature 8** — managed co-located agents: `AgentTemplate`, `AgentSupervisor` (spawn/scale/stop/relaunch/boot), template UI form with replica count. *(dep: F7)*

### Phase 4 — Documentation
- [ ] `AGENTS.md`: worker / review / PR-merge / planner-bootstrap+self-improvement / security patterns (Feature 3 Part L), plus skills and managed-agent operation.

## Recommended single-threaded sequence

If building one item at a time: ~~Bug 8~~ → ~~Feature 3 (+ Bug 9 B/C/D)~~ → **Feature 2** → Feature 5 → Feature 4 → Feature 6 → Feature 7 → Feature 8 → ~~Bug 10~~ → ~~Bug 11~~ → AGENTS.md. (Struck-through = done; **next up = Feature 2**.) This keeps every step on already-built foundations and reaches a working autonomous loop (through Feature 4) before starting the agent-management track.

## Cross-cutting verification

After each phase, run the full Go test suite plus the phase's new tests. Phase 2 should end with the integration test `TestFullWorkflow_E2E` (worker → reviewer PR → deployer merge → COMPLETED) green; Phase 1 with `TestAutoQueue_Terminates` deferred to Phase 2's Feature 4; Phase 3 with `TestManagedAgent_E2E`. Treat the existing `TestPushTransitionsToAwaitingReview` and merge integration tests as must-pass-unchanged regression guards throughout.
