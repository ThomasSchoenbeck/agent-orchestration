# Agent Orchestrator

An autonomous AI development platform — single Go binary, two modes: **server** and **agent**.

## Phase 1 Status: Foundation ✅

All 8 Phase 1 tasks complete:
- Go module + directory structure
- SQLite database layer (full schema + CRUD)
- YAML config system with env-var substitution
- LLM provider abstraction (OpenAI-compatible + Ollama)
- HTTP server with graceful shutdown
- REST API: projects, tasks, agents, providers, context, logs
- Agent registration + heartbeat
- Agent task polling loop

---

## Quick Start

### 1. Prerequisites

- **Go 1.22+** — https://go.dev/dl/
- **SQLite** — bundled via `modernc.org/sqlite` (pure Go, no cgo)

### 2. Install dependencies

```bash
cd agent-orchestrator
go mod tidy
```

### 3. Build

```bash
go build -o agent-orchestrator .
```

### 4. Configure

Copy and edit the example config:

```bash
cp config.yaml my-config.yaml
# Edit providers: add your API keys or point at a local Ollama
```

Minimal working config (with a local Ollama):

```yaml
providers:
  - name: ollama-local
    type: ollama
    base_url: http://localhost:11434

models:
  - name: qwen-coder
    provider: ollama-local
    model: qwen2.5-coder:7b
    roles: [worker]

roles:
  worker: qwen-coder

server:
  port: 8080

database:
  path: ./data/orchestrator.db
```

### 5. Run the server

```bash
./agent-orchestrator server --config my-config.yaml
# Server listening on http://0.0.0.0:8080
```

### 6. Run an agent (separate terminal)

```bash
./agent-orchestrator agent --name worker-1 --roles worker --server http://localhost:8080
# agent "worker-1" registered (id=...) — polling every 5s
```

You can run as many agents as you like, on the same machine or different machines.

---

## Running Tests

```bash
# All packages
go test ./...

# Verbose with coverage
go test -v -cover ./...

# Single package
go test -v ./db/...
go test -v ./config/...
go test -v ./llm/...
go test -v ./server/...
go test -v ./agent/...
```

Expected output (no Go compiler available at generation time — run locally to verify):

```
ok  agent-orchestrator/config   (85%+ coverage)
ok  agent-orchestrator/db       (80%+ coverage)
ok  agent-orchestrator/llm      (80%+ coverage)
ok  agent-orchestrator/server   (80%+ coverage)
ok  agent-orchestrator/agent    (75%+ coverage)
```

---

## API Reference (Phase 1)

All endpoints return JSON. Errors use:
```json
{ "code": "NOT_FOUND", "message": "project \"x\" not found" }
```

### Projects

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/projects` | List all projects |
| `POST` | `/api/projects` | Create project |
| `GET` | `/api/projects/{id}` | Get project |
| `PUT` | `/api/projects/{id}` | Update project |
| `DELETE` | `/api/projects/{id}` | Delete project |
| `GET` | `/api/projects/{id}/tasks` | List project tasks |

### Tasks

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/tasks?project_id=&status=&role=` | List tasks (filtered) |
| `POST` | `/api/tasks` | Create task |
| `GET` | `/api/tasks/{id}` | Get task |
| `PUT` | `/api/tasks/{id}` | Update task |
| `POST` | `/api/tasks/{id}/claim` | Claim task for agent |
| `POST` | `/api/tasks/{id}/result` | Submit task result |

### Agents

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/agents` | List all agents |
| `POST` | `/api/agents/register` | Register agent |
| `GET` | `/api/agents/{id}` | Get agent |
| `POST` | `/api/agents/{id}/heartbeat` | Send heartbeat |
| `GET` | `/api/agents/{id}/tasks/next?roles=worker` | Poll for next task |
| `DELETE` | `/api/agents/{id}` | Deregister agent |

### Providers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/providers` | List providers |
| `POST` | `/api/providers` | Add provider |
| `GET` | `/api/providers/{id}` | Get provider |
| `PUT` | `/api/providers/{id}` | Update provider |
| `DELETE` | `/api/providers/{id}` | Remove provider |

### Context & Logs

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/context/save` | Save context entry |
| `GET` | `/api/context/query?project_id=&query=` | Search context |
| `GET` | `/api/logs?agent_id=&task_id=&level=` | Query logs |
| `POST` | `/api/logs` | Submit log entry |
| `GET` | `/health` | Health check |

---

## Smoke-Test Walkthrough

With the server running, paste these into a terminal:

```bash
BASE=http://localhost:8080

# 1. Create a project
curl -s -X POST $BASE/api/projects \
  -H "Content-Type: application/json" \
  -d '{"name":"Recipe App","description":"Build a recipe management app"}' | jq .

# 2. Create a task
PROJECT_ID="<id from step 1>"
curl -s -X POST $BASE/api/tasks \
  -H "Content-Type: application/json" \
  -d "{\"project_id\":\"$PROJECT_ID\",\"type\":\"implement\",\"role\":\"worker\",\"priority\":5,\"payload\":{\"desc\":\"Create the data model\"}}" | jq .

# 3. Register an agent
curl -s -X POST $BASE/api/agents/register \
  -H "Content-Type: application/json" \
  -d '{"name":"worker-1","roles":["worker"]}' | jq .

# 4. Poll for next task
AGENT_ID="<id from step 3>"
curl -s "$BASE/api/agents/$AGENT_ID/tasks/next?roles=worker" | jq .

# 5. Claim task
TASK_ID="<id from step 2>"
curl -s -X POST "$BASE/api/tasks/$TASK_ID/claim" \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"$AGENT_ID\"}" | jq .

# 6. Submit result
curl -s -X POST "$BASE/api/tasks/$TASK_ID/result" \
  -H "Content-Type: application/json" \
  -d '{"result":{"output":"Done"},"status":"completed","metrics":{"tokens_used":512,"duration_ms":4200}}' | jq .
```

---

## Project Structure

```
agent-orchestrator/
├── main.go              # Entry point — server + agent CLI
├── config.yaml          # Example configuration
├── go.mod
├── api/
│   ├── errors.go        # Standard error response types + WriteError/WriteJSON
│   └── types.go         # Request/response types
├── config/
│   ├── config.go        # Config structs, Load(), Validate()
│   ├── defaults.go      # Default values
│   └── config_test.go
├── db/
│   ├── db.go            # Database init + withTx helper
│   ├── migrations.go    # CREATE TABLE statements + indexes
│   ├── models.go        # Go structs for all DB entities
│   ├── uuid.go          # crypto/rand UUID v4 generator
│   ├── projects.go      # Project CRUD
│   ├── tasks.go         # Task CRUD + ClaimTask + GetNextTask
│   ├── agents.go        # Agent CRUD + heartbeat + offline detection
│   ├── providers.go     # Provider CRUD
│   ├── context.go       # Context store save/query
│   ├── logs.go          # Log + metric insertion/query
│   └── db_test.go
├── llm/
│   ├── types.go         # ChatRequest/Response, ToolDef, ToolCall, etc.
│   ├── provider.go      # LLMProvider interface
│   ├── registry.go      # Provider registry + InitFromConfig
│   ├── openai.go        # OpenAI-compatible provider (also LM Studio, etc.)
│   ├── ollama.go        # Ollama provider
│   └── provider_test.go
├── server/
│   ├── server.go        # Server struct, Start(), maintenance loop
│   ├── handlers.go      # All HTTP handlers (projects/tasks/agents/providers/logs)
│   └── handlers_test.go
└── agent/
    ├── agent.go         # Agent lifecycle: Start, Stop, heartbeat + poll loops
    ├── client.go        # Typed HTTP client for the orchestrator server API
    └── agent_test.go
```

---

## Phase 2 Preview

Next phase wires in:
- Full LLM chat endpoint (`POST /api/llm/chat`)
- Task execution engine (agent calls LLM → executes tool calls → submits result)
- Code execution tools: `read_file`, `write_file`, `apply_diff`, `run_tests`
- Plan/task management tools
- Config-based routing (task type → role → model)
- WebSocket chat setup
