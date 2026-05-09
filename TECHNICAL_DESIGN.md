# Agent Orchestrator - Technical Design Document

**Version**: 1.0  
**Last Updated**: May 2026

---

## Overview

This document provides the technical foundation for implementing Agent Orchestrator. It defines core interfaces, data structures, package organization, and architectural patterns.

---

## Table of Contents

1. [Package Structure](#package-structure)
2. [Core Interfaces](#core-interfaces)
3. [Data Models](#data-models)
4. [Configuration Architecture](#configuration-architecture)
5. [Provider System](#provider-system)
6. [Task Execution Flow](#task-execution-flow)
7. [Database Architecture](#database-architecture)
8. [API Architecture](#api-architecture)
9. [Tool System](#tool-system)
10. [Concurrency Model](#concurrency-model)

---

## Package Structure

```
agent-orchestrator/
├── cmd/
│   └── main.go                 # Entry point, CLI setup
├── config/
│   ├── config.go               # Config structs and loading
│   ├── validation.go           # Config validation
│   └── defaults.go             # Default values
├── db/
│   ├── db.go                   # Database initialization
│   ├── migrations.go           # Schema setup
│   ├── queries.go              # Prepared queries
│   ├── models.go               # Database models (Project, Task, Agent, etc.)
│   ├── projects.go             # Project-related queries
│   ├── tasks.go                # Task-related queries
│   ├── agents.go               # Agent-related queries
│   ├── providers.go            # Provider-related queries
│   ├── context.go              # Context store queries
│   └── logs.go                 # Log queries
├── llm/
│   ├── provider.go             # LLMProvider interface
│   ├── registry.go             # Provider registry
│   ├── openai.go               # OpenAI-compatible implementation
│   ├── ollama.go               # Ollama implementation
│   ├── anthropic.go            # Anthropic implementation
│   ├── azure.go                # Azure OpenAI implementation
│   └── types.go                # Common LLM types (ChatRequest, etc.)
├── server/
│   ├── server.go               # Server struct and lifecycle
│   ├── handlers.go             # HTTP handlers
│   ├── router.go               # HTTP router setup
│   ├── websocket.go            # WebSocket chat handler
│   └── static/                 # Embedded UI assets (via go:embed)
├── agent/
│   ├── agent.go                # Agent struct and lifecycle
│   ├── client.go               # Server client (HTTP)
│   ├── executor.go             # Task execution logic
│   ├── poller.go               # Task polling loop
│   └── tools.go                # Tool execution (local)
├── router/
│   ├── router.go               # Config-based routing logic
│   ├── prompt.go               # Prompt template engine
│   └── context.go              # Context builder
├── workflow/
│   ├── scheduler.go            # Task scheduling logic
│   ├── executor.go             # Workflow execution
│   └── state.go                # Task state machine
├── tools/
│   ├── plan.go                 # Planning tools
│   ├── tasks.go                # Task management tools
│   ├── code.go                 # Code execution tools
│   ├── context.go              # Context tools
│   ├── monitor.go              # Monitoring tools
│   └── executor.go             # Tool execution registry
├── api/
│   ├── types.go                # API request/response types
│   └── errors.go               # Standard error responses
├── storage/
│   ├── context.go              # Context storage interface
│   └── cache.go                # Optional caching layer
└── logging/
    ├── logger.go               # Logging utilities
    └── metrics.go              # Metrics collection
```

---

## Core Interfaces

### 1. LLMProvider Interface

The provider abstraction allows swapping between different LLM backends.

```go
package llm

import "context"

// ChatRequest represents a chat/completion request
type ChatRequest struct {
    Model       string          `json:"model"`
    Messages    []Message       `json:"messages"`
    Temperature float32         `json:"temperature,omitempty"`
    MaxTokens   int             `json:"max_tokens,omitempty"`
    Tools       []ToolDef       `json:"tools,omitempty"`
    ToolChoice  string          `json:"tool_choice,omitempty"` // "auto", "required", "none"
}

// ChatResponse represents a chat/completion response
type ChatResponse struct {
    Content   string       `json:"content"`
    ToolCalls []ToolCall   `json:"tool_calls,omitempty"`
    StopReason string      `json:"stop_reason"` // "end_turn", "tool_use", etc.
    TokensUsed int         `json:"tokens_used"`
}

// EmbedRequest represents an embedding request
type EmbedRequest struct {
    Model string   `json:"model"`
    Input []string `json:"input"`
}

// EmbedResponse represents an embedding response
type EmbedResponse struct {
    Embeddings [][]float32 `json:"embeddings"`
    TokensUsed int         `json:"tokens_used"`
}

// RerankRequest represents a reranking request
type RerankRequest struct {
    Model   string   `json:"model"`
    Query   string   `json:"query"`
    Docs    []string `json:"documents"`
    TopK    int      `json:"top_k"`
}

// RerankResponse represents a reranking response
type RerankResponse struct {
    Results []RerankResult `json:"results"`
}

// RerankResult is a single reranked result
type RerankResult struct {
    Index int     `json:"index"`
    Score float32 `json:"score"`
}

// Message represents a single message in a conversation
type Message struct {
    Role    string `json:"role"` // "user", "assistant", "system"
    Content string `json:"content"`
}

// ToolDef defines an available tool
type ToolDef struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    InputSchema InputSchema `json:"input_schema"`
}

// InputSchema defines the input format for a tool
type InputSchema struct {
    Type       string             `json:"type"` // "object"
    Properties map[string]Property `json:"properties"`
    Required   []string           `json:"required"`
}

// Property describes a single input property
type Property struct {
    Type        string `json:"type"` // "string", "number", "boolean", "array", etc.
    Description string `json:"description"`
}

// ToolCall represents a tool call issued by the LLM
type ToolCall struct {
    ID        string          `json:"id"`
    Name      string          `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
}

// LLMProvider is the interface all LLM providers must implement
type LLMProvider interface {
    // Chat performs a chat/completion request
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    
    // Embed generates embeddings for text
    Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error)
    
    // Rerank reranks documents by relevance to a query
    Rerank(ctx context.Context, req RerankRequest) (RerankResponse, error)
    
    // Name returns the provider name (e.g., "ollama", "openai", "anthropic")
    Name() string
    
    // Close closes any open connections
    Close() error
}
```

### 2. TaskStore Interface

Database abstraction for task persistence.

```go
package db

import (
    "context"
    "time"
)

// Task represents a work unit to be executed
type Task struct {
    ID             string          `json:"id"`
    ProjectID      string          `json:"project_id"`
    Type           string          `json:"type"` // "plan", "implement", "review", etc.
    Role           string          `json:"role"` // "orchestrator", "worker", "reviewer", etc.
    Status         string          `json:"status"` // "planned", "in_progress", "needs_review", etc.
    Priority       int             `json:"priority"`
    AssignedAgentID string          `json:"assigned_agent_id,omitempty"`
    Payload        map[string]interface{} `json:"payload"` // Task-specific data
    Result         map[string]interface{} `json:"result,omitempty"` // Agent's response
    Attempts       int             `json:"attempts"`
    CreatedAt      time.Time       `json:"created_at"`
    UpdatedAt      time.Time       `json:"updated_at"`
    StartedAt      *time.Time      `json:"started_at,omitempty"`
    CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

// TaskStore manages task persistence
type TaskStore interface {
    // CreateTask creates a new task
    CreateTask(ctx context.Context, task *Task) error
    
    // GetTask retrieves a task by ID
    GetTask(ctx context.Context, id string) (*Task, error)
    
    // UpdateTask updates a task
    UpdateTask(ctx context.Context, task *Task) error
    
    // ListTasks lists tasks with optional filtering
    ListTasks(ctx context.Context, filters TaskFilters) ([]*Task, error)
    
    // ClaimTask claims a task for an agent (atomic operation)
    ClaimTask(ctx context.Context, taskID, agentID string) error
    
    // SubmitTaskResult submits a task result
    SubmitTaskResult(ctx context.Context, taskID string, result map[string]interface{}, status string) error
    
    // GetNextTask gets the next task for an agent matching given roles
    GetNextTask(ctx context.Context, agentID string, roles []string) (*Task, error)
    
    // IncrementAttempts increments attempt counter
    IncrementAttempts(ctx context.Context, taskID string) error
}

// TaskFilters defines optional filters for ListTasks
type TaskFilters struct {
    ProjectID string
    Status    string
    Role      string
    AgentID   string
    Priority  *int
    Limit     int
    Offset    int
}
```

### 3. Agent Interface

Local agent representation.

```go
package agent

import "context"

// Agent represents an autonomous agent
type Agent struct {
    ID              string    `json:"id"`
    Name            string    `json:"name"`
    Roles           []string  `json:"roles"`
    Status          string    `json:"status"` // "online", "offline", "idle", "busy"
    CurrentTaskID   string    `json:"current_task_id,omitempty"`
    RegisteredAt    time.Time `json:"registered_at"`
    LastHeartbeat   time.Time `json:"last_heartbeat"`
    TasksCompleted  int       `json:"tasks_completed"`
    TokensUsed      int       `json:"tokens_used"`
}

// Agent struct for agent process
type Agent struct {
    id            string
    name          string
    roles         []string
    serverURL     string
    client        *http.Client
    pollingTicker *time.Ticker
    done          chan struct{}
    config        *config.Config
    executor      *TaskExecutor
}

// NewAgent creates a new agent
func NewAgent(name string, roles []string, serverURL string, cfg *config.Config) *Agent {
    return &Agent{
        name:      name,
        roles:     roles,
        serverURL: serverURL,
        config:    cfg,
        client:    &http.Client{Timeout: 30 * time.Second},
        done:      make(chan struct{}),
    }
}

// Start registers the agent and begins polling for tasks
func (a *Agent) Start(ctx context.Context) error {
    // Register with server
    agentID, err := a.register(ctx)
    if err != nil {
        return fmt.Errorf("agent registration failed: %w", err)
    }
    a.id = agentID
    
    // Start polling loop
    go a.pollLoop(ctx)
    
    return nil
}

// Stop gracefully shuts down the agent
func (a *Agent) Stop(ctx context.Context) error {
    close(a.done)
    return a.deregister(ctx)
}
```

### 4. ToolExecutor Interface

Framework for tool execution.

```go
package tools

import "context"

// ToolDef defines a tool
type ToolDef struct {
    Name        string
    Description string
    InputSchema interface{} // JSON schema
    Handler     ToolHandler
}

// ToolHandler executes a tool
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// ToolExecutor manages available tools
type ToolExecutor interface {
    // Register registers a tool
    Register(def ToolDef) error
    
    // Execute executes a tool by name
    Execute(ctx context.Context, toolName string, args map[string]interface{}) (interface{}, error)
    
    // List returns all available tools
    List() []ToolDef
    
    // Get retrieves a tool definition
    Get(name string) (*ToolDef, error)
}

// toolExecutorImpl is the standard implementation
type toolExecutorImpl struct {
    tools map[string]ToolDef
    mu    sync.RWMutex
}

func (te *toolExecutorImpl) Execute(ctx context.Context, toolName string, args map[string]interface{}) (interface{}, error) {
    te.mu.RLock()
    tool, ok := te.tools[toolName]
    te.mu.RUnlock()
    
    if !ok {
        return nil, fmt.Errorf("tool not found: %s", toolName)
    }
    
    return tool.Handler(ctx, args)
}
```

---

## Data Models

### Project Model

```go
package db

type Project struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    RepoPath    string    `json:"repo_path"` // Local path or Git URL
    Status      string    `json:"status"` // "planned", "in_progress", "completed", "failed"
    Config      map[string]interface{} `json:"config"` // Project-specific overrides
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### Agent Model

```go
package db

type Agent struct {
    ID            string    `json:"id"`
    Name          string    `json:"name"`
    Roles         []string  `json:"roles"` // Stored as JSON array
    Status        string    `json:"status"` // "online", "offline", "idle", "busy"
    CurrentTaskID string    `json:"current_task_id"`
    RegisteredAt  time.Time `json:"registered_at"`
    LastHeartbeat time.Time `json:"last_heartbeat"`
}
```

### Provider Model

```go
package db

type Provider struct {
    ID           string `json:"id"`
    Name         string `json:"name"`
    Type         string `json:"type"` // "openai_compatible", "anthropic", "ollama", etc.
    BaseURL      string `json:"base_url"`
    ModelName    string `json:"model_name"`
    APIKey       string `json:"api_key"` // Encrypted
    Capabilities []string `json:"capabilities"` // ["chat", "embed", "rerank"]
    Config       map[string]interface{} `json:"config"`
}
```

### Context Model

```go
package db

type Context struct {
    ID        string    `json:"id"`
    ProjectID string    `json:"project_id,omitempty"`
    TaskID    string    `json:"task_id,omitempty"`
    Type      string    `json:"type"` // "summary", "embedding", "snippet", "note"
    Content   string    `json:"content"`
    Embedding []float32 `json:"embedding,omitempty"` // Vector embedding
    Metadata  map[string]interface{} `json:"metadata"`
    CreatedAt time.Time `json:"created_at"`
}
```

---

## Configuration Architecture

### Config Struct

```go
package config

type Config struct {
    Providers    []ProviderConfig `yaml:"providers"`
    Models       []ModelConfig    `yaml:"models"`
    Roles        map[string]string `yaml:"roles"` // role → model name
    Routing      map[string]string `yaml:"routing"` // task type → role
    Prompts      map[string]string `yaml:"prompts"`
    ContextRules map[string]ContextRule `yaml:"context_rules"`
    Server       ServerConfig `yaml:"server"`
    Database     DatabaseConfig `yaml:"database"`
    Agents       AgentConfig `yaml:"agents"`
}

type ProviderConfig struct {
    Name    string `yaml:"name"`
    Type    string `yaml:"type"`
    BaseURL string `yaml:"base_url"`
    APIKey  string `yaml:"api_key"`
    // ... other fields
}

type ModelConfig struct {
    Name     string   `yaml:"name"`
    Provider string   `yaml:"provider"`
    Model    string   `yaml:"model"`
    Roles    []string `yaml:"roles"`
}

type ContextRule struct {
    Include []string `yaml:"include"`
    Exclude []string `yaml:"exclude"`
}
```

### Loading and Validation

```go
package config

// Load loads config from a YAML file
func Load(path string) (*Config, error) {
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config: %w", err)
    }
    
    // Substitute env vars
    content := os.ExpandEnv(string(data))
    
    var cfg Config
    if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }
    
    // Validate
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }
    
    return &cfg, nil
}

func (c *Config) Validate() error {
    if len(c.Providers) == 0 {
        return errors.New("no providers configured")
    }
    if len(c.Models) == 0 {
        return errors.New("no models configured")
    }
    if len(c.Roles) == 0 {
        return errors.New("no roles configured")
    }
    // ... more validation
    return nil
}
```

---

## Provider System

### Provider Registry

```go
package llm

// Registry manages available providers
type Registry struct {
    providers map[string]LLMProvider
    mu        sync.RWMutex
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
    return &Registry{
        providers: make(map[string]LLMProvider),
    }
}

// Register registers a provider
func (r *Registry) Register(name string, provider LLMProvider) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if _, ok := r.providers[name]; ok {
        return fmt.Errorf("provider already registered: %s", name)
    }
    
    r.providers[name] = provider
    return nil
}

// Get retrieves a provider by name
func (r *Registry) Get(name string) (LLMProvider, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    provider, ok := r.providers[name]
    if !ok {
        return nil, fmt.Errorf("provider not found: %s", name)
    }
    
    return provider, nil
}

// InitializeFromConfig initializes providers from config
func (r *Registry) InitializeFromConfig(cfg *config.Config) error {
    for _, pcfg := range cfg.Providers {
        var provider LLMProvider
        
        switch pcfg.Type {
        case "openai_compatible":
            provider = llm.NewOpenAIProvider(pcfg.BaseURL, pcfg.APIKey, pcfg.Model)
        case "ollama":
            provider = llm.NewOllamaProvider(pcfg.BaseURL)
        case "anthropic":
            provider = llm.NewAnthropicProvider(pcfg.APIKey)
        case "azure":
            provider = llm.NewAzureProvider(pcfg.BaseURL, pcfg.APIKey, pcfg.Model)
        default:
            return fmt.Errorf("unsupported provider type: %s", pcfg.Type)
        }
        
        if err := r.Register(pcfg.Name, provider); err != nil {
            return err
        }
    }
    
    return nil
}
```

---

## Task Execution Flow

### Agent Task Execution

```go
package agent

type TaskExecutor struct {
    serverClient *ServerClient
    toolExec     *tools.ToolExecutor
    config       *config.Config
    providerReg  *llm.Registry
}

// ExecuteTask executes a single task
func (te *TaskExecutor) ExecuteTask(ctx context.Context, task *db.Task) error {
    // 1. Fetch context
    contextData, err := te.fetchContext(ctx, task)
    if err != nil {
        return fmt.Errorf("failed to fetch context: %w", err)
    }
    
    // 2. Build LLM request (using config routing)
    llmReq := te.buildLLMRequest(task, contextData)
    
    // 3. Get provider for this role
    modelName := te.config.Roles[task.Role]
    providerName := te.getProviderForModel(modelName)
    provider, err := te.providerReg.Get(providerName)
    if err != nil {
        return fmt.Errorf("provider not found: %w", err)
    }
    
    // 4. Call LLM
    resp, err := provider.Chat(ctx, llmReq)
    if err != nil {
        return fmt.Errorf("LLM call failed: %w", err)
    }
    
    // 5. Execute tool calls in a loop
    for resp.StopReason == "tool_use" && len(resp.ToolCalls) > 0 {
        // Execute each tool call
        toolResults := []ToolResult{}
        for _, tc := range resp.ToolCalls {
            result, err := te.toolExec.Execute(ctx, tc.Name, tc.Arguments)
            toolResults = append(toolResults, ToolResult{
                ToolCallID: tc.ID,
                Content:    result,
                Error:      err,
            })
        }
        
        // Add results to messages and call LLM again
        llmReq.Messages = append(llmReq.Messages, Message{
            Role:    "assistant",
            Content: resp.Content, // Or encode tool calls
        })
        llmReq.Messages = append(llmReq.Messages, Message{
            Role:    "user",
            Content: encodeToolResults(toolResults),
        })
        
        resp, err = provider.Chat(ctx, llmReq)
        if err != nil {
            return fmt.Errorf("LLM call failed: %w", err)
        }
    }
    
    // 6. Report result to server
    result := map[string]interface{}{
        "content": resp.Content,
        "tokens_used": resp.TokensUsed,
    }
    
    status := "completed"
    if resp.StopReason != "end_turn" {
        status = "failed"
    }
    
    return te.serverClient.SubmitTaskResult(ctx, task.ID, result, status)
}

func (te *TaskExecutor) buildLLMRequest(task *db.Task, contextData string) llm.ChatRequest {
    // Load prompt template from config
    promptTemplate := te.config.Prompts[task.Type]
    if promptTemplate == "" {
        promptTemplate = "Execute this task: {payload}"
    }
    
    // Fill in template variables
    prompt := fillPromptTemplate(promptTemplate, map[string]interface{}{
        "payload": task.Payload,
        "context": contextData,
    })
    
    // Get available tools
    availableTools := te.toolExec.List()
    toolDefs := []llm.ToolDef{}
    for _, tool := range availableTools {
        toolDefs = append(toolDefs, llm.ToolDef{
            Name:        tool.Name,
            Description: tool.Description,
            InputSchema: tool.InputSchema,
        })
    }
    
    return llm.ChatRequest{
        Model:      te.config.Roles[task.Role],
        Messages:   []llm.Message{{Role: "user", Content: prompt}},
        Tools:      toolDefs,
        ToolChoice: "auto",
    }
}
```

---

## Database Architecture

### SQLite Schema

```sql
-- Projects table
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    repo_path TEXT,
    status TEXT DEFAULT 'planned',
    config JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tasks table
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    type TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT DEFAULT 'planned',
    priority INT DEFAULT 0,
    assigned_agent_id TEXT,
    payload JSON,
    result JSON,
    attempts INT DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME,
    FOREIGN KEY (project_id) REFERENCES projects(id),
    FOREIGN KEY (assigned_agent_id) REFERENCES agents(id)
);

-- Agents table
CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    roles JSON,
    status TEXT DEFAULT 'offline',
    current_task_id TEXT,
    registered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_heartbeat DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (current_task_id) REFERENCES tasks(id)
);

-- Providers table
CREATE TABLE providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    base_url TEXT,
    model_name TEXT,
    api_key TEXT,
    capabilities JSON,
    config JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Context store table
CREATE TABLE context (
    id TEXT PRIMARY KEY,
    project_id TEXT,
    task_id TEXT,
    type TEXT NOT NULL,
    content TEXT NOT NULL,
    embedding BLOB,
    metadata JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id),
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

-- Logs table
CREATE TABLE logs (
    id TEXT PRIMARY KEY,
    agent_id TEXT,
    task_id TEXT,
    project_id TEXT,
    level TEXT,
    message TEXT,
    metadata JSON,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(id),
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

-- Metrics table
CREATE TABLE metrics (
    id TEXT PRIMARY KEY,
    task_id TEXT,
    agent_id TEXT,
    tokens_used INT,
    cost DECIMAL(10, 6),
    duration_ms INT,
    success BOOLEAN,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);

-- Indexes for performance
CREATE INDEX idx_tasks_project_status ON tasks(project_id, status);
CREATE INDEX idx_tasks_agent_status ON tasks(assigned_agent_id, status);
CREATE INDEX idx_tasks_role ON tasks(role);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_logs_agent ON logs(agent_id);
CREATE INDEX idx_logs_task ON logs(task_id);
CREATE INDEX idx_context_project ON context(project_id);
```

---

## API Architecture

### HTTP Router (Standard Library)

The server uses only the Go standard library `net/http` package with `http.ServeMux` for routing. No external HTTP frameworks (chi, echo, gin, gorilla, etc.) are used.

**Standard HTTP Handler Pattern:**

```go
package server

import (
    "encoding/json"
    "net/http"
    "strings"
)

// Server wraps the HTTP server and dependencies
type Server struct {
    mux  *http.ServeMux
    db   *db.Database
    llm  *llm.Registry
    // ... other fields
}

// NewServer creates a new server
func NewServer(cfg *config.Config, db *db.Database, llm *llm.Registry) *Server {
    s := &Server{
        mux: http.NewServeMux(),
        db:  db,
        llm: llm,
    }
    s.registerHandlers()
    return s
}

// registerHandlers registers all HTTP handlers
func (s *Server) registerHandlers() {
    s.mux.HandleFunc("/api/projects", s.handleProjects)
    s.mux.HandleFunc("/api/projects/", s.handleProjectDetail)
    s.mux.HandleFunc("/api/tasks", s.handleTasks)
    s.mux.HandleFunc("/api/tasks/", s.handleTaskDetail)
    s.mux.HandleFunc("/api/agents", s.handleAgents)
    // ... more handlers
    s.mux.Handle("/ws/chat", websocket.Handler(s.handleChatWebSocket))
}

// handleProjects handles GET/POST /api/projects
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        s.listProjects(w, r)
    case http.MethodPost:
        s.createProject(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

// handleProjectDetail handles GET/PUT/DELETE /api/projects/{id}
func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request) {
    // Extract ID from path: /api/projects/{id}
    parts := strings.Split(r.URL.Path, "/")
    if len(parts) < 4 {
        http.Error(w, "Not found", http.StatusNotFound)
        return
    }
    id := parts[3]
    
    switch r.Method {
    case http.MethodGet:
        s.getProject(w, r, id)
    case http.MethodPut:
        s.updateProject(w, r, id)
    case http.MethodDelete:
        s.deleteProject(w, r, id)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

// Actual handler implementations
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
    projects, err := s.db.ListProjects(r.Context())
    if err != nil {
        s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(projects)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, id string) {
    project, err := s.db.GetProject(r.Context(), id)
    if err != nil {
        s.writeError(w, http.StatusNotFound, "NOT_FOUND", "Project not found")
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(project)
}

// writeError writes a standard error response
func (s *Server) writeError(w http.ResponseWriter, statusCode int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    err := ErrorResponse{
        Code:    code,
        Message: message,
    }
    json.NewEncoder(w).Encode(err)
}

// Start starts the HTTP server
func (s *Server) Start(addr string) error {
    return http.ListenAndServe(addr, s.mux)
}
```

**Advantages of Standard Library:**
- No external dependencies
- Small binary size
- Easy to understand and maintain
- Full control over routing logic
- Sufficient for this use case

### REST Endpoints Organization

```
/api/projects
  GET    - List projects
  POST   - Create project

/api/projects/{id}
  GET    - Get project details
  PUT    - Update project
  DELETE - Delete project

/api/tasks
  GET    - List tasks (with filters)
  POST   - Create task

/api/tasks/{id}
  GET    - Get task details
  PUT    - Update task
  POST /claim - Claim task
  POST /result - Submit result

/api/agents
  GET    - List agents
  POST /register - Register agent
  GET /{id} - Get agent details
  GET /{id}/tasks/next - Get next task
  POST /{id}/heartbeat - Send heartbeat

/api/llm/chat
  POST   - Chat with LLM (with tool support)

/api/context
  POST /save - Save context
  GET /query - Query context

/api/logs
  GET    - Get logs (with filters)
  POST   - Submit log

/api/metrics
  GET    - Get metrics (by type)

/api/providers
  GET    - List providers
  POST   - Create provider
  GET /{id} - Get provider details
  PUT /{id} - Update provider
  DELETE /{id} - Delete provider
  POST /{id}/test - Test provider

WebSocket:
/ws/chat - Chat interface
```

---

## Tool System

### Tool Registration and Execution

```go
package tools

// Initialize registers all built-in tools
func Initialize(exec ToolExecutor, store db.Store, config *config.Config) error {
    // Planning tools
    exec.Register(ToolDef{
        Name: "plan_project",
        Description: "Create architecture and work breakdown for a project",
        Handler: PlanProject,
    })
    
    // Task management tools
    exec.Register(ToolDef{
        Name: "create_work_package",
        Description: "Create a work package/task",
        Handler: CreateWorkPackage,
    })
    
    // Code execution tools
    exec.Register(ToolDef{
        Name: "read_file",
        Description: "Read a file from the repository",
        Handler: ReadFile,
    })
    
    // Context tools
    exec.Register(ToolDef{
        Name: "query_context",
        Description: "Query saved context for a project",
        Handler: QueryContext,
    })
    
    // ... more tools
    
    return nil
}

// Tool handlers follow this pattern:
func PlanProject(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    projectName := args["project_name"].(string)
    requirements := args["requirements"].(string)
    
    // Call LLM to generate plan
    // Store plan in database
    // Return plan
    
    return map[string]interface{}{
        "architecture": "...",
        "work_packages": []string{...},
    }, nil
}
```

---

## Concurrency Model

### Agent Polling and Task Locking

```go
package workflow

// Agent polling loop
func (a *Agent) pollLoop(ctx context.Context) {
    ticker := time.NewTicker(a.config.Agents.TaskPollIntervalSec * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-a.done:
            return
        case <-ticker.C:
            task, err := a.serverClient.GetNextTask(ctx, a.id, a.roles)
            if err != nil {
                log.Printf("error fetching task: %v", err)
                continue
            }
            
            if task != nil {
                // Execute task (non-blocking)
                go a.executeTaskAsync(ctx, task)
            }
        }
    }
}

// Task claiming (atomic operation in database)
// Only one agent can claim a task at a time
func (s *Server) ClaimTask(ctx context.Context, taskID, agentID string) error {
    return s.db.WithTx(func(tx Tx) error {
        task, err := tx.GetTask(taskID)
        if err != nil {
            return err
        }
        
        // Check if already claimed
        if task.AssignedAgentID != "" && task.Status == "in_progress" {
            return errors.New("task already claimed")
        }
        
        // Update atomically
        task.AssignedAgentID = agentID
        task.Status = "in_progress"
        task.StartedAt = time.Now()
        task.Attempts += 1
        
        return tx.UpdateTask(task)
    })
}
```

### Database Transaction Safety

```go
package db

// WithTx wraps operations in a transaction
type Tx interface {
    // ... all Store methods
    Commit() error
    Rollback() error
}

func (d *Database) WithTx(fn func(Tx) error) error {
    tx, err := d.db.Begin()
    if err != nil {
        return err
    }
    
    if err := fn(tx); err != nil {
        tx.Rollback()
        return err
    }
    
    return tx.Commit().Err()
}
```

---

## Error Handling Strategy

### Standard Error Responses

```go
package api

type ErrorResponse struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}

// Define error codes
const (
    ErrCodeNotFound       = "NOT_FOUND"
    ErrCodeInvalidInput   = "INVALID_INPUT"
    ErrCodeConflict       = "CONFLICT"
    ErrCodeInternal       = "INTERNAL_ERROR"
    ErrCodeUnavailable    = "UNAVAILABLE"
)

// Helper to write error responses
func WriteError(w http.ResponseWriter, statusCode int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(ErrorResponse{
        Code:    code,
        Message: message,
    })
}
```

---

## Testing Strategy

### Unit Test Structure

```go
package config_test

import (
    "testing"
    "agent-orchestrator/config"
)

func TestLoadConfig(t *testing.T) {
    cfg, err := config.Load("testdata/config.yaml")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if cfg == nil {
        t.Fatal("config is nil")
    }
    if len(cfg.Providers) != 2 {
        t.Errorf("expected 2 providers, got %d", len(cfg.Providers))
    }
}

func TestConfigValidation(t *testing.T) {
    cfg := &config.Config{}
    err := cfg.Validate()
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    
    expectedMsg := "no providers"
    if !contains(err.Error(), expectedMsg) {
        t.Errorf("expected error containing '%s', got: %v", expectedMsg, err)
    }
}

// Helper function for string contains check
func contains(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}
```

### Integration Test Structure

```go
package server_test

import (
    "context"
    "testing"
    "agent-orchestrator/server"
    "agent-orchestrator/db"
    "agent-orchestrator/agent"
)

func TestAgentTaskExecution(t *testing.T) {
    // Start server
    s := server.New(cfg)
    go s.Start()
    defer s.Stop()
    
    // Register agent
    agentID, err := registerAgent("worker-1", []string{"worker"})
    assert.NoError(t, err)
    
    // Create project and task
    projectID, err := createProject("test", "test project")
    assert.NoError(t, err)
    
    taskID, err := createTask(projectID, "implement", "worker", "implement feature X")
    assert.NoError(t, err)
    
    // Fetch next task
    task, err := getNextTask(agentID, []string{"worker"})
    assert.NoError(t, err)
    assert.Equal(t, taskID, task.ID)
    
    // Verify task execution flow...
}
```

---

## Key Design Decisions

1. **Single Binary**: Compile Svelte UI into Go binary via `go:embed`
2. **Standard Library Only**: Use only Go standard library `net/http` for HTTP routing (no chi, echo, gin, gorilla, etc.)
   - Minimizes dependencies
   - Easier to understand and maintain
   - Uses `http.ServeMux` with helper functions for routing
3. **Config-Driven Routing**: Use YAML config for provider selection and model routing, not hard-coded logic
4. **Provider Abstraction**: Interface-based design allows swapping LLM providers easily
5. **Database Transactions**: Use transactions for atomic updates (task claiming, state changes)
6. **Tool System**: Registry pattern for extensibility (add tools without modifying core)
7. **Agent Model**: Agent polls for tasks (not pushed) for simplicity and independence
8. **Context Architecture**: Store context separately from tasks for flexibility
9. **Error Handling**: Comprehensive error codes and logging for debugging
10. **Testing**: Standard library `testing` package only (no testify or other assertion libraries)

---

**Next Steps**: Use this technical design as the foundation for implementation. Start with Phase 1 tasks using these interfaces and patterns.

