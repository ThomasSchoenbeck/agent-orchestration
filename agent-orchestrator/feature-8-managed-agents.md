# Feature 8 — Server-managed co-located agents

**Status:** `[ ]` pending
**Related:** Feature 7 — agent lifecycle (start params, stop); Feature 6 — skills; Bug 3 — unified git transport.

---

## Goal

Let the server launch and manage co-located agent processes on its own host, so an operator can define an agent configuration once in the UI and start *N* instances of it without manually running CLI commands. The server becomes the launcher/supervisor for these agents; remote agents (started elsewhere) continue to work exactly as before.

"Co-located" here means **lifecycle managed by the server on the same host** — not a special git path. Per Bug 3 (unified transport), managed agents clone/push from the embedded git server over `http://localhost:{port}` just like any other agent. There is no filesystem shortcut.

## Entity — `AgentTemplate`

A reusable definition the server spawns instances from. New table `agent_templates`.

```go
type AgentTemplate struct {
    ID         string    `json:"id"`
    Name       string    `json:"name"`        // base name; instances get "-1", "-2" suffixes
    Roles      []string  `json:"roles"`       // start roles for spawned agents (Feature 7)
    Skills     []string  `json:"skills"`      // start skills (Feature 6/7)
    Replicas   int       `json:"replicas"`    // desired number of running instances
    Autostart  bool      `json:"autostart"`   // relaunch desired replicas when the server boots
    Enabled    bool      `json:"enabled"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

CRUD in a new `db/agent_templates.go`. Migration: `CREATE TABLE agent_templates (...)`.

Instances spawned from a template are ordinary agents (rows in `agents`), linked back via a nullable `template_id` column on `agents` so the UI can group them and the supervisor can reconcile. Each instance's name is stable per slot — `name-1`, `name-2`, … — so a relaunched instance re-registers to the same agent row (and, per Feature 7, resets its live config to the template's start params).

## The AgentSupervisor (process management)

A server-side component `workflow/agent_supervisor.go` that owns spawned processes.

- **Launch:** spawns the orchestrator's own binary (`os.Executable()`) in `agent` mode via `os/exec`, passing the template's params as start flags:
  `agent --name {template.Name}-{i} --roles {roles} --skills {skills} --server http://localhost:{port} --mode colocated`.
  Each child registers normally (Feature 7 sets its start params from these flags).
- **Track:** an in-memory map `templateID → []instance{slot, pid, cmd, agentID}`, mirrored to the `agents` rows (via `template_id`) for visibility across server restarts.
- **Scale:** changing `Replicas` up launches new slots; scaling down picks the highest-numbered slots and stops them.
- **Stop:** graceful via Feature 7 — set the instance's `desired_state = stop`; the agent finishes/aborts its task, goes offline, and exits. The supervisor waits a grace period, then hard-kills the process if it has not exited.
- **Crash handling:** if a tracked child exits unexpectedly while the template is enabled and below desired replicas, relaunch that slot (with backoff). A repeatedly-crashing slot is marked failed and surfaced in the UI rather than hot-looping.
- **Boot:** on server start, for each `enabled && autostart` template, launch up to `Replicas` instances. Reconcile against any `agents` rows still marked online from a previous run (re-adopt by name or stop-and-relaunch).
- **Shutdown:** on server shutdown, send `stop` to all managed instances and wait briefly before exit.

The supervisor only manages processes it spawned; remote agents are never touched.

## Endpoints

- `GET/POST /api/agent-templates` — list / create templates.
- `PATCH /api/agent-templates/{id}` — edit config or `replicas` (scaling triggers launch/stop).
- `DELETE /api/agent-templates/{id}` — stop all instances and remove the template.
- `POST /api/agent-templates/{id}/scale` — body `{replicas}` (convenience for the UI stepper).
- `POST /api/agent-templates/{id}/start` / `/stop` — start desired replicas / stop all instances.

## UI

A "Managed Agents" section (`ui/src/pages/Agents.svelte` or a new `AgentTemplates.svelte`):
- **Form** to define a template: name, roles (multi-select from `/api/meta/task-roles`), skills (from `/api/meta/skills`), replicas (number), autostart (checkbox).
- **Start / Stop / Scale** controls; a replica stepper that calls `scale`.
- Live list of running instances grouped under their template, each showing status (`online`/`stopping`/`offline`/`failed`), PID, and the per-instance Stop (Feature 7) for individual control.
- Clear separation between **managed** agents (started here) and **remote** agents (registered externally), since only managed ones can be scaled/relaunched by the server.

## Reconciliation with other features

- **Feature 7:** managed agents are normal agents — the template provides their start params; UI runtime overrides and per-instance stop work unchanged. Relaunch resets live → start (template values), which is the desired behavior.
- **Feature 6:** template `skills` become the instances' start skills; persona composition is identical.
- **Bug 3:** instances use the embedded git server over localhost; the obsolete colocated-worktree path is not reintroduced. `Agent.Mode` stays a display label only.

## Security / safety note

Spawning OS processes from a UI action is privileged. Gate template create/scale/start endpoints behind the same admin authorization as other privileged operations, cap total managed instances (a server-config `max_managed_agents`), and run children with the server's own privileges only. Document that managed agents execute task code locally — the same trust assumption as running an agent by hand on that host.

## Files to touch

- `db/models.go` — add `AgentTemplate`; add `TemplateID` to `Agent`
- `db/agent_templates.go` — new: CRUD
- `db/agents.go` — persist `template_id`; query instances by template
- `db/migrations.go` — `CREATE TABLE agent_templates`; `agents.template_id`
- `workflow/agent_supervisor.go` — new: spawn/track/scale/stop/relaunch/boot/shutdown
- `server/handlers.go` — agent-template CRUD + scale/start/stop endpoints (admin-gated)
- `server/server.go` — register routes; start the AgentSupervisor; wire graceful shutdown
- `config/config.go` — `MaxManagedAgents` cap; optional default templates
- `cmd/main.go` — ensure `agent` subcommand accepts `--skills`, `--server`, `--mode` (with Feature 7)
- `main.go` — construct/start AgentSupervisor; autostart enabled templates
- `ui/src/pages/Agents.svelte` (or new `AgentTemplates.svelte`) — template form, scale/start/stop, instance list
- `ui/src/lib/api.js` — template CRUD + scale/start/stop

## Tests

- `TestAgentTemplate_CRUDRoundTrip` — create/read/update/delete a template
- `TestSupervisor_StartsNInstances` — start with `replicas=3`; verify 3 child processes spawn and register as `name-1/2/3`
- `TestSupervisor_ScaleUp` — replicas 2→4 launches 2 more slots
- `TestSupervisor_ScaleDown` — replicas 4→2 stops the highest 2 slots gracefully
- `TestSupervisor_StopTemplate` — stop sets `desired_state=stop` on all instances; processes exit; rows go offline
- `TestSupervisor_RelaunchOnCrash` — kill a child; supervisor relaunches the same slot while enabled
- `TestSupervisor_CrashLoopMarksFailed` — repeated immediate crashes → slot marked failed, no hot-loop
- `TestSupervisor_AutostartOnBoot` — enabled+autostart template launches its replicas at startup
- `TestSupervisor_RespectsMaxManagedCap` — scaling beyond `max_managed_agents` is rejected
- `TestSupervisor_IgnoresRemoteAgents` — externally-registered agents are not stopped/scaled by the supervisor
- Integration `TestManagedAgent_E2E` — define template via API → start → instance claims and completes a task → scale to 0 → instance exits
