# Role ID Migration — Plan (2026-06-04)

Goal (Task 9): make a role's **id** the canonical identity and reference everywhere,
so the **name** becomes a freely-editable attribute. Done as an expand → migrate →
contract migration so the app keeps working (and is testable) after every phase.

Progress: `[x]` done · `[ ]` pending · `[~]` in progress

- [x] Phase 1 — Foundation: id↔name resolvers + safe rename (cascade) + unlock UI (tests green)
- [ ] Phase 2 — Migrate readers (routing/matching) to resolve via id
- [ ] Phase 3 — Migrate writers (CLI, registration, config seed, task create) to id
- [ ] Phase 4 — Contract: store ids in reference fields, drop name-based fallback

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

## [ ] Phase 2 — Migrate readers to id
Introduce id-based resolution in the routing/matching layer while storage stays
name-based (resolve name→id at the boundary). Switch `router.rolesByName` to a
canonical id map; `RouteByRole` resolves name→id→definition. Keep name lookups as
a thin shim. **Verify:** existing router/registry/GetNextTask tests still pass.

## [ ] Phase 3 — Migrate writers to id
Agent registration, `--roles`, task create, provider/template editors, config seed
write/accept role ids (names resolved at the edge). Backfill: ensure every
referenced role name has a role-definition row (migration). **Verify:** end-to-end
agent pickup test with id-based roles.

## [ ] Phase 4 — Contract
Store ids in `providers.roles`/`models[].roles`, `agents.roles`/`start_roles`,
`agent_templates.roles`, `tasks.role`/`review_role` (migration converts existing
name data → ids). Remove name-based fallbacks and the rename cascade (no longer
needed — refs are ids). **Verify:** full suite + manual smoke.
