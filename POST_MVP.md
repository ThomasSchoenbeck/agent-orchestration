# Agent Orchestrator — Post-MVP Feature Roadmap

Builds on the completed MVP (Phases 1–5). Each numbered item below is an
implementation unit — scoped to fit in one focused session.

Format: `[ ]` = pending · `[~]` = in progress · `[x]` = done

---

## Area 1 — Project Detail Page

### What
Clicking a project card navigates to a full detail view rather than a dead end.
The detail page is the primary workspace for a project: it shows metadata,
associated tasks, and an AI assistant sidebar.

### Backend changes

- [x] **#42 Expose project-scoped task endpoint**
  - `GET /api/projects/:id/tasks` — returns tasks filtered to that project,
    same query params as `GET /api/tasks` (status, role, type, limit, offset)
  - `GET /api/projects/:id` — already exists; verify it returns all fields
    including the new `local_path` and `git_url` columns (see #44 below)
  - No new files needed; add handler to `server/handlers.go` and wire in
    `server/router.go`

- [x] **#43 Project update endpoint**
  - `PUT /api/projects/:id` — partial update (name, description, local_path,
    git_url, status)
  - Add `UpdateProject(ctx, id, fields)` to `db/db.go`; use a simple
    `map[string]interface{}` patch approach so callers only send changed fields
  - Return the updated project object

- [x] **#44 Add `local_path` and `git_url` to projects schema**
  - Add two nullable TEXT columns to the `projects` table migration
  - Update `db.Project` struct and all CRUD methods
  - `local_path`: absolute path on the orchestrator host's filesystem
  - `git_url`: any URL `git clone` accepts (https or ssh)
  - Expose both fields in all project JSON responses
  - Add a lightweight validation step: if `local_path` is set, check that the
    path exists on the server when the project is created or updated (return a
    warning, not a hard error, since the agent host may differ)

### Frontend changes

- [x] **#45 Routing: project detail navigation**
  - Add a client-side route `/projects/:id` — Svelte 5 route state via the
    existing `stores.js` router or a minimal hash-router extension
  - `ProjectCard` component (extracted from `Projects.svelte`): the whole card
    is a clickable link; keep the delete button as a stop-propagation action
  - New page component `src/pages/ProjectDetail.svelte`

- [x] **#46 ProjectDetail page — metadata panel**
  - Shows: name (inline-editable), description (markdown editor — see #51),
    status badge, created_at, local_path, git_url
  - Edit button reveals a form; `PUT /api/projects/:id` on save
  - Breadcrumb: `Projects › <project name>`

- [x] **#47 ProjectDetail page — tasks panel**
  - Embedded task list fetched from `GET /api/projects/:id/tasks`
  - Same status/role filter controls as the Tasks page
  - "New task" button opens the create-task form pre-filled with the project id
  - Task rows are expandable to show full payload without leaving the page
  - Status badge colours match the Tasks page

---

## Area 1.5 — Task Detail Page and Queue Management

### What
Clicking a task navigates to a full detail view with the ability to inspect and
manage the task. Tasks can be removed from the queue, and descriptions use the
reusable markdown editor component for consistency with project descriptions.

### Backend changes

- [x] **#48a Unqueue task endpoint**
  - `POST /api/tasks/:id/unqueue` — removes a task from the queue (sets status
    to `planned`; validates task is queued or planned before allowing)
  - Returns the updated task object
  - Only permit this action on queued/pending tasks; return 400 if task is
    already completed or running

### Frontend changes

- [x] **#48b Routing: task detail navigation**
  - Client-side route `/tasks/:id` added to App.svelte
  - Task rows in both Tasks page and ProjectDetail page are clickable
    links to the detail view

- [x] **#48c TaskDetail page — metadata panel**
  - Shows: title (inline-editable), status badge, project name (link), role,
    type, created_at, updated_at
  - Edit button reveals a form; `PUT /api/tasks/:id` on save
  - Breadcrumb: `Projects › <project name> › Tasks › <task title>`

- [x] **#48d TaskDetail page — description editor**
  - Full-width MarkdownEditor component (from #52) for the task description /
    payload.description field
  - "Preview only" mode for read-only display when task is not being edited
  - Emits `PUT /api/tasks/:id` on save with updated description

- [x] **#48e TaskDetail page — queue controls**
  - Status-dependent action buttons:
    - Status 'planned' or 'failed' → "Queue" button calls `PUT /api/tasks/:id` with status='queued'
    - Status 'queued' or 'planned' → "Unqueue" button calls `POST /api/tasks/:id/unqueue`
    - Any status → "Delete" button (with confirmation)
  - Success feedback (toast) after queue/unqueue actions
  - Queue button also available in main Tasks list and ProjectDetail view

---

## Area 2 — AI Assistant Sidebar (Project Detail)

### What
A collapsible right-hand panel on the Project Detail page hosts a chat thread
scoped to that project. The assistant has the project's context pre-loaded and
can help write the description, draft specs, or answer questions about the task
breakdown.

### Backend changes

- [x] **#48 Project-scoped chat endpoint**
  - `POST /api/projects/:id/chat` — accepts `{ message, provider_id?,
    conversation_id? }`
  - Before forwarding to the LLM, prepend a system message that includes:
    - Project name and description
    - Recent context entries from `QueryContext` (top 5 by relevance)
    - Current task list summary (counts by status)
  - Returns `{ reply, conversation_id, usage }` — reuses conversation/message DB layer
  - Automatically creates conversations for projects

### Frontend changes

- [x] **#49 AssistantSidebar component**
  - `src/components/AssistantSidebar.svelte`
  - Fixed-width (384 px) right panel; toggled with a button in the ProjectDetail header
  - State persisted in `localStorage` per project
  - Input at the bottom; message thread scrolls upward
  - "Apply to description" button appears on any assistant message
  - Provider selector (dropdown) at the top of the sidebar
  - Reuses project-scoped chat endpoint (#48) for conversation management

---

## Area 3 — Task Type and Role Improvements

### What
Replace the free-text type/role inputs with dropdowns backed by well-documented
enumerations. The values should be visible, discoverable, and consistent with
what the workflow engine actually understands.

### Backend changes

- [x] **#50 Enum metadata endpoint**
  - `GET /api/meta/task-types` and `GET /api/meta/task-roles`
  - Return arrays of `{ value, label, description }` objects so the UI can
    render helpful tooltips
  - Hard-coded in a new `server/meta.go` file (these values change rarely;
    no DB table needed for now)

  **Task types**

  | value | label | description |
  |---|---|---|
  | `implement` | Implement | Write or modify code to fulfil a spec |
  | `review` | Code Review | Review code for correctness and style |
  | `test` | Write Tests | Create unit / integration tests |
  | `design` | Design | Architecture decision, diagram, or spec document |
  | `research` | Research | Investigate options and write up findings |
  | `debug` | Debug | Diagnose and fix a failing test or runtime error |
  | `document` | Document | Write or update docs, READMEs, or comments |
  | `refactor` | Refactor | Improve code quality without changing behaviour |
  | `deploy` | Deploy | CI/CD, infra, or release step |

  **Task roles**

  | value | label | description |
  |---|---|---|
  | `worker` | Worker | General-purpose implementation agent |
  | `reviewer` | Reviewer | Reviews and approves work from a worker |
  | `tester` | Tester | Writes and runs tests; validates correctness |
  | `architect` | Architect | High-level design and task breakdown |
  | `analyst` | Analyst | Research, requirements analysis, reporting |

### Frontend changes

- [x] **#50b Update task create/edit form**
  - Replace type and role `<input type="text">` with `<select>` populated from
    `GET /api/meta/task-types` and `GET /api/meta/task-roles`
  - Show the description as a tooltip or sub-label under each option
  - Keep a final "Custom…" option in both dropdowns that re-enables free-text
    entry (for future extensibility)
  - Update `Tasks.svelte` filter dropdowns to use the same enum list

---

## Area 4 — Markdown Editor Component

### What
Any large text input (project description, task payload description, chat
messages) uses a split-pane markdown editor: left = raw markdown, right = live
preview. A toolbar covers the common operations. On narrow viewports the panes
stack vertically with a tab toggle.

### Implementation notes

- Use [CodeMirror 6](https://codemirror.net/) with the `@codemirror/lang-markdown`
  extension for the editor pane — it gives syntax highlighting, bracket
  completion, and a small install footprint (no iframe, no contenteditable hacks)
- Render the preview with [marked](https://marked.js.org/) + DOMPurify (XSS
  safety)
- Package both as a single reusable Svelte component

### Frontend changes

- [x] **#51 Install markdown editor deps**
  - `pnpm add @codemirror/view @codemirror/state @codemirror/lang-markdown @codemirror/theme-one-dark marked dompurify`
  - Add types if needed: `pnpm add -D @types/dompurify`

- [x] **#52 MarkdownEditor component**
  - `src/components/MarkdownEditor.svelte`
  - Props: `value` (bound string), `placeholder`, `minHeight`, `readonly`
  - Toolbar buttons: Bold, Italic, Heading, Link, Code, Code Block, Unordered
    List, Ordered List, Blockquote, Horizontal Rule
  - Split-pane layout: 50/50, draggable divider
  - "Preview only" toggle for read-only display contexts
  - Emits `change` event on every keystroke (debounced 150 ms)
  - Respects the page's dark/light mode via CSS variables

- [x] **#53 Wire MarkdownEditor into existing pages**
  - `Projects.svelte` — project description field — done
  - `ProjectDetail.svelte` — description editor panel (from #46) — done
  - `TaskDetail.svelte` — task description / payload.description field (from #48d) — done
  - `Chat.svelte` — message input (render markdown in sent messages) — done

---

## Area 5 — Provider Management in the UI

### What
Move the LLM provider configuration from the static YAML file into the
database. The config file remains useful as an initial seed (`import on first
run`) but all ongoing management (add, edit, test, delete, enable/disable)
happens in the UI. This makes it possible to deploy the orchestrator and then
configure it entirely through the browser.

### Backend changes

- [x] **#54 `providers` DB table and CRUD**
  - New table `providers`: `id TEXT PK, name TEXT UNIQUE, type TEXT, api_key
    TEXT, base_url TEXT, default_model TEXT, enabled INTEGER, config_json TEXT,
    created_at TEXT, updated_at TEXT`
  - `config_json` stores provider-specific extras (e.g. Azure deployment name,
    Ollama model list)
  - `db/db.go`: `CreateProvider`, `GetProvider`, `ListProviders`,
    `UpdateProvider`, `DeleteProvider`
  - Migration: add table if not exists (no data loss)

- [x] **#55 Provider API endpoints**
  - `GET    /api/providers`          — list all, include `enabled` flag
  - `POST   /api/providers`          — create (name, type, api_key, base_url,
    default_model, config_json, enabled)
  - `GET    /api/providers/:id`      — get one
  - `PUT    /api/providers/:id`      — update any field
  - `DELETE /api/providers/:id`      — delete (soft-error if referenced by a
    conversation)
  - `POST   /api/providers/:id/test` — make a minimal `"Say hi"` request and
    return `{ ok, latency_ms, error? }`
  - `POST   /api/providers/seed`     — import providers from the loaded config
    file into the DB (idempotent; skip if `name` already exists)

- [x] **#55b Config-to-DB migration on startup**
  - On server start, if the `providers` table is empty and the config file
    defines providers, automatically call the seed logic so first-run experience
    is seamless
  - Log a notice: `"Seeded N providers from config file into database"`

- [x] **#55c Update LLM registry to read from DB**
  - `llm/registry.go`: add `InitFromDB(db)` alongside the existing
    `InitFromConfig`
  - Server startup calls `InitFromDB`; agents call it when they register
  - When a provider is updated via the API, re-initialise only that entry in
    the registry (no full restart needed)

### Frontend changes

- [x] **#55d Providers page rewrite**
  - `src/pages/Providers.svelte`
  - Provider list: card per provider showing name, type badge, model, status
    (enabled/disabled toggle), last-test result
  - "Add provider" button → inline form or slide-over panel with fields:
    - Name (free text)
    - Type (select: openai / anthropic / azure / ollama / custom)
    - API key (password input, masked, with show/hide toggle)
    - Base URL (shown/required only for azure and ollama)
    - Default model (text input; for ollama show a "fetch models" button)
    - Azure extras (deployment name, api-version) — shown only when type=azure
    - Enabled toggle
  - "Test connection" button per provider card — shows latency or error inline
  - "Import from config" button calls `/api/providers/seed`
  - Delete with confirmation guard

---

## Area 6 — Chat Conversations

### What
The Chat page gains a conversation history sidebar and a provider selector.
Users can create named conversations, switch between them, and delete them.
Messages are persisted so refreshing the page restores the last conversation.

### Backend changes

- [x] **#56 `conversations` and `messages` DB tables**
  - `conversations`: `id TEXT PK, title TEXT, provider_id TEXT, created_at TEXT, updated_at TEXT`
  - `messages`: `id TEXT PK, conversation_id TEXT FK, role TEXT, content TEXT, tokens_used INTEGER, created_at TEXT`
  - Added to `db/conversations.go` with full CRUD methods
  - Added indexes for query performance

- [x] **#57 Conversation API endpoints**
  - `GET    /api/conversations` — list all conversations
  - `POST   /api/conversations` — create new conversation
  - `GET    /api/conversations/:id` — get conversation with messages
  - `PUT    /api/conversations/:id` — update conversation
  - `DELETE /api/conversations/:id` — delete conversation and messages
  - `GET    /api/conversations/:id/messages` — list paginated messages
  - `POST   /api/conversations/:id/messages` — add message to conversation

- [x] **#57b Update chat to support conversations**
  - Frontend (Chat.svelte) handles conversation persistence via REST API
  - Messages are saved to DB via `addMessage()` calls in Chat component
  - Conversation history loaded when selecting a conversation
  - WebSocket continues to handle real-time streaming

### Frontend changes

- [x] **#58 Chat page redesign**
  - Two-column layout: conversation list (224 px) | message thread
  - ConversationList sub-component: shows title, provider badge, timestamps
  - "New conversation" button creates and selects conversation immediately
  - Renaming: inline edit with save/cancel on each conversation
  - MessageThread sub-component: shows user/assistant messages with provider context
  - Provider selector: dropdown to switch providers per conversation
  - Full conversation history persisted in database

---

## Area 7 — Agent Role Definitions in the UI

### What
The four config-file sections that define how agents behave — `roles`,
`routing`, `prompts`, and `context_rules` — are moved into the database and
made fully editable in the UI. The config file remains a first-run seed source,
but after that every aspect of a role (which provider it uses, what its system
prompt says, which task types it handles, and what context it receives) can be
changed live without restarting the server.

### Data model

An **agent role definition** is a single record that merges what was previously
spread across the four config sections:

| field | type | maps from config |
|---|---|---|
| `id` | TEXT PK (uuid) | — |
| `name` | TEXT UNIQUE | key in `roles:` map |
| `label` | TEXT | display name (new field) |
| `description` | TEXT | human-readable purpose |
| `provider_id` | TEXT FK → providers | `roles: name → model → provider` |
| `model_override` | TEXT | `models[].model` (empty = use provider default) |
| `system_prompt` | TEXT | `prompts: name:` block |
| `context_include` | TEXT (JSON array) | `context_rules: name: include:` |
| `context_exclude` | TEXT (JSON array) | `context_rules: name: exclude:` |
| `task_types` | TEXT (JSON array) | inverse of `routing:` — task types that route to this role |
| `temperature` | REAL | new (not in config today) |
| `max_tokens` | INTEGER | new |
| `enabled` | INTEGER | new |
| `created_at` / `updated_at` | TEXT | — |

**Routing is role-owned**: instead of a separate routing table, each role
declares which task types it handles via `task_types`. The router resolves a
task's role by finding the role definition whose `task_types` array contains
the task's type. If no match is found, it falls back to the role whose `name`
exactly matches the task's `role` field.

**Context types** (for `context_include` / `context_exclude`):
- `project_memory` — context entries from `QueryContext` for the project
- `recent_tasks` — last N completed tasks in the project (summary)
- `agent_history` — this specific agent's recently completed tasks

**System prompt template variables** (Go `text/template` syntax):
- `{{.ProjectName}}` — project name
- `{{.ProjectDescription}}` — project description
- `{{.TaskType}}` — task type string (`implement`, `review`, etc.)
- `{{.TaskTitle}}` — task title from payload
- `{{.TaskDescription}}` — task description from payload
- `{{.Context}}` — injected context block (assembled from `context_include`)
- `{{.RecentTasks}}` — recent task summaries
- `{{.AgentName}}` — the executing agent's registered name

### Backend changes

- [x] **#62 `agent_role_definitions` table**
  - Add to `db/db.go`: `CreateRoleDefinition`, `GetRoleDefinition`,
    `GetRoleDefinitionByName`, `ListRoleDefinitions`, `UpdateRoleDefinition`,
    `DeleteRoleDefinition`
  - `context_include` and `context_exclude` stored as JSON arrays; helpers to
    marshal/unmarshal alongside the struct
  - `task_types` stored as JSON array
  - Migration: `CREATE TABLE IF NOT EXISTS agent_role_definitions (...)` — safe
    to add to existing databases

- [x] **#63 Role definition API endpoints**
  - `GET    /api/roles`           — list all role definitions; include
    `task_types`, `context_include`, `context_exclude` inline (not raw JSON)
  - `POST   /api/roles`           — create a new role definition
  - `GET    /api/roles/:id`       — get one by id
  - `PUT    /api/roles/:id`       — full or partial update; after saving,
    notify the router to reload this role (see #65)
  - `DELETE /api/roles/:id`       — soft-error if any registered agent
    currently declares this role; return a warning listing affected agents
  - `POST   /api/roles/seed`      — parse the loaded config file's `roles`,
    `routing`, `prompts`, and `context_rules` sections and insert records for
    any role name not already in the DB (idempotent)
  - `POST   /api/roles/:id/preview-prompt` — accepts `{ project_name,
    task_type, task_title, context_snippet }` and renders the role's
    `system_prompt` template with those values; returns the rendered string;
    used by the UI's "Preview" button in the prompt editor

- [x] **#64 Config-to-DB seed on startup**
  - On server start, if `agent_role_definitions` is empty and the config file
    defines at least one role, call the seed logic automatically
  - Seed process per config role (e.g. `worker`):
    1. Look up the model name from `roles: worker: gpt-4o`
    2. Look up the provider name from `models[name=gpt-4o].provider`
    3. Resolve the `provider_id` FK from the `providers` table (providers must
       be seeded first — #55b — so run role seeding after provider seeding)
    4. Collect system prompt from `prompts.worker`
    5. Collect `context_include` / `context_exclude` from
       `context_rules.worker`
    6. Collect `task_types` as the inverse of `routing` (find all routing
       entries whose value == `"worker"`)
    7. Insert with `enabled = true`, `temperature = 0.7`, `max_tokens = 4096`
  - Log: `"Seeded N role definitions from config file into database"`

- [x] **#65 Router reads role definitions from DB**
  - `router/router.go` currently builds its routing table from `config.Config`
    at startup
  - Add `LoadRolesFromDB(db) error` — queries `agent_role_definitions` and
    populates an in-memory map `roleByTaskType map[string]*RoleDefinition` and
    `roleByName map[string]*RoleDefinition`
  - Add `ReloadRole(id string)` — called after a `PUT /api/roles/:id`; updates
    only the affected entry without a full reload
  - `BuildSystemPrompt(role, task, project) (string, error)` — renders the
    stored Go template; replaces the current hardcoded prompt logic
  - `ContextForRole(role) ContextSpec` — returns include/exclude lists;
    replaces the current config-driven context assembly in `agent/executor.go`
  - Fallback: if DB returns no roles (e.g. during migration), fall back to
    config-driven behaviour and log a warning

- [x] **#66 Agent executor uses DB-backed roles**
  - `agent/executor.go`: replace all direct reads of `config.Roles`,
    `config.Prompts`, `config.ContextRules`, `config.Routing` with calls to
    the router's new DB-backed methods
  - No change to the agent's wire protocol — it still sends its role list on
    registration; the server matches those role strings against
    `agent_role_definitions.name`

### Frontend changes

- [x] **#67 New "Roles" nav section**
  - Add "Roles" between "Agents" and "Providers" in `App.svelte`'s sidebar
  - New page: `src/pages/Roles.svelte`
  - Fetches `GET /api/roles` on mount

- [x] **#68 Role list view**
  - Card grid (same visual language as Providers)
  - Each card shows:
    - Role name (slug) and label
    - Provider + model badge (e.g. "openai · gpt-4o")
    - Task types this role handles (pill tags: `implement`, `test`, …)
    - Context injected (pill tags: `project_memory`, `recent_tasks`, …)
    - Enabled/disabled toggle (calls `PUT /api/roles/:id` immediately)
    - Edit and Delete buttons
  - "New role" button at the top right
  - "Import from config" button calls `/api/roles/seed`; disabled if all
    config roles already exist in the DB

- [x] **#69 Role editor — general tab**
  - Slide-over or dedicated `/roles/:id/edit` route
  - Fields:
    - **Name** (slug, e.g. `worker`) — validated: lowercase, alphanumeric +
      hyphen, unique; read-only after first save to avoid breaking agent
      registrations
    - **Label** — display name shown in the UI
    - **Description** — short MarkdownEditor (see #52)
    - **Enabled** — toggle
  - Save calls `POST /api/roles` (create) or `PUT /api/roles/:id` (update)

- [x] **#70 Role editor — model tab**
  - **Provider** — dropdown populated from `GET /api/providers` (enabled only)
  - **Model override** — text input; placeholder = provider's default model;
    a "Use provider default" checkbox clears the field
  - **Temperature** — slider 0.0–2.0, step 0.05, with numeric readout
  - **Max tokens** — number input (256–128 000)
  - Changes are staged locally and saved with the main Save button

- [x] **#71 Role editor — system prompt tab**
  - Full-width CodeMirror editor (reuses the CodeMirror instance from #51 but
    with Go template syntax highlighting rather than markdown)
  - Template variable reference panel on the right: lists all available
    `{{.Variable}}` placeholders with a one-line description; clicking one
    inserts it at the cursor
  - **"Preview prompt"** button — opens a modal with sample inputs (project
    name, task type, task title, a stub context) and calls
    `POST /api/roles/:id/preview-prompt`; displays the rendered output so the
    author can verify template correctness before saving
  - Template errors (unclosed braces, unknown variables) are shown inline below
    the editor as a red banner

- [x] **#72 Role editor — routing tab**
  - **Task types handled by this role** — checkbox list of all known task types
    (fetched from `GET /api/meta/task-types` defined in #50); each row shows
    the type value, label, and description
  - Conflict warning: if a task type is already claimed by another enabled
    role, the checkbox is highlighted amber with a tooltip naming the conflicting
    role; saving is still allowed (last writer wins) but the warning is
    prominent
  - **Fallback matching** — a sub-section explains that if no task type matches,
    agents whose registered role name equals this role's `name` will still be
    dispatched to this role; this is shown as an informational callout, not a
    configurable field

- [x] **#73 Role editor — context rules tab**
  - Two multi-select checkbox groups: **Include** and **Exclude**
  - Options: `project_memory`, `recent_tasks`, `agent_history`
  - Each option has a tooltip explaining what data is injected
  - Visual callout: "Excluded items override included ones"
  - `agent_history` shows a note: "Useful for worker roles; can add noise for
    orchestrator and reviewer roles"

- [x] **#74 Agents page — show resolved role definition**
  - Currently the Agents page lists each agent with its self-reported role
    strings
  - Add a "Resolved definition" column: for each role string, look up the
    matching `agent_role_definitions.label`; if found, show it as a badge
    linking to the role editor; if not found, show a warning badge ("no
    definition")
  - This helps operators spot agents using stale or misspelled role names

---

## Cross-cutting

- [ ] **#59 Toast / error system improvements**
  - Currently toasts are plain text; add a `type` field (`success | error |
    warning | info`) with matching icon and colour
  - Auto-dismiss after 5 s for success; persist until dismissed for errors
  - Stack up to 3 visible toasts; older ones slide out the top

- [ ] **#60 Loading skeletons**
  - Replace the plain "Loading…" text in every page with animated skeleton
    placeholder cards that match the real layout
  - `src/components/Skeleton.svelte` — configurable rows, card vs. table mode

- [ ] **#61 Keyboard navigation and accessibility**
  - All modals trap focus (focus-trap-svelte or a small bespoke implementation)
  - All icon-only buttons have `aria-label`
  - Color contrast passes WCAG AA for both light and dark modes

---

## Sequencing recommendation

The items above are roughly independent but a sensible order is:

1. **#44 + #43 + #42** (schema + API) — unblocks all project-detail UI work
2. **#51 + #52** (markdown editor) — used by multiple pages; do this early
3. **#45 + #46 + #47** (project detail page)
4. **#48a + #48b + #48c + #48d + #48e** (task detail page and unqueue) — depends
   on markdown editor (#52); complements project detail work
5. **#50 + #50b** (task type/role dropdowns) — quick win, no dependencies
6. **#54 + #55 + #55b + #55c + #55d** (providers in DB) — roles depend on this
7. **#62 + #63 + #64 + #65 + #66** (role definitions — backend) — must follow
   providers-in-DB so seed logic can resolve `provider_id` FKs
8. **#67 + #68 + #69 + #70 + #71 + #72 + #73 + #74** (role definitions — UI)
9. **#56 + #57 + #57b + #58** (chat conversations)
10. **#48 + #49** (AI assistant sidebar) — depends on conversations (#56) and
    providers-in-DB (#54)
11. **#59 + #60 + #61** (polish) — any time, incrementally
