# Agent Orchestration Platform — Project Specification

> Working codename: **Forge** (an edge-deployable forge for autonomous software construction).
> Document status: **v0.3 — sharpened agent/platform boundary: platform coordinates, agents execute end-to-end, code stays in git.**
> Owner: Thomas Schoenbeck.
> License intent: source-available (final license TBD before public release).

---

## 1. Executive Summary

Forge is a self-hostable, edge-deployable platform for orchestrating autonomous AI software-engineering agents. It ships as a **single Go binary** that can be invoked in different modes (`server`, `agent`, `migrate`, `doctor`, `init`), embeds its own database, embeds its own Svelte UI, embeds its own MCP server, and exposes a provider-agnostic catalog of LLM providers and external MCP tools that agents can use to do real work.

There is a clean split of responsibilities:

- **The platform manages.** Projects, work packages, tasks, agents, LLM provider catalog, MCP server catalog, prompts, configuration, observability, audit, UI, chat brain. It does not run AI models, does not check out code, does not run builds or tests. It needs no GPU.
- **Agents execute.** Once an agent claims a task, it does the entire job end-to-end: it clones / fetches the project from its git storage, talks to LLMs through the providers it has been told about, executes tool calls (web search, REST, external MCP servers), produces and applies diffs, runs the verification harness locally, commits and pushes results to git, and streams logs and metrics back to the platform.
- **Code stays in git.** No source code is ever exchanged between platform and agents over the agent channel. Both treat the project's git remote (external or the platform's embedded git server) as the source of truth. The platform-agent channel carries metadata, logs, metrics, and configuration only.

The platform drives a strict task lifecycle (`plan → design → implement → verify → review → integrate → done`) and consumes the harness reports the agents produce, with audit and review gates on top.

The platform is designed to (a) be safe and auditable enough to run unattended for long periods and (b) be capable of improving itself: once Forge is bootstrapped, new features for Forge are themselves planned and implemented through Forge.

---

## 2. Vision & Goals

### 2.1 Vision

A single, secure, auditable, edge-deployable binary that anyone can drop on a laptop, a homelab box, or a small VM and use to plan, build, review, test, and ship software through a fleet of AI agents — with the platform itself becoming the meta-tool that builds and improves itself.

### 2.2 Primary goals

- **One binary, multiple modes.** Server, agent, migrate, doctor, init — all from the same artifact.
- **Clean responsibility split.** Platform = coordination, configuration, observability, UI. Agents = end-to-end execution of tasks. Code stays in git.
- **Provider-agnostic.** Any chat-capable LLM with tool calling can drive the system; any embeddings-capable model can power retrieval. Providers are configured once on the platform and propagated to agents.
- **Self-contained but pluggable.** Embedded UI, embedded SQLite, embedded MCP, embedded scheduler, optional embedded git server. Heavier components (DB, project storage) can be swapped for external services without code changes.
- **Spec-driven.** Every feature begins with a spec; specs live in the repo; the workflow refuses to implement without them.
- **Verifiable.** Every agent output is gated by a deterministic verification harness (run by the agent, audited by the platform) before it can move forward.
- **Secure by default.** Sandboxed agent-side execution, encrypted secrets at rest, signed artifacts, least-privilege agents, complete audit trail, no code crossing the agent ↔ platform channel.
- **Measurably autonomous.** The platform's reason for existing is to do work without human steering. Its own metrics surface track autonomy, not just uptime.
- **Self-improving.** The platform can pick up tasks against its own repository and ship updates to itself under human approval.

### 2.3 Non-goals (for v1)

- A managed cloud service. (Self-hosting only for v1.)
- A VS Code / JetBrains plugin. (Forge's own UI is the IDE surface.)
- Multi-tenant SaaS isolation. (Single-tenant per server instance for v1.)
- A general-purpose chat product. (Chat exists, but it is purpose-built for orchestration.)
- Replacement for source control or CI. Forge integrates with Git and CI; it does not replace them.

---

## 3. Glossary

| Term | Definition |
|---|---|
| **Server** | The Forge binary running in `server` mode: API, MCP server, UI, DB, scheduler. |
| **Agent** | The Forge binary running in `agent` mode: pulls tasks, executes runs, reports results. |
| **Project** | A long-lived unit of work, typically backed by a Git repository, with its own context and configuration. |
| **Plan** | A structured, machine-readable design artifact for a project or work package. |
| **Work Package** | A coherent slice of a plan, scoped small enough that a single agent can implement it within a context window. |
| **Task** | A single unit of work with a type (`plan`, `implement`, `review`, `verify`, …) and a lifecycle. |
| **Run** | A concrete attempt at executing a task. A task may have many runs. |
| **Role** | A logical capability label (`orchestrator`, `worker`, `reviewer`, `ui_designer`, `system_designer`, `embedder`, `reranker`, `chat_brain`). |
| **Provider** | A configured LLM backend (Anthropic, Azure, Ollama, LM Studio, OpenAI-compat, etc.). |
| **Model** | A named binding of `(provider, model_id, parameters)` mapped to one or more roles. |
| **Routing rule** | A mapping from `task type` (or other selectors) to a role, model, prompt, and context recipe. |
| **Prompt** | A versioned, named template with typed inputs. |
| **Context recipe** | A declarative description of which sources (files, summaries, embeddings, diffs, terminal output) to include for a given task. |
| **Harness** | The deterministic verification layer that runs between every "agent says it's done" and "the system believes it." |
| **Tool (MCP)** | A capability exposed by the server's MCP backend (e.g. `plan_project`, `claim_task`, `submit_result`). |
| **Chat Brain** | The tool-calling LLM that drives the orchestration chat, dispatches to roles via tools. |
| **Sandbox** | The isolated execution environment in which the harness runs repo commands, builds, and tests on the server side. |
| **Memory / Context Store** | The DB-backed store of summaries, embeddings, snapshots, and notes used to assemble context for runs. |
| **Project storage backend** | Where the project's source files live: a local folder, an external git remote (GitHub/GitLab/Gitea/Bitbucket), or the platform's own embedded git server. |
| **DB backend** | Where the platform's metadata lives: embedded SQLite (default) or an external SQL database (Postgres, MySQL/MariaDB). |
| **Efficacy metric** | A KPI that measures the platform's success at being autonomous (e.g. % of tasks completed without human intervention, mean attempts per task, repair-loop length). |

---

## 4. Guiding Principles

1. **Platform manages, agents execute.** The platform never compiles, tests, or runs project code. It coordinates and observes. Agents own the execution loop end-to-end.
2. **Code stays in git.** Source code is never transferred over the platform-agent channel. Both sides treat the project's git remote (external or embedded) as the only place code lives.
3. **Specs before code.** No work package starts implementation without a written, versioned spec stored in the project's git repo.
4. **Determinism wraps non-determinism.** Every LLM call inside an agent is bracketed by deterministic input assembly and a deterministic verification harness afterward.
5. **Small surface, deep core.** A small number of orthogonal concepts (project, task, run, role, provider, MCP server, prompt, context, harness, agent) compose into everything.
6. **One binary.** Anything that requires installing more than `forge` is a design failure unless explicitly justified.
7. **Local-first.** Defaults must work offline, against local models, on a single machine. Networked features are opt-in.
8. **Pull-first, push-when-needed.** Agents pull tasks from a queue; the server may push targeted assignments and may interrupt.
9. **Configuration distributed, not duplicated.** LLM providers and MCP servers are configured once on the platform and shared with agents (versioned, cached, refreshed on change). Agents never need their own copy of credentials in config files on disk.
10. **Idempotent, resumable, observable.** Any task or workflow can be safely retried; every agent action is streamed to the platform with enough detail to reconstruct it.
11. **Least privilege.** Agents only receive the providers, MCP servers, secrets, and project credentials that the current task needs.
12. **Audit everything.** Every decision (which model, which prompt, which tool call, which provider config version) is recorded on the platform.
13. **Self-improvement requires guardrails.** When Forge edits Forge, additional human-approval gates and twin-agent verification apply.

---

## 5. System Architecture

### 5.1 Top-level shape

```
                                 ┌──────────────────────────────────────────┐
                                 │           Operator (Human)               │
                                 └───────────────┬──────────────────────────┘
                                                 │ HTTPS (browser)
                                 ┌───────────────▼──────────────────────────┐
                                 │     Embedded Svelte UI (served by Server) │
                                 └───────────────┬──────────────────────────┘
                                                 │ JSON REST + WS
┌────────────────────────────────────────────────▼─────────────────────────────────────────┐
│                          forge server  (management plane, no GPU)                         │
│                                                                                          │
│   ┌────────────┐ ┌────────────┐ ┌─────────────┐ ┌────────────┐ ┌─────────────────────┐   │
│   │  HTTP API  │ │  MCP API   │ │ Chat Brain  │ │ Scheduler  │ │ Config Distributor   │   │
│   └─────┬──────┘ └─────┬──────┘ └──────┬──────┘ └─────┬──────┘ └─────────┬───────────┘   │
│         │              │               │              │                  │                │
│   ┌─────▼─────┐ ┌──────▼──────┐ ┌──────▼──────┐ ┌─────▼──────┐ ┌─────────▼───────────┐   │
│   │ Projects  │ │  Tasks /    │ │ Audit /     │ │ Provider & │ │ Observability Sink   │   │
│   │ Service   │ │  Runs Svc   │ │ Review      │ │ MCP Catalog│ │ (logs/metrics/traces)│   │
│   └─────┬─────┘ └──────┬──────┘ └──────┬──────┘ └─────┬──────┘ └─────────┬───────────┘   │
│         │              │               │              │                  │                │
│         └──────┬───────┴──────┬────────┴──────┬───────┴────────┬─────────┘                │
│                │              │               │                │                          │
│       ┌────────▼─────────┐ ┌──▼───────────┐ ┌──▼──────────┐ ┌─▼───────────────┐          │
│       │ Pluggable SQL DB │ │ Secrets KMS  │ │ Blob Store  │ │ Embedded Git    │          │
│       │ (SQLite | PG |MY)│ │ (sealed)     │ │ (FS or S3)  │ │ Server (opt)    │          │
│       └──────────────────┘ └──────────────┘ └─────────────┘ └─────────────────┘          │
└─────────────────┬──────────────────────────────────────────────────┬──────────────────────┘
                  │                                                  │
                  │ control plane (REST + WS, mTLS, signed)          │ git smart-http (opt)
                  │ task pull, result post, log/metric stream        │
                  │ provider+MCP catalog sync, heartbeat             │
                  ▼                                                  ▼
┌────────────────────────────────────────────────────────────────┐  ┌──────────────────────┐
│                       forge agent  (execution plane)            │  │ External git remote  │
│                                                                 │  │ (GitHub/GitLab/Gitea │
│  ┌──────────────┐  ┌─────────────────┐  ┌──────────────────┐    │  │  Bitbucket / SSH)    │
│  │ Task Loop    │  │ Provider+MCP    │  │ Local Sandbox    │    │  └──────────┬───────────┘
│  │ (pull/push)  │  │ Cache (signed,  │  │ (process / OCI / │    │             │
│  │              │  │  versioned)     │  │  firecracker)    │    │             │
│  └──────┬───────┘  └────────┬────────┘  └────────┬─────────┘    │             │
│         │                   │                    │              │             │
│         └────────┬──────────┴────────┬───────────┘              │             │
│                  │                   │                          │             │
│        ┌─────────▼─────────┐ ┌───────▼──────────┐               │             │
│        │ LLM Client        │ │ Repo Worktree    │◄──────────────┼─────────────┘
│        │ (direct → providers│ │ (clone, branch, │  git over HTTPS/SSH
│        │   from catalog)   │ │  diff, commit,   │
│        └────────┬──────────┘ │  push)           │
│                 │            └──────────────────┘
│        ┌────────▼──────────┐  ┌──────────────────┐
│        │ Tool Executors:   │  │ Local Harness:   │
│        │  • web_search     │  │  • schema check  │
│        │  • http_fetch     │  │  • diff sanity   │
│        │  • external MCP   │  │  • secret scan   │
│        │    servers (cat.) │  │  • lint / type   │
│        └───────────────────┘  │  • build / test  │
│                               │  • dep / cov     │
│                               └────────┬─────────┘
│                                        │
│                                        ▼
│                            ┌────────────────────────────┐
│                            │ Streamed back to platform: │
│                            │  logs, metrics, harness    │
│                            │  report, commit refs,      │
│                            │  tool-call audit trail     │
│                            └────────────────────────────┘
└────────────────────────────────────────────────────────────────┘

                         ▲
                         │ direct HTTPS to LLM endpoints
                         ▼
       ┌──────────────────────────────────────────────────────┐
       │ LLM endpoints (Anthropic, company API, Ollama, …)     │
       │ External MCP servers (context7, project-specific, …)  │
       └──────────────────────────────────────────────────────┘
```

Key points to read out of this diagram:

- The **agent ↔ platform** channel carries metadata only: task descriptors, provider/MCP catalog, results, logs, metrics, audit. No source code traverses it.
- The **agent ↔ git** channel carries source code (clone, branch, commit, push).
- The **agent ↔ LLM** and **agent ↔ external MCP** channels are direct outbound from the agent. The platform is not in the data path of LLM calls. (An optional LLM proxy mode is documented for hardened deployments — Section 9.5.)
- The platform's chat brain has its own LLM client co-located with the server; it uses the same provider catalog but does not borrow agents to do its inference.
- The platform has no GPU and does not run AI models.

### 5.2 Modes of operation

| Mode | Command | Purpose |
|---|---|---|
| Server | `forge server` | Management plane: API + UI + DB + MCP + scheduler + config distributor + observability sink. No execution. |
| Agent | `forge agent --name w1 --roles worker,reviewer` | Execution plane: pulls tasks, clones project from git, talks to LLMs, runs harness, pushes results back to git, streams telemetry. |
| Init | `forge init` | Bootstraps config, DB, default providers, default prompts (server side). |
| Migrate | `forge migrate up` / `down` | Schema migrations for the platform DB. |
| Doctor | `forge doctor` | Health check: DB integrity, provider reachability (server-side smoke test), agent registry, git connectivity. |
| Harness | `forge harness run <work_dir>` | Run the verification harness locally against a working directory. Used by agents internally; also runnable by humans for debugging. |
| Tool | `forge tool ...` | One-shot CLI tools (export, import, key rotation, agent-cert generation). |

All modes link the same code; they share the same config loader. Server and agent share the protocol package but use disjoint subsets of the core services.

### 5.3 Deployment topologies

Agents are intentionally lightweight: a Go CLI that pulls a task, fetches code, talks to an LLM via the gateway, executes tool calls (web search, REST, MCP), and submits a result. Agents do not run builds, test suites, or GPU workloads — those happen server-side in the harness sandbox. This keeps the agent footprint tiny and the deployment matrix simple.

**Single-machine (default).** One process running `forge server`, one or more `forge agent` processes on the same host. Project storage in a local folder.

**Edge / homelab.** `forge server` on a low-power box (Pi, NUC, small VM); agents on the same box or on any other small machine with network access to the server. Communication over LAN with mTLS. Project storage either local on the server or on an external git remote.

**Hybrid.** Server on a small VM in the user's network; agents anywhere with outbound HTTP/WS to the server (laptops, cheap VPSes, ephemeral CI runners). Provider endpoints can be local (Ollama) on the server side and/or remote (Anthropic, company API). Project storage on an external git remote so multiple instances can collaborate.

**Scaled / external services.** Server connects to an external Postgres for metadata and to an external git provider for project storage. Agents are stateless; many can be spawned and torn down on demand. The server itself stays a single binary.

**Hardened.** Any of the above plus: network egress allowlist, signed agent registration tokens, sandbox hardening (gVisor or Firecracker), and outbound LLM traffic restricted to a single provider.

### 5.4 Why Go

Concurrency primitives, single-binary distribution, mature `crypto/tls`, mature `embed`, easy cross-compilation, stable runtime for long-lived daemons, strong stdlib for HTTP/WS. Avoids a Python deployment story.

### 5.5 Why Svelte

Reactive UI ideal for live dashboards (tasks, runs, agents), small bundle size to keep the embedded asset blob small, no server-side runtime needed (SPA + server-sent events / WS).

---

## 6. Component Catalog

Each component below has a section in this spec. Each component must be implementable as an independently testable Go package. Components are tagged by **plane**: `S` = server / management plane, `A` = agent / execution plane, `P` = protocol / shared.

| # | Component | Plane | Package | Summary |
|---|---|---|---|---|
| C1 | CLI / Bootstrap | S+A | `internal/cli` | Cobra-based CLI; mode dispatch; config bootstrap. |
| C2 | Config Loader | S+A | `internal/config` | Loads layered config; client variant for agents loads only what's relevant. |
| C3 | Storage Layer | S | `internal/store` | Pluggable SQL (SQLite default; Postgres / MySQL external) + file blobs + vector index abstraction. Server-only — agents do not touch this. |
| C4 | Migrator | S | `internal/store/migrate` | Dialect-aware SQL migrations, idempotent and reversible. |
| C5 | Secrets / KMS | S | `internal/secrets` | Sealed at rest using a master key from OS keystore or env. Source of truth for all credentials. |
| C6 | Project Service | S | `internal/projects` | CRUD, project storage binding, project-level config. |
| C7 | Task Service | S | `internal/tasks` | Task lifecycle, transitions, idempotency keys. |
| C8 | Run Service | S | `internal/runs` | Run records, attempts, harness reports, costs, links to git refs. |
| C9 | Workflow Engine | S | `internal/workflow` | State machine; declarative workflow defs; replay. |
| C10 | Scheduler | S | `internal/scheduler` | Priority queue, leases, fairness, backoff. |
| C11 | Provider Catalog | S | `internal/providers` | Configured LLM providers and their roles, models, parameters. Versioned. Source of truth for agents. |
| C12 | MCP Server Catalog | S | `internal/mcpcat` | Configured *external* MCP servers (e.g. context7, search, project-specific) that agents are allowed to call. Versioned. |
| C13 | Config Distributor | S+A | `internal/configdist` | Push (WS event) + pull (REST) sync of provider + MCP catalog to agents; signed, version-tagged, cached agent-side. |
| C14 | Prompt Engine | S | `internal/prompts` | Versioned templates, typed inputs, golden tests. Distributed to agents alongside catalog. |
| C15 | Context Engine | S | `internal/context` | Recipes, retrieval, summarization. Provides per-task context bundles to agents. |
| C16 | Memory / Embeddings | S | `internal/memory` | Vector index (sqlite-vec / pgvector / HNSW), chunk store. Server-side only. |
| C17 | MCP Endpoint | S | `internal/mcp` | Forge's *own* MCP server (for external tools like Continue, Claude desktop). Distinct from C12. |
| C18 | Chat Brain | S | `internal/chat` | Server-side tool-calling chat brain; uses its own LLM client (C18a) against the same provider catalog. |
| C18a | Server LLM Client | S | `internal/llm/client` | Used by the chat brain and by any server-side scheduled summarization. Reads catalog directly from DB. |
| C19 | Agent Service | S | `internal/agents` | Registration, heartbeat, capabilities, leases, session keys. |
| C20 | Agent Runtime | A | `cmd/forge/agent` + `internal/agentruntime` | The agent's main loop: pull/receive task, clone repo, drive LLM, execute tools, run harness, push to git, stream results. |
| C21 | Agent LLM Client | A | `internal/agentruntime/llm` | LLM client running inside the agent; uses the cached provider catalog to call providers directly. |
| C22 | Tool Executor | A | `internal/agentruntime/tools` | Built-in agent-side tools: `web_search`, `http_fetch`, `mcp_call` (talks to external MCP servers from the catalog), `read_file`, `write_file`, `exec_in_sandbox`. |
| C23 | Local Sandbox | A | `internal/agentruntime/sandbox` | Agent-side process isolation for build/test/exec calls (process / OCI / firecracker). |
| C24 | Repo Manager | A | `internal/agentruntime/repo` | Project storage operations: clone, fetch, branch, commit, push, diff. Runs inside the agent. |
| C25 | Verification Harness | A | `internal/harness` | Deterministic verification suite. Runs on the agent immediately after an LLM run. Produces a `HarnessReport` that is streamed to the platform. |
| C26 | Embedded Git Server | S | `internal/gitserver` | Optional Smart-HTTP git server (go-git based) so projects can live inside the platform without an external git host. Agents talk to this over standard git protocol, not the agent control channel. |
| C27 | Observability | S+A | `internal/obs` | Structured logs, OpenTelemetry traces, metrics, audit log. Agent emits, platform sinks and renders. |
| C28 | UI (embedded) | S | `web/` + `internal/web` | Svelte SPA bundled via `go:embed`. |
| C29 | API (REST + WS) | S | `internal/api` | Versioned REST + WS endpoints. |
| C30 | Agent Protocol | P | `internal/proto` | Wire types for the agent control channel (task, result, log frame, catalog version). |
| C31 | Auth & RBAC | S | `internal/auth` | Local users + tokens; OIDC opt-in; agent session tokens. |
| C32 | Updater (opt) | S+A | `internal/updater` | Self-update with signature verification. |

---

## 7. Data Model

The data model is the contract between every component. All entities have `id` (ULID), `created_at`, `updated_at`, and `deleted_at` (soft-delete).

### 7.1 Core entities

```
Project
  id, name, slug, description,
  storage_kind (local_folder | git_remote | embedded_git),
  storage_path        (filesystem path; for local_folder or embedded_git working copy),
  storage_remote_url  (https/ssh URL for git_remote; null otherwise),
  storage_remote_auth_ref (id of secret holding token / ssh key; null otherwise),
  default_branch,
  status (active|paused|archived),
  config_blob_id (project-level overrides),
  created_at, updated_at

WorkPackage
  id, project_id, parent_id (nullable, for nesting),
  title, summary, spec_blob_id,
  status (draft|approved|in_progress|done|cancelled),
  priority (int), tags, created_at, updated_at

Task
  id, project_id, work_package_id (nullable for project-level tasks),
  type (plan|design|implement|verify|review|integrate|test|summarize|spec|custom),
  status (queued|claimed|running|needs_review|approved|failed|cancelled|done),
  role_required (orchestrator|worker|reviewer|...),
  priority (int), idempotency_key (unique),
  payload_blob_id, result_blob_id,
  attempts (int), max_attempts (int),
  scheduled_for (nullable), claimed_by_agent_id (nullable),
  parent_task_id (nullable), depends_on (json array of task_ids),
  created_at, updated_at, started_at, finished_at

Run
  id, task_id, agent_id, attempt_number,
  status (running|succeeded|failed|cancelled),
  inputs_blob_id, outputs_summary_blob_id,
  -- Git references (the actual code lives in git, not in the platform DB):
  base_commit_sha, head_commit_sha (nullable),
  branch_name (nullable), pushed_remote (bool),
  -- Verification artifacts produced by the agent-side harness:
  harness_report_blob_id (nullable), harness_status (pass|fail|partial|missing),
  -- Cost / activity:
  llm_calls_count, tokens_in, tokens_out, cost_cents,
  tool_calls_count, sandbox_exec_count,
  -- Catalog versions used (for audit / reproducibility):
  provider_catalog_version, mcp_catalog_version, prompt_catalog_version,
  started_at, finished_at, error (text)

Agent
  id, name, fingerprint (public key hash),
  roles (json array), capabilities (json),
  status (online|offline|paused|disabled),
  current_task_id (nullable), last_seen_at,
  registered_at, version, host

Provider
  id, name, type (anthropic|openai_compat|ollama|azure|copilot|...),
  base_url, auth_ref (id of secret), capabilities (json),
  status (active|disabled),
  catalog_version (int, monotonic; bumped on any change),
  visibility (all_agents|labels:<list>|project:<id>),
  created_at, updated_at

Model
  id, name (logical), provider_id, model_id (provider-side),
  default_params (json), roles (json array),
  context_window, supports_tools (bool), supports_vision (bool),
  cost_per_mtoken_in, cost_per_mtoken_out

McpServer                          -- external MCP servers agents may call as tools
  id, name, description,
  transport (stdio|http|ws),        -- agent's MCP client uses this to connect
  endpoint (url or command line),
  auth_ref (id of secret, nullable),
  tool_allowlist (json, nullable),  -- which tools from this server agents may call
  status (active|disabled),
  catalog_version (int),
  visibility (all_agents|labels:<list>|project:<id>),
  created_at, updated_at

CatalogSnapshot                    -- canonical snapshots distributed to agents
  id, kind (provider|mcp|prompt|context_recipe|composite),
  version (int, monotonic),
  payload_blob_id,                 -- signed JSON manifest
  signature, signed_at, signing_key_id,
  created_at

Prompt
  id, name, version (semver), description,
  template (text, with {{vars}}), input_schema (json schema),
  context_recipe_id (nullable), tags

ContextRecipe
  id, name, version,
  include (json: e.g. ["docs","spec","diff","terminal"]),
  exclude (json), max_tokens (int), retrieval (json: e.g. {"k":12,"min_sim":0.6})

ContextItem
  id, project_id, kind (file|summary|embedding|note|snapshot|test_result|diff),
  source_ref (text), content_blob_id, embedding_id (nullable),
  hash, tokens, created_at

Embedding
  id, project_id, model_id, dim, vector (blob), chunk_id (ref ContextItem)

Conversation
  id, project_id (nullable), title, status (open|archived),
  created_at, updated_at

Message
  id, conversation_id, role (user|assistant|tool|system),
  content_blob_id, tool_call_id (nullable),
  tokens, model_id (nullable), created_at

ToolCall
  id, message_id, tool_name, arguments_blob_id,
  result_blob_id, status (pending|succeeded|failed),
  duration_ms

Secret
  id, name, kind (api_key|token|cert|ssh),
  ciphertext, nonce, created_at, rotated_at

User
  id, email, display_name, password_hash (nullable),
  oidc_subject (nullable), roles (json), created_at

ApiToken
  id, user_id, name, hash, scopes (json),
  expires_at, last_used_at

AuditEvent
  id, actor_kind (user|agent|system|chat_brain),
  actor_id, action, target_kind, target_id,
  before (json), after (json), correlation_id, created_at

Blob (large content, content-addressed)
  id, hash (sha256), size, mime, content (bytes or fs path)
```

### 7.2 Storage strategy

#### 7.2.1 Database backend (pluggable)

The platform talks to its database through a single `Store` interface backed by a SQL driver. Two modes are supported and chosen at server startup via config:

- **Internal (default).** Embedded **SQLite** (`modernc.org/sqlite` for the pure-Go path; `mattn/go-sqlite3` available as a CGO build for performance). WAL mode, foreign keys on. Zero-config; no external service required.
- **External.** **PostgreSQL** (≥ 14) or **MySQL/MariaDB** (≥ 8.0 / 10.6). Used when the operator wants HA, off-host storage, multi-server scale-out, or shared metadata across environments.

Implementation details:
- A single SQL schema is maintained, with a small dialect layer (`internal/store/dialect/`) that handles the few places where SQLite, Postgres, and MySQL diverge (`AUTOINCREMENT` vs `SERIAL`/`AUTO_INCREMENT`, JSON column types, upsert syntax, returning clauses, full-text indexes).
- All write paths use transactions and a single statement style that round-trips across dialects.
- Migrations live in `internal/store/migrate/migrations/<NNN>_*.<dialect>.sql`; a `common.sql` is generated from a templated source so the three dialects stay in lock-step.
- Connection pooling (`max_open_conns`, `max_idle_conns`, `conn_max_lifetime`) is configurable; sensible defaults per backend.
- Switching backends is supported via export/import (`forge tool export-db`, `forge tool import-db`); live migration from SQLite to Postgres is a documented operation, not an automatic one.

#### 7.2.2 Blobs and large content

Large content (prompt outputs, diffs, terminal logs, harness evidence) is stored as **blobs** content-addressed by SHA-256, on the filesystem under `${data_dir}/blobs/aa/bb/<hash>` with optional zstd compression. The blob store is independent of the DB backend; in scaled deployments, the `${data_dir}` can be a shared mount or replaced with an S3-compatible object store via the `BlobStore` interface (v1.1).

#### 7.2.3 Embeddings

Embeddings via **sqlite-vec** when the DB backend is SQLite, or via **pgvector** when the backend is Postgres. For MySQL or when neither is available, an internal HNSW index in a separate file is used. Pluggable via a `VectorStore` interface.

#### 7.2.4 Project files

Project source files do not live in the platform DB. They live in the project's storage backend (Section 7.3): a local folder, an external git remote, or the platform's embedded git server. Per-project working data (transient worktrees, snapshots, harness evidence) is kept under `${data_dir}/projects/<project_id>/`.

#### 7.2.5 Backups

`forge backup` produces a single archive containing: a logical dump of the metadata DB, the blob tree, per-project working data, and the secrets (re-encrypted under a backup key). For external DB backends, the dump uses the backend's native tooling (`pg_dump`, `mysqldump`) wrapped by Forge so the operator gets a single command.

### 7.3 Project storage backends

Every project picks one of three backends at creation time. A project can be migrated between backends later (e.g., promote a local folder to a git remote) via `forge tool migrate-project-storage`.

#### 7.3.1 `local_folder`

A path on the server's filesystem. The repo manager initializes it as a git working copy on first use (so diffs, branches, and rollbacks are still version-controlled) but never pushes anywhere. Best for: trying things out, single-machine setups, throwaway experiments. Concurrency: only one server can own a `local_folder` project at a time.

#### 7.3.2 `git_remote`

The project's source of truth is an external git remote: GitHub, GitLab, Gitea, Bitbucket, an internal company git host, or any plain SSH/HTTPS git server. Forge clones the repo into a managed working copy under `${data_dir}/projects/<project_id>/work/`. Branch creation, commit, push, and pull are performed via go-git or git-cli (configurable). Authentication is via a referenced secret (PAT, deploy key, ssh key). Best for: serious work, team collaboration, CI integration, multi-instance deployments.

The repo manager supports:
- Per-task feature branches (e.g. `forge/wp-<id>/<task-type>-<run_id>`).
- Force-push protection (forbidden on `default_branch`).
- Optional PR / MR creation through provider-specific adapters when supported (GitHub, GitLab, Gitea); falls back to a plain branch push when no adapter exists.
- Webhook intake (Section 7.3.4).

#### 7.3.3 `embedded_git`

Forge ships an optional **embedded Smart-HTTP git server** (`internal/gitserver`, built on go-git). It exposes git endpoints under `/git/<project_slug>.git` with the same auth as the rest of the API. This gives users a self-contained git host without needing GitHub/Gitea, and lets external clients (a developer's laptop, Continue.dev, another agent fleet) clone, push, and pull project repositories directly to and from the platform.

Operational details:
- Storage layout: bare repos at `${data_dir}/git/<project_id>.git`.
- Hooks: `post-receive` triggers an indexing run that updates the context engine and may auto-create a `plan` task if so configured.
- Mirror mode: an `embedded_git` project can also be mirrored to an external remote for backup or CI.
- This backend is **off by default**; the operator must enable `gitserver.enabled: true` in config.

#### 7.3.4 Webhooks and intake

For `git_remote` and `embedded_git`, the platform listens at `/hooks/git/<project_id>` for push events. On a relevant event (push to default branch, new tag, new PR), the workflow engine can be configured to:
- Re-index context (always).
- Trigger a `review` task on the diff.
- Trigger a `plan` task if the push contains a recognized intake artifact (e.g. `IDEAS.md`).

Webhook payloads are verified against a per-project secret. Replay is rejected via timestamp + nonce.

#### 7.3.5 Forbidden paths and write fences

Regardless of backend, the repo manager refuses to apply diffs that:
- Touch paths outside the work package's `allowed_paths`.
- Touch paths in the project's `forbidden_paths` (typically `.forge/`, secrets directories, signing material, the harness profile itself).
- Modify files larger than a configured threshold without an explicit allowance in the task payload.

These rules are enforced before any diff is committed locally and re-checked by the harness before integrate.

### 7.4 Identifiers

- ULID for all primary keys (sortable, opaque, URL-safe, unique).
- `idempotency_key` on tasks: callers can submit the same task twice and only one is enqueued.

### 7.5 Migrations

- Versioned, numbered SQL files in `internal/store/migrate/migrations/NNN_*.sql`, with a dialect layer that emits the right flavor for SQLite, Postgres, or MySQL.
- `forge migrate up` is run automatically by `forge server` on startup unless `--no-auto-migrate` is set.
- Every migration must be reversible.
- Cross-backend migration (e.g. SQLite → Postgres) is a separate, explicit operator action, not part of `forge migrate`.

---

## 8. Configuration System (Continue.dev-inspired)

### 8.1 Layered config

Effective config is the merge of (lowest to highest precedence):

1. Compiled-in defaults.
2. `${data_dir}/config.yaml`.
3. Project-level overrides (`<project>/.forge/config.yaml`).
4. Environment variables (`FORGE_*`).
5. CLI flags.
6. Live overrides set via the UI/API (stored in DB, scoped to project or global).

### 8.2 Schema (excerpt)

```yaml
server:
  bind: 127.0.0.1:7777
  tls:
    enabled: true
    cert_file: ${data_dir}/tls/server.crt
    key_file: ${data_dir}/tls/server.key
  cors:
    allowed_origins: ["http://localhost:5173"]

storage:
  data_dir: ~/.forge
  blob_dir: ${data_dir}/blobs
  vector_backend: auto          # auto | sqlite-vec | pgvector | hnsw

database:
  driver: sqlite                # sqlite | postgres | mysql
  # SQLite (default):
  sqlite_path: ${data_dir}/forge.db
  # External examples (only the matching driver block is read):
  postgres_dsn: ""              # e.g. postgres://forge:***@db:5432/forge?sslmode=require
  mysql_dsn:    ""              # e.g. forge:***@tcp(db:3306)/forge?parseTime=true
  pool:
    max_open_conns: 25
    max_idle_conns: 5
    conn_max_lifetime: 30m
  auto_migrate: true

gitserver:
  enabled: false                # turn on to use embedded_git project storage
  bind_path: /git               # exposed under the same listener as the API
  repo_dir: ${data_dir}/git
  allow_anonymous_read: false

project_storage_defaults:
  default_kind: local_folder    # local_folder | git_remote | embedded_git
  base_dir: ${data_dir}/projects

secrets:
  master_key_source: keyring   # keyring | env | file
  master_key_env: FORGE_MASTER_KEY

providers:
  - name: company
    type: openai_compat
    base_url: https://api.company.com/v1
    api_key_secret: company_api_key
    headers:
      X-Company-Tenant: ${COMPANY_TENANT}
  - name: anthropic
    type: anthropic
    api_key_secret: anthropic_api_key
  - name: ollama
    type: ollama
    base_url: http://localhost:11434
  - name: lmstudio
    type: openai_compat
    base_url: http://localhost:1234/v1

models:
  - name: brain-sonnet
    provider: anthropic
    model: claude-sonnet-4-6
    roles: [chat_brain, reviewer, system_designer]
    supports_tools: true
    context_window: 200000
  - name: worker-qwen
    provider: ollama
    model: qwen2.5-coder:14b
    roles: [worker]
    context_window: 32000
  - name: orchestrator-o4
    provider: company
    model: o4-mini
    roles: [orchestrator, ui_designer]
  - name: embedder-default
    provider: company
    model: o3-mini-embed
    roles: [embedder]

routing:
  - when: { task_type: plan }
    role: orchestrator
    prompt: plan@1.2.0
    context_recipe: plan-default@1
  - when: { task_type: implement }
    role: worker
    prompt: implement@1.4.0
    context_recipe: implement-default@1
  - when: { task_type: review }
    role: reviewer
    prompt: review@1.1.0
    context_recipe: review-default@1
  - when: { task_type: design, subtype: ui }
    role: ui_designer
    prompt: ui-design@1
  - when: { task_type: design, subtype: system }
    role: system_designer
    prompt: system-design@1

agents:
  default_roles: [worker]
  registration:
    require_token: true
    token_ttl: 24h
  catalog_sync:
    mode: hybrid                  # pull | push | hybrid
    pull_interval: 60s            # agent re-checks catalog version this often
    pre_task_check: true          # always re-check version before starting a new task
    push_via: websocket           # WS event tells agents "catalog changed → re-fetch"
    cache_ttl: 24h                # how long agent may use cached catalog when offline
  llm_routing:
    mode: direct                  # direct | proxy   (Section 9.5)
    proxy_path: /v1/llm           # only used when mode=proxy
  log_streaming:
    transport: websocket          # websocket | rest_batch
    flush_interval: 1s
    flush_bytes: 64KiB
    backpressure: drop_debug      # drop_debug | block | spool

sandbox:
  # Sandbox config is delivered to agents via the catalog and applied agent-side.
  default_backend: process        # process | oci | firecracker
  image: ghcr.io/forge/runtime:latest
  network: deny                   # deny | allow_egress | proxy
  cpu_limit: 2
  mem_limit_mb: 4096
  timeout_seconds: 1800

harness:
  # Harness runs on the agent. Profiles below are distributed via the catalog.
  required_checks:
    - schema
    - diff_sanity
    - secret_scan
    - lint
    - typecheck
    - build
    - tests
  optional_checks:
    - perf_smoke
    - dep_audit
    - reproducibility

observability:
  logs:
    format: json
    level: info
  otel:
    enabled: false
    endpoint: ""
  audit:
    retention_days: 365

self_update:
  enabled: false
  channel: stable
  signing_key_file: ${data_dir}/keys/forge-release.pub
```

### 8.3 Live config

The DB stores a `config_overrides` table and an `effective_config` view. The UI exposes a config editor; changes are versioned, diffed, and audited. Sensitive values are stored in the secrets table and referenced as `xxx_secret`.

### 8.4 Validation

Config is validated against a JSON schema on load. `forge doctor` runs the validation plus connectivity tests for each configured provider.

---

## 9. LLM Provider Abstraction

The provider abstraction is shared code (`internal/llm`) used in two places: inside agents (when they make LLM calls during a task) and inside the server (when the chat brain talks to an LLM). Provider definitions are *configured* once on the server, distributed to agents via the catalog (Section 9.6), and consumed by either side through the same interface.

### 9.1 Interface

```go
type Provider interface {
    Name() string
    Capabilities() ProviderCapabilities
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Stream(ctx context.Context, req ChatRequest) (Stream, error)
    Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error)
    Rerank(ctx context.Context, req RerankRequest) (RerankResponse, error)
    Health(ctx context.Context) error
}
```

`ChatRequest` carries a normalized message list, a normalized tool schema (OpenAI-style by convention; adapted per provider), parameters (temperature, max_tokens, stop), and a `RoutingHint` with the role and the originating task. The same `ChatRequest` shape is used whether the call originates in the chat brain or in an agent.

### 9.2 Adapters

Adapters live in `internal/llm/providers/` and implement `Provider`. Each adapter:
- Translates the canonical request to the provider's wire format.
- Translates tool-call shapes (OpenAI tools, Anthropic tool_use, Ollama function-call hack, etc.) to and from the canonical shape.
- Surfaces rate-limit and quota errors as typed errors so callers can react.
- Reports normalized usage (tokens, cost when known) to the local instrumentation, which is then either logged (server) or streamed back to the platform (agent).

Adapters required for v1: `anthropic`, `openai_compat`, `ollama`. Adapters planned for v1.1: `azure`, `copilot`, `bedrock`, `vertex`.

### 9.3 Where the provider client runs

There are two locations:

**Agent-side LLM client (default, primary path).** Agents instantiate `Provider` adapters from the cached catalog (Section 9.6). Calls go directly from the agent to the LLM endpoint over HTTPS. The platform is *not* in the data path. Token counts, costs, and call latencies are streamed back to the platform as telemetry (Section 16), but the prompt and response payloads stay on the agent unless the operator opts in to verbose audit logging.

**Server-side LLM client.** Used only by the chat brain (Section 14) and any server-scheduled summarization jobs (e.g. context summarization). Reads the same provider catalog directly from the DB.

The interface is identical; the location is configured per caller.

### 9.4 Routing

Routing is a pure function: `(task_type, role_hint, project_overrides) → (model_id, prompt_id, parameters)`. The routing table is part of the catalog and is identical on both sides. An agent that wants to do a `worker` call resolves the rule the same way the server's chat brain would.

### 9.5 Direct vs proxied LLM calls

Two modes, configurable per agent (`agents.llm_routing.mode`):

**`direct` (default).** Agent calls providers directly. Provider credentials are delivered to the agent as part of the catalog (over the agent's authenticated, mTLS-protected control channel; Section 15). The agent caches them in memory only and never writes them to disk. Pros: lowest latency, no platform load on the LLM data path, no platform component touches prompt/response payloads. Cons: agents see provider credentials.

**`proxy`.** Agent calls a server endpoint (`POST /v1/llm/chat`) and the server makes the upstream call on the agent's behalf. Pros: agents never see provider credentials; centralized rate limiting and cost cap enforcement; useful when agents run in less-trusted environments. Cons: platform is in the data path; doubles latency for streaming; server must scale to the LLM call volume.

The two modes share the same wire types; switching is a config-only change. Hardened deployments default to `proxy`; default homelab installs use `direct`.

### 9.6 The provider + MCP catalog

The platform owns a versioned catalog with everything an agent needs to make routing decisions and reach LLMs / MCP servers:

- Provider entries (type, base URL, credentials, headers, capabilities).
- Model entries (logical name, provider binding, role tags, parameters, context window, cost).
- MCP server entries (transport, endpoint, auth, tool allowlist).
- Prompt templates (versioned).
- Routing rules.
- Context recipes.
- Sandbox / harness profile defaults.

The catalog is distributed to agents through the **Config Distributor** (Section 9.7). Each entry has a monotonic `catalog_version`. The catalog as a whole has a composite version (the max of its parts). Every run records the catalog version it used so reproductions can be exact.

### 9.7 Config distribution to agents

Distribution is **hybrid** (push + pull) because each model alone has a failure case:

1. **Pull on registration.** When an agent registers, it fetches the current catalog snapshot.
2. **Pull before each task.** Before claiming a task the agent calls `GET /v1/catalog/version`. If the platform's version is higher than the agent's cached version, the agent re-fetches the snapshot before starting work. This is cheap (one int) and guarantees no run uses a stale catalog.
3. **Pull on interval.** The agent polls the version endpoint every `pull_interval` (default 60s) so idle agents stay current.
4. **Push on change.** The platform broadcasts a `catalog_changed` event over the agent's WebSocket the moment the operator commits a change in the UI or via API. Agents fetch immediately. If the WS is broken, no harm — the next pre-task or interval pull catches up.
5. **Cache TTL when offline.** If an agent loses contact with the platform but is mid-task, it may keep using its cached catalog up to `cache_ttl`. Beyond that, no new tasks may be claimed and in-flight tasks degrade to a `stale_catalog` failure (logged, retryable).

Each catalog snapshot is **signed** by the server's signing key. Agents verify the signature before applying. This protects agents from a hijacked control channel substituting a malicious provider.

The credentials inside the catalog are wrapped: each `auth_ref` is replaced with a per-agent unwrapped credential bundle when the snapshot is sent. The unwrapping uses a key tied to the agent's session, so a snapshot captured for agent A is unusable on agent B.

### 9.8 Tool calling normalization

The provider client exposes a single canonical tool schema (JSON Schema for `parameters`, with `name` and `description`). Adapters map this to and from each provider's native tool shape. Tools available to the chat brain are the platform's MCP tools (Section 13.2). Tools available to the agent during a task are the union of: built-in agent-side tools (`web_search`, `http_fetch`, `read_file`, `write_file`, `exec_in_sandbox`), plus calls to external MCP servers from the catalog the project authorizes.

---

## 10. Workflow & Task Lifecycle

### 10.1 Top-level project workflow

```
   ┌──────────┐
   │  Intake  │  user submits "I want X"
   └────┬─────┘
        │
        ▼
   ┌──────────┐    plan task
   │  Plan    │───▶ orchestrator role
   └────┬─────┘    output: PROJECT_PLAN.md + work_packages[]
        │
        ▼
   ┌──────────┐    one design task per WP, optional
   │  Design  │───▶ system_designer / ui_designer
   └────┬─────┘    output: design docs, spec.md per WP
        │
        ▼
   ┌──────────┐    implement task per WP
   │ Implement│───▶ worker role
   └────┬─────┘    output: branch + diff + test results
        │
        ▼
   ┌──────────┐    deterministic
   │  Verify  │───▶ harness
   └────┬─────┘    output: harness_report
        │
        ▼
   ┌──────────┐    review task
   │  Review  │───▶ reviewer role
   └────┬─────┘    output: approval | change_requests
        │
        ▼
   ┌──────────┐    deterministic
   │ Integrate│───▶ merges branch, runs full suite
   └────┬─────┘
        │
        ▼
     done / re-plan if blocked
```

### 10.2 Task state machine (Agentic Development Lifecycle)

The platform implements a full software-development lifecycle as a strict state machine. Every state change is recorded in `task_state_transitions` for audit and UI timeline rendering.

```
                        ┌──────────┐
                        │  BACKLOG │  task created / re-queued after failure
                        └────┬─────┘
                             │ dev agent claims
                             ▼
                        ┌──────────────┐
           ┌──────────▶ │  DEVELOPING  │  worker agent has a worktree / clone
           │            └──────┬───────┘
           │                   │ agent pushes branch (post-receive hook)
           │                   │  — or — POST /tasks/{id}/submit-for-review
           │                   ▼
           │            ┌──────────────────┐
           │            │ AWAITING_REVIEW  │  waiting for a reviewer agent
           │            └──────┬───────────┘
           │                   │ reviewer agent claims
           │                   ▼
           │            ┌───────────┐
           │            │ REVIEWING │  reviewer reads diff, writes markdown review
           │            └──────┬────┘
           │          ┌────────┴──────────────┐
           │          │ changes_requested      │ approved
           │          ▼                        ▼
           │   ┌──────────────────┐    ┌───────────────────┐
           └───│ AWAITING_REVISION│    │  AWAITING_MERGE   │
               └──────────────────┘    └────────┬──────────┘
                                                │ merge supervisor releases lock
                                                ▼
                                        ┌─────────┐
                                        │ MERGING │  supervisor merges branch → main
                                        └────┬────┘
                                   ┌─────────┴──────────────┐
                                   │ conflict / test fail    │ success
                                   ▼                         ▼
                          ┌──────────────────┐       ┌───────────┐
                          │ AWAITING_REVISION │       │ COMPLETED │
                          └──────────────────┘       └───────────┘

FAILED is a terminal state reachable from any execution state; retried by moving back to BACKLOG.
```

**State categories**

| Category | States |
|---|---|
| Queue (waiting for agent) | `BACKLOG`, `AWAITING_REVIEW`, `AWAITING_REVISION`, `AWAITING_MERGE` |
| Execution (agent holds the task) | `DEVELOPING`, `REVIEWING`, `MERGING` |
| Terminal | `COMPLETED`, `FAILED` |

**Claim routing by role**

| Agent role | Picks up from | Transitions to |
|---|---|---|
| `worker` | `BACKLOG`, `AWAITING_REVISION` | `DEVELOPING` |
| `reviewer` | `AWAITING_REVIEW` | `REVIEWING` |
| `merge_supervisor` | `AWAITING_MERGE` _(background)_ | `MERGING` |

**Invariants**
- Every state change writes a `task_state_transitions` row with `from_state`, `to_state`, `actor_agent_id`, `reason`, and `created_at`.
- A task in any execution state always has `assigned_agent_id` set.
- `COMPLETED` and `FAILED` are terminal; no further transitions without explicit human override.
- A task with `attempts >= max_attempts` cannot auto-retry; requires human re-queue.

### 10.3 Workflows are declarative

Workflows live in `internal/workflow/defs/*.yaml` and declare nodes (task types), edges (transitions), and branch rules. The default workflow is `default-software.yaml`. Custom workflows can be registered per project. The engine is a deterministic state machine with replay: given the run history, the next node is uniquely determined.

### 10.4 Task creation rules

- Only the workflow engine, the chat brain (via tools), and explicit user actions can create tasks.
- Every task has an `idempotency_key`. Repeat creations are merged.
- Tasks created for a self-modifying workflow against the Forge repo itself carry a `target=self` flag and require an extra `human_approval` gate.

### 10.5 Embedded git server

The platform embeds a smart-HTTP git server (pure Go, no `git` binary required) using `github.com/go-git/go-git/v5`. It serves one bare repository per project under `/git/{project_slug}.git/`.

**Endpoints**

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/git/{slug}.git/info/refs?service=git-upload-pack` | Clone / fetch advertisement |
| `POST` | `/git/{slug}.git/git-upload-pack` | Clone / fetch pack transfer |
| `GET` | `/git/{slug}.git/info/refs?service=git-receive-pack` | Push advertisement |
| `POST` | `/git/{slug}.git/git-receive-pack` | Push pack transfer + post-receive hook |

**Post-receive hook**: when an agent pushes a branch matching `task/{task_id}` and the task is in `DEVELOPING`, the server atomically transitions it to `AWAITING_REVIEW` and records the pushed commit SHA on the task row (`branch_head_sha`, `last_push_at`).

**Repository layout** (under `storage.root`, default `./data`):

```
./data/
  repos/{project_id}.git/    ← bare repo; canonical source of truth
  worktrees/{task_id}/       ← colocated-agent working tree (task-lifetime)
```

**Agent modes**

| Mode | Claim response includes | Agent operation |
|---|---|---|
| `colocated` | `worktree_path` (on-disk path) | Server creates worktree; agent `cd`s there |
| `remote` | `repo_url` + `branch` | Agent clones / fetches over HTTP |

**Upstream mirroring**: if a project has `remote_url` configured, the server pushes `main` to the `upstream` remote after every successful merge. Push failures are logged and do not roll back the merge.

**Auth**: out of scope for v1. Assume trusted network or reverse-proxy off-load.

### 10.6 Code-review schema

Every formal review of a task is a `task_reviews` row. Reviews drive state transitions; informal back-and-forth uses `task_comments` threaded under the review via `review_id`.

**Schema**

| Field | Type | Notes |
|---|---|---|
| `id` | TEXT | ULID primary key |
| `task_id` | TEXT | FK → tasks |
| `author_type` | TEXT | `user` \| `agent` |
| `author_role` | TEXT | `reviewer`, `merge_supervisor`, etc. |
| `author_id` | TEXT | agent ID or empty for users |
| `status` | TEXT | `approved` \| `changes_requested` \| `revision_requested` |
| `body` | TEXT | Markdown (fenced diff blocks, inline severity tags) |
| `branch_head_sha` | TEXT | Commit the review was made against |
| `created_at` | DATETIME | |

**Status → state transition**

| Review status | Task must be in | Transitions to |
|---|---|---|
| `approved` | `REVIEWING` | `AWAITING_MERGE` |
| `changes_requested` | `REVIEWING` | `AWAITING_REVISION` |
| `revision_requested` | `REVIEWING` | `AWAITING_REVISION` |

**Synthetic reviews**: on merge failure, the merge supervisor creates a review with `author_type=system`, `author_role=merge_supervisor`, `status=revision_requested`, and the merge-fail log as the body. This moves the task to `AWAITING_REVISION` and surfaces the conflict to the developer.

**API**

- `GET /api/tasks/{id}/reviews` — list all reviews, oldest first
- `POST /api/tasks/{id}/reviews` — create review (drives state transition)
- `GET /api/tasks/{id}/reviews/{reviewID}` — single review

### 10.7 Parallel merge orchestration

The merge supervisor is a background goroutine that polls `AWAITING_MERGE` tasks and orchestrates conflict-free parallel merges.

**File-lock algorithm**

1. For each `AWAITING_MERGE` candidate, compute its changed-path set via `git diff main...task/{id} --name-only` (go-git tree walk).
2. Check the `merge_locks` table (persisted; survives restarts) for any in-flight task whose path set overlaps with the candidate's.
3. If **no overlap**: acquire a lock (insert row), transition task to `MERGING`, perform merge in a temporary worktree, release lock.
4. If **overlap**: leave task in `AWAITING_MERGE`; retry next poll tick.

Non-overlapping tasks merge **concurrently**. Overlapping tasks merge **serially** in arrival order.

**Merge steps**

1. Create a temporary worktree off current `main`.
2. `git merge task/{id}` (go-git).
3. On success: push updated `main` to bare repo, transition task to `COMPLETED`, optionally push to upstream remote.
4. On failure (conflict or test fail): create synthetic review (§ 10.6), transition task to `AWAITING_REVISION`.

**Configuration**

| Key | Default | Description |
|---|---|---|
| `agents.merge_supervisor_interval_sec` | `10` | Polling interval |

---

## 11. The Verification Harness

The harness is the deterministic gate that every run must pass. It is the single most important correctness feature of the platform. **The harness runs on the agent**, immediately after the agent's LLM-driven work and before the agent declares the run complete. The platform consumes the report; the platform does not run the harness itself (it has no project worktree and no project toolchain).

### 11.1 Goals

- Catch fabricated outputs (e.g., the LLM claims a test passed when it didn't).
- Catch unsafe patches (e.g., dependency on an unpinned upstream, secrets leaked, network calls in tests).
- Reject low-quality output deterministically before any LLM-based reviewer or human reviewer sees it.
- Produce an immutable, replayable report that the platform can audit and that another agent can re-run.

### 11.2 Inputs

Per run, the agent-side harness receives:
- The pre-run commit SHA in the project repo.
- The post-run worktree (still owned by the agent at this point).
- The diff between pre and post.
- The structured agent output (claimed result).
- The task type, work package, and the harness profile pulled from the catalog (project-specific overrides applied).

### 11.3 Phases

Each phase is implemented as an independently runnable check. Each check returns a typed `CheckResult { name, status, severity, evidence_blob_id, duration_ms }`.

1. **Schema check.** Agent output validates against the task's expected output JSON schema.
2. **Diff sanity.** Diff applies cleanly. No edits outside the work package's allowed paths. No edits to forbidden paths (e.g., `secrets/`, signing keys, harness itself unless `target=self`).
3. **Secret scan.** `gitleaks`-style ruleset across the diff and untracked files.
4. **Lint.** Project-configured linter (`golangci-lint`, `eslint`, `ruff`, etc.).
5. **Typecheck.** Project-configured typechecker (`go vet`, `tsc --noEmit`, `mypy`, …).
6. **Build.** Project-configured build command must succeed.
7. **Tests.** Project-configured test command. The harness re-runs the tests itself and parses results into a structured report; it does not trust the agent's claim. New tests must exist if the work package's spec required them.
8. **Coverage delta.** Coverage must not drop below configured threshold for changed files.
9. **Dependency audit.** No new dependencies without explicit declaration in the spec; `govulncheck` / `npm audit` clean for high+critical.
10. **Reproducibility.** Re-running the build is deterministic (same hash on a second run) where the language toolchain supports it.
11. **Self-modification gate (only for `target=self`).** Extra checks: signed-off-by present, harness-on-harness change requires two human approvals, no edits to crypto or auth code without an explicit `security_review` task.

### 11.4 Output

```
HarnessReport
  run_id, started_at, finished_at,
  agent_id, harness_version, profile_version,
  status (pass|fail|partial),
  checks: [CheckResult, ...]
  summary_md (text),
  evidence_root_blob_id,
  signed_by_agent (Ed25519 signature over the canonical report)
```

The agent submits the report to the platform via `POST /v1/runs/{run_id}/harness-report`. The platform stores the report and its evidence blobs but does **not** re-execute the checks. Trust is established by:

- The agent's signature on the report (so a hijacked control channel cannot forge passes).
- Recording the catalog version and harness profile used.
- Audit log entries for every check failure or skip.
- Optional twin-agent verification for sensitive task types (Section 11.6).

A failed harness causes the run to transition to `failed` and the workflow engine creates a follow-up `repair` task with the harness report as input.

### 11.5 Evidence

All check stdout/stderr, test JSON, lint JSON, and timing are produced agent-side and bundled into a tarball that the agent uploads as a blob over the existing observability channel. Evidence may be verbose (whole test output) or compact (a structured summary) depending on the project profile and storage budget. Every blob is content-addressable; UI deep-links go to the evidence.

### 11.6 Twin-agent verification (opt-in)

For high-stakes task types — most notably `target=self` runs (Section 17) — the workflow can require **two independent agents** to produce matching harness reports before the run is accepted. The second agent claims a `verify` task that points at the head commit produced by the first agent, runs the harness on a fresh worktree, and signs an independent report. The platform compares the two reports' check matrices; mismatches abort the integrate phase and surface an explicit divergence in the UI.

This is the platform's answer to "who watches the agent that watches the harness" without dragging the harness back into the platform.

### 11.6 Determinism budget

The harness aims to be hermetic. If a check needs network (e.g., `npm audit`), it is marked `non_hermetic` and runs through a caching proxy whose entries are persisted with the project so reruns are reproducible.

---

## 12. Agent Specification

### 12.1 What an agent is (and isn't)

An agent is a stateless Go process that owns the **execution plane** of one or more concurrent tasks. It is not a thin shuttle — it is the worker. Once a task is claimed, the agent owns the entire end-to-end implementation loop.

It **does**:
- Connect to the platform on startup, register, and fetch the signed catalog (providers, MCP servers, prompts, routing rules, harness profile, sandbox profile).
- Pull or receive tasks from the platform.
- Clone or fetch the project's source code from its git backend (external remote or the platform's embedded git server).
- Drive the LLM directly: run the chat loop, execute tool calls (web search, HTTP, external MCP servers from the catalog, file reads/writes, sandboxed shell exec).
- Apply changes to the working copy (write files, edit files), commit them on a feature branch, and push to the project's git backend.
- Run the verification harness locally on its own host (lint, typecheck, build, tests, secret scan, dep audit, etc.).
- Stream logs, metrics, traces, harness evidence, and progress events to the platform over the control channel.
- Submit a structured result: pre/post commit SHAs, branch name, harness report, signed by the agent's session key.

It **does not**:
- Receive or send source code over the platform-agent control channel. Code only flows through git.
- Read or write the platform's database. State on the platform is changed exclusively through REST/WS calls validated by the platform's auth/RBAC.
- Need a GPU, run AI models locally, or host language toolchains it doesn't actually use. The toolchains the agent host needs are determined by the projects it works on (Go, Node, Python, etc.) and are operator's responsibility — but they live on the agent host, not on the platform.
- Hold long-term state across restarts beyond its keypair and session token. Cached catalog and in-flight working copies are recoverable from the platform and from git, respectively.

This makes agents simple to deploy and easy to scale horizontally: spin up another agent process, register it, and it joins the pool.

### 12.2 Host requirements

Because the harness runs on the agent, the agent host must have whatever toolchains the projects it claims tasks for need: a Go toolchain for Go projects, Node + a package manager for Node projects, etc. The platform never assumes a specific toolchain on the agent. Two patterns work well:
- **Generalist agents** with a broad toolbox image, capable of any project.
- **Specialist agents** that advertise narrow `tools` capabilities (e.g. `tool:go-1.22`, `tool:node-20`) and only get matched to tasks for projects that need them.

The platform does not push toolchains to agents; the operator builds the agent image they want.

### 12.3 Lifecycle

1. Agent starts: `forge agent --name w1 --roles worker,reviewer --server https://server:7777 --token-file ./agent.token`.
2. On first run, agent generates an Ed25519 keypair stored under `${agent_data_dir}/`.
3. Agent registers via `POST /v1/agents/register` with its public key, capabilities, version, host info, and the registration token.
4. Server returns an `agent_id` and a session token bound to the public key.
5. Agent fetches the catalog (`GET /v1/catalog/snapshot`), verifies the signature, caches it in memory.
6. Agent opens a WebSocket to `/v1/agents/{id}/events` and starts polling `/v1/agents/{id}/tasks/next` and/or waiting for `task_assigned` push events.

### 12.4 Task execution loop

For each claimed task:

1. **Pre-flight catalog check.** Agent calls `GET /v1/catalog/version`. If newer than cached, agent refetches and re-verifies the signature.
2. **Fetch task descriptor.** `GET /v1/runs/{run_id}/inputs` returns: task type, work package id, spec reference (a path inside the project repo, not its content), allowed/forbidden paths, harness profile id, role hint, prompt id, context recipe id, and any extra payload. No source code in this response.
3. **Materialize working copy.** Agent uses the project's git URL (which it gets from the project descriptor returned by the platform) and the credential ref it was authorized for to clone into a per-run directory: `${agent_work_dir}/runs/<run_id>/`. For repeat work on the same project, the agent reuses a worktree cache.
4. **Assemble context.** Agent calls `GET /v1/runs/{run_id}/context-bundle` for retrieved summaries / embeddings the platform has computed. The agent supplements this with file reads from its own working copy as the LLM tool-calls for them.
5. **Drive LLM.** Agent constructs the chat request from the prompt template + assembled context + tool definitions, then streams the chat loop with the provider chosen by routing. Tool calls are executed agent-side (Section 12.5). The loop terminates when the LLM emits a final structured result that conforms to the task's output schema.
6. **Apply diff and commit.** Agent applies the LLM's file edits inside the sandbox, refuses any edits outside `allowed_paths` or in `forbidden_paths`, commits on a feature branch (e.g. `forge/wp-<id>/<task-type>-<run_id>`).
7. **Run harness.** Agent executes the harness profile from the catalog against the new commit (Section 11). Evidence is captured locally and uploaded as a blob.
8. **Push to git.** If the harness passes (or partially passes per profile), agent pushes the branch to the project's git backend. If it fails outright, the branch is kept locally for debugging and not pushed.
9. **Submit result.** Agent posts the run result over REST: pre/post commit SHAs, branch name, harness report id, summary of what was done, token/cost stats, tool-call log. The body is small and metadata-only — the diff itself is *not* in the body.
10. **Stream telemetry.** Throughout steps 5–9 the agent streams structured log frames and metric samples over the WS log channel.

The agent never sends code over the control channel. Reviewers fetch the diff from git when they need to read it.

### 12.5 Tool execution (agent-side)

Inside the LLM loop, the agent executes tool calls using a small built-in registry plus the catalog of external MCP servers:

Built-in tools:
- `read_file(path)` / `list_dir(path)` / `glob(pattern)` — read inside the working copy.
- `write_file(path, content)` / `apply_diff(diff)` — write inside the working copy, fenced by `allowed_paths`.
- `exec_in_sandbox(cmd, cwd, timeout)` — run a command inside the local sandbox.
- `web_search(query)` — only if the agent has a configured search backend (e.g. Tavily, Brave, Bing).
- `http_fetch(url)` — only against an allowlist supplied by the catalog; per-task overrides possible.
- `mcp_call(server_name, tool_name, args)` — calls an external MCP server from the catalog.

Each tool call is logged with arguments and result digests in the run's tool-call log, which is streamed to the platform.

### 12.6 Capabilities

An agent advertises:
- `roles`: e.g. `worker`, `reviewer`, `orchestrator`.
- `tools`: agent-side tools enabled (e.g. `web_search:tavily`, `mcp:context7`, `http_fetch`).
- `toolchains`: free-form labels declaring what the host can build/test (e.g. `go-1.22`, `node-20`, `python-3.12`, `docker`). Used for scheduler matching when projects declare required toolchains.
- `network`: `none` | `egress` | `proxy` — whether and how the agent has outbound network beyond the platform itself.
- `sandbox_backends`: which sandbox modes this agent supports (`process`, `oci`, `firecracker`).
- `concurrency`: max parallel runs.
- `labels`: free-form tags (e.g. `behind-vpn`, `gpu-host` if the operator wants GPU-only LLM hosts to run agents).

Notably absent: GPU info as a *requirement*. Agents do not need a GPU. (If an operator chooses to put an agent on a GPU host because it also serves Ollama on the same machine, that's fine, but it's not modeled.)

### 12.7 Concurrency

An agent may run more than one task concurrently (configurable, default 1). Tasks are leased; lease renewal is automatic; if the agent crashes, leases expire and tasks are re-queued. Each concurrent run gets its own working-copy directory and its own sandbox.

### 12.8 Versioning

Agent version and protocol version are reported on registration. Server may refuse agents whose protocol version is incompatible. A `protocol_version` constant lives in `internal/proto/version.go`. Catalog format also has its own version.

### 12.9 Failure modes

- **Lost connection to platform mid-run.** Agent finishes the run if possible (uses cached catalog), buffers logs, attempts to submit the result on reconnect. If reconnect fails before the result is submitted, the run lease expires and the platform requeues the task; the agent will not double-submit (idempotency on `run_id`).
- **Catalog signature mismatch.** Agent refuses to use the snapshot, alerts the platform, and stops claiming new tasks until a valid snapshot arrives.
- **LLM provider 5xx / rate limit.** Agent backs off, optionally tries a fallback model from the catalog if routing allows, otherwise marks the run failed with a typed reason.
- **Sandbox failure.** Agent marks the run failed at phase `sandbox_init` and may quarantine itself (stop claiming) until `forge doctor --agent` clears it.
- **Disk full / git push rejected.** Agent retries; persistent failure marks the run failed with a typed reason.

### 12.10 Footprint

Target: idle agent < 35 MB RSS (slightly larger than v0.2 because the harness lives here now); single binary < 35 MB on disk; no platform-side dependencies. The agent host's footprint depends on the toolchains installed for the projects it serves.

---

## 13. MCP Tool Surface

There are **two distinct MCP-related roles** in Forge. Conflating them causes confusion, so be explicit:

1. **Forge as an MCP server** (`internal/mcp`, component C17). Forge exposes its own management surface (project create, task pull, submit result, query context, etc.) as an MCP server so that *external clients* — Continue.dev, Claude desktop, IDE plugins, other LLMs — can drive Forge as a tool provider. This is described in Sections 13.1–13.3.

2. **Forge's catalog of external MCP servers** (`internal/mcpcat`, component C12). The platform stores a list of *outside* MCP servers that agents are allowed to call as tools during a task: e.g. context7, web-search MCPs, project-specific MCPs supplied by the operator. The platform never calls these itself; it just publishes the list to agents via the catalog. Agents instantiate MCP clients to talk to them directly. This is described in Section 13.4.

These two paths use the same MCP protocol but go in opposite directions: in (1) Forge is the *server*; in (2) Forge curates servers but the *agent* is the client.

### 13.1 Why MCP and REST both

REST is for humans, for the Svelte UI, and for Forge's own agents (which speak REST/WS to the platform). MCP is for **other LLMs / IDEs** that want to use Forge as a tool provider — to push project ideas, pull tasks, submit results, query context. Internally, Forge agents do not talk MCP to the platform; they speak the REST/WS agent protocol because they are first-class clients with rich state on each call.

### 13.2 Tool catalog (v1)

Project & planning:
- `create_project(name, description, repo?)`
- `plan_project(project_id, brief)` → triggers an orchestrator task
- `list_projects()`, `get_project(project_id)`

Work packages & tasks:
- `list_work_packages(project_id)`
- `create_task(project_id, type, payload, work_package_id?, priority?)`
- `claim_task(role, capabilities)`
- `get_next_task(role, capabilities)` (pull)
- `submit_run_result(run_id, result_payload)`
- `report_progress(run_id, message, percent?)`

Context & memory:
- `query_context(project_id, query, recipe?)`
- `save_context(project_id, kind, content, tags?)`
- `summarize(scope_id, kind)` (kicks off summarization task)

Agents:
- `list_agents(filter?)`
- `pause_agent(agent_id)` / `resume_agent(agent_id)`

Providers / models:
- `list_models(role?)`
- `test_provider(provider_id)`

Audit / observability:
- `read_audit(filter)`
- `read_run_logs(run_id, since?)`

All tools have JSON Schemas and produce JSON Schema'd outputs. Authorization is per-token; the operator can grant a tool-call token to Continue.dev with only the scopes it needs (typically `project:create`, `task:pull`, `run:submit`, `context:query`).

### 13.3 Tool execution

Tool calls into Forge's MCP server are first-class entities in the DB (`ToolCall`). Each call is correlated with the upstream conversation and message that produced it, the audit event chain, and the resulting task or run.

### 13.4 The catalog of external MCP servers (used by agents)

Operators register external MCP servers in the platform via the UI or API. Each entry includes transport (`stdio` / `http` / `ws`), endpoint, optional auth secret, optional tool allowlist, and visibility rules (which agents/projects may use it). These entries are part of the catalog (Section 9.6) and are signed and distributed to agents.

When an agent runs a task and the LLM calls `mcp_call(server_name, tool_name, args)`:

1. Agent looks up `server_name` in the cached catalog. If absent or not visible to this project, the call is rejected before any I/O happens.
2. Agent looks at the project's per-project MCP authorization (some projects may restrict the MCP set further than the global visibility).
3. Agent opens (or reuses) an MCP client connection to the server, using its cached credentials.
4. Agent executes the tool, captures input/output digests, returns the result to the LLM.
5. The full call (server, tool, arg digest, result digest, latency, error) is logged to the run's tool-call log and streamed to the platform.

Common MCP servers operators are expected to register: a code-search MCP (e.g. context7), a web-search MCP, an issue-tracker MCP for the project, a documentation MCP, internal company MCP servers. Forge does not bundle or proxy these; it just publishes their addresses and credentials to authorized agents.

---

## 14. Chat Router & Brain

### 14.1 The chat surface

The server exposes a chat endpoint (`POST /chat/messages`) and a WS event stream (`/chat/{conversation_id}/events`). The Svelte UI's chat tab is the primary client; external MCP clients can also drive a conversation.

### 14.2 The brain

The "Chat Brain" is the LLM bound to the `chat_brain` role. Requirements: tool-calling support, large context window, low latency. Its job is **routing and orchestration**, not implementation.

The brain runs **inside the server process** using the server's own LLM client (component C18a). It uses the same provider catalog as agents — it just reads it directly from the DB instead of receiving a signed snapshot. The platform process therefore makes outbound LLM calls for the chat brain only. (This is consistent with "the platform doesn't run AI models locally" — it doesn't host them, it just calls them, the same way the UI calls the platform.)

### 14.3 Brain prompt (canonical)

The brain is given:
- The current conversation history (trimmed via context engine).
- A system prompt describing Forge: available roles, available tools, the workflow, the security model, what it can and cannot do.
- The catalog of MCP tools as tool schemas.
- Project context if a project is active (summary + recent activity).

The brain is **not** allowed to write code directly into the repo. To make code changes it must create or update tasks via the task tools and let the worker role handle them. This is enforced by withholding any "write_file" / "apply_diff" tool from the brain.

### 14.4 Execution loop

```
loop:
  1. user_message arrives
  2. assemble system prompt + history + active project ctx
  3. call brain via gateway (streamed)
  4. while brain emits tool_call:
       a. validate args against tool schema
       b. authorize against caller's scopes
       c. execute tool (may create tasks, run queries, etc.)
       d. feed tool_result back into the conversation
  5. when brain emits a final assistant message: stream to UI, persist
```

Every step is audited.

### 14.5 Hand-off pattern

When the brain decides that a task should be worked on, it calls `create_task(...)` or `plan_project(...)`. The worker / orchestrator agents pick the task up via the normal scheduler. The brain then either waits on the run via `wait_for_run(run_id)` or returns control to the user. This separation is what keeps the brain stateless about implementation details.

---

## 15. Security Architecture

Security is a first-class non-functional requirement and a major reason for the platform's design choices.

### 15.1 Threat model

Adversaries considered:
- A malicious or compromised LLM provider (returns hostile outputs).
- A malicious agent process on a shared host.
- A malicious or compromised plugin or external MCP client.
- An attacker on the network between agent and server.
- A user with low privileges trying to escalate.
- A malicious or compromised dependency.

Out of scope for v1: attackers with kernel-level access to the host, hardware-level side-channel attacks.

### 15.2 Identity & authentication

- **Operator users** authenticate via local username/password (Argon2id) or OIDC. MFA via TOTP optional.
- **API tokens** are scoped (least privilege) and rotatable.
- **Agents** authenticate with an Ed25519 keypair generated on first run. Server stores the public key fingerprint at registration. Every agent request is signed; signature includes a nonce and timestamp.
- **mTLS** is supported and recommended for agent ↔ server traffic; auto-generated CA + per-agent certs available via `forge tool gen-agent-cert`.

### 15.3 Authorization

RBAC at the resource level: `project`, `task`, `run`, `agent`, `provider`, `model`, `prompt`, `chat`, `secret`, `audit`. Roles: `admin`, `operator`, `viewer`, `agent`, `tool` (for MCP). Tokens carry scopes; every API handler is wrapped in an authz middleware that checks `(actor, action, target)`.

### 15.4 Secrets at rest (server side)

- Master key sourced from OS keystore (Keychain on macOS, Credential Manager on Windows, libsecret on Linux), or env, or sealed file.
- Per-secret encryption with AES-256-GCM and a random nonce; ciphertexts stored in `secrets` table.
- API responses never leak ciphertexts; only references (`name`).
- Rotation: `forge tool rotate-master-key` re-encrypts all secrets atomically.

### 15.5 Credential delivery to agents

Provider API keys, MCP server credentials, and per-project git credentials are stored encrypted on the platform. They reach agents as part of the catalog snapshot (Section 9.7) under three rules:

- **Wrapped per agent.** When a snapshot is sent to agent A, each credential is encrypted under a key derived from agent A's session keypair. Agent B cannot decrypt agent A's snapshot.
- **In-memory only on the agent.** Agents never persist credentials to disk. Cached catalog files on the agent contain wrapped ciphertexts only.
- **Visibility-scoped.** A credential is only included in the wrapped snapshot if the agent's labels and the project assignment match the credential's visibility rules. Least privilege by default.

When the operator chooses `agents.llm_routing.mode: proxy`, no provider credentials are delivered to agents at all — the platform makes the upstream calls itself. This is the recommended mode when agents run in less-trusted environments.

### 15.6 No-code-on-the-platform invariant

The platform never receives, stores, or transmits project source code over the agent control channel. Concretely:
- Agent results are metadata only (commit SHAs, branch names, harness summaries, log digests).
- Diffs are not sent to the platform; reviewers fetch them from git.
- The platform DB has no `code` or `diff_content` columns.
- Logs and harness evidence may contain compiler output and stack traces; these are sanitized for the well-known credential patterns the secret-scan check uses.

This invariant is enforced by API-level validators on the result endpoint (refuses payloads above a small size limit and rejects suspicious shapes) and is asserted in the conformance test suite.

### 15.7 Sandboxing (agent side)

The harness and tool calls run inside an agent-side sandbox. Three backends, selectable per agent via the catalog-distributed sandbox profile:
- `process`: minimal — UID/GID drop, `chroot`/`pivot_root`, rlimits, seccomp filter (Linux-only). Default on Linux.
- `oci`: a small OCI image (`ghcr.io/forge/runtime`) launched per run via the embedded runtime (containerd-shim or buildah-style); resource limits via cgroups.
- `firecracker`: per-run microVM. Strongest isolation; for self-modification runs and untrusted code, the harness runs in this mode by default.

Sandbox network policy:
- `deny`: no network.
- `proxy`: through an HTTP proxy with an allowlist (e.g., the configured registries and LLM endpoints).
- `allow_egress`: full egress (only for development).

The platform never runs a sandbox itself — it has no project code to sandbox.

### 15.8 Self-modification gates

When a task targets the Forge repo itself:
- The agent host must have `firecracker` available; the harness profile forces firecracker for `target=self`.
- Twin-agent verification (Section 11.6) is mandatory: a second, distinct agent re-runs the harness on a fresh worktree and signs an independent report. Mismatch aborts.
- The diff cannot touch `internal/auth`, `internal/secrets`, `internal/sandbox`, `internal/harness`, or `internal/store/migrate` without an explicit `security_review` task that itself requires two human approvals.
- The integrate phase produces a signed PR/branch artifact, never an in-place merge.
- A change to the harness itself triggers a full self-test gauntlet (Section 17.4).

### 15.7 Supply chain

- The release pipeline produces SLSA-style provenance.
- Releases are signed (cosign). `forge` verifies its own update before applying it.
- Dependency pins: Go modules with `go.sum` enforced; `govulncheck` runs in CI.

### 15.8 Audit log

Append-only, hash-chained audit log. Every state-changing action emits an event. The log is exposed in the UI with filtering. Integrity verified by `forge tool audit-verify`.

### 15.9 Privacy

No telemetry by default. Optional anonymous metrics opt-in. Project content never leaves the host except via configured providers.

---

## 16. Observability & Telemetry

### 16.1 Logs

Structured JSON, level-aware, with correlation IDs threaded through every request, run, and tool call.

**Server-side logs.** Sink: stdout + rolling file at `${data_dir}/logs/forge.log`.

**Agent-side logs.** Each agent emits structured log frames carrying `run_id`, `phase`, `timestamp`, `level`, `message`, and a small structured payload. Frames are written to a local rolling file on the agent (for offline debugging) and **streamed to the platform** over the agent's WebSocket as they happen, so the operator can watch a run progress live in the UI.

**Stream design.** A single multiplexed WS frame format carries log lines, metric samples, progress percentages, tool-call records, and harness check completions. Frames are batched (default flush every 1 s or 64 KiB, configurable in `agents.log_streaming`). When the platform is unreachable, frames spill to a bounded local ring buffer (drop-oldest) and are flushed on reconnect; agents never block their work loop on telemetry.

**Backpressure.** The platform may signal `slow_down` to an agent producing unusually high log volume; agents respond by dropping `debug` frames first, then `info`, before degrading to summary-only. Errors and harness frames are never dropped. This is what makes streaming "better for optimization" than agents holding logs and posting in bulk: log size is bounded server-side at ingest.

**UI.** The log viewer streams from the platform over WS to the operator's browser. There is no agent-to-browser direct path.

### 16.2 Metrics

Metrics are split into two families: **operational** (is the platform healthy?) and **efficacy** (is the platform actually doing its job — building software autonomously?). Both are emitted as Prometheus-style counters / gauges / histograms and surfaced in the UI.

#### 16.2.1 Operational metrics

- `forge_tasks_total{type,status}`
- `forge_runs_duration_seconds{role,task_type}` (histogram)
- `forge_llm_tokens_total{provider,model,direction}`
- `forge_llm_cost_cents_total{provider,model}`
- `forge_llm_call_duration_seconds{provider,model}` (histogram)
- `forge_llm_call_errors_total{provider,model,kind}`
- `forge_harness_checks_total{name,status}`
- `forge_harness_check_duration_seconds{name}` (histogram)
- `forge_agents_online`
- `forge_queue_depth{role}`
- `forge_db_query_duration_seconds{op}` (histogram)
- `forge_gitserver_ops_total{op,status}` (when embedded git is enabled)

#### 16.2.2 Efficacy metrics (autonomy KPIs)

These exist specifically to answer the question: *how well is the platform performing as an autonomous agent platform?* Each metric has a dimension breakdown so the UI can attribute failures and surface bottlenecks.

**Outcome rates**
- `forge_runs_outcome_total{task_type,role,outcome}` — `outcome` ∈ {`succeeded`, `failed`, `cancelled`, `timed_out`}.
- `forge_tasks_first_attempt_success_ratio{task_type}` — gauge: proportion of tasks that completed on the first run, no retries.
- `forge_tasks_autonomous_completion_ratio{project_id}` — gauge: proportion of tasks completed without a single human intervention (no manual override, no rejected review). This is the headline autonomy KPI.
- `forge_human_interventions_total{kind,task_type}` — `kind` ∈ {`override`, `review_reject`, `manual_retry`, `manual_cancel`, `replan`, `approval_blocked`}.

**Failure attribution (where things break)**
- `forge_run_failures_total{task_type,role,phase,reason}` — `phase` is the lifecycle phase that failed: {`fetch_inputs`, `clone`, `llm_call`, `tool_call`, `submit_result`, `harness`, `review`, `integrate`}. `reason` is a normalized error class (`schema_invalid`, `tool_call_failed`, `llm_timeout`, `llm_refused`, `diff_outside_allowed_paths`, `lint_failed`, `tests_failed`, `secret_detected`, `coverage_dropped`, `merge_conflict`, `provider_rate_limited`, `provider_5xx`, `budget_exceeded`, `manual_reject`, `unknown`).
- `forge_harness_failures_total{check}` — counter: which harness check failed, broken down by name.
- `forge_repair_loop_length{work_package_id}` — histogram: how many repair runs were required before a work package's implement task converged.
- `forge_replan_count{project_id}` — counter: replans per project.

**Throughput and time-in-stage**
- `forge_task_lifecycle_seconds{task_type,phase}` — histogram: time spent in each phase (queued, claimed, running, needs_review, approved, integrate).
- `forge_intake_to_done_seconds{project_id}` — histogram: end-to-end wall-clock from project intake to v1 done.
- `forge_agent_utilization_ratio{agent_id}` — gauge: time-in-run / time-online.
- `forge_throughput_tasks_per_hour{role}` — gauge.

**Cost efficiency**
- `forge_cost_per_task_cents{task_type,outcome}` — histogram: total LLM cost per task, segmented by outcome so you can see how expensive failures are.
- `forge_tokens_per_successful_run{role}` — histogram.
- `forge_budget_breach_total{scope,kind}` — counter: budget breaches at project / role / global scope.

**Quality**
- `forge_review_changes_requested_ratio{role}` — gauge: proportion of runs sent back by reviewer.
- `forge_post_integrate_revert_total{project_id}` — counter: integrations that had to be rolled back.
- `forge_test_flake_ratio{project_id}` — gauge: test results that flipped between identical re-runs (signal that the project's test suite is flaky and harness verdicts are noisy).

**Self-improvement**
- `forge_self_modification_runs_total{outcome}` — counter.
- `forge_self_test_gauntlet_failures_total{check}` — counter.

These metrics are derived from the underlying state in the DB; the metric exporter reads from a small set of materialized views so dashboards are fast even with months of history.

### 16.3 Traces

OpenTelemetry traces, off by default. When enabled, every chat turn and every run is a trace; tool calls and LLM calls are spans.

### 16.4 Health

`/healthz` for liveness, `/readyz` for readiness (DB ok, master key loaded, providers reachable). `forge doctor` produces a one-shot human-readable report.

### 16.5 UI dashboards

- **Overview**: running/queued/failed counts, recent runs, current spend.
- **Autonomy**: the headline efficacy view. Cards for autonomous-completion ratio, first-attempt success ratio, mean repair-loop length, mean intake-to-done time, cost per successful task. A stacked bar chart of `forge_run_failures_total` by phase × reason so the operator can see at a glance *where* things tend to fail (e.g., "60% of failures are at the `harness` phase, of which 70% are `tests_failed`"). A funnel from intake through plan → design → implement → verify → review → integrate, with drop-off rates and median dwell time at each stage. A leaderboard of the most-failed harness checks and the most-failed task types per project.
- **Project**: timeline, work packages, latest plan, branch status, project-scoped efficacy KPIs.
- **Run**: streaming logs, diff viewer, harness check tree, LLM call tree, tool-call tree, costs, links to the same run's place in the funnel.
- **Agent fleet**: online/offline, current task, capabilities, utilization, recent throughput.
- **Provider health**: latency, error rate, budgets, per-provider efficacy attribution (does switching providers change autonomous-completion ratio?).
- **Audit**: chronological feed, filter by actor/action.

---

## 17. Self-Improvement (Bootstrapping)

### 17.1 Promise

Once Forge is bootstrapped, all *new* features for Forge are tracked as projects in Forge and implemented through Forge's own workflow. The Forge repo is itself a "project" with `target=self`.

### 17.2 Bootstrap sequence

The first version is built by humans and Claude using conventional tools. The first feature implemented through Forge against itself is a small, low-risk feature (e.g., a new lint rule, a new dashboard tile). This is the "Hello, self" milestone.

### 17.3 Guardrails for self-edits

- All `target=self` runs use Firecracker sandboxing on the agent host. An agent without firecracker available cannot claim such tasks.
- **Twin-agent verification is mandatory** (Section 11.6): two distinct agents must produce matching harness reports.
- Allowlisted file paths required. Forbidden paths (`internal/auth`, `internal/secrets`, `internal/sandbox`, `internal/harness`, migrations) are gated behind a `security_review` task that itself requires two human approvals.
- Two human approvals required before integrate.
- Harness includes a "self-test gauntlet" (Section 17.4) that both agents must independently pass.

### 17.4 Self-test gauntlet

For self-modification runs, the harness additionally runs:
- Full unit + integration test suite.
- A bootstrap test: build the new binary, start a server, register an agent, run a canary project end-to-end against a local Ollama provider, assert success.
- A migration test: existing DB upgrades cleanly to the new schema.
- A rollback test: downgrade migration applies cleanly.
- A signature test: the produced binary verifies against the expected signing key.

### 17.5 Drift control

Forge keeps a `self_capabilities.json` describing what the platform can do. Self-modification tasks must update this file; reviewers verify it matches the diff.

---

## 18. Spec-Driven Development Practices

These practices apply both to building Forge initially and to Forge using itself.

### 18.1 Spec hierarchy

```
project_spec.md (this document, for Forge itself)
└── work_package_spec.md (per WP)
    ├── design.md (when applicable)
    ├── api.md (endpoints, schemas)
    ├── tests.md (acceptance scenarios)
    └── risks.md (security, perf, rollback)
```

For projects built by Forge, the orchestrator role generates this hierarchy as the output of its `plan` and `design` tasks.

### 18.2 Acceptance criteria are executable

Every work package has acceptance criteria that map 1-1 to test names. The harness fails the run if a criterion's test is missing.

### 18.3 Definition of done

A work package is done when:
- Code merged to the project's default branch.
- All acceptance tests pass on a clean checkout.
- Coverage delta is non-negative for changed files.
- All harness checks green.
- Spec file updated to reflect any deviations.
- An entry exists in `CHANGELOG.md`.

### 18.4 Plan change discipline

Mid-flight plan changes are first-class: they create a `replan` task that updates the work-package graph and is itself reviewed. Silent scope changes are rejected by the harness (diff must match the work package's allowed paths).

### 18.5 Documentation as code

Every public API and every MCP tool has a doc-comment block that the build pipeline extracts into a generated `docs/api.md` and `docs/mcp.md`. The harness fails if the doc surface and the code drift.

### 18.6 Prompt versioning

Prompts are versioned (semver). A prompt change is a commit with a migration note describing impact. The harness runs golden-output tests for prompts (small fixed inputs → expected outputs verified by structural assertions, not exact match).

### 18.7 Reviewable diffs

Worker outputs are constrained to small diffs. If a worker produces a diff above N lines without a justification, the harness fails the run and the workflow splits the work package.

---

## 19. CLI & Operational Surface

```
forge init                                   # bootstraps config, db, default keys
forge server [--config FILE] [--bind ADDR]   # run server
forge agent --name N --roles R1,R2 [...]     # run agent
forge migrate up | down | status
forge doctor                                 # health + config check
forge harness run <run_id>
forge tool gen-agent-cert <name>
forge tool rotate-master-key
forge tool export <project_id>
forge tool import <archive>
forge tool audit-verify
forge update [--check] [--apply]
forge version
```

All commands accept `--json` for machine-readable output.

---

## 20. Build, Packaging, Deployment

### 20.1 Repo layout

```
/cmd/forge/                 # main package; CLI entry; mode dispatch
/internal/
  api/        cli/        config/    chat/      agents/   agentruntime/
  context/    harness/    llm/        memory/   mcp/       obs/
  prompts/    proto/      projects/   repo/     runs/      sandbox/
  scheduler/  secrets/    store/      tasks/    testrun/   updater/
  web/        workflow/
/web/                       # Svelte source; built into web/dist; embedded
/specs/                     # this spec and per-WP specs
/docs/                      # generated docs
/scripts/                   # build/release/dev tooling
/test/                      # integration tests; bootstrap test
```

### 20.2 Build

- `make build` → cross-compiles `forge` for darwin/linux/windows on amd64/arm64 with `CGO_ENABLED=0` (using `modernc.org/sqlite` for the pure-Go path; fall back to a CGO build for performance when desired).
- Web assets: `pnpm -C web build` produces `web/dist`, embedded via `//go:embed web/dist`.
- Reproducible builds: `-trimpath -buildvcs=true -ldflags "-s -w -X main.version=..."`.

### 20.3 Release

- Release pipeline produces signed binaries + SBOM + provenance.
- A small homepage is generated from `/docs`.
- Release notes are auto-extracted from `CHANGELOG.md`.

### 20.4 Resource footprint targets

- Cold start (server, no projects): RSS < 80 MB, CPU near zero idle.
- DB at 1k tasks, 100 projects: < 200 MB.
- Single binary size: target < 60 MB stripped (UI + DB + everything).

---

## 21. Non-Functional Requirements

| NFR | Target |
|---|---|
| Cold-start time (server) | < 1.5 s on a modern laptop |
| Median API latency (read) | < 25 ms |
| Median API latency (write) | < 80 ms |
| Tasks/sec sustained | ≥ 50 simple task transitions/sec |
| Concurrent agents | ≥ 32 on a single server |
| LLM streaming TTFB pass-through overhead | < 50 ms |
| Recovery after crash | DB consistent; in-flight runs requeued within 60 s |
| Test suite total runtime | < 10 min on a 4-core laptop |
| Binary size | < 60 MB stripped |
| Memory at idle | < 80 MB |
| Backup time for 1 GB data | < 30 s |
| Restore time for same | < 60 s |

---

## 22. Testing Strategy

### 22.1 Unit tests

Every package ships with table-driven unit tests; coverage gate ≥ 80 % for `internal/` packages on changed lines.

### 22.2 Integration tests

`/test/integration` exercises:
- Full task lifecycle through the engine.
- Agent register → poll → run → submit → harness → done.
- Provider adapters against a recorded-tape (`go-vcr`-style) harness; one live test per provider gated by env var.
- Chat brain orchestration with a stub LLM (deterministic tool-call sequences).

### 22.3 Bootstrap test

`/test/bootstrap` builds the binary, starts a server with a temp dir, registers a fake agent, plans + implements a tiny canary project ("write a function that returns 42 with a passing test"), and asserts success. This test runs in CI and as part of the self-test gauntlet.

### 22.4 Property tests

For state machines (task lifecycle, workflow engine), property-based tests assert invariants under random transition sequences.

### 22.5 Fuzz tests

Adapters and config parsers are fuzz-tested.

### 22.6 Soak tests

A nightly soak test runs 1k tasks across 8 agents and asserts that memory, file descriptors, and DB size remain bounded.

### 22.7 Security tests

- Authn/authz negative tests for every endpoint.
- Sandbox escape tests (canary writes outside the jail must fail).
- Self-modification negative tests (forbidden paths rejected).

---

## 23. Acceptance Criteria (v1)

The platform is "v1" when:

1. A fresh user can `forge init && forge server` and reach the UI in under 90 seconds on a clean laptop, with embedded SQLite as the DB.
2. The same operator can swap the DB to an external Postgres by changing config and running `forge migrate up`, with no other changes.
3. A user can configure at least one local provider (Ollama) and one remote provider (Anthropic) via the UI without editing files.
4. A user can create a project from the UI by selecting one of three storage backends: a local folder path, an external git remote URL with a stored credential, or the embedded git server.
5. A user can spawn one or more agents from the same binary on the same or another machine; agents work behind a NAT with only outbound HTTPS.
6. The orchestrator role plans the project into work packages.
7. Worker agents pick up implement tasks, clone the project from its git backend, talk to LLMs directly using the cached, signed catalog, execute tool calls (web search, REST, external MCP servers from the catalog), commit and push to git, and submit a metadata-only result. No source code crosses the agent ↔ platform channel.
8. The agent-side harness runs every required check and submits a signed `HarnessReport`. The platform stores the report and the evidence blobs, and never re-executes the checks itself (twin-agent verification handles trust for self-modification).
8b. Provider and MCP-server catalog updates committed in the UI propagate to all online agents within 5 seconds via WS push, and within `pull_interval` for any agent whose WS is briefly disconnected. Agents refuse to start a new task without a current catalog version.
9. The reviewer role gates merge; a human can override.
10. The full plan → done loop runs unattended for the canary "answer is 42" project against each of the three project storage backends.
11. The chat brain can drive all of the above through tool calls from the UI's chat tab.
12. The Autonomy dashboard shows live values for: autonomous-completion ratio, first-attempt success ratio, mean repair-loop length, failure breakdown by phase × reason, and cost per successful task.
13. Every state-changing action is in the audit log; the log integrity-verifies.
14. The platform survives `kill -9` of the server with no DB corruption and resumes in-flight tasks (validated on both SQLite and Postgres backends).
15. A self-modification run for a low-risk feature passes the self-test gauntlet end-to-end (Section 17).
16. Single binary size is below the target; cold-start and idle memory are within targets.
17. Agent binary is < 30 MB, idle RSS < 25 MB, runs in a `scratch` or distroless container, has no GPU or language-toolchain dependencies.
18. Documentation is generated and published from `/docs`.

---

## 24. Open Questions & Decisions to Lock Before Implementation

These need an explicit decision early; defaults are listed for each.

| # | Question | Default proposal |
|---|---|---|
| Q1 | SQLite via CGO or pure Go? | Pure Go (`modernc.org/sqlite`) for v1; CGO build available as opt-in. |
| Q2 | External DB drivers in scope for v1? | Postgres in v1; MySQL/MariaDB in v1.1 unless someone needs it sooner. |
| Q3 | Vector store: sqlite-vec, pgvector, or in-house HNSW? | Auto-select per DB backend; sqlite-vec on SQLite, pgvector on Postgres, internal HNSW otherwise. |
| Q4 | Sandbox default backend on Linux (server-side harness)? | `oci` if available, falling back to `process`. |
| Q5 | Workflow engine: home-grown DSL or Temporal-style? | Home-grown deterministic FSM with replay; revisit if complexity warrants. |
| Q6 | UI build tool? | Vite + Svelte 5; bundle to `web/dist`. |
| Q7 | Auth default? | Local users + tokens; OIDC opt-in. |
| Q8 | License? | TBD before public release. |
| Q9 | Telemetry default? | Off; opt-in only. |
| Q10 | Self-update default? | Off; opt-in only. |
| Q11 | Schema migrator? | Plain SQL files + a small Go runner; dialect-aware; avoid heavy frameworks. |
| Q12 | Embedded git server in v1? | Yes, but off by default; minimal Smart-HTTP via go-git. |
| Q13 | First external git provider with PR/MR adapter? | GitHub; GitLab and Gitea soon after. |
| Q14 | Agent build target? | Static Go binary; the operator builds a host image that includes whatever language toolchains the projects need (Go, Node, etc.). The platform does not ship toolchains. |
| Q15 | LLM call mode default? | `direct` for the homelab/default install; `proxy` for hardened deployments. |
| Q16 | Catalog sync default mode? | `hybrid` (push via WS + pull on interval + pre-task pull). Pre-task pull is non-negotiable. |
| Q17 | Twin-agent verification default? | Off for normal runs; **mandatory** for `target=self` and any task type the operator marks `verification: twin`. |

---

## 25. Roadmap (Phases)

Each phase ends with a runnable, testable artifact.

### Phase 0 — Bedrock (week 1)
- Repo scaffold, CI, build, lint, format.
- Spec frozen (this document) and per-WP spec template adopted.

### Phase 1 — Single binary, single mode (weeks 2–3)
- CLI dispatch, config loader, embedded SQLite, dialect-aware migrations, secrets.
- `forge init`, `forge doctor`, `forge migrate`.

### Phase 2 — Server core (weeks 4–5)
- HTTP API skeleton, auth, projects + tasks CRUD, audit log.
- Embedded UI shell (Svelte); projects + tasks views.
- Project storage backend `local_folder` working end-to-end.

### Phase 3 — LLM gateway + first provider (week 6)
- Provider interface; Anthropic and Ollama adapters.
- `/llm/chat` end-to-end with budgets and caching.

### Phase 4 — Agent runtime + catalog distribution (weeks 7–8)
- Agent registration, session keys, leases, pull/push task loop.
- Catalog distributor (signed snapshots, version endpoint, WS push of `catalog_changed`).
- Agent LLM client + first provider adapters (Anthropic, Ollama) used directly from agent.
- Agent tool executor: `read_file`, `write_file`, `exec_in_sandbox`, `web_search`, `http_fetch`, `mcp_call`.
- Agent repo manager: clone, branch, commit, push.
- End-to-end "implement" run on a hello-world project against a local folder.

### Phase 5 — Harness v1 (week 9)
- Agent-side sandbox (`process` backend on Linux).
- Schema/diff_sanity/secret_scan/lint/typecheck/build/tests checks running on the agent.
- Signed `HarnessReport` upload + evidence blob streaming.
- UI surfacing of harness reports and evidence.

### Phase 6 — Workflow engine + roles (weeks 10–11)
- Plan → implement → verify → review → done loop.
- Orchestrator and reviewer roles.

### Phase 7 — Project storage v2 + git remotes (week 12)
- `git_remote` backend with GitHub PR adapter and webhook intake.
- Embedded git server (`embedded_git`) behind a flag.

### Phase 8 — External DB + efficacy metrics (weeks 13–14)
- Postgres backend with pgvector; switchable via config.
- Efficacy metric exporter, materialized views, Autonomy dashboard.

### Phase 9 — Chat brain + MCP server (weeks 15–16)
- Tool registry, brain prompt, conversation persistence.
- MCP endpoint compatible with Continue.dev / Claude desktop.

### Phase 10 — Hardening (weeks 17–18)
- Sandbox `oci` backend, mTLS for agents, signed releases.
- Soak tests, fuzz tests, security tests.

### Phase 11 — Self-improvement gauntlet (weeks 19–20)
- `target=self` flag, Firecracker sandbox on agent, twin-agent verification, self-test gauntlet.
- First self-implemented feature ("Hello, self").

### Phase 12 — v1 release.

---

## 26. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Hallucinated outputs slip past harness | Medium | High | Deterministic checks on the agent, signed harness report, twin-agent verification for sensitive runs, reviewer role on top, audit trail. |
| Compromised agent forges harness reports | Low | Critical | Agent signs reports with session-bound key registered at platform; twin-agent verification for `target=self`; agent quarantine on signature anomalies; mTLS on control channel. |
| Provider credentials leak via agent | Low | High | Per-agent wrapped credentials, in-memory only, visibility-scoped, opt-in `proxy` mode for hardened deployments. |
| Catalog drift (agents using stale config) | Medium | Medium | Pre-task version check, WS push, signed snapshots, run records the catalog version it used. |
| Provider API drift | High | Medium | Adapter pattern, recorded-tape tests, capability flags. |
| Self-modification corrupts the platform | Medium | Critical | Firecracker, allowlisted paths, two-human approval, rollback test. |
| SQLite write contention | Medium | Medium | WAL mode, single writer, batched writes; benchmark and shard if needed. |
| Single-binary size bloat | High | Low | Build matrix, brotli for embedded UI, periodic budget. |
| Secret leakage via prompts | Medium | High | Secret scan in harness, redaction in logs, per-task allowlist. |
| Agent compromise | Low | High | mTLS + signed requests, capability tokens, sandboxed execution. |
| Vendor lock-in to one LLM | Low | Medium | Provider abstraction; capability-typed tools; golden tests across two providers. |
| Cost overruns | Medium | Medium | Per-project budgets, hard caps, cost dashboard. |
| Dialect drift between SQLite and external SQL | Medium | Medium | Single schema with a thin dialect layer; CI runs the full integration suite against SQLite, Postgres, and MySQL. |
| Project storage credentials leak | Low | High | Credentials referenced by id, encrypted with master key, never echoed in logs or API responses; secret scan in harness. |
| Embedded git server abuse | Low | Medium | Off by default; same auth as the API; no anonymous read by default; rate limits on push. |
| Efficacy metrics give false confidence | Medium | Medium | Surface raw counts alongside ratios; flag projects with low denominator; manual override events visibly tracked in autonomous-completion. |

---

## 27. Appendix

### 27.1 Example project intake → done

```
user (chat): Build me a small CLI that pretty-prints JSON with colors.
brain: calls plan_project(...)
orchestrator: produces PROJECT_PLAN.md + 4 work packages:
  WP1 spec; WP2 minimal CLI scaffold; WP3 colorizer; WP4 docs+release.
worker(WP1): writes spec → harness green → review approves
worker(WP2): scaffolds Go module, base CLI → harness green → review approves
worker(WP3): implements colorizer + tests → harness green → review approves
worker(WP4): writes README, CHANGELOG → harness green → review approves
integrator: tags v0.1.0, builds artifacts.
```

### 27.2 Example tool call (chat brain → server)

```json
{
  "tool": "create_task",
  "arguments": {
    "project_id": "01HXXX...",
    "type": "implement",
    "work_package_id": "01HYYY...",
    "payload": {
      "spec_blob_id": "sha256:...",
      "allowed_paths": ["cmd/jcat/**", "internal/colorize/**"],
      "acceptance_tests": ["TestColorizesObject","TestColorizesArray"]
    },
    "priority": 50
  }
}
```

### 27.3 Example provider adapter signature (Go)

```go
package providers

type Anthropic struct{ /* ... */ }

func (a *Anthropic) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) { /* ... */ }
func (a *Anthropic) Stream(ctx context.Context, req llm.ChatRequest) (llm.Stream, error)    { /* ... */ }
func (a *Anthropic) Embed(ctx context.Context, req llm.EmbedRequest) (llm.EmbedResponse, error) { return llm.EmbedResponse{}, llm.ErrUnsupported }
func (a *Anthropic) Rerank(ctx context.Context, req llm.RerankRequest) (llm.RerankResponse, error){ return llm.RerankResponse{}, llm.ErrUnsupported }
func (a *Anthropic) Health(ctx context.Context) error { /* ... */ }
```

### 27.4 Example harness profile (per project)

```yaml
harness:
  required:
    - schema
    - diff_sanity
    - secret_scan
    - lint:    { command: "golangci-lint run ./..." }
    - typecheck: { command: "go vet ./..." }
    - build:   { command: "go build ./..." }
    - tests:   { command: "go test ./...", report: "go-test-json" }
  optional:
    - dep_audit: { command: "govulncheck ./..." }
    - reproducibility: true
  allowed_paths:
    - "cmd/**"
    - "internal/**"
    - "web/**"
  forbidden_paths:
    - "internal/secrets/**"
    - "internal/auth/**"
```

### 27.5 Naming and codename

`Forge` is a working title. Alternates to consider: `Anvil`, `Atlas`, `Axiom`, `Conductor`, `Foundry`. Locking the name is part of Phase 0.

### 27.6 What's intentionally deferred

- Multi-tenant / multi-user concurrency on the same server beyond a small team.
- A plugin marketplace.
- Hosted cloud version.
- Mobile UI.
- Auto-scaling agent fleets.

These are sensible v2 topics; v1 should be ruthless about not bringing them forward.

---

**End of v0.1.** Next step (separate document): decompose this spec into work packages and tasks, produce per-WP specs, and queue them for the first internal run.
