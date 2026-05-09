# Agent Orchestrator - Implementation Roadmap

**Date**: May 2026  
**Status**: Planning  
**Total Tasks**: 41 across 5 phases

---

## Overview

This roadmap breaks down the Agent Orchestrator implementation into 5 progressive phases:

- **Phase 1 (Foundation)**: Core infrastructure, database, configuration, basic server/agent
- **Phase 2 (Execution)**: LLM integration, task execution, tools, routing
- **Phase 3 (UI & Chat)**: Svelte UI, chat interface with tool support
- **Phase 4 (Features & Optimization)**: Advanced providers, workflow scheduling, context management, multi-agent safety
- **Phase 5 (Polish & Testing)**: Testing, documentation, deployment, performance tuning

---

## Phase 1: Foundation (8 tasks)
**Goal**: Get a working server + agent foundation with database and config system.  
**Duration**: ~2-3 weeks  
**Key Deliverable**: Server runs, accepts agents, basic task CRUD works  
**Testing Approach**: Unit tests included in each task; 70%+ coverage for core modules

### Dependencies
- All Phase 1 tasks are relatively independent
- Task #1 (Project Setup) should complete first
- Task #2 (Database) and #3 (Config) can run in parallel with #4-5
- **Note**: All tasks include unit tests; tests are written alongside implementation

### Task Breakdown

**#1 - Project Setup and Foundation**
- Create Go module (go.mod)
- Set up project directory structure
- Add initial dependencies (chi/gorilla for HTTP, modernc.org/sqlite)
- Create main.go entry point
- Set up git repo

**#2 - Database Layer - SQLite Schema**
- Create database initialization code
- Define all SQL schemas (projects, tasks, agents, providers, context, logs)
- Implement migration system (or just create schema on startup)
- Write database package with basic CRUD operations
- **Unit tests**: Test schema creation, all CRUD operations, transaction safety
- **Test file**: `db/db_test.go`, `db/models_test.go` (target: 80%+ coverage)

**#3 - Configuration System**
- Create config.go structs for YAML structure
- Implement YAML parser with go-yaml
- Add env var substitution (${ENV_VAR})
- Create config validation
- **Unit tests**: Test YAML parsing, env var substitution, validation, error cases
- **Test file**: `config/config_test.go`, `config/validation_test.go` (target: 85%+ coverage)

**#4 - LLM Provider Abstraction**
- Define LLMProvider Go interface (Chat, Embed, Rerank)
- Create ProviderRegistry
- Implement OpenAI-compatible provider (with mocked HTTP responses)
- Implement Ollama provider (with mocked HTTP responses)
- Add error handling and retry logic
- **Unit tests**: Test provider registry, provider initialization, Chat/Embed/Rerank with mocked responses, error handling
- **Test file**: `llm/provider_test.go`, `llm/openai_test.go`, `llm/ollama_test.go`, `llm/registry_test.go` (target: 80%+ coverage)

**#5 - Server - CLI and Startup**
- Add CLI using cobra (or standard flag package)
- Implement "server" subcommand
- Implement "agent" subcommand
- Server startup: load config, init DB, start HTTP server
- Graceful shutdown handling
- **Unit tests**: Test CLI parsing, server startup, config loading errors, graceful shutdown
- **Test file**: `cmd/main_test.go`, `server/server_test.go` (target: 75%+ coverage)

**#6 - Server - Basic HTTP API**
- Set up HTTP router using standard library `net/http` with `http.ServeMux`
- Implement /api/projects endpoints (GET list, POST create, GET by id, PUT update)
- Implement /api/tasks endpoints (GET with filters, POST create, GET by id, PUT update)
- Add JSON request/response handling via `encoding/json`
- Add basic error responses
- Helper functions for routing (method-based dispatch, path parsing)
- **Unit tests**: Test all endpoints with valid/invalid inputs, response formats, HTTP status codes, error cases
- **Test file**: `server/handlers_test.go`, `api/errors_test.go` (target: 80%+ coverage)

**#7 - Agent - Registration and Heartbeat**
- Implement agent registration: POST /api/agents/register
- Track agent status (online/offline/idle/busy)
- Implement heartbeat endpoint: POST /api/agents/{id}/heartbeat
- Auto-mark agents offline if no heartbeat for N seconds
- Store agent registration in database
- **Unit tests**: Test agent registration, heartbeat timing, status transitions, offline detection
- **Test file**: `agent/agent_test.go`, `db/agents_test.go` (target: 80%+ coverage)

**#8 - Agent - Task Polling Loop**
- Implement task polling: GET /api/agents/{id}/tasks/next?roles=X,Y
- Agent claims task: POST /api/tasks/{id}/claim
- Implement polling loop in agent (configurable interval, e.g., 5 sec)
- Mark tasks as "in_progress" when claimed
- Handle task timeout (if agent doesn't report result, re-queue)
- **Unit tests**: Test task polling, claiming, timeout logic, state transitions
- **Test file**: `agent/poller_test.go`, `workflow/executor_test.go` (target: 80%+ coverage)

### Success Criteria for Phase 1
- ✅ Server starts without errors
- ✅ Agent can register and poll for tasks
- ✅ Projects and tasks can be created via API
- ✅ Database persists all data
- ✅ Config loads and validation works
- ✅ At least 2 LLM providers (OpenAI-compatible + Ollama) are implemented

---

## Phase 2: Execution & Tools (8 tasks)
**Goal**: Enable agents to execute tasks by calling LLMs and tools.  
**Duration**: ~2-3 weeks  
**Depends on**: Phase 1 completion  
**Key Deliverable**: Agent executes a task end-to-end (calls LLM, uses tools, reports result)  
**Testing Approach**: Unit tests for all tools, LLM routing, context building; mocked LLM responses

### Dependencies
- #9 (LLM Chat Endpoint) depends on #4 (Provider abstraction)
- #10 (Plan/Task Tools) depends on #6 (Basic API) and #2 (Database)
- #11 (Task Execution Engine) depends on #9 and #10
- #12 (Code Execution Tools) can start once #6 is done
- #13, #14, #15, #16 depend on #6 (Basic API)

### Task Breakdown

**#9 - Server - LLM Chat Endpoint**
- Implement POST /api/llm/chat
- Input: role, messages, optional tools
- Route to correct provider based on config.roles mapping
- Handle streaming responses (optional but recommended)
- Detect and return tool calls in response
- **Unit tests**: Test routing logic, provider selection, tool call detection, streaming, error handling with mocked providers
- **Test file**: `router/router_test.go`, `server/handlers_test.go` (target: 80%+ coverage)

**#10 - Tools - Plan and Task Management**
- Implement plan_project tool (input: project_name, requirements → output: architecture, work_packages)
- Implement create_work_package tool (input: project_id, title → output: task_id)
- Implement list_tasks tool (input: project_id, status → output: task list)
- Implement submit_task_result tool (input: task_id, result, status → output: next_task_id or null)
- Implement get_next_task tool (input: agent_id → output: task object)
- Each tool updates database and returns JSON
- **Unit tests**: Test each tool with valid/invalid inputs, database operations, error cases
- **Test file**: `tools/plan_test.go`, `tools/tasks_test.go` (target: 85%+ coverage)

**#11 - Agent - Task Execution Engine**
- Agent reads task payload (type, role, description, repo_path, etc.)
- Agent calls GET /api/context/query to fetch context (if context API exists, else mock)
- Agent calls POST /api/llm/chat with role=task.role, messages=task, context=fetched_context
- Agent parses response for tool calls
- Agent executes tool calls (read_file, write_file, run_tests, etc.)
- Agent calls POST /api/tasks/{id}/result with output, status, metrics
- **Unit tests**: Test task payload parsing, context fetching, LLM request building, tool call parsing, result submission with mocked server/LLM
- **Test file**: `agent/executor_test.go` (target: 80%+ coverage)

**#12 - Tools - Code Execution**
- Implement read_file tool (input: repo_path, file_path → output: content or error)
- Implement write_file tool (input: repo_path, file_path, content → output: success/error)
- Implement apply_diff tool (input: repo_path, diff → output: success/error)
- Implement run_tests tool (input: repo_path, test_command → output: test_results, passed, failed)
- Implement git_clone tool (input: repo_url, target_path → output: success/error)
- Implement git_checkout tool (input: repo_path, branch → output: success/error)
- **Unit tests**: Test file operations with temp directories, diff application, git operations (mocked or local test repo), error cases
- **Test file**: `tools/code_test.go` (target: 80%+ coverage)

**#13 - Server - Context Management API**
- Implement POST /api/context/save (input: project_id, task_id, content_type, content)
- Implement GET /api/context/query (input: project_id, query → output: context list)
- Store context in database
- Initially: simple text search (no embeddings yet)
- Return compact bundles of relevant context
- **Unit tests**: Test context saving, querying, filtering, bundling with various content types
- **Test file**: `storage/context_test.go`, `server/handlers_test.go` (target: 80%+ coverage)

**#14 - Router - Config-Based Task Routing**
- Create Router package
- Read config.routing table to map task_type → role
- Read config.roles to map role → model
- Implement context builder that reads context_rules and assembles appropriate context
- When task is created with type=implement, automatically select role=worker, model=qwen-coder
- Template and fill prompt from config.prompts
- **Unit tests**: Test routing logic, prompt templating, context building with various configs
- **Test file**: `router/router_test.go`, `router/prompt_test.go`, `router/context_test.go` (target: 85%+ coverage)

**#15 - Server - Logging and Monitoring API**
- Implement POST /api/logs (input: agent_id, task_id, level, message, metadata)
- Implement GET /api/metrics (input: type=tokens|duration|costs)
- Store logs in database
- Track token usage per LLM call
- Aggregate metrics by task, agent, project
- Expose metrics for UI consumption
- **Unit tests**: Test log submission, querying, filtering, metric aggregation, API endpoints
- **Test file**: `logging/logger_test.go`, `logging/metrics_test.go`, `server/handlers_test.go` (target: 80%+ coverage)

**#16 - Server - WebSocket Chat Setup**
- Implement WebSocket endpoint /ws/chat
- Handle client connections and disconnections
- Implement message queue for async processing
- Store conversation history in database
- Prepare for chat brain model integration (Phase 3)
- Support streaming responses from LLM
- **Unit tests**: Test WebSocket connections, message handling, message queue, history storage with mocked WebSocket clients
- **Test file**: `server/websocket_test.go` (target: 75%+ coverage)

### Success Criteria for Phase 2
- ✅ Agent can claim a task and execute it end-to-end
- ✅ Agent calls LLM with correct role and prompt
- ✅ Tool calls are detected and executed
- ✅ Task result is reported back and stored
- ✅ Context is fetched and provided to agent
- ✅ Logs and metrics are recorded
- ✅ Task routing follows config (type → role → model)

---

## Phase 3: UI & Chat Interface (8 tasks)
**Goal**: Build web UI and implement chat brain for user interaction.  
**Duration**: ~3-4 weeks  
**Depends on**: Phase 1 & 2 foundation  
**Key Deliverable**: UI is fully functional, chat can plan and create projects, agents execute and report  
**Testing Approach**: Vitest unit tests for Svelte components and utilities; mocked API calls

### Dependencies
- #19 (Svelte Setup) is foundational
- #20-25 (UI Pages) depend on #19 and #6 (API endpoints)
- #17-18 (Chat Brain) depend on #9 (LLM Chat) and #10 (Tools)

### Task Breakdown

**#19 - UI - Svelte Project Setup**
- Create Svelte + SvelteKit project
- Set up build pipeline (pnpm build → static assets)
- Configure go:embed in Go to bundle compiled assets into binary
- Implement dev mode (pnpm dev) for local development
- Set up API client (fetch or axios wrapper)
- Create store for global state (projects, agents, config)
- **Unit tests**: Set up Vitest, test API client mocking, test store mutations/getters
- **Test setup**: `src/lib/__tests__/`, test coverage target 75%+, `pnpm test` command

**#20 - UI - Projects Dashboard**
- Create Projects.svelte page
- Implement project list with filters (status, date)
- Create new project form (name, description, repo_url)
- Show project details modal: tasks, agents, progress, status
- Fetch from GET /api/projects, POST /api/projects
- Real-time status updates
- **Unit tests**: Test component rendering, filtering, form submission, API calls (mocked)
- **Test file**: `src/lib/__tests__/Projects.test.js` (target: 80%+ coverage)

**#21 - UI - Tasks View**
- Create Tasks.svelte page
- Implement task list with multi-column sort/filter (project, status, role, agent)
- Show task details: payload, prompt used, result, logs, metrics
- Implement task actions: claim, assign to agent, re-queue, approve/reject
- Fetch from GET /api/tasks, PUT /api/tasks/{id}, POST /api/tasks/{id}/result
- **Unit tests**: Test filtering/sorting logic, action buttons, task detail display with mocked API
- **Test file**: `src/lib/__tests__/Tasks.test.js` (target: 80%+ coverage)

**#22 - UI - Agents Dashboard**
- Create Agents.svelte page
- List agents with status (online/offline/idle/busy)
- Show agent details: roles, current task, metrics (tasks completed, tokens used, uptime)
- Implement spawn new agent form (name, roles, server url)
- Real-time status updates (WebSocket)
- **Unit tests**: Test agent list display, status display, spawn form, WebSocket connection handling
- **Test file**: `src/lib/__tests__/Agents.test.js` (target: 80%+ coverage)

**#23 - UI - Providers Configuration**
- Create Providers.svelte page
- List configured LLM providers with details
- Form to add/edit provider (type, base_url, model_name, api_key)
- Test provider connectivity button
- Show provider metrics (calls, tokens, costs)
- Fetch/update via GET/POST /api/providers
- **Unit tests**: Test provider list display, add/edit forms, test button, metrics display
- **Test file**: `src/lib/__tests__/Providers.test.js` (target: 80%+ coverage)

**#24 - UI - Chat Interface**
- Create Chat.svelte page
- Message input box
- Conversation history displayed above
- Show streaming responses in real-time
- Display tool calls being executed (in real-time)
- Auto-refresh projects/tasks list when chat issues tool calls
- Connect via WebSocket /ws/chat
- **Unit tests**: Test message input/submission, history display, streaming response handling, WebSocket mocking
- **Test file**: `src/lib/__tests__/Chat.test.js` (target: 75%+ coverage)

**#25 - UI - Monitoring and Logs**
- Create Monitoring.svelte page
- Logs view: filter by agent, task, project, level
- Token usage chart (by model, by project, over time)
- Task duration chart
- Error tracking and stack traces
- Real-time log streaming
- Fetch from GET /api/logs, GET /api/metrics
- **Unit tests**: Test filtering, chart rendering, log display, real-time updates with mocked API/WebSocket
- **Test file**: `src/lib/__tests__/Monitoring.test.js` (target: 80%+ coverage)

**#17 - Chat Interface - Chat Brain Model Integration**
- Implement chat brain logic in server
- When message received on /ws/chat:
  - Load config (models, roles, tools, routing)
  - Select chat_brain model (e.g., o4-mini)
  - Build system prompt describing available roles, tools, projects
  - Send to LLM with tool support enabled
  - Handle response (text + tool calls)
- Support multi-turn conversations (maintain message history)
- Stream responses back to UI

**#18 - Chat Interface - Tool Call Execution Loop**
- When chat brain issues tool calls:
  - Extract tool name and arguments
  - Execute tool (plan_project, create_work_package, spawn_agent, etc.)
  - Feed result back to chat brain
  - Chat brain continues reasoning
  - Loop until no more tool calls (or iteration limit)
  - Return final response to user
- Implement timeout (don't loop forever)
- Log all tool calls for debugging

### Success Criteria for Phase 3
- ✅ UI serves from http://localhost:8080/
- ✅ All dashboard pages load without errors
- ✅ Can create project via UI
- ✅ Chat interface works: user types, chat brain responds
- ✅ Chat can issue tool calls (plan, create tasks)
- ✅ Tool calls are executed and visible to user
- ✅ Tasks created via chat are picked up by agents
- ✅ Real-time updates via WebSocket

---

## Phase 4: Features & Optimization (8 tasks)
**Goal**: Add advanced features, multi-provider support, advanced tooling, and ensure multi-agent safety.  
**Duration**: ~2-3 weeks  
**Depends on**: Phase 1, 2, 3  
**Key Deliverable**: System is production-ready with advanced routing, context management, cost tracking  
**Testing Approach**: Unit tests for new providers, workflow logic, concurrency; integration tests for multi-agent scenarios

### Dependencies
- #26, #27 (Additional Providers) depend on #4 (Provider abstraction)
- #28 (Workflow Scheduler) depends on #11 (Task Execution)
- #29 (Embeddings) depends on #13 (Context API) and #4 (Provider)
- #30 (Token Tracking) depends on #15 (Logging)
- #31 (Project Context) depends on #13 and #29
- #32 (Concurrency) is independent but critical before production
- #33 (Error Handling) is independent but critical before production

### Task Breakdown

**#26 - LLM Providers - Anthropic (Claude) Support**
- Implement AnthropicProvider
- Handle Claude's tool_use format (vs OpenAI's)
- Support streaming and non-streaming
- Handle error codes specific to Anthropic API
- Test with claude-3-sonnet model
- **Unit tests**: Test provider initialization, Chat method with mocked API, tool use handling, error cases
- **Test file**: `llm/anthropic_test.go` (target: 85%+ coverage)

**#27 - LLM Providers - Azure OpenAI Support**
- Implement AzureOpenAIProvider
- Handle Azure-specific auth (API keys, deployment names)
- Support all Azure model variations
- Handle Azure-specific errors and quotas
- **Unit tests**: Test Azure auth, deployment naming, API requests/responses with mocked Azure API, error handling
- **Test file**: `llm/azure_test.go` (target: 85%+ coverage)

**#28 - Workflow Scheduler and Task Lifecycle**
- Implement state machine for task lifecycle: planned → in_progress → needs_review → approved → completed
- When a task completes, automatically create follow-on tasks:
  - After implement task → create review task
  - After review with issues → requeue implement task
  - After review approved → create test task
  - After test failures → requeue implement task
- Implement retry logic: failed task retries with exponential backoff (up to 3x)
- Implement priority queue (high-priority tasks get picked up first)
- **Unit tests**: Test state transitions, follow-on task creation, retry logic, priority ordering
- **Test file**: `workflow/scheduler_test.go`, `workflow/state_test.go` (target: 85%+ coverage)

**#29 - Embedding and Context Search**
- Add embedder model call to context save flow
- When context is saved, call embedder model to generate embeddings
- Store embeddings in database (as vectors or JSON)
- Implement semantic search in query_context tool
- Return top-K most relevant context by similarity
- Reduce token usage: include only necessary context
- **Unit tests**: Test embedding generation, similarity search, vector storage/retrieval with mocked embedder
- **Test file**: `storage/context_test.go`, `tools/context_test.go` (target: 80%+ coverage)

**#30 - Token Usage and Cost Tracking**
- Track tokens in each LLM call (input + output)
- Record cost per call (based on model pricing)
- Aggregate by task, agent, project, provider
- Implement pricing config (per-model cost per 1K tokens)
- Expose GET /api/metrics/tokens and /api/metrics/costs
- Show in UI: cost trends, top-cost tasks, cost per project
- **Unit tests**: Test token counting, cost calculation, metric aggregation, API response formatting
- **Test file**: `logging/metrics_test.go`, `server/handlers_test.go` (target: 85%+ coverage)

**#31 - Project Context and Memory**
- Implement project-level context store
- Save: architecture decisions, design docs, important diffs, test results
- Create system prompts to include project context in agent tasks
- Implement context pruning: remove old context if >N items
- Enable agents to learn from past work on same project
- **Unit tests**: Test context saving/retrieval, pruning logic, prompt building with project context
- **Test file**: `storage/context_test.go`, `router/context_test.go` (target: 80%+ coverage)

**#32 - Multi-Agent Concurrency and Safety**
- Implement task locking: only one agent can claim/execute task at a time
- Use database transactions for atomic updates
- Test with 5-10 concurrent agents (same machine)
- Monitor for race conditions, deadlocks
- Implement timeouts for long-running tasks
- Verify task distribution is fair (no agent starving)
- **Unit tests**: Test task locking, transaction isolation, race condition scenarios
- **Integration tests**: Concurrent agent tests (5-10 agents, 100 tasks)
- **Test files**: `db/db_test.go` (concurrency), `agent/agent_test.go` (target: 80%+ coverage)

**#33 - Error Handling and Resilience**
- Agent crash recovery: agent reconnects, re-registers, continues polling
- Task retry with exponential backoff (1s, 2s, 4s, etc.)
- LLM provider fallback: if primary provider fails, use alternative (if configured)
- Circuit breaker: disable failing endpoints temporarily, retry after delay
- Comprehensive logging of all errors
- Graceful degradation: system continues with reduced capacity if agent fails
- **Unit tests**: Test error scenarios, retry logic, provider fallback, circuit breaker, logging
- **Test file**: `agent/agent_test.go`, `server/server_test.go` (target: 80%+ coverage)

### Success Criteria for Phase 4
- ✅ System supports 3+ LLM providers (OpenAI-compatible, Ollama, Claude, Azure)
- ✅ Agents can execute multi-step workflows (plan → implement → review → test)
- ✅ Context search via embeddings works
- ✅ Token usage and costs are tracked and visible
- ✅ 5+ agents run concurrently without conflicts
- ✅ Failed tasks retry automatically
- ✅ Agent crashes don't crash server or lose state

---

## Phase 5: Testing, Docs & Polish (9 tasks)
**Goal**: Comprehensive integration testing, documentation, deployment, and performance optimization.  
**Duration**: ~2-3 weeks  
**Depends on**: Phase 1-4 (note: unit tests are integrated into phases 1-4)  
**Key Deliverable**: Production-ready system with integration tests, docs, and deployment guide  
**Testing Approach**: Integration tests (server + agents + LLM), multi-agent scenario tests, end-to-end workflows

### Dependencies
- #34-36 (Integration & E2E Testing) can run in parallel
- #37-38 (Documentation) can run after #16 (chat) and #25 (UI)
- #39 (Docker) can run after #5 (Server startup)
- #40 (Performance) should run after all core features are complete
- #41 (Final Validation) should run last

### Note on Testing Strategy
- **Unit tests**: Integrated into each task in Phases 1-4 (75-85%+ coverage per module)
- **Integration tests**: Grouped in Phase 5 (server + agents, multiple providers, workflows)
- **E2E tests**: Full system workflow from UI to completion

### Task Breakdown

**#34 - Testing - Unit Tests Coverage Review & Gaps**
- Review unit test coverage across all modules (from Phases 1-4)
- Identify any coverage gaps (<75%)
- Add missing unit tests to reach 75%+ coverage across:
  - Config package
  - Provider implementations
  - Router and context builder
  - Task state machine
  - Tools and tool executor
- Use Go's testing package, testify for assertions, and coverage reports (`go test -cover`)
- **Acceptance**: Overall codebase has 75%+ unit test coverage

**#35 - Testing - Integration Tests (Server + Agent + LLM)**
- Integration test: start server, register agent, create project via API
- Integration test: agent polls task, executes (with mocked LLM), reports result
- Integration test: full task lifecycle (plan → implement → review → completed)
- Integration test: tool execution (read_file, write_file, run_tests)
- Integration test: multiple LLM providers working together (OpenAI + Ollama)
- Integration test: context saving/fetching during task execution
- Mock LLM responses or use test Ollama instance
- Use docker-compose to spin up test environment
- **Test framework**: Go's testing package + testcontainers for database
- **Test file**: `integration_test.go` (target: 70%+ coverage of main workflows)

**#36 - Testing - Multi-Agent Scenario Tests**
- Scenario: 3 agents with different roles execute tasks concurrently
- Scenario: High-load test (10 agents, 100 tasks)
- Verify: task distribution is fair, no duplicates, correct routing
- Verify: no race conditions, deadlocks, or data corruption
- Stress test: measure response time under load
- Load test: measure memory usage over time
- Test agent crash/recovery: kill agent mid-task, verify recovery and task re-execution
- Test provider failover: primary provider fails, agent switches to backup
- **Test tool**: Go's concurrency primitives + custom load test harness
- **Test file**: `concurrency_test.go` (target: measure and establish baseline metrics)

**#37 - Documentation - API Reference**
- Generate OpenAPI/Swagger spec (optional, or document manually)
- Document all endpoints: path, method, request, response, error codes
- Include example curl commands for each endpoint
- Document tool definitions: name, input schema, output schema, examples
- Document WebSocket /ws/chat protocol

**#38 - Documentation - User Guide and Examples**
- "Getting Started" guide: start server, configure provider, create project
- "Running Agents" guide: spawn agents, assign roles, monitor
- "Using Chat" guide: chat with system, issue commands, track progress
- Example workflows:
  - "Build a recipe app" (simple project)
  - "Implement a feature" (mid-complexity)
  - "Review and refine code" (iterative workflow)
- Troubleshooting guide: common issues and solutions

**#39 - Docker and Deployment**
- Create Dockerfile: build Go binary, embed UI, create image
- Create docker-compose.yml with:
  - server service (port 8080)
  - example agent services (worker, reviewer, designer)
  - optional: Ollama container for local LLM
- Write deployment guide: local development, single-machine, multi-machine, cloud
- Include environment setup instructions (API keys, config)
- Test multi-container deployment

**#40 - Performance Optimization and Benchmarking**
- Benchmark task polling (latency from agent request to task assignment)
- Benchmark LLM calls (time from request to response)
- Benchmark database queries (most common queries)
- Benchmark context search (semantic search performance)
- Profile for bottlenecks using pprof
- Optimize hot paths (reduce allocations, improve caching)
- Target: task assignment <100ms, LLM response <5s (depends on model)

**#41 - Final Integration and System Validation**
- End-to-end system test:
  - Start server, load UI in browser
  - Configure LLM provider
  - Spawn 3 agents (orchestrator, worker, reviewer)
  - Use chat to create project ("Build a recipe app")
  - Verify agents pick up tasks
  - Verify tasks execute end-to-end (implement → review → test)
  - Verify project marks completed
- Validate monitoring: logs visible, metrics tracked, costs calculated
- Validate recovery: kill agent, verify recovery; kill server, verify restart
- Create system demo video or walkthrough

### Success Criteria for Phase 5
- ✅ Unit tests pass with 70%+ coverage
- ✅ Integration tests pass (server + agents + LLM workflows)
- ✅ Multi-agent load test passes (10 agents, 100 tasks)
- ✅ API documentation is complete and accurate
- ✅ User guide covers all major workflows
- ✅ Docker image builds and runs cleanly
- ✅ Performance meets targets (<100ms task assignment, <5s LLM response)
- ✅ End-to-end system test passes from UI to completion

---

## Cross-Phase Considerations

### Database Design
- Design schema early in Phase 1
- Use transactions for consistency
- Add indices for common queries (agent_id, project_id, status, priority)
- Plan for migrations (schema version tracking)

### Configuration Management
- Keep config.yaml simple and well-documented
- Support env var substitution for secrets (API keys)
- Validate config at startup (fail fast)
- Reload config without restart (stretch goal for later)

### Error Handling
- Define standard error response format
- Include error codes for client handling
- Log all errors with context (task_id, agent_id, etc.)
- Graceful degradation (don't crash on LLM provider failure)

### Testing Strategy
- Unit tests: test logic in isolation
- Integration tests: test with real database and APIs
- End-to-end tests: test full workflows through UI
- Use containers for consistent test environment

### Monitoring & Observability
- Log all significant operations (task created, agent registered, LLM called, tool executed)
- Track metrics (tokens, duration, cost, success rate)
- Expose metrics for UI and external monitoring
- Include debug logging (can be toggled via config)

### Security (Future)
- API key encryption in database (Phase 4+)
- TLS support for server (Phase 5)
- Agent authentication (token-based, Phase 5)
- Rate limiting (Phase 5)

---

## Timeline Estimate

| Phase | Duration | Start | End | Key Deliverable |
|-------|----------|-------|-----|-----------------|
| 1 | 2-3 weeks | Week 1 | Week 3 | Server + agent foundation |
| 2 | 2-3 weeks | Week 3 | Week 6 | Agent task execution |
| 3 | 3-4 weeks | Week 6 | Week 10 | Web UI + chat |
| 4 | 2-3 weeks | Week 10 | Week 13 | Advanced features |
| 5 | 2-3 weeks | Week 13 | Week 16 | Production-ready |

**Total: ~12-16 weeks** (3-4 months for full system)

**MVP (Quick Start): Phases 1-2 (~5 weeks)**
- Server runs, agents execute tasks, basic task lifecycle works
- Good for early validation and feedback

---

## Development Tips

### Start with Phase 1
- Get database, config, and basic API working first
- Don't optimize prematurely
- Use simple implementations (no fancy algorithms)

### Test As You Go
- Write tests as you implement features
- Use integration tests to catch bugs early
- Test with real (or mocked) LLM calls

### Keep Dependencies Minimal
- Use Go standard library where possible
- Add third-party libraries only when necessary
- Avoid complex frameworks

### Iterate Fast
- Build features end-to-end (server + agent) quickly
- Get feedback from testing/experimentation
- Refactor later after validating approach

### Document As You Code
- Add code comments for complex logic
- Keep README updated
- Document API changes

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| LLM provider API changes | Design provider abstraction early; add tests for each provider |
| Database schema issues | Design schema carefully in Phase 1; add migration system |
| Agent concurrency bugs | Write concurrency tests early; use database transactions |
| UI complexity | Start with simple pages; add features incrementally |
| Token/cost tracking errors | Audit token counting against LLM API; test pricing calculations |
| Performance issues | Benchmark early; profile and optimize hot paths |
| Deployment complexity | Containerize early; test Docker build and multi-container setup |

---

**Next Steps**: Begin Phase 1 tasks in order. Start with Project Setup (#1), then Database (#2) and Config (#3) in parallel, then proceed to Provider Abstraction (#4), Server CLI (#5), and API (#6).

