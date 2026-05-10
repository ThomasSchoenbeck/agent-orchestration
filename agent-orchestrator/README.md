# Agent Orchestrator

A self-hosted, multi-agent task orchestration system built in Go with a Svelte 5 UI.
Agents poll for work, call an LLM to complete tasks, and report results — all coordinated through a central HTTP server backed by SQLite.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Browser                                                │
│  Svelte 5 UI  ──── REST / WebSocket ────┐               │
└─────────────────────────────────────────┼───────────────┘
                                          │
┌─────────────────────────────────────────▼───────────────┐
│  Server  (Go HTTP)                                      │
│                                                         │
│  ┌─────────────┐  ┌──────────┐  ┌────────────────────┐ │
│  │  REST API   │  │ WebSocket│  │  Workflow Scheduler │ │
│  │  /api/*     │  │ /ws/chat │  │  (background gortn) │ │
│  └──────┬──────┘  └──────────┘  └─────────┬──────────┘ │
│         │                                  │            │
│  ┌──────▼──────────────────────────────────▼──────────┐ │
│  │              SQLite  (agent.db)                    │ │
│  │  projects · tasks · agents · metrics · context     │ │
│  └────────────────────────────────────────────────────┘ │
│         │                                               │
│  ┌──────▼──────┐                                        │
│  │   Router    │  role → model → provider               │
│  └──────┬──────┘                                        │
└─────────┼───────────────────────────────────────────────┘
          │
   ┌──────▼──────────────────────────────────────────┐
   │  LLM Providers                                  │
   │  OpenAI · Anthropic · Azure OpenAI · Ollama     │
   │  (circuit breaker + failover per provider)      │
   └─────────────────────────────────────────────────┘

        ▲  poll for tasks   ▼  submit results
┌───────┴─────────────────────────────────────────────┐
│  Agents  (one or many, any machine)                 │
│  worker-1 · worker-2 · reviewer-1 · …               │
│  Each: register → poll → claim → LLM loop → submit  │
└─────────────────────────────────────────────────────┘
```

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | ≥ 1.23 | Build the server binary |
| Node.js | ≥ 22 | Build the Svelte UI |
| pnpm | ≥ 9 | UI package manager (`corepack enable`) |
| [go-task](https://taskfile.dev) | ≥ 3 | Task runner (`brew install go-task`) |
| Docker + Compose | any recent | Optional — containerised deployment |

---

## Quick Start — Docker Compose

The fastest way to run everything:

```bash
# 1. Clone the repo
git clone https://github.com/you/agent-orchestrator.git
cd agent-orchestrator/agent-orchestrator

# 2. Copy and configure
cp config.example.yaml config.yaml
# Edit config.yaml: add your API keys under providers

# 3. Export API keys
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=sk-ant-...

# 4. Build and start (server + 2 worker agents)
docker compose up --build
```

The UI will be available at **http://localhost:8080**.

```bash
# Stop and remove all data
docker compose down -v
```

---

## Quick Start — Local (go-task)

```bash
cd agent-orchestrator

# Install UI dependencies
cd ui && pnpm install && cd ..

# Copy and edit config
cp config.example.yaml config.yaml

# Build UI + Go binary
task build

# Start the server
task run:server

# In another terminal — start agents
task agents

# Or start everything at once
task start
```

### Development mode (hot-reload)

```bash
task dev      # Go server with live-reload
task dev:ui   # Vite dev server on :5173 (proxied to :8080)
```

---

## Configuration

Copy `config.example.yaml` to `config.yaml`. Every field is annotated in the example file.

**Minimal working config:**

```yaml
server:
  port: 8080
database:
  path: "./agent.db"
providers:
  - name: openai
    type: openai_compatible
    base_url: "https://api.openai.com/v1"
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
models:
  - name: gpt-4o
    provider: openai
    model: "gpt-4o"
    roles: [orchestrator, worker, reviewer]
roles:
  orchestrator: gpt-4o
  worker: gpt-4o
  reviewer: gpt-4o
```

### Supported provider types

| `type` | Notes |
|--------|-------|
| `openai_compatible` | OpenAI, local llama.cpp, LM Studio, … |
| `anthropic` | Anthropic Claude (Messages API) |
| `azure` | Azure OpenAI — requires `deployment:` field |
| `ollama` | Local Ollama — no `api_key` needed |

### Error resilience (`agents:`)

```yaml
agents:
  max_retries: 3                  # LLM retries with exponential backoff
  fallback_provider: ollama-local # try this if the primary fails
  circuit_breaker_threshold: 5    # failures that open the circuit
```

---

## API Reference

All endpoints return `application/json`. Errors use `{"error": {"code": "...", "message": "..."}}`.

### Projects

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/projects` | List all projects |
| `POST` | `/api/projects` | Create a project |
| `GET` | `/api/projects/:id` | Get project by ID |
| `PUT` | `/api/projects/:id` | Update project |
| `DELETE` | `/api/projects/:id` | Delete project |

### Tasks

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/tasks` | List tasks (`?project_id=&status=&role=`) |
| `POST` | `/api/tasks` | Create a task |
| `GET` | `/api/tasks/:id` | Get task by ID |
| `POST` | `/api/tasks/:id/claim` | Claim a task for an agent |
| `POST` | `/api/tasks/:id/result` | Submit task result |
| `POST` | `/api/tasks/:id/queue` | Re-queue a task |

### Agents

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/agents/register` | Register an agent |
| `GET` | `/api/agents` | List agents |
| `POST` | `/api/agents/:id/heartbeat` | Send heartbeat |
| `GET` | `/api/agents/:id/tasks/next` | Poll for the next task |

### LLM & Metrics

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/llm/chat` | One-shot LLM call (routes by `role` or `task_type`) |
| `GET` | `/api/metrics` | Aggregated task/token/cost summary |
| `GET` | `/api/metrics/tokens` | Input/output token counts by project and agent |
| `GET` | `/api/metrics/costs` | Cost breakdown by project and agent |

### WebSocket

| Path | Description |
|------|-------------|
| `/ws/chat` | Bi-directional chat stream (JSON messages) |

### Health

```
GET /health  →  {"status":"ok"}
```

---

## Running Tests

```bash
# Go unit tests (all packages)
task test:server

# Go tests with coverage
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out

# UI unit tests (Vitest)
task test:ui

# UI coverage
task test:ui:coverage

# Integration tests (requires running server)
INTEGRATION_URL=http://localhost:8080 task test:integration

# Full CI (lint + test + build)
task ci
```

---

## Task Reference

```
task build            Build UI + Go binary
task build:ui         Build only the Svelte UI
task build:server     Build only the Go binary

task run:server       Start the server
task dev              Server with live-reload
task dev:ui           Vite dev server

task agents           Start N worker agents  (N=2 default, e.g. N=4 task agents)
task start            Server + agents with health-check loop

task test             Run all tests
task test:server      Go tests
task test:ui          UI tests (Vitest)
task lint             Lint all (Go + ESLint)
task ci               Full CI: lint + test + build

task clean            Remove built artifacts
task clean:all        Remove artifacts + node_modules + agent.db
```

---

## Project Structure

```
agent-orchestrator/
├── cmd/                  Entry point (server & agent CLI)
├── config/               YAML config loader + validation
├── db/                   SQLite schema, migrations, CRUD
├── llm/                  LLM provider abstraction
│   ├── openai.go         OpenAI-compatible
│   ├── anthropic.go      Anthropic Claude
│   ├── azure.go          Azure OpenAI
│   ├── ollama.go         Ollama
│   └── circuit_breaker.go  Circuit breaker + failover
├── agent/                Agent polling loop, executor, reconnect
├── server/               HTTP handlers, WebSocket, static files
├── router/               Role→provider routing, prompt assembly
├── tools/                LLM tools (plan, tasks, context, code)
├── workflow/             Task state machine + scheduler
├── storage/              Semantic search, cosine similarity
├── logging/              Structured logger, metrics, cost tracking
├── api/                  Shared types, error helpers
└── ui/                   Svelte 5 + Vite 8 + Tailwind CSS 4
```

---

## Contributing

1. Fork and create a feature branch.
2. Run `task ci` to verify linting, tests, and build pass.
3. Open a PR describing what changed and why.

---

## License

MIT — see [LICENSE](LICENSE).
