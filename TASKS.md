# Agent Orchestrator — Task Log

All work on this project is recorded here.
Format: `[x]` = done · `[ ]` = pending · `[~]` = in progress

---

## Phase 1 — Go Backend Core

- [x] **#1 Project Setup** — `go.mod`, directory structure, `main.go`
- [x] **#2 Database Layer** — SQLite schema + CRUD (`db/models.go`, `db/db.go`)
- [x] **#3 Configuration System** — YAML parser, env-var substitution, validation (`config/`)
- [x] **#4 LLM Provider Abstraction** — interface, registry, OpenAI + Ollama implementations (`llm/`)
- [x] **#5 Server CLI + Startup** — flag-based CLI (no cobra), graceful shutdown (`cmd/server.go`)
- [x] **#6 Server HTTP API** — projects + tasks REST endpoints (`server/handlers.go`)
- [x] **#7 Agent Registration + Heartbeat API** — `POST /api/agents/register`, `POST /api/agents/:id/heartbeat`
- [x] **#8 Agent Task Polling Loop** — agent polls for queued tasks, claims and executes them
- [x] **Verify Phase 1** — built binary, ran Go tests, smoke-tested server + agent

---

## Phase 2 — Orchestration Engine

- [x] **#9 Router** — config-based routing, prompt templates, context builder (`router/`)
- [x] **#10 Server extras** — `POST /api/llm/chat` (one-shot LLM), `GET /api/metrics`
- [x] **#11 Tools** — plan, task-management, code-execution, context tools (`tools/`)
- [x] **#12 Agent execution engine** — full LLM-driven task loop with tool calling (`agent/executor.go`)
- [x] **#13 Logging package** — structured logger + metrics aggregation (`logging/`)
- [x] **#14 WebSocket `/ws/chat`** — connection handling, message queue, broadcast
- [x] **Verify Phase 2** — static analysis, cross-checked all Go signatures and handler contracts

---

## Phase 3 — Svelte UI

- [x] **#15 UI scaffold** — `package.json` (Svelte 5 / Vite 8 / Tailwind 4), `embed.go`, `vite.config.js`
- [x] **#16 UI core** — `src/lib/api.js`, `src/lib/stores.js`, `src/lib/ws.js`, `src/App.svelte`
- [x] **#17 UI pages** — `Projects.svelte`, `Tasks.svelte`, `Agents.svelte`, `Providers.svelte`, `Logs.svelte`, `Chat.svelte` — all rewritten for Svelte 5 runes + Tailwind v4
- [x] **Verify Phase 3** — cross-checked Go embed, build instructions, API field alignment

---

## Phase 3b — UI Upgrade & Test Suite

### Dependency upgrade (user-initiated)
- [x] Updated all UI deps to latest: Svelte 5.55.5, Vite 8.0.11, Tailwind CSS 4.3.0, `@sveltejs/vite-plugin-svelte` 7.1.2
- [x] Rewrote all pages for Svelte 5 rune syntax (`$state`, `$derived`, `onclick` vs `on:click`, etc.)
- [x] Updated `app.css` for Tailwind v4 (`@import "tailwindcss"`, `@theme {}` custom tokens)
- [x] Updated `postcss.config.js` for `@tailwindcss/postcss` (v4 plugin, no separate autoprefixer)
- [x] Added `svelte.config.js` (required by `@sveltejs/vite-plugin-svelte` v7)

### Test suite (new)
- [x] Added test deps to `package.json`: `vitest@3.2.3`, `@testing-library/svelte@5.2.7`, `@testing-library/jest-dom@6.6.3`, `@testing-library/user-event@14.6.1`, `@vitest/coverage-v8@3.2.3`, `jsdom@26.1.0`
- [x] Added test scripts to `package.json`: `test`, `test:watch`, `test:coverage`, `test:integration`
- [x] Created `vitest.config.js` — jsdom environment, globals, setupFiles, v8 coverage
- [x] Created `src/__tests__/setup.js` — `MockWebSocket` double, `mockFetch` helper, auto-cleanup
- [x] Created `src/__tests__/api.test.js` — 20 unit tests covering every API function
- [x] Created `src/__tests__/stores.test.js` — router, toasts, loading counter
- [x] Created `src/__tests__/ws.test.js` — WebSocket lifecycle, reconnect, send, ready getter
- [x] Created `src/__tests__/Projects.test.js` — rendering, create form, delete with confirm guard
- [x] Created `src/__tests__/Tasks.test.js` — rendering, filters, create form, queue action
- [x] Created `src/__tests__/Agents.test.js` — rendering, polling interval, response shapes
- [x] Created `src/__tests__/Chat.test.js` — WebSocket lifecycle, send/receive, typing indicator
- [x] Created `src/__tests__/integration.test.js` — full-stack tests (skipped unless `INTEGRATION_URL` set)

### go-task Taskfile
- [x] Created `agent-orchestrator/Taskfile.yml` covering:
  - `build`, `build:ui`, `build:server`
  - `test`, `test:server`, `test:ui`, `test:ui:watch`, `test:ui:coverage`, `test:integration`
  - `lint`, `lint:server`, `lint:ui`, `ci`
  - `run:server`, `dev`, `dev:ui`
  - `agents` (N agents, configurable roles + LLM config)
  - `start` (full stack: server + N agents, health-check loop, Ctrl+C cleanup)
  - `clean`, `clean:all`

---

## Phase 3c — Test Fixes

- [x] **Fix: Svelte 5 server/client resolution** — added `resolve: { conditions: ['browser'] }` to `vitest.config.js` so Vitest picks `svelte/src/index-client.js` instead of the SSR entry point (was causing `mount() is not available on the server` on every component test)
- [x] **Fix: `Tasks.test.js` — status badge multi-match** — `getByText('pending'/'completed'/'failed')` was matching both badge `<span>` elements AND the identically-named `<option>` elements in the status filter `<select>`; fixed with `{ selector: 'span' }`
- [x] **Fix: `Tasks.test.js` — Queue button multi-match** — both `t1` (pending) and `t3` (failed) render a Queue button; `getByText('Queue')` threw "multiple elements found"; changed to `getAllByText('Queue')`
- [x] **Fix: `Tasks.test.js` — `waitFor` Queue multi-match** — same issue in the queue-action test's `waitFor`; changed to `getAllByText`
- [x] **Fix: `Agents.test.js` — fake timer cleanup** — `vi.useRealTimers()` was inside the test body only; if the test failed it was never called, leaving fake timers on for all subsequent tests; moved to `afterEach`

---

---

## Phase 4 — Features & Optimization ✅

**Goal**: Add advanced providers, workflow scheduling, embeddings, token tracking, project context/memory, multi-agent safety, and error resilience.

### LLM Providers

- [x] **#26 Anthropic (Claude) Provider** — `llm/anthropic.go` + `llm/anthropic_test.go`
  - Implement `AnthropicProvider` satisfying `LLMProvider`
  - Handle Claude's `tool_use` / `tool_result` message format (different from OpenAI)
  - Support streaming and non-streaming responses
  - Handle Anthropic-specific error codes and rate limits
  - Register `"anthropic"` type in `registry.go` `InitFromConfig`
  - Tests: provider init, Chat with mocked API, tool_use round-trip, error cases (85%+ coverage)

- [x] **#27 Azure OpenAI Provider** — `llm/azure.go` + `llm/azure_test.go`
  - Implement `AzureOpenAIProvider` (wraps OpenAI wire format)
  - Azure URL pattern: `{base_url}/openai/deployments/{deployment}/chat/completions?api-version=2024-02-01`
  - Auth via `api-key` header (not `Authorization: Bearer`)
  - Handle Azure-specific errors (quota, deployment not found, content filter)
  - Register `"azure"` type in `registry.go` `InitFromConfig`
  - Tests: Azure URL building, auth header, deployment naming, error handling (85%+ coverage)

### Workflow Scheduler

- [x] **#28 Workflow Scheduler + Task Lifecycle** — `workflow/scheduler.go` + `workflow/state.go` + `workflow/scheduler_test.go`
  - State machine: `planned → queued → in_progress → needs_review → approved → completed | failed`
  - Auto follow-on task creation:
    - `implement` completes → create `review` task (same project)
    - `review` needs changes → re-queue `implement` task
    - `review` approved → create `test` task
    - `test` fails → re-queue `implement` task
  - Retry with exponential backoff: 1s, 2s, 4s … (max 3 attempts), then mark `failed`
  - Priority queue: tasks with higher `priority` field are claimed first
  - `Scheduler` struct wired into server startup; runs as a background goroutine
  - New DB helpers: `GetTimedOutTasks`, `RequeueTask`, `CreateFollowOnTask`
  - Tests: state transitions, follow-on creation, retry/backoff, priority ordering (85%+ coverage)

### Context & Memory

- [x] **#29 Embedding + Semantic Context Search** — `storage/context.go` + `tools/context_tool.go` updates
  - Add `EmbedAndSave(ctx, entry, provider)` — calls `provider.Embed()`, stores float32 slice as JSON blob in `context_store.embedding`
  - Implement cosine similarity in pure Go (`storage/similarity.go`)
  - `QueryContext(ctx, projectID, query, embedder, topK)` — embed query, rank stored entries by cosine similarity, return top-K
  - Graceful fallback: if no embedder configured, fall back to keyword search (existing behaviour)
  - Update `context_tool` to call semantic search when an embedder is available
  - Tests: embedding storage/retrieval, cosine similarity, top-K ranking, fallback path (80%+ coverage)

- [x] **#31 Project Context and Memory** — `storage/project_context.go` + `router/context.go` updates
  - `SaveProjectContext(projectID, type, content)` — persists architecture decisions, design docs, diffs, test results
  - `GetProjectContext(projectID, maxItems)` — returns most recent N entries (pruning: drop oldest beyond limit)
  - Router injects project context into agent system prompt via `ContextRule` `include: ["project_memory"]`
  - Config support: `context_rules.worker.include: [project_memory, recent_tasks]`
  - Tests: save/retrieve, pruning, system-prompt assembly with project context (80%+ coverage)

### Metrics & Cost

- [x] **#30 Token Usage and Cost Tracking** — `logging/metrics.go` update + `server/handlers.go` new routes
  - Extend `Collector` to record `input_tokens`, `output_tokens` separately (migrate schema if needed)
  - Pricing config in `config.yaml`: `pricing: {gpt-4o: {input: 5.00, output: 15.00}}` (per 1M tokens)
  - `CostForCall(model, inputTokens, outputTokens) float64`
  - New DB helpers: `GetTokenMetrics(projectID, agentID)`, `GetCostMetrics(projectID)`
  - New HTTP routes: `GET /api/metrics/tokens`, `GET /api/metrics/costs`
  - Tests: token counting, cost calculation, aggregation, API response format (85%+ coverage)

### Multi-Agent Safety

- [x] **#32 Multi-Agent Concurrency and Safety** — `db/tasks.go` updates + concurrency tests
  - `ClaimTask(ctx, agentID, taskID)` wrapped in a SQLite transaction with `SELECT … FOR UPDATE` equivalent (`BEGIN IMMEDIATE`)
  - `GetTimedOutTasks(ctx, timeoutSec)` — returns `in_progress` tasks with `started_at` older than timeout
  - Scheduler requeues timed-out tasks (called every 30s by background loop)
  - Stress tests: 10 goroutines competing for 100 tasks — verify no double-claims, no deadlocks
  - Fair distribution test: all agents receive tasks, no starvation (80%+ coverage)

### Error Resilience

- [x] **#33 Error Handling and Resilience** — `agent/agent.go` + `llm/circuit_breaker.go`
  - Agent reconnect loop: on server disconnect, retry registration with backoff (1s→2s→4s, capped 60s)
  - Task retry: failed LLM call retries up to `config.agents.max_retries` times with exponential backoff
  - Provider fallback: if `primary` provider errors, try `fallback` provider name from config
  - Circuit breaker (`llm/circuit_breaker.go`): after N consecutive failures, open circuit for T seconds, then half-open
  - Config additions: `agents.max_retries`, `agents.fallback_provider`, `agents.circuit_breaker_threshold`
  - Tests: reconnect logic, retry backoff, fallback switching, circuit breaker state machine (80%+ coverage)

---

---

## Phase 5 — Testing, Docs & Polish

**Goal**: Close all test-coverage gaps, add missing integration tests, ship a Dockerfile + docker-compose, and produce a complete annotated config reference and README quickstart.

### Test Coverage — Go

- [x] **#34 Workflow state machine tests** — `workflow/state_test.go`
  - Table-driven tests for every valid transition in `validTransitions` (8 pairs)
  - Table-driven tests for representative invalid transitions (planned→completed, completed→anything, failed→completed, etc.)
  - `FollowOnType`: implement+completed→review, review+approved→test, all other combos return `("", false)`
  - `RoleForType`: plan→orchestrator, implement→worker, review→reviewer, test→worker, unknown→worker
  - `ValidateTransition`: returns nil for valid, non-nil error for invalid (error message contains "→")

- [x] **#35 Logging metrics tests** — `logging/metrics_test.go`
  - Helper `openMetricsDB(t)` wraps `db.Open` + `logging.NewCollector`
  - `TestTokenMetrics_Empty`: empty DB → all zero fields, no ByProject/ByAgent rows
  - `TestTokenMetrics_SingleEntry`: insert one metric with 100 input + 50 output tokens → totals match, ByAgent row present
  - `TestTokenMetrics_ByProject`: two projects, two tasks, insert per-task metrics → ByProject has two rows with correct per-project totals
  - `TestCostMetrics_Empty`: TotalCost == 0.0, no rows
  - `TestCostMetrics_MultipleAgents`: insert metrics for 3 agents with known costs → TotalCost == sum, ByAgent has 3 rows sorted by cost desc

- [x] **#36 Server metrics endpoint tests** — `server/metrics_handler_test.go`
  - Reuse `newTestServer` and `do` helpers from `handlers_test.go` (same package `server_test`)
  - `TestMetricsTokens_Empty`: GET `/api/metrics/tokens` → 200, body has `total_input_tokens`, `total_output_tokens`, `total_tokens` all zero
  - `TestMetricsTokens_MethodNotAllowed`: POST → 405
  - `TestMetricsCosts_Empty`: GET `/api/metrics/costs` → 200, body has `total_cost` == 0
  - `TestMetricsCosts_MethodNotAllowed`: POST → 405
  - `TestMetrics_Empty`: GET `/api/metrics` → 200, body has `total_tokens`, `total_tasks`, `success_rate`

- [x] **#37 Tools — plan tests** — `tools/plan_test.go`
  - `openToolDB(t)` helper: open DB, create a project, return DB + project ID
  - `TestPlanProject_Basic`: call `plan_project` handler with 2 work packages → success, task_count==2, both tasks in DB as planned
  - `TestPlanProject_SingleObject`: work_packages as JSON object (not array) → 1 task created
  - `TestPlanProject_InvalidProject`: non-existent project_id → error
  - `TestPlanProject_BadJSON`: malformed work_packages string → error
  - `TestCreateWorkPackage_Defaults`: call `create_work_package` with no role/priority → role="worker", priority=5, type="implement"
  - `TestCreateWorkPackage_Custom`: explicit role="reviewer", priority=8, task_type="review" → task persisted with correct fields

- [x] **#38 Tools — tasks + context tests** — `tools/tasks_test.go` + `tools/context_tool_test.go`
  - `tools/tasks_test.go`:
    - `TestListTasks_Empty`: list_tasks for project with no tasks → count==0
    - `TestListTasks_Filter`: 3 tasks (planned/in_progress/completed) → filter by status returns correct subset
    - `TestSubmitTaskResult_Completed`: mark task completed → DB reflects completed + result.output
    - `TestSubmitTaskResult_Failed`: status="failed", error="boom" → DB reflects failed, result.error set
    - `TestGetNextTask_NoTask`: empty project → task==nil
    - `TestGetNextTask_Returns`: one planned task → task returned
  - `tools/context_tool_test.go`:
    - `TestSaveContext_Basic`: save_context → success, context_id non-empty, entry in DB
    - `TestSaveContext_WithTaskID`: optional task_id stored correctly
    - `TestQueryContext_Match`: save entry containing "authentication", query "auth" → entry returned
    - `TestQueryContext_NoMatch`: query "zzz" → count==0

### Deployment

- [x] **#39 Dockerfile + docker-compose** — `Dockerfile` + `docker-compose.yml` at repo root
  - Multi-stage `Dockerfile`:
    - Stage 1 `builder`: `node:22-alpine` → `npm ci && npm run build` in `agent-orchestrator/ui/`
    - Stage 2 `gobuilder`: `golang:1.23-alpine` → copies UI dist, runs `go build -o /agent-orchestrator ./cmd`
    - Stage 3 `final`: `alpine:3.20` with ca-certificates → copies binary only, `EXPOSE 8080`, `ENTRYPOINT`
  - `docker-compose.yml`: `server` service + two `agent` services (`worker-1`, `worker-2`) sharing a named volume for `agent.db`
  - `.dockerignore`: exclude `node_modules`, `.git`, `*.db`, `testdata`

### Documentation

- [x] **#40 config.example.yaml** — `agent-orchestrator/config.example.yaml`
  - Full annotated YAML with comments on every field
  - Sections: `server`, `database`, `agents` (MaxRetries, FallbackProvider, CircuitBreakerThreshold), `logging`, `routing`, `providers` (openai + anthropic + azure + ollama examples), `pricing` (gpt-4o + claude-3-5-sonnet examples), `workflow`, `context_rules`
  - Environment-variable substitution examples: `api_key: "${OPENAI_API_KEY}"`

- [x] **#41 README.md** — `agent-orchestrator/README.md`
  - Sections: Overview, Architecture diagram (ASCII), Prerequisites, Quick Start (Docker Compose), Quick Start (Local / go-task), Configuration, API Reference summary, Running Tests, Contributing
  - Architecture: ASCII block diagram showing UI → Server → Router → LLM Provider, with Agent polling loop and SQLite DB

### Verification

- [x] **Verify Phase 5** — static analysis complete: zero duplicate function names across all test packages, all imports resolve, all 11 new files present
