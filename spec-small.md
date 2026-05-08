
AI Development Platform – Project Specification
1. Goal
Build a small, self-contained AI software development platform that can:

Manage multiple projects
Orchestrate multiple AI agents
Track progress and results
Work with different LLM providers (not tied to Continue.dev)
Run completely from a single Go binary
The system should be lightweight, extensible, and easy to run locally.


2. Core Concept
The system is a central server + distributed agents:

Server (brain + UI + DB)
Agents (workers)
Pluggable AI providers (LLMs)
Everything revolves around projects and tasks (work packages).


3. High-Level Architecture
Single Binary Modes

myagent serverRuns API
Hosts UI
Stores data (SQLite)
Orchestrates workflows
myagent agentConnects to server
Pulls tasks
Executes work
Reports results


4. Core Components
4.1 Server (Go)
Responsibilities:

Project management
Task/work package management
Agent registry
Workflow orchestration
LLM routing
Context storage
Monitoring/logging
UI hosting (embedded Svelte)
Interfaces:

REST API (primary)
Optional MCP endpoint (for external tools like Continue.dev)


4.2 Agents (Go, same binary)
Responsibilities:

Register with server
Pull tasks from queue
Execute tasks using LLM providers
Run code/tests if needed
Report results back
Supports:

Multiple agents running in parallel
Different roles per agent


4.3 Database (SQLite)
Serves as system memory:
Stores:

Projects
Tasks (work packages)
Agents
Providers / models
Logs & metrics
Context (summaries, notes, embeddings)


4.4 UI (Svelte, embedded)
Single control interface:
Features:

Project dashboard
Task tracking
Agent monitoring
Logs & results
Provider configuration
No IDE required.


5. Task Model (Work Packages)
Each task includes:

id
project_id
type (plan, implement, review, etc.)
role (worker, orchestrator, etc.)
priority
status (planned, running, done, failed)
payload (instructions/context)
assigned agent (optional)
Task Flow
Supports both:

Pull model (agents request tasks)
Push model (server assigns tasks)


6. Agent Roles
Roles define behavior, not implementation:

orchestrator (planning)
worker (coding)
reviewer (validation)
designer (UI/system design)
Agents declare supported roles and fetch matching tasks.


7. LLM Provider System
Goal
Allow any backend:

Company API
Ollama
LM Studio
Claude
Azure / OpenAI
GitHub Copilot
Abstraction
All providers implement:

Chat
Embed (optional)
Rerank (optional)


8. Configuration System (Core Feature)
A central YAML/JSON config replaces Continue.dev config.
Defines:
Providers
Connection details (API endpoints, keys)
Models
Mapping to providers
Roles
Maps roles → models
Routing
Maps task types → roles
Prompts
Reusable templates per role/task
Context Rules
Defines what data is included in each request


9. Chat Interface (Built-in)
The system includes a chat UI acting as a router.
Behavior

Uses a “chat brain” model (tool-calling capable)
Reads configuration
Decides:which role to use
which prompt to apply
which tools to call
Tools available to chat:

create_project
plan_project
create_tasks
get_tasks
submit_result
query_context
This mimics Continue.dev behavior but runs fully inside the system.


10. Context System
Stores structured knowledge:

Task summaries
Project architecture
Important decisions
Past results
Provides:

Context bundles per task
Lightweight summaries instead of full history


11. External Integration
REST API
Primary interface for:

Agents
Custom tools
Scripts
Optional MCP Endpoint
Allows integration with:

Continue.dev
Other AI tools
Used for:

pushing project ideas
pulling tasks
reporting results


12. Deployment

Single compiled Go binary
No external services required
Embedded UI
Embedded database (SQLite)
Run modes:

Local (default)
Distributed agents (optional)


13. Minimal MVP Scope
To keep it small, first version should include:
Required

Server mode
Agent mode
SQLite storage
Basic task queue
LLM provider abstraction (1–2 providers)
Config system (roles + prompts)
Simple UI (projects + tasks)
Optional Later

Full MCP support
Advanced context engine
Multi-provider routing logic
Observability dashboards


14. Summary
This project is a lightweight autonomous development platform with:

Single binary deployment
Multi-project support
Multi-agent orchestration
Provider-agnostic AI integration
Built-in UI and chat control
It replaces IDE-centric workflows with a centralized AI-driven system.


15. Go Project Structure (Suggested)

myagent/
├── cmd/
│   └── myagent/
│       └── main.go            # entry point (server/agent switch)
│
├── internal/                  # private application code
│   ├── server/
│   │   ├── api/               # REST + WebSocket handlers
│   │   ├── router/            # HTTP routing setup
│   │   ├── middleware/        # logging, auth, etc.
│   │   └── server.go
│   │
│   ├── agent/
│   │   ├── runner.go          # main agent loop
│   │   ├── client.go          # server communication
│   │   └── executor.go        # task execution logic
│   │
│   ├── core/
│   │   ├── project/           # project logic
│   │   ├── task/              # task lifecycle + queue
│   │   ├── agent/             # agent registry logic
│   │   └── workflow/          # orchestration engine
│   │
│   ├── provider/
│   │   ├── interface.go       # LLMProvider interface
│   │   ├── registry.go        # provider registry
│   │   ├── openai/
│   │   ├── ollama/
│   │   └── anthropic/
│   │
│   ├── config/
│   │   ├── loader.go          # load YAML/JSON config
│   │   └── schema.go
│   │
│   ├── context/
│   │   ├── store.go           # context persistence
│   │   └── builder.go         # context assembly logic
│   │
│   ├── db/
│   │   ├── sqlite.go          # DB init
│   │   ├── migrations/
│   │   └── repository/        # data access layer
│   │
│   └── chat/
│       ├── router.go          # chat routing logic
│       ├── tools.go           # tool definitions
│       └── executor.go        # tool execution loop
│
├── web/                       # Svelte frontend (built & embedded)
│   ├── src/
│   └── dist/
│
├── pkg/                       # optional public packages
│   └── client/                # external client SDK
│
├── configs/
│   └── config.yaml
│
├── scripts/
│   └── dev.sh
│
├── go.mod
└── README.md


Key Design Notes

cmd/ → keeps binary entry clean
cmd/internal/ → all core logic (not exportable)
internal/provider/ → isolates LLM integrations
provider/chat/ → implements Continue.dev-like routing logic
chat/agent/ vs server/ → clean separation but same binary
agent/server/web/dist → embedded into Go binary via embedweb/distembedThis structure keeps the project modular but still small and easy to evolve.
16. Database Schema (SQLite)
Below is a minimal but complete schema to support projects, tasks, agents, providers, and observability.
Projects

CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    status TEXT,
    repo_url TEXT,
    created_at DATETIME,
    updated_at DATETIME
);


Tasks (Work Packages)

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    type TEXT,
    role TEXT,
    status TEXT,
    priority INTEGER DEFAULT 0,
    payload TEXT,
    result TEXT,
    assigned_agent_id TEXT,
    attempts INTEGER DEFAULT 0,
    created_at DATETIME,
    updated_at DATETIME,
    FOREIGN KEY(project_id) REFERENCES projects(id)
);


Agents

CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT,
    roles TEXT,
    status TEXT,
    current_task_id TEXT,
    last_seen DATETIME
);


Providers

CREATE TABLE providers (
    id TEXT PRIMARY KEY,
    name TEXT,
    type TEXT,
    base_url TEXT,
    api_key TEXT,
    model TEXT,
    config TEXT
);


Context Storage

CREATE TABLE context_entries (
    id TEXT PRIMARY KEY,
    project_id TEXT,
    task_id TEXT,
    type TEXT,
    content TEXT,
    created_at DATETIME,
    FOREIGN KEY(project_id) REFERENCES projects(id),
    FOREIGN KEY(task_id) REFERENCES tasks(id)
);


Logs

CREATE TABLE logs (
    id TEXT PRIMARY KEY,
    task_id TEXT,
    agent_id TEXT,
    level TEXT,
    message TEXT,
    created_at DATETIME
);


Metrics / Usage

CREATE TABLE metrics (
    id TEXT PRIMARY KEY,
    task_id TEXT,
    agent_id TEXT,
    tokens_input INTEGER,
    tokens_output INTEGER,
    duration_ms INTEGER,
    created_at DATETIME
);



Design NotesAll IDs are TEXT (UUID recommended)
JSON fields stored as TEXT (payload, result, config)Simple schema for fast iteration (no over-normalization)
Can evolve later with indexes and relationsThis schema is intentionally minimal but fully supports the MVP.


17. API Endpoint Definitions
Minimal REST API supporting server, agents, UI, and external tools.
Projects

POST   /projects            # create project
GET    /projects            # list projects
GET    /projects/{id}       # get project
PUT    /projects/{id}       # update project
DELETE /projects/{id}       # delete project


Tasks (Work Packages)

POST   /tasks                        # create task
GET    /tasks                        # list/filter tasks
GET    /tasks/{id}                   # get task
PUT    /tasks/{id}                   # update task
POST   /tasks/{id}/claim             # claim task
POST   /tasks/{id}/result            # submit result


Query params (example):

/tasks?project_id=...&status=...&role=...




Agent Endpoints

POST   /agents/register              # register agent
POST   /agents/heartbeat             # update status
GET    /agents                       # list agents
GET    /agents/{id}                  # get agent
GET    /agents/{id}/tasks/next       # pull next task


Example:

GET /agents/{id}/tasks/next?roles=worker,reviewer




LLM / Provider Access (Server-side routing)

POST /llm/chat
POST /llm/embed
POST /llm/rerank


Request example:

{
  "role": "worker",
  "prompt": "Implement feature X",
  "context": {}
}




Context API

POST /context/save
GET  /context/query


Example:

GET /context/query?project_id=...&task_id=...




Logs & Metrics

POST /logs
GET  /logs

POST /metrics
GET  /metrics




Providers / Models

POST   /providers
GET    /providers
GET    /providers/{id}
PUT    /providers/{id}
DELETE /providers/{id}




Chat Interface
POST /chat

Request:

{
  "message": "Build a recipe app"
}


Response:

{
  "reply": "Project created and planning started",
  "actions": [ ... ]
}




Optional MCP Endpoint

POST /mcp


Used by external tools to:

create projects
fetch tasks
submit results


Design Notes

JSON everywhere
Stateless HTTP API
Agents use polling (simple + robust)
Server handles routing, orchestration, context
Chat endpoint acts as intelligent router (tool-calling model)
This API is intentionally minimal and fully aligned with the MVP.


18. Implementation Task List (MVP Roadmap)
A minimal, ordered set of tasks to build the system end-to-end.
Phase 1 – Bootstrap

Initialize Go module (go mod init myagent)
go mod init myagentImplement CLI entry (cmd/myagent/main.go) with modes: 

cmd/myagent/main.goserver
agent

Add basic config loader (YAML)


Phase 2 – Database Layer

Initialize SQLite connection
Implement migrations for all tables (projects, tasks, agents, providers, etc.)
Create repository layer (CRUD for projects, tasks, agents)


Phase 3 – Core Domain Logic

Project service (create, update, list)
Task service: 

create tasks
update status
filter tasks

Agent registry: 

register agent
heartbeat tracking

Basic task queue logic (priority + role filtering)


Phase 4 – REST API

Implement HTTP server (router + middleware)
Projects endpoints
Tasks endpoints (including claim + result)
Agents endpoints (register, heartbeat, next task)
Providers endpoints
Context endpoints
Logs + metrics endpoints


Phase 5 – Agent Runtime

Agent process bootstrap
Agent registration on startup
Polling loop for /tasks/next/tasks/nextTask execution handler (switch by task type)
Result reporting (/tasks/{id}/result)
/tasks/{id}/resultPhase 6 – LLM Provider System

Define LLMProvider interface
LLMProviderImplement at least one provider: 

openai-compatible OR ollama

Provider registry + config mapping
Role → model resolution
/llm/chat endpoint
/llm/chatPhase 7 – Configuration System

Parse config.yaml
Load: 

providers
models
roles
routing
prompts

Prompt template resolver


Phase 8 – Basic Workflow

Implement simple lifecycle: 

create project → create plan task
plan → create work tasks
work → mark completed

Task status transitions


Phase 9 – Chat Interface (Core Feature)

Implement /chat endpoint
/chatIntegrate tool-calling model (chat brain)
Define tools: 

create_project
plan_project
create_tasks
query_context

Tool execution loop (model ↔ server)


Phase 10 – Context System (Minimal)

Save task summaries to DB
Query context by project/task
Attach context to LLM requests


Phase 11 – UI (Minimal Svelte)

Setup Svelte app
Projects list + create UI
Task list view
Agent status view
Integrate REST API
Build and embed into Go binary (embed)
embedPhase 12 – Observability

Log ingestion (/logs)
/logsMetrics tracking (/metrics)
/metricsBasic UI views for logs


Phase 13 – Polish

Error handling across API + agent
Basic validation
Config sanity checks
Simple task retry logic


Result (MVP)
After completing these tasks, you will have:

Running server + UI
Connectable agents
Task queue + execution
LLM integration
Chat-driven orchestration
This is a fully functional minimal autonomous development platform.



















