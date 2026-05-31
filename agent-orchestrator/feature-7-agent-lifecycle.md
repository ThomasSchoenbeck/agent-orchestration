# Feature 7 — Agent lifecycle: durable start config + runtime UI overrides + stop

**Status:** `[ ]` pending
**Related:** Feature 6 — skills/focus; Feature 3 — roles/capabilities.

---

## Goal

An agent is preconfigured from the CLI and/or a config file (its **start params**). While it is running, an operator can change its roles/skills from the UI and stop it from the UI. UI changes are **runtime-only**: when the process stops or dies and relaunches, it re-registers and honors its start params again, discarding any prior override.

The poll-based architecture makes this clean — the server already sees the agent ID on every poll/heartbeat, so all control is pull-based; nothing needs to connect *into* the agent.

## Two config layers

Add to `db.Agent`:

```go
// Durable start params — captured from CLI/config at every registration.
StartRoles   []string `json:"start_roles"`
StartSkills  []string `json:"start_skills"`
// Live (effective) values — initialised to the start params on registration,
// mutable at runtime via the UI. Routing and persona use these.
Roles        []string `json:"roles"`     // existing field, now "live"
Skills       []string `json:"skills"`    // from Feature 6, now "live"
// Control.
DesiredState string   `json:"desired_state"` // run | stop  (default: run)
```

Migration: `ALTER TABLE agents ADD COLUMN start_roles TEXT NOT NULL DEFAULT '[]'`, `start_skills TEXT NOT NULL DEFAULT '[]'`, `desired_state TEXT NOT NULL DEFAULT 'run'`.

### Registration semantics (the reset rule)

`RegisterAgent` (called on every process start, including restart) sets, atomically:
- `StartRoles = StartSkills =` the payload's roles/skills (from CLI/config),
- `Roles = StartRoles`, `Skills = StartSkills` (live reset to start),
- `DesiredState = run`.

So any runtime override or prior `stop` is wiped on restart — the agent always comes back as its start params describe. Agents are matched across restarts by `name` (re-register updates the existing row rather than creating a duplicate; today registration already keys agents and `GetAgentByName` exists).

## Routing reads live config from the DB row

Today the poll is `GET /api/agents/{id}/tasks/next?roles=...` — the agent self-reports roles in the query string. Change the handler to **ignore the query param and load the agent's live `Roles` + `Skills` from its DB row** (the `{id}` is already in the path), then pass those into `GetNextTask`. This is the single behavioral change that makes UI overrides take effect on the next poll. The `roles` query param is kept accepted-but-ignored for backward compatibility, or dropped.

## Control + persona via the heartbeat response

The heartbeat (`POST /api/agents/{id}/heartbeat`) currently returns nothing. Enrich its response:

```json
{
  "desired_state": "run",
  "roles":  ["worker"],
  "skills": ["backend", "go"]
}
```

On each heartbeat the agent:
- **Honors `desired_state`:** if `stop`, it finishes (or aborts) its current task, calls the existing `SetOffline`, and exits the process. (If `pause` is later added: stop claiming new tasks but keep heartbeating.)
- **Recomposes persona:** if the live `skills`/`roles` differ from what it currently holds, it rebuilds its effective system prompt/context via the Feature 6 `ResolveAgentPersona` path before the next task. This is how a UI skill change mid-run reaches the agent.

Because routing already uses the DB row (above), role/skill overrides affect *what the agent is offered* immediately; the heartbeat sync only matters for the agent's own prompt composition.

## UI

Agent detail / list (`ui/src/pages/Agents.svelte`):
- Edit **live** roles and skills (multi-selects from `/api/meta/task-roles` and `/api/meta/skills`). Saving calls `PATCH /api/agents/{id}` (live fields only).
- A **"runtime override"** indicator when live ≠ start, with the start values shown and a "reset to start params" action (sets live = start without restarting).
- A **Stop** button → `POST /api/agents/{id}/stop` (sets `desired_state = stop`). Status reflects `stopping → offline` as the agent winds down.
- Show start params (read-only) vs live values so the distinction is visible.

## Endpoints

- `PATCH /api/agents/{id}` — update **live** `roles` / `skills` only (never `start_*`).
- `POST /api/agents/{id}/stop` — set `desired_state = stop`.
- `POST /api/agents/{id}/reset` — set live = start params (runtime, no restart).
- Heartbeat response enriched as above.

## Config-file preconfiguration

Agents may be defined in a config file consumed by the agent process at launch (name, roles, skills, server URL, mode). The server is agnostic to the source — whatever the agent sends at registration becomes its start params. No server-side agent registry file is required; the durable definition lives with the agent launcher (CLI flags or its config file).

## Files to touch

- `db/models.go` — add `StartRoles`, `StartSkills`, `DesiredState` to `Agent` (`Skills` from Feature 6)
- `db/agents.go` — persist new columns; `RegisterAgent`/`CreateAgent` reset live = start + `desired_state=run`; add `SetAgentDesiredState`, `UpdateAgentLiveConfig`, `ResetAgentToStart`
- `db/migrations.go` — new agent columns
- `api/types.go` — `RegisterAgentRequest` carries `skills`; heartbeat response type; `PATCH` agent request
- `server/handlers.go` (agents) — registration reset rule; `next` reads live row not query param; `PATCH` / `stop` / `reset` endpoints; heartbeat returns control + live config
- `agent/client.go` — `Heartbeat` decodes the response; add stop handling; send `skills` on register
- `agent/agent.go` — poll loop honors `desired_state`; recomposes persona on live config change; graceful stop → `SetOffline` → exit
- `cmd/main.go` — agent CLI gains `--skills`; optional `--config` file load for start params
- `ui/src/pages/Agents.svelte` — live edit, override indicator, Stop, reset, start-vs-live display
- `ui/src/lib/api.js` — `updateAgent`, `stopAgent`, `resetAgent`

## Tests

- `TestRegister_ResetsLiveToStart` — register with roles `[worker]`; override live to `[worker,reviewer]`; re-register → live back to `[worker]`, `desired_state=run`
- `TestPatchAgent_UpdatesLiveNotStart` — PATCH live roles; `start_roles` unchanged
- `TestGetNextTask_UsesLiveRolesFromRow` — override live roles via PATCH; next poll offers tasks for the new roles, ignoring the query param
- `TestStopAgent_SetsDesiredState` — `POST /stop` sets `desired_state=stop`; heartbeat response carries it
- `TestAgentHonorsStop` — agent loop receives `stop` on heartbeat → calls `SetOffline` and exits
- `TestResetAgent_LiveEqualsStart` — after override, `POST /reset` makes live = start without restart
- `TestHeartbeatResponse_CarriesLiveConfig` — heartbeat returns current live roles/skills
- `TestAgentRecomposesPersonaOnSkillChange` — live skills change → agent's resolved system prompt includes the new skill fragment (ties to Feature 6)
