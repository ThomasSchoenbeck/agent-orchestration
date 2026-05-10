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

- [ ] **#42 Expose project-scoped task endpoint**
  - `GET /api/projects/:id/tasks` — returns tasks filtered to that project,
    same query params as `GET /api/tasks` (status, role, type, limit, offset)
  - `GET /api/projects/:id` — already exists; verify it returns all fields
    including the new `local_path` and `git_url` columns (see #44 below)
  - No new files needed; add handler to `server/handlers.go` and wire in
    `server/router.go`

- [ ] **#43 Project update endpoint**
  - `PUT /api/projects/:id` — partial update (name, description, local_path,
    git_url, status)
  - Add `UpdateProject(ctx, id, fields)` to `db/db.go`; use a simple
    `map[string]interface{}` patch approach so callers only send changed fields
  - Return the updated project object

- [ ] **#44 Add `local_path` and `git_url` to projects schema**
  - Add two nullable TEXT columns to the `projects` table migration
  - Update `db.Project` struct and all CRUD methods
  - `local_path`: absolute path on the orchestrator host's filesystem
  - `git_url`: any URL `git clone` accepts (https or ssh)
  - Expose both fields in all project JSON responses
  - Add a lightweight validation step: if `local_path` is set, check that the
    path exists on the server when the project is created or updated (return a
    warning, not a hard error, since the agent host may differ)

### Frontend changes

- [ ] **#45 Routing: project detail navigation**
  - Add a client-side route `/projects/:id` — Svelte 5 route state via the
    existing `stores.js` router or a minimal hash-router extension
  - `ProjectCard` component (extracted from `Projects.svelte`): the whole card
    is a clickable link; keep the delete button as a stop-propagation action
  - New page component `src/pages/ProjectDetail.svelte`

- [ ] **#46 ProjectDetail page — metadata panel**
  - Shows: name (inline-editable), description (markdown editor — see #51),
    status badge, created_at, local_path, git_url
  - Edit button reveals a form; `PUT /api/projects/:id` on save
  - Breadcrumb: `Projects › <project name>`

- [ ] **#47 ProjectDetail page — tasks panel**
  - Embedded task list fetched from `GET /api/projects/:id/tasks`
  - Same status/role filter controls as the Tasks page
  - "New task" button opens the create-task form pre-filled with the project id
  - Task rows are expandable to show full payload without leaving the page
  - Status badge colours match the Tasks page

---

## Area 2 — AI Assistant Sidebar (Project Detail)

### What
A collapsible right-hand panel on the Project Detail page hosts a chat thread
scoped to that project. The assistant has the project's context pre-loaded and
can help write the description, draft specs, or answer questions about the task
breakdown.

### Backend changes

- [ ] **#48 Project-scoped chat endpoint**
  - `POST /api/projects/:id/chat` — accepts `{ message, provider_id?,
    conversation_id? }`
  - Before forwarding to the LLM, prepend a system message that includes:
    - Project name and description
    - Recent context entries from `QueryContext` (top 5 by relevance)
    - Current task list summary (counts by status)
  - Returns `{ reply, conversation_id, usage }` — same shape as the general
    `/api/llm/chat` but always conversation-aware
  - Reuses the conversation/message DB layer introduced in #56 (Chat
    conversations)

### Frontend changes

- [ ] **#49 AssistantSidebar component**
  - `src/components/AssistantSidebar.svelte`
  - Fixed-width (380 px) right panel; toggled with a button in the ProjectDetail
    header; state persisted in `localStorage` per project
  - Input at the bottom; message thread scrolls upward; streaming responses via
    the existing WebSocket or chunked fetch
  - "Apply to description" button appears on any assistant message — clicking it
    populates the project description editor with that message content
  - Provider selector (dropdown) at the top of the sidebar; defaults to the
    first enabled provider

---

## Area 3 — Task Type and Role Improvements

### What
Replace the free-text type/role inputs with dropdowns backed by well-documented
enumerations. The values should be visible, discoverable, and consistent with
what the workflow engine actually understands.

### Backend changes

- [ ] **#50 Enum metadata endpoint**
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

- [ ] **#50b Update task create/edit form**
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

- [ ] **#51 Install markdown editor deps**
  - `pnpm add @codemirror/view @codemirror/state @codemirror/lang-markdown @codemirror/theme-one-dark marked dompurify`
  - Add types if needed: `pnpm add -D @types/dompurify`

- [ ] **#52 MarkdownEditor component**
  - `src/components/MarkdownEditor.svelte`
  - Props: `value` (bound string), `placeholder`, `minHeight`, `readonly`
  - Toolbar buttons: Bold, Italic, Heading, Link, Code, Code Block, Unordered
    List, Ordered List, Blockquote, Horizontal Rule
  - Split-pane layout: 50/50, draggable divider
  - "Preview only" toggle for read-only display contexts
  - Emits `change` event on every keystroke (debounced 150 ms)
  - Respects the page's dark/light mode via CSS variables

- [ ] **#53 Wire MarkdownEditor into existing pages**
  - `Projects.svelte` — project description field
  - `ProjectDetail.svelte` — description editor panel (from #46)
  - `Tasks.svelte` — task description / payload.description field
  - `Chat.svelte` — message input (render markdown in sent messages)

---

## Area 5 — Provider Management in the UI

### What
Move the LLM provider configuration from the static YAML file into the
database. The config file remains useful as an initial seed (`import on first
run`) but all ongoing management (add, edit, test, delete, enable/disable)
happens in the UI. This makes it possible to deploy the orchestrator and then
configure it entirely through the browser.

### Backend changes

- [ ] **#54 `providers` DB table and CRUD**
  - New table `providers`: `id TEXT PK, name TEXT UNIQUE, type TEXT, api_key
    TEXT, base_url TEXT, default_model TEXT, enabled INTEGER, config_json TEXT,
    created_at TEXT, updated_at TEXT`
  - `config_json` stores provider-specific extras (e.g. Azure deployment name,
    Ollama model list)
  - `db/db.go`: `CreateProvider`, `GetProvider`, `ListProviders`,
    `UpdateProvider`, `DeleteProvider`
  - Migration: add table if not exists (no data loss)

- [ ] **#55 Provider API endpoints**
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

- [ ] **#55b Config-to-DB migration on startup**
  - On server start, if the `providers` table is empty and the config file
    defines providers, automatically call the seed logic so first-run experience
    is seamless
  - Log a notice: `"Seeded N providers from config file into database"`

- [ ] **#55c Update LLM registry to read from DB**
  - `llm/registry.go`: add `InitFromDB(db)` alongside the existing
    `InitFromConfig`
  - Server startup calls `InitFromDB`; agents call it when they register
  - When a provider is updated via the API, re-initialise only that entry in
    the registry (no full restart needed)

### Frontend changes

- [ ] **#55d Providers page rewrite**
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

- [ ] **#56 `conversations` and `messages` DB tables**
  - `conversations`: `id TEXT PK, title TEXT, provider_id TEXT, created_at
    TEXT, updated_at TEXT`
  - `messages`: `id TEXT PK, conversation_id TEXT FK, role TEXT, content TEXT,
    tokens_used INTEGER, created_at TEXT`
  - `db/db.go`: `CreateConversation`, `ListConversations`, `GetConversation`,
    `UpdateConversation`, `DeleteConversation`, `AddMessage`, `ListMessages`

- [ ] **#57 Conversation API endpoints**
  - `GET    /api/conversations`             — list all (id, title, provider_id,
    updated_at, message_count)
  - `POST   /api/conversations`             — create `{ title?, provider_id }`
  - `GET    /api/conversations/:id`         — get with last 50 messages
  - `PUT    /api/conversations/:id`         — update title or provider_id
  - `DELETE /api/conversations/:id`         — delete with all messages
  - `GET    /api/conversations/:id/messages`— paginated message history
  - `POST   /api/conversations/:id/messages`— send a message; appends both the
    user message and assistant reply to the DB, returns `{ message, usage }`

- [ ] **#57b Update `/ws/chat` and `/api/llm/chat`**
  - Accept optional `conversation_id` in the request payload
  - If provided, load recent messages from DB as conversation history before
    sending to the LLM (up to the provider's context window limit)
  - If not provided, behave as today (stateless one-shot)

### Frontend changes

- [ ] **#58 Chat page redesign**
  - Three-column layout: conversation list (240 px) | message thread | (optional
    assistant info panel)
  - `ConversationList` sub-component: shows title, provider badge, relative
    timestamp; "New conversation" button at the top; delete (×) per row
  - New conversation flow: clicking "New" opens a name input and provider
    selector, then creates via API and immediately selects it
  - Renaming: double-click conversation title in the list to edit inline
  - `MessageThread` sub-component: reuses existing bubble layout; shows sender
    label (You / assistant name); renders markdown in assistant messages (using
    `marked` from #52)
  - Provider selector: dropdown in the thread header; changing it calls
    `PUT /api/conversations/:id` and takes effect on the next message

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
4. **#50 + #50b** (task type/role dropdowns) — quick win, no dependencies
5. **#54 + #55 + #55b + #55c + #55d** (providers in DB)
6. **#56 + #57 + #57b + #58** (chat conversations)
7. **#48 + #49** (AI assistant sidebar) — depends on conversations (#56) and
   providers-in-DB (#54)
8. **#59 + #60 + #61** (polish) — any time, incrementally
