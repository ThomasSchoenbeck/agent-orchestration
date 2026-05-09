# Agent Orchestrator - Project Specification

**Version**: 1.0  
**Status**: Specification  
**Date**: May 2026

---

## Executive Summary

Agent Orchestrator is an **autonomous AI development platform** packaged as a single Go binary. It serves as a unified hub for:
- Managing software development projects
- Orchestrating multiple autonomous AI agents
- Configuring and routing to multiple LLM providers (Ollama, Claude, Azure, company APIs, etc.)
- Tracking tasks, agents, context, and observability
- Providing a web UI for project and agent management

The platform operates in two modes:
- **Server mode**: Hosts the API (standard library `net/http`), database (SQLite), UI, and orchestration engine
- **Agent mode**: Runs worker processes that pull tasks and execute workflows

**Technology Stack**:
- **Go**: Standard library only (no chi, echo, gin, or other HTTP frameworks)
- **Database**: SQLite with `modernc.org/sqlite` (pure Go, no cgo required)
- **Frontend**: Svelte + SvelteKit with pnpm package manager
- **No external dependencies** for core functionality (single binary deployment)

---

## Core Architecture

### 1. Single Go Binary with Two Modes

#### Server Mode
```bash
myagent server --config config.yaml --port 8080
```

Responsibilities:
- HTTP/WebSocket API for projects, tasks, agents, providers, context, logs
- Embedded Svelte UI (served from compiled static assets)
- SQLite database for persistent state
- LLM provider orchestration
- Task scheduling and lifecycle management
- Chat interface with tool support and config-driven routing
- MCP-like capabilities exposed as REST endpoints

#### Agent Mode
```bash
myagent agent --name worker-1 --roles worker,reviewer --server http://localhost:8080
myagent agent --name orchestrator-1 --roles orchestrator --server http://localhost:8080
myagent agent --name designer-1 --roles ui_designer,system_designer --server http://localhost:8080
```

Responsibilities:
- Register with server
- Poll for tasks (pull model) or receive assignments (push model)
- Execute workflows using LLM providers via server
- Run tests, apply diffs, validate code
- Report results and metrics back to server

---

### 2. Database (SQLite / BadgerDB)

**Choice: SQLite with modernc.org/sqlite**  
- Single-file database (portable, no external process)
- Sufficient for multi-agent workloads
- Easy backups and snapshots

#### Core Schema

**Projects**
```
- id (UUID)
- name (string)
- description (string)
- repo_path (string) - local path or git URL
- status (enum: planned, in_progress, completed, failed)
- created_at, updated_at (timestamps)
- config (JSON) - project-specific overrides
```

**Tasks / Work Packages**
```
- id (UUID)
- project_id (UUID) - foreign key
- type (enum: plan, implement, review, ui_design, system_design, test)
- role (enum: orchestrator, worker, reviewer, ui_designer, system_designer)
- status (enum: planned, in_progress, needs_review, approved, completed, failed)
- priority (int)
- assigned_agent_id (UUID, nullable)
- payload (JSON) - task-specific data
- result (JSON, nullable) - agent's response
- attempts (int)
- created_at, updated_at (timestamps)
```

**Agents**
```
- id (UUID)
- name (string)
- roles (array of strings, stored as JSON)
- status (enum: online, offline, idle, busy)
- current_task_id (UUID, nullable)
- capabilities (JSON)
- last_heartbeat (timestamp)
- registered_at (timestamp)
```

**Providers / LLM Clients**
```
- id (UUID)
- name (string)
- type (enum: openai_compatible, anthropic, ollama, lm_studio, github_copilot, etc.)
- base_url (string)
- model_name (string)
- api_key (encrypted string)
- capabilities (JSON) - e.g., [chat, embed, rerank]
- config (JSON) - provider-specific settings
- created_at, updated_at
```

**Models**
```
- id (UUID)
- name (string)
- provider_id (UUID) - foreign key
- model_name (string)
- roles (array of strings, stored as JSON)
- config (JSON)
```

**Context Store**
```
- id (UUID)
- project_id (UUID, nullable)
- task_id (UUID, nullable)
- type (enum: summary, embedding, snippet, note)
- content (text)
- embedding (vector, nullable)
- metadata (JSON)
- created_at, updated_at
```

**Logs / Monitoring**
```
- id (UUID)
- agent_id (UUID, nullable)
- task_id (UUID, nullable)
- project_id (UUID, nullable)
- level (enum: debug, info, warn, error)
- message (text)
- metadata (JSON)
- timestamp
```

**Execution Metrics**
```
- id (UUID)
- task_id (UUID)
- agent_id (UUID)
- tokens_used (int)
- cost (decimal)
- duration_ms (int)
- success (boolean)
- created_at
```

---

### 3. Embedded Svelte UI

**Framework**: Svelte + SvelteKit  
**Packaging**: Compiled to static assets, embedded in Go binary via `embed`  
**Served from**: `/` (root path)

#### Core Pages / Sections

**Projects Dashboard**
- List all projects
- Create new project (with initial idea)
- View project details: status, tasks, agents, progress
- Edit project configuration

**Tasks View**
- Filter by project, status, role, agent
- View task payload, results, diffs, logs
- Re-assign tasks
- Manually approve/reject tasks

**Agents Dashboard**
- List online/offline agents
- View agent capabilities (roles)
- View current task and status
- Spawn new agents
- Metrics (tasks completed, token usage, uptime)

**Providers Configuration**
- List configured LLM providers
- Add/edit providers (type, base_url, model_name, API key)
- Test provider connectivity
- View provider metrics (calls, tokens, costs)

**Chat Interface**
- Chat with the server (uses chat brain model)
- Chat understands config, available agents, available tools
- Can issue tool calls (plan, create tasks, assign, etc.)
- Maintains conversation history

**Monitoring / Observability**
- Logs view (per project, agent, task)
- Token usage tracking (per task, agent, provider)
- Error tracking
- Metrics dashboard

**Context Browser**
- View project summaries
- Search context by keyword or embedding similarity
- View task-specific context

---

### 4. Configuration System (Continue.dev-inspired)

**Format**: YAML  
**Location**: `config.yaml` in current directory or specified path  
**Loaded at**: Server startup

#### Example Configuration

```yaml
# Providers: LLM backends
providers:
  - name: company-api
    type: openai-compatible
    base_url: https://api.company.com/v1
    api_key: ${COMPANY_API_KEY}  # env var substitution
    
  - name: ollama-local
    type: ollama
    base_url: http://localhost:11434
    
  - name: lmstudio
    type: openai-compatible
    base_url: http://localhost:1234/v1
    
  - name: claude
    type: anthropic
    api_key: ${ANTHROPIC_API_KEY}

# Models: named combinations of provider + model name
models:
  - name: o4-mini
    provider: company-api
    model: o4-mini
    roles: [orchestrator, reviewer, chat_brain]
    config: {}
    
  - name: qwen-coder
    provider: ollama-local
    model: qwen2.5-coder
    roles: [worker]
    
  - name: claude-sonnet
    provider: claude
    model: claude-3-5-sonnet-20241022
    roles: [system_designer, ui_designer]

# Roles: abstractions for different agent types
roles:
  orchestrator: o4-mini
  worker: qwen-coder
  reviewer: o4-mini
  ui_designer: claude-sonnet
  system_designer: claude-sonnet
  chat_brain: o4-mini  # special role for chat interface
  embedder: o4-mini
  reranker: o4-mini

# Routing: which role handles which task type
routing:
  plan: orchestrator
  implement: worker
  review: reviewer
  ui-design: ui_designer
  system-design: system_designer

# Prompts: reusable prompt templates
prompts:
  plan: |
    You are the orchestrator agent for an autonomous AI development system.
    Your role is to analyze project requirements and create a detailed architecture and work breakdown structure.
    
    Project: {project_name}
    Requirements: {requirements}
    
    Return a JSON structure with:
    - architecture: high-level system design
    - workPackages: array of implementation tasks
    - estimatedTokens: rough token estimate
    
  implement: |
    You are a senior coding agent responsible for implementing work packages.
    
    Task: {task_description}
    Repo Path: {repo_path}
    Context: {context}
    
    Use tool calls to:
    1. Read relevant files
    2. Implement the feature
    3. Run tests
    4. Report results
    
  review: |
    You are a senior code reviewer.
    Review the implementation for correctness, testing, and alignment with requirements.
    
    Return JSON with:
    - approved: boolean
    - issues: array of issues
    - suggestions: array of improvement suggestions

# Context rules: which context to include for which role
context_rules:
  plan:
    include: [docs, folder_structure, existing_code]
    exclude: [diff, terminal_output]
    
  implement:
    include: [code, diff, test_results, problems]
    exclude: [docs, folder_structure]
    
  review:
    include: [diff, code, test_results]
    exclude: [docs, folder_structure]

# Server configuration
server:
  port: 8080
  host: 0.0.0.0
  tls_enabled: false
  
# Database configuration
database:
  type: sqlite  # or badgerdb
  path: ./data/orchestrator.db
  
# Agent configuration
agents:
  heartbeat_interval_sec: 30
  task_poll_interval_sec: 5
  task_timeout_sec: 300
```

---

## API Specification

### REST Endpoints

#### Projects

```
GET  /api/projects
POST /api/projects
GET  /api/projects/{id}
PUT  /api/projects/{id}
DELETE /api/projects/{id}
```

#### Tasks / Work Packages

```
GET  /api/tasks?project_id=X&status=Y&role=Z
POST /api/tasks
GET  /api/tasks/{id}
PUT  /api/tasks/{id}
POST /api/tasks/{id}/claim
POST /api/tasks/{id}/result
```

#### Agents

```
GET  /api/agents
POST /api/agents/register
GET  /api/agents/{id}
GET  /api/agents/{id}/tasks/next?roles=worker,reviewer
POST /api/agents/{id}/heartbeat
DELETE /api/agents/{id}
```

#### Providers

```
GET  /api/providers
POST /api/providers
GET  /api/providers/{id}
PUT  /api/providers/{id}
DELETE /api/providers/{id}
POST /api/providers/{id}/test
```

#### LLM Chat / Tool Calls

```
POST /api/llm/chat
  {
    "role": "worker",
    "messages": [...],
    "tools": [...],
    "context": {...}
  }

POST /api/tools/{tool_name}
  { "arguments": {...} }
```

#### Context Management

```
POST /api/context/save
GET  /api/context/query?project_id=X&task_id=Y&query=Z
```

#### Chat Interface

```
WebSocket /ws/chat
  - message: user input
  - tool_calls: auto-executed by server
  - response: streamed back to client
```

#### Logs / Monitoring

```
GET /api/logs?agent_id=X&task_id=Y&level=Z
GET /api/metrics?type=tokens|duration|costs
```

---

## Agent Workflow / Task Execution

### Task Lifecycle

```
planned → in_progress → needs_review → approved → completed
  ↓
failed (with retry logic)
```

### Execution Flow

1. **Agent Registration**
   - Agent calls `POST /api/agents/register` with name, roles, capabilities
   - Server stores agent, returns agent_id

2. **Task Polling**
   - Agent periodically calls `GET /api/agents/{id}/tasks/next?roles=worker,reviewer`
   - Server returns highest-priority unassigned task matching roles
   - Agent calls `POST /api/tasks/{id}/claim` to claim task

3. **Task Execution**
   - Agent reads task payload (requirements, repo path, context)
   - Agent calls `POST /api/llm/chat` with role=task.role
   - LLM provider responds with result or tool calls
   - Agent executes tool calls (read files, apply diffs, run tests)
   - Agent calls `POST /api/tasks/{id}/result` with output, status, metrics

4. **Result Processing**
   - Server stores result in database
   - Server may create follow-on tasks (review, test, etc.)
   - Server updates context store with learnings
   - Task marked as `completed` or `needs_review`

5. **Review Cycle**
   - Agent with role=reviewer picks up review task
   - Reviewer calls `POST /api/llm/chat` with diff + code
   - Reviewer returns approval, issues, or suggestions
   - If approved, task marked `completed`
   - If issues, task requeued or marked `failed`

---

## Tool Definitions (MCP-like)

The server exposes these tools to LLM models via tool calls:

### Planning Tools

```
plan_project
  - input: project_name, requirements
  - output: architecture, work_packages
  
create_work_package
  - input: project_id, title, description, estimated_tokens
  - output: task_id
```

### Task Management Tools

```
get_next_task
  - input: agent_id
  - output: task object
  
submit_task_result
  - input: task_id, result, status, metrics
  - output: success, next_task_id
  
list_tasks
  - input: project_id, status, role
  - output: array of tasks
```

### Code Execution Tools

```
read_file
  - input: repo_path, file_path
  - output: file contents
  
write_file
  - input: repo_path, file_path, content
  - output: success
  
apply_diff
  - input: repo_path, diff
  - output: success, errors
  
run_tests
  - input: repo_path, test_command
  - output: test_results, passed, failed
```

### Context Management Tools

```
save_context
  - input: project_id, task_id, content_type, content
  - output: context_id
  
query_context
  - input: project_id, query, limit
  - output: array of context objects
  
embed_text
  - input: text
  - output: embedding vector
```

### Agent Management Tools

```
spawn_agent
  - input: name, roles
  - output: agent_id
  
list_agents
  - input: status (online|offline|all)
  - output: array of agent objects
  
assign_task
  - input: task_id, agent_id
  - output: success
```

### Monitoring Tools

```
log_event
  - input: level, message, metadata
  - output: success
  
report_metrics
  - input: task_id, agent_id, metrics_dict
  - output: success
```

---

## Chat Interface Design

### Chat Brain Model

The chat interface uses a special **chat_brain** role mapped to a model that supports tool calls.

### Chat Flow

1. User types message in UI: *"Build a recipe app"*
2. Server loads config, selects chat_brain model (e.g., o4-mini)
3. Server builds system prompt including:
   - Config (available models, roles, routing, prompts)
   - Available agents
   - Available tools
   - Current projects and context
4. Server sends message + tools to chat_brain model
5. Chat brain responds with:
   - Conversation text
   - Tool calls (e.g., `plan_project`, `create_work_package`, `spawn_agent`)
6. Server executes tool calls, stores results in DB
7. Server streams results back to chat UI
8. Chat brain may make additional tool calls
9. Final response shown to user

### Chat System Prompt Example

```
You are the Chat Brain of an autonomous AI development system.

Available Roles:
- orchestrator: plans architecture and work breakdown
- worker: implements code
- reviewer: reviews code quality
- ui_designer: designs UI/UX
- system_designer: designs system architecture
- embedder: generates embeddings
- reranker: ranks search results

Available Tools:
- plan_project: create architecture and work breakdown
- create_work_package: create implementation tasks
- spawn_agent: start a new agent with specific roles
- list_agents: see running agents
- submit_task_result: record task completion
- query_context: fetch relevant context
- save_context: store summaries and learnings

Available Projects:
{projects_list}

When the user asks for help, use tool calls to:
1. Plan the project (if needed)
2. Create work packages
3. Spawn agents with appropriate roles
4. Monitor progress

Explain your reasoning and next steps to the user.
```

---

## Project Structure (Go)

```
agent-orchestrator/
├── main.go                      # Entry point, CLI handling
├── go.mod, go.sum
├── config/
│   ├── config.go                # Config structs and loading
│   └── defaults.go
├── server/
│   ├── server.go                # Server startup, HTTP setup
│   ├── handlers.go              # HTTP handlers
│   ├── websocket.go             # WebSocket chat
│   └── static/                  # Embedded Svelte UI (compiled)
├── agent/
│   ├── agent.go                 # Agent registration and polling
│   ├── executor.go              # Workflow execution
│   └── tools.go                 # Tool implementations
├── db/
│   ├── migrations.go            # Schema setup
│   ├── projects.go              # Project queries
│   ├── tasks.go                 # Task queries
│   ├── agents.go                # Agent queries
│   ├── providers.go             # Provider queries
│   ├── context.go               # Context queries
│   └── logs.go                  # Logging queries
├── llm/
│   ├── provider.go              # LLMProvider interface
│   ├── openai.go                # OpenAI-compatible impl
│   ├── anthropic.go             # Anthropic impl
│   ├── ollama.go                # Ollama impl
│   ├── lmstudio.go              # LM Studio impl
│   └── registry.go              # Provider registry
├── router/
│   ├── router.go                # Config-based routing
│   ├── prompt.go                # Prompt template engine
│   └── context_builder.go       # Context assembly
├── workflow/
│   ├── scheduler.go             # Task scheduling
│   ├── executor.go              # Workflow execution
│   └── lifecycle.go             # Task state machine
├── tools/
│   ├── plan.go
│   ├── task_mgmt.go
│   ├── code_execution.go
│   ├── context.go
│   └── monitoring.go
└── ui/
    └── (compiled Svelte assets embedded via go:embed)
```

---

## Deployment

### Single Binary Build

```bash
# Build Svelte UI
cd ui && pnpm build && cd ..

# Build Go binary (Svelte assets embedded)
go build -o myagent .
```

### Server Deployment

```bash
./myagent server --config config.yaml --port 8080
```

**Output:**
- HTTP API listening on `http://localhost:8080`
- UI available at `http://localhost:8080/`
- SQLite database created at `./data/orchestrator.db`
- Ready to accept agents

### Agent Deployment

```bash
# On same machine or different machine
./myagent agent --name worker-1 --roles worker --server http://localhost:8080
./myagent agent --name orchestrator-1 --roles orchestrator --server http://localhost:8080
./myagent agent --name designer-1 --roles ui_designer,system_designer --server http://localhost:8080
```

**Each agent:**
- Registers with server
- Polls for tasks matching its roles
- Executes workflows
- Reports results back

### Scaling

- **Single machine**: Server + multiple agents on same host
- **Multi-machine**: Server on one host, agents spread across multiple hosts (all connect to server)
- **Cloud**: Package as Docker image, scale agents horizontally

---

## Integration Points

### Continue.dev (Optional)

Continue.dev becomes an optional client:

- Push project ideas to server via REST
- Pull tasks from server to implement manually
- Push results back to server

No longer required; purely optional.

### External MCP Clients

If you want external tools to access orchestrator capabilities, expose MCP endpoints:

- `mcp://localhost:8080/plan`
- `mcp://localhost:8080/task-mgmt`
- `mcp://localhost:8080/context`

### Git / Repo Integration

Agents checkout/pull repos from:
- Local file system
- GitHub, GitLab, Gitea
- Private Git servers
- Auto-commit changes or create PRs

---

## Key Features Summary

✅ **Single Go Binary**
- One executable for server + agent modes
- No external dependencies for core functionality
- Portable, easy deployment

✅ **Embedded Database (SQLite/BadgerDB)**
- All state (projects, tasks, agents, context) persisted locally
- No external DB setup required
- Easy snapshots and backups

✅ **Embedded UI (Svelte)**
- Modern web interface
- Project and agent management
- Chat interface with tool support
- Monitoring and observability

✅ **Config-Driven Routing**
- Inspired by Continue.dev
- Declarative model definitions, role mappings, prompts
- Support for multiple LLM providers
- No code changes to swap models

✅ **Pluggable LLM Providers**
- Ollama, LM Studio, Claude, Azure OpenAI, GitHub Copilot
- Company APIs (with API key auth)
- Easy to add new providers

✅ **Multi-Agent Orchestration**
- Agents pull tasks or receive assignments
- Concurrent workflow execution
- Task lifecycle management and retry logic

✅ **Chat Interface with Tool Support**
- Chat brain model with config awareness
- Tool calls for planning, task creation, context management
- Streaming responses
- Conversation history

✅ **Comprehensive Monitoring**
- Task logs and metrics per agent
- Token usage tracking
- Error tracking and debugging
- Progress dashboards

✅ **Context Management**
- Save summaries, embeddings, notes
- Query context for task context building
- Reduce token usage via smart context inclusion

---

## Non-Goals

- ❌ Real-time collaboration (focus: autonomous agents, not team collab)
- ❌ Enterprise auth/RBAC (focus: single user / small team)
- ❌ Advanced DevOps (no Kubernetes, no complex infra)
- ❌ VS Code integration (UI is the control center)
- ❌ Opinionated agent frameworks (support: any config-driven agent)

---

## Success Criteria

1. Single binary compiles and runs
2. Server starts, serves UI at http://localhost:8080
3. Agents can register and poll for tasks
4. Chat interface understands config and issues tool calls
5. Tasks are created, executed, reviewed, and marked complete
6. Multiple agents run concurrently without conflicts
7. All state persists to SQLite
8. LLM provider swaps work without code changes
9. Token usage and metrics are tracked
10. System remains responsive with 10+ agents running

---

## Timeline (Rough)

- **Phase 1 (Foundation)**: Core server, DB schema, basic REST API, agent registration
- **Phase 2 (Execution)**: Task lifecycle, LLM routing, tool execution
- **Phase 3 (UI)**: Svelte dashboard, chat interface
- **Phase 4 (Polish)**: Config system, monitoring, context engine, multi-provider support
- **Phase 5 (Optimization)**: Performance tuning, scaling tests

---

## References

- **Continue.dev**: Configuration and routing concepts
- **Devin**: Multi-agent autonomous development
- **Conductor**: Workflow engines
- **AgentCraft**: Multi-project agent orchestration
- **Modern Go**: SQLite (modernc.org/sqlite), concurrency patterns
- **Svelte**: Reactive UI framework

---

## Questions for Discussion

1. **Database**: SQLite or BadgerDB? (SQLite chosen for simplicity)
2. **Authentication**: None (local) or JWT for server API?
3. **Initial LLM Providers**: Which 3-4 to support first?
4. **Chat Brain Model**: O4-mini or user-selectable?
5. **Scaling**: How many concurrent agents to support initially?
6. **Code Repo Integration**: Git operations or abstract via file tools?
7. **Testing Strategy**: Unit tests, integration tests, or full system e2e?
8. **Monitoring**: Prometheus/Grafana or custom dashboard?

---

**End of Specification**
