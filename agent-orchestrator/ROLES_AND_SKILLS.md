# Roles & Skills — quick reference

The currently-supported, seeded set, plus the capability and tool vocabularies you
assign to a role. Everything is editable at runtime (Roles page / Skills page).
See `AGENTS.md` for the full model.

A role has two independent fields you set in the form:
**Capabilities** (authority flags) and **Allowed tools** (the functions the LLM
can call). An agent inherits a role's capabilities just by holding that role.

---

## Role setup — what to put in each role

| Role | Capabilities | Allowed tools (use these exact names) |
|---|---|---|
| `worker` | — | `read_file, write_file, list_files, apply_diff, run_tests, task_comment` |
| `reviewer` | `handles_review` | `read_file, list_files, task_comment` |
| `planner` | `creates_tasks` | `bootstrap_project, sync_scope, complete_project, create_work_package, plan_project, list_tasks, query_context, save_context, task_comment` |
| `researcher` | `creates_tasks` | `query_context, save_context, create_work_package, task_comment` |
| `designer` | `creates_tasks` | `read_file, write_file, create_work_package, task_comment` |
| `security` | — | `read_file, list_files, run_tests, task_comment` |
| `deployer` | `handles_merge`, `handles_deploy` | `read_file, run_tests, task_comment` |

Plus a provider/model on each role. (The merge action itself is the approve/reject
endpoint, not a tool — `handles_merge` is what lets the deployer claim the PR gate.)

---

## Capabilities (authority — only these four do anything)

| Capability | What it grants |
|---|---|
| `creates_tasks` | Plan work and edit scope: required for `complete_project`; the authority behind `bootstrap_project` / `sync_scope` / `create_work_package`. |
| `handles_review` | Claim work-review tasks (`AWAITING_REVIEW`). |
| `handles_merge` | Claim the PR / merge gate (`AWAITING_MERGE`) and approve/reject the merge. |
| `handles_deploy` | Deploy actions (semantic marker for the deployer). |

Skills never carry capabilities — authority lives only on roles.

---

## Tools catalog (the real registered names)

| Tool | Purpose |
|---|---|
| `read_file`, `write_file`, `apply_diff`, `list_files` | Read/edit files in the worktree |
| `run_tests` | Run the project's test suite |
| `git_clone`, `git_checkout` | Low-level git (rarely needed — cloning is automatic) |
| `task_comment` | Post a comment on the current task |
| `query_context`, `save_context` | Read / write the project memory (context store) |
| `list_tasks`, `get_next_task`, `submit_task_result` | Task lifecycle (the last two are driven by the agent loop) |
| `plan_project`, `create_work_package` | Create tasks (planning) |
| `bootstrap_project`, `sync_scope`, `complete_project` | Scope: derive/reconcile requirements & features, complete a project |

> Note: `web_search` and `run_command` are **not** shipped tools — don't add them
> to an allowlist (they'll be silently ignored). Tool names must match exactly.

---

## Skills (specializations, orthogonal to roles)

| Skill | Focus | Default context |
|---|---|---|
| `backend` | Server-side, APIs, data, business logic | `server/**`, `db/**`, `api/**` |
| `frontend` | UI, components, client-side state | `ui/**` |
| `infra` | Build, CI/CD, deployment, configuration | `deploy/**`, `config/**`, `.github/**` |

An agent is `roles × skills` (e.g. `roles=[worker] skills=[backend]`). A task may
optionally set a `focus` (e.g. `[frontend]`); it is then only offered to agents
whose skills include every focus tag. Empty focus = any role-matching agent.
Skills add a prompt fragment + context globs + (optionally) tools, but no
capabilities.
