# Feature 6 — Skills/focus dimension: configurable agent specialization

**Status:** `[ ]` pending
**Related:** Feature 3 (revised) — roles & capabilities; Feature 5 — scope context.

---

## Goal

Let agents specialize by stack/technology (backend, frontend, go, react, infra, …) **without** baking focus into role names (which would explode the role list and complicate task picking). Focus is a separate, orthogonal dimension. Crucially, a skill is **not an opaque tag**: each skill is a configurable entity carrying its own prompt fragment ("focus/soul"), context rules, and optional tools, which an agent composes on top of its role(s). Skills are managed like roles, fully at runtime.

## The two axes

| Axis | Answers | Count | Lives on |
|---|---|---|---|
| **Role** (Feature 3) | function + lifecycle | small, stable (7) | task `role` (one) + agent `roles` (1+) |
| **Skill/focus** (this feature) | stack / technology / persona | many, free-form | agent `skills` (0+) + task `focus` (0+, optional) |

An agent is `roles × skills`: e.g. `roles=[worker], skills=[backend, go]` and another `roles=[worker], skills=[frontend, react]`. The role taxonomy never grows to accommodate tech; that's what skills are for.

## `SkillDefinition` — a configurable entity (mirrors `RoleDefinition`)

New table `skill_definitions`. Managed via a Skills page exactly like roles.

```go
type SkillDefinition struct {
    ID             string    `json:"id"`
    Name           string    `json:"name"`            // slug: "backend", "react"
    Label          string    `json:"label"`
    Description    string    `json:"description"`
    PromptFragment string    `json:"prompt_fragment"` // the "soul"/focus injected into the system prompt
    ContextInclude []string  `json:"context_include"` // globs/types added to the agent's context (e.g. server/**)
    ContextExclude []string  `json:"context_exclude"`
    AllowedTools   []string  `json:"allowed_tools"`   // optional tools added to the role's set
    Enabled        bool      `json:"enabled"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}
```

CRUD + scans in a new `db/skills.go` mirror `db/roles.go`. `SeedSkillDefinitions` seeds a starter set (`backend`, `frontend`, `infra`) idempotently by name. Skills carry **no capabilities** — lifecycle authority comes only from roles, keeping the security model in the role layer.

## Agent gains `Skills`; task gains `Focus`

- `db.Agent` adds `Skills []string` (the skill names the agent provides). Registered via CLI `--skills backend,go` and editable wherever agent roles are managed. (`Agent.Capabilities` is a free-form map and could hold this, but a typed `Skills` field is clearer and queryable.)
- `db.Task` adds `Focus []string` (`json:"focus,omitempty"`). Migration: `ALTER TABLE tasks ADD COLUMN focus TEXT NOT NULL DEFAULT '[]'`. Optional; empty means "any agent with the role."

## Composition — role ⊕ skills

An agent's effective persona is its role(s) plus its skills, resolved where the LLM request is built (the role's `SystemPrompt` and `BuildContextForRole` today):

- **System prompt** = `role.SystemPrompt` + each assigned skill's `PromptFragment`, appended in stable order. This is the "soul" layering: a backend-go worker's prompt is the worker prompt plus the backend fragment plus the go fragment.
- **Context include/exclude** = union of the role's rules and every assigned skill's rules. So a `frontend` skill adding `ui/**` makes the agent see the UI tree; reuses the Feature 5 / `BuildWithRules` machinery unchanged.
- **Allowed tools** = union of the role's allowlist and the skills' `AllowedTools`.
- **Capabilities** = from roles only (skills add none).

The agent's skill fragments are always applied (they define *who the agent is*), independent of any individual task's `focus`; `focus` is purely a routing filter. Add a `ResolveAgentPersona(role *RoleDefinition, skills []*SkillDefinition)` helper that returns the merged prompt, context rules, and tool set; call it from the prompt-build path in `agent/executor.go` / `router`.

## Routing — one optional clause

`GetNextTask` keeps the role/capability rules from Feature 3 and adds, on the work-claim branches:

- task `focus` empty → no change (any role-matching agent claims it).
- task `focus` non-empty → claim only if `task.focus ⊆ agent.skills` (agent has *all* required focus tags).

Resolved in Go alongside the existing capability resolution (the agent's skills are already in hand from the poll). Empty focus is the common case, so simple projects never encounter the extra rule — the pick workflow stays "role required, focus optional."

## Management & UI

- **Skills page** (`ui/src/pages/Skills.svelte`) — CRUD for `SkillDefinition`: name, label, description, prompt fragment (textarea — the editable "soul"), context include/exclude, allowed tools, enabled. Mirrors the Roles page.
- **Agent config** — assign skills alongside roles (CLI flag now; an agent-management surface later). The Roles page already configures role internals; skills get their own equivalent.
- **Task form** (`Tasks.svelte`, Feature 3 Part F) — an optional **focus** multiselect populated from enabled skills, beside the role dropdown. Empty by default.
- **`GET /api/meta/skills`** — enabled skills for populating the focus multiselect (parallels `task-roles`).

## Interaction with other features

- **Feature 5 (scope context):** scope visibility stays role-driven via `context_include`; skill `context_include` merges in via the same union, so a backend worker can be shown `server/**` without being shown the project roadmap. No conflict.
- **Feature 3 (capabilities):** unchanged — skills never grant `handles_review` / `creates_tasks` / etc. A reviewer can also carry a `frontend` skill to review UI code; routing still gates review by `handles_review` + `review_role`, and `focus` can additionally require the reviewer be frontend-skilled.

## Files to touch

- `db/models.go` — add `SkillDefinition`; add `Skills []string` to `Agent`; add `Focus []string` to `Task`
- `db/skills.go` — new: CRUD + scans + `SeedSkillDefinitions` (mirror `db/roles.go`)
- `db/agents.go` — persist `Skills`
- `db/tasks.go` — `focus` in CreateTask + scans; focus-subset clause in `GetNextTask`
- `db/migrations.go` — `CREATE TABLE skill_definitions`; `agents.skills`; `tasks.focus`
- `api/types.go` — `Focus` on `CreateTaskRequest`; agent registration accepts `skills`
- `router/router.go`, `router/context.go` — `ResolveAgentPersona` merging role ⊕ skills for prompt/context/tools
- `agent/executor.go` — build the request from the merged persona; agent passes its skills
- `agent/agent.go` / `agent/client.go` — send `skills` on registration and `get_next_task`
- `server/meta.go`, `server/server.go` — `GET /api/meta/skills`
- `server/handlers.go` — skill CRUD endpoints; accept `focus` on task create; resolve `Skills` on agent register
- `cmd/main.go` (agent CLI) — `--skills` flag
- `main.go` / seed file — `SeedSkillDefinitions` starter set; seed agents with skills if applicable
- `ui/src/pages/Skills.svelte` — new management page
- `ui/src/pages/Tasks.svelte` — focus multiselect
- `ui/src/lib/api.js` — skills CRUD; `getSkills`; focus on task payload; skills on agent

## Tests

- `TestSkillDefinition_CRUDRoundTrip` — create/read/update a skill incl. prompt fragment + context rules
- `TestSeedSkills_Idempotent` — second seed adds no duplicates
- `TestResolveAgentPersona_MergesPromptAndContext` — role + two skills → system prompt contains role prompt + both fragments; context include is the union
- `TestResolveAgentPersona_ToolsUnion` — skill `AllowedTools` added to role's set
- `TestResolveAgentPersona_SkillsAddNoCapabilities` — capabilities come only from roles
- `TestGetNextTask_FocusSubsetMatch` — task focus `[frontend]`; agent with skills `[frontend,react]` claims; backend agent does not
- `TestGetNextTask_EmptyFocusUnrestricted` — task with no focus claimable by any role-matching agent regardless of skills
- `TestGetNextTask_FocusRequiresAllTags` — task focus `[frontend,react]`; agent with only `[frontend]` does not claim
- `TestMetaSkills_LiveFromDB` — `GET /api/meta/skills` returns only enabled skills
- UI: Skills page edits a prompt fragment; task form focus multiselect populated from enabled skills
