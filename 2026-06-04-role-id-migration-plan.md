# Role ID Migration — Plan (2026-06-04)

Goal (Task 9): make a role's **id** the canonical identity and reference everywhere,
so the **name** becomes a freely-editable attribute. Done as an expand → migrate →
contract migration so the app keeps working (and is testable) after every phase.

Progress: `[x]` done · `[ ]` pending · `[~]` in progress

- [x] Phase 1 — Foundation: id↔name resolvers + safe rename (cascade) + unlock UI (tests green)
- [~] Phase 2 — id-tolerant matching (accept name OR id) — implemented, pending test run
- [x] Phase 3 — Writers store ids (register + task create) + UI id→name display (tests green)
- [x] Phase 4 — Contract: pure id-only matching, tolerance removed (tests green)

> Run `cd agent-orchestrator && go test ./...` and `cd agent-orchestrator/ui && pnpm test`
> at the end of EVERY phase before starting the next.

---

## Current coupling (why this is phased)
Role **name** is the de-facto key. Names are stored in: `providers.roles` +
`providers.models[].roles`, `agents.roles`/`start_roles`, `agent_templates.roles`,
`tasks.role`/`review_role`. Matching/routing is name-keyed across ~13 files
(`router`, `llm/registry`, `db/tasks` GetNextTask, `agent`, `server/*`, `tools`,
`main`). Agents are launched/registered/poll by name; config seeds roles by name.
A role name can also be referenced with **no** role-definition row (built-in
taxonomy + fallbacks) — Phase 3/4 must enforce "every referenced role has a row".

---

## [x] Phase 1 — Foundation + safe rename (DONE — tests green)
Make rename work today without breaking references, and add the id↔name resolver
layer every later phase needs. References stay name-based but are kept consistent
on rename (cascade), which is the scaffolding that lets Phases 2–4 backfill ids
from a consistent name graph.

**Do:**
1. `db.RoleIDByName(ctx, name)` and `db.RoleNameByID(ctx, id)` — resolver helpers.
2. `db.RenameRoleReferences(ctx, oldName, newName)` — read-modify-write cascade
   across providers (roles + models[].roles), agents (roles + start_roles),
   agent_templates (roles), tasks (role + review_role).
3. Roles PUT handler: when the name changes, call RenameRoleReferences (after the
   role-definition update) then `router.ReloadFromDB`.
4. UI: remove `readonly` on the Name field in edit mode + a hint.

**Files:** `db/roles.go`, new `db/role_rename.go`, `server/roles_handler.go`,
`ui/src/pages/Roles.svelte`; tests: new `db/role_rename_test.go`, `ui/src/__tests__/Roles.test.js`.

**Verify:** db test renames a role used by a provider/agent/template/task and
asserts every reference now holds the new name; UI test asserts the Name field is
editable when editing.

**Note on atomicity:** the cascade is sequential read-modify-write via existing
Update methods (the per-table updaters aren't tx-aware). Acceptable for this
local single-writer DB; Phase 4 removes the need entirely.

---

## [~] Phase 2 — id-tolerant matching (accept name OR id) — implemented
**Revised approach (vs original):** readers can't move to id without writers
moving too (matching compares two ref sets). So instead of switching readers to
id, Phase 2 makes matching *tolerant* — a role referenced by name or id matches
interchangeably. This breaks nothing and unblocks switching writers in Phase 3.
**Done:** `db.ExpandRoleRefs(ctx, refs)` (ref → both name+id); `GetNextTask`
expands the agent's roles for the `role IN (...)` clause; `capabilityRoles`
matches by name-or-id and emits both forms (review path); router gains a
`rolesByID` index and `RouteByRole` resolves by id too.
**Files:** `db/role_rename.go`, `db/tasks.go`, `router/router.go`; tests:
`db/role_rename_test.go`, `router/router_test.go`.
**Verify:** `go test ./...` — existing name-based tests still pass + new id tests.

## [x] Phase 3 — Writers store ids + UI display mapping — DONE (tests green)
**Scope (kept tight):** resolve role refs → ids only at the two matching-relevant
write boundaries — **agent registration** and **task creation** (`db.ResolveRoleRefs`).
Provider/template role lists stay as names (still tolerated + displayed directly),
so only agent/task role displays needed id→name mapping.
**Done — backend:** `db.ResolveRoleRefs`; wired into `handleAgentRegister` +
`handleTasks` POST; `metaItem.ID` added so `/api/meta/task-roles` carries the id.
**Done — UI:** `lib/roles.js` `roleLabel(ref, defs)` (id|name → name, fallback to
ref); `MultiSelect` gains a `labelFor` prop for chip display; role displays mapped
in `Agents` (chips + live-edit MultiSelect), `AgentDetail`, `Tasks`, `TaskDetail`.
**Files:** `db/role_rename.go`, `server/handlers.go`, `server/meta.go`,
`ui/src/lib/roles.js`, `ui/src/components/MultiSelect.svelte`,
`ui/src/pages/{Agents,AgentDetail,Tasks,TaskDetail}.svelte`; tests:
`db/role_rename_test.go`, `ui/src/__tests__/{roleLabel,MultiSelect}.test.js`,
`AgentDetail.test.js` (mock).
**Verify:** `go test ./...` + `pnpm test`.

## [x] Phase 4 — Contract: pure id-only matching — DONE (tests green)
**Decision (user):** go fully id-only and wipe old dummy data (config rework will
seed basic roles out of the box).
**Done:** removed `ExpandRoleRefs` (the name↔id bridge); `GetNextTask` matches the
agent's role ids directly; `capabilityRoles` matches by id and emits ids;
`defaultReviewRole` returns a role **id** (or "" when none). `roleLabel` (id→name
display) and the router's name+id resolution stay (needed for hardcoded
"orchestrator" routing and human/CLI input). Rewrote the routing test suite to be
id-based (`getnexttask_routing_test.go`, `role_rename_test.go`).
**Invariant now enforced:** a role must have a definition (with an id) to be
matched — ad-hoc/undefined role names no longer route. Existing name-based dummy
data must be recreated (re-register agents, recreate tasks) or the DB reset.
**Files:** `db/tasks.go`, `db/role_rename.go`, tests.
**Verify:** `go test ./...` green.
