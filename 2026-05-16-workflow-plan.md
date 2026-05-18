# Plan — 2026-05-16 — Agentic Development Lifecycle (workflow)

Progress markers: `[x]` done · `[ ]` pending · `[~]` in progress

Sequel to `2026-05-16-plan.md`. That plan ships first; this one builds on top.

---

## Source intent (from `project_git_agent_workflow.md`)

The platform manages projects through a real dev lifecycle: planning → development → review (with code suggestions, not just pass/fail) → revision loop → merge (with file-overlap parallelism) → completed. Server hosts a bare git repo per project as the source of truth; an optional `upstream` remote is treated as backup. Agents either worktree off the server's repo (same machine) or clone over HTTP (remote machine). Server and agent are the same binary, different commands; they communicate over the existing REST/WS API.

## Locked-in decisions

- **Git transport**: embedded smart-HTTP in pure Go using `github.com/go-git/go-git/v5`. No `git` binary required on the host.
- **State machine**: wholesale replacement. New states: `BACKLOG → DEVELOPING → AWAITING_REVIEW → REVIEWING → AWAITING_REVISION → AWAITING_MERGE → MERGING → COMPLETED`. One-shot migration maps the existing `planned/queued/in_progress/needs_review/completed/failed` to the new vocabulary. `FAILED` is kept as a terminal alongside `COMPLETED`.
- **Merge parallelism**: diff-driven. Each task's changed-path set is the basis of a file-lock graph; non-overlapping tasks merge concurrently.
- **Plan relationship**: separate, sequential. The first plan ships first. Several first-plan items are reshaped by this work — see section X.

## Standing assumptions (call out if any are wrong)

- Storage root configurable as `storage.root` (default `./data`). Layout: `{root}/repos/{project_id}.git/` (bare), `{root}/worktrees/{task_id}/` (only when an agent runs colocated).
- Branch naming: `task/{task_id}`. Integration branch (for parallel-merge testing): `integration/{merge_batch_id}`.
- Agent mode is declared by flag: `--mode=colocated|remote`. Colocated agents additionally pass `--storage-root=...` so they can resolve worktree paths; remote agents pass `--server=https://…` and use HTTP git.
- Worktree lifecycle: created on transition into `DEVELOPING`/`REVIEWING`/`MERGING`, deleted on `COMPLETED`, retained on `FAILED` (configurable retention).
- Auth for the embedded git HTTP server is **out of scope for this plan** (assume trusted network or reverse proxy). Called out as a follow-up.
- Library choice: `go-git/v5` for repo ops (init bare, clone, fetch, merge, diff, push, smart-HTTP transport).

---

## W1. Server-managed git storage (foundation)

### W1.1 [x] Storage root + filesystem layout
Outline:
- Add `storage.root` to `config.go` (default `./data`).
- Add `storage.worktree_retention_failed_hours` (default 168 = 7 days).
- Helpers: `storage.RepoPath(projectID)`, `storage.WorktreePath(taskID)`. Single source of truth.
- Create directories on server boot (idempotent).

Files:
- `agent-orchestrator/config/config.go`, `config/defaults.go`, `config/config_test.go`
- `agent-orchestrator/storage/paths.go` (new)
- `agent-orchestrator/server/server.go` (boot-time directory ensure)
- `agent-orchestrator/config.yaml` (commented example)

Verify: server boots with custom `storage.root`; directories appear; paths_test asserts helpers.

### W1.2 [x] Bare repo per project (`git init --bare` via go-git)
Outline:
- New package `agent-orchestrator/git/` wrapping go-git.
- `git.InitBare(path)`: creates a bare repo with an empty `main` branch.
- On project create handler, after row insert: call `InitBare(storage.RepoPath(project.id))`. If a remote URL is supplied on the project, add it as `upstream`.
- Migration adds `projects.server_repo_initialised_at` column. Backfill is no-op (existing projects get a bare repo on first access).
- The existing `repo_path` field on projects is deprecated. It stays in the schema and is editable, but is now a hint for agents that want to bring their own checkout (used only when `mode=colocated` and the agent points at it). New canonical path is `storage.RepoPath(project.id)`.

Files:
- `agent-orchestrator/git/bare.go` (new)
- `agent-orchestrator/git/git_test.go` (new)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/db/projects.go` (new column + helper `EnsureBareRepo`)
- `agent-orchestrator/server/handlers.go` (project create path)
- `agent-orchestrator/go.mod`/`go.sum` (add go-git/v5)

Verify: create project via API; assert `{root}/repos/{id}.git/HEAD` exists and points at `main`.

### W1.3 [x] Optional `upstream` mirroring
Outline:
- Project optional fields `remote_url`, `remote_credentials_ref` (just a credential lookup key; secrets in env or platform_settings).
- On project create with `remote_url`: `git.AddRemote("upstream", url)`.
- On task transition to `COMPLETED`, the merge supervisor pushes the updated `main` to `upstream` if configured. Failure to push to upstream does NOT roll back the merge — it logs and continues; surfaced via a new `project_upstream_sync_failed` system log.
- On project create with `remote_url`, also offer an `initial_pull=true` flag — fetches from upstream and resets local `main` to upstream's `main`. (Lets users bootstrap from existing repos.)

Files:
- `agent-orchestrator/git/remote.go` (new — AddRemote, Fetch, Push)
- `agent-orchestrator/db/projects.go` (new columns)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/server/handlers.go` (project create + a `POST /projects/{id}/sync-upstream` button on demand)
- `agent-orchestrator/workflow/merge_supervisor.go` (push-on-complete)
- `agent-orchestrator/ui/src/pages/ProjectDetail.svelte` (remote URL field + "Sync upstream" button)

Verify: integration test creates a project with a local "upstream" path; completes a task; asserts upstream's `main` advances.

### W1.4 [x] Embedded smart-HTTP git server
Outline:
- New `git/httpserver.go` implementing the smart-HTTP protocol via go-git's `transport/server` and `transport/http`:
  - `GET /git/{project_slug}.git/info/refs?service=git-upload-pack` → advertisement for clone/fetch
  - `POST /git/{project_slug}.git/git-upload-pack` → upload-pack for clone/fetch
  - `GET /git/{project_slug}.git/info/refs?service=git-receive-pack` → advertisement for push
  - `POST /git/{project_slug}.git/git-receive-pack` → receive-pack for push
- `project_slug`: derive from project name (lowercased, dashed). Stored alongside `id` in the projects table.
- A receive-pack post-hook updates the task in SQLite: if the pushed branch matches `task/{task_id}` and the task is in `DEVELOPING`, flip it to `AWAITING_REVIEW`. Records the pushed commit SHA on the task.
- Out of scope for v1: auth, ACLs, LFS. Section "Follow-ups".

Files:
- `agent-orchestrator/git/httpserver.go` (new)
- `agent-orchestrator/git/hooks.go` (new — post-receive logic)
- `agent-orchestrator/server/server.go` (route mount)
- `agent-orchestrator/server/git_handler.go` (new — wires hooks to DB)
- `agent-orchestrator/db/projects.go` (`slug` column + uniqueness)
- `agent-orchestrator/db/tasks.go` (`branch_head_sha`, `last_push_at` columns)
- `agent-orchestrator/db/migrations.go`

Verify: integration test runs a real `git clone http://localhost:8080/git/foo.git`, commits, pushes, asserts the task transitions to `AWAITING_REVIEW` and the SHA is recorded.

---

## W2. Task state machine

### W2.1 [x] State enum + migration
Outline:
- New canonical states: `BACKLOG`, `DEVELOPING`, `AWAITING_REVIEW`, `REVIEWING`, `AWAITING_REVISION`, `AWAITING_MERGE`, `MERGING`, `COMPLETED`, `FAILED`.
- One-shot DB migration: `planned/queued → BACKLOG`, `in_progress → DEVELOPING`, `needs_review → AWAITING_REVIEW`, `completed → COMPLETED`, `failed → FAILED`. Records the mapping in a `state_migration_log` table for audit.
- Helper `db.IsQueueState(s)` and `db.IsExecutionState(s)`: queue = `BACKLOG, AWAITING_REVIEW, AWAITING_REVISION, AWAITING_MERGE`; execution = `DEVELOPING, REVIEWING, MERGING`; terminal = `COMPLETED, FAILED`.

Files:
- `agent-orchestrator/db/models.go` (state constants)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/db/tasks.go` (state filters, claim/release queries updated)
- `agent-orchestrator/api/types.go` (no aliasing — the API speaks the new vocabulary)

Verify: migration test inserts old-state rows, runs migration, asserts new states + log entries.

### W2.2 [x] Scheduler + claim path updated
Outline:
- The scheduler claims tasks by role + state: `dev` agents claim `BACKLOG` or `AWAITING_REVISION` → transition to `DEVELOPING`. `reviewer` agents claim `AWAITING_REVIEW` → `REVIEWING`. `merge` agents claim `AWAITING_MERGE` → `MERGING` (only after the merge supervisor releases the file-locks for the task — see W6).
- The existing claim transaction stays; only the state predicates change.

Files:
- `agent-orchestrator/workflow/scheduler.go`
- `agent-orchestrator/workflow/state.go`
- `agent-orchestrator/db/tasks.go` (`ClaimTask` filters)

Verify: unit tests for each role/state combination.

### W2.3 [x] UI vocabulary update
Outline:
- Replace every reference to the old states in Svelte files. `statusColors` map and dropdowns updated to the new states (with two new colours — `AWAITING_REVISION` and `AWAITING_MERGE`).
- TaskDetail status select shows the full new list.
- Tasks page filter dropdowns updated.
- Charts include the new state-transition events.

Files:
- `agent-orchestrator/ui/src/pages/Tasks.svelte`
- `agent-orchestrator/ui/src/pages/TaskDetail.svelte`
- `agent-orchestrator/ui/src/pages/ProjectDetail.svelte`
- `agent-orchestrator/ui/src/lib/api.js` (no logical change; refresh JSDoc)
- `agent-orchestrator/ui/src/__tests__/Tasks.test.js`, `Settings.test.js` (existing assertions on state strings)

Verify: the A6 interaction crawler (from plan 1) must pass; vitest updated.

---

## W3. Agent modes — colocated vs remote

### W3.1 [x] Agent mode flag + identity
Outline:
- `./agent-orchestrator agent --mode=colocated|remote --storage-root=… --server=…`.
- Agent registers with `mode`, `hostname`, `pid`. New column `agents.mode`. Server uses this to decide whether to assign worktree-creation to itself (colocated) or expect the agent to clone over HTTP (remote).

Files:
- `agent-orchestrator/cmd/main.go`
- `agent-orchestrator/agent/agent.go`
- `agent-orchestrator/db/agents.go`
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/api/types.go` (registration request)

Verify: register a colocated and a remote agent; assert mode persisted.

### W3.2 [x] Colocated worktree provisioning (server-side)
Outline:
- When a colocated agent claims a task moving into an execution state, the server creates the worktree at `{root}/worktrees/{task_id}/` pointed at the right branch:
  - `DEVELOPING` from `BACKLOG`: branch `task/{id}` is created from `main`.
  - `DEVELOPING` from `AWAITING_REVISION`: worktree on the existing `task/{id}` branch (which was previously pushed).
  - `REVIEWING`: worktree on the pushed `task/{id}` branch.
  - `MERGING`: worktree on an integration branch derived from `main`.
- The claim response includes `worktree_path`. Agent `cd`s there.

Files:
- `agent-orchestrator/git/worktree.go` (new — wraps go-git's worktree create/remove)
- `agent-orchestrator/workflow/scheduler.go` (provision on claim)
- `agent-orchestrator/db/tasks.go` (`worktree_path` column)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/api/types.go` (claim response)

Verify: claim a task, assert worktree exists and is on the expected branch.

### W3.3 [x] Remote agent clone/fetch via embedded git HTTP
Outline:
- Remote agent receives `repo_url` + `branch` on claim instead of `worktree_path`.
- Agent uses go-git over HTTP to `Clone --depth 1` into a temp directory derived from `os.TempDir() / "agent-{id}/task-{taskID}"`. Subsequent revisions on the same task fetch + checkout rather than re-cloning.
- On task completion (success or failure), agent removes its temp directory unless `--keep-on-fail` is set.

Files:
- `agent-orchestrator/agent/repo.go` (new — clone, fetch, checkout, commit, push)
- `agent-orchestrator/agent/executor.go` (use repo.go regardless of mode; mode just changes the URL/path)
- `agent-orchestrator/cmd/main.go` (flags)

Verify: e2e starts server + remote agent, claims a task, asserts the clone happens via `/git/...` and the temp dir is created.

### W3.4 [x] Test-server port assignment for agents
Outline:
- Server keeps a free-port pool (config: `agents.port_pool_start`, default 18000, plus `agents.port_pool_size`, default 100).
- On claim, the server allocates a port from the pool and returns it as `assigned_port` in the claim response. Released on transition out of an execution state.
- Agent passes the port to test/build commands via env (`PORT={assigned}`). Existing test/build tool calls in `tools/code.go` honour it.

Files:
- `agent-orchestrator/workflow/portpool.go` (new)
- `agent-orchestrator/workflow/scheduler.go` (allocate/release)
- `agent-orchestrator/db/tasks.go` (`assigned_port` column, nullable)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/tools/code.go` (env passing)
- `agent-orchestrator/api/types.go`

Verify: claim two tasks simultaneously; assert distinct ports allocated; release one; reclaim returns the freed port.

### W3.5 [x] `.agent_context/` per task
Outline:
- On worktree/clone setup, populate `.agent_context/` in the work directory:
  - `last_review.md` — body of the latest review if state is `DEVELOPING` from revision; else absent.
  - `review_thread.md` — flat dump of comments threaded under that review (chronological), if any.
  - `project_rules.md` — project description + any coding-rules field.
  - `task.md` — task title/description.
  - `merge_history.log` — past merge attempts on this branch.
- `.agent_context/` is added to `.git/info/exclude` so it never accidentally gets committed.

Files:
- `agent-orchestrator/agent/context.go` (new — writer)
- `agent-orchestrator/agent/executor.go` (call on task pickup)
- `agent-orchestrator/git/worktree.go` (excludes)
- `agent-orchestrator/db/projects.go` (`coding_rules` column, optional)

Verify: claim a task; assert `.agent_context/task.md` exists; assert `git status` is clean.

---

## W4. Dev cycle — commit, push, transition to AWAITING_REVIEW

### W4.1 [x] Branch creation + commit + push
Outline:
- Agent's executor, after performing work on the worktree/clone:
  1. `git add -A` (via go-git Worktree.Add).
  2. `git commit -m "{task title} ({task_id})"` with author/email = agent identity.
  3. Push the branch (`task/{id}`) — for colocated, to the local bare repo path; for remote, to the server's HTTP endpoint.
- After push, the agent posts `POST /tasks/{id}/submit-for-review` which is a metadata-only confirmation. The state transition is driven by the post-receive hook (W1.4); this endpoint just acknowledges and lets the agent release its claim cleanly.

Files:
- `agent-orchestrator/agent/repo.go` (commit, push)
- `agent-orchestrator/agent/executor.go` (orchestrate)
- `agent-orchestrator/server/handlers.go` (submit-for-review endpoint)
- `agent-orchestrator/api/types.go`

Verify: integration test runs an agent through a fake task; asserts a branch exists on the bare repo at the expected SHA and the task is in `AWAITING_REVIEW`.

### W4.2 [x] Pre-submit gates (tests + lint + build) tied to task checklist
Outline:
- Reuses A1.2 (per-task checklist) from plan 1. Before submit, executor walks the checklist items marked as required-pre-submit; if any are `failed`, the agent does NOT push and instead emits `task_pre_submit_failed` log + holds the task. (Surfaced to the user via the existing event panel.)
- Default required items: `tests_pass`, `lint_pass`, `build_pass`. These are emitted by existing `tools/code.go` runs.

Files:
- `agent-orchestrator/agent/executor.go`
- `agent-orchestrator/db/task_checklist.go` (from plan 1)
- `agent-orchestrator/db/task_logs.go` (new event type)

Verify: integration test where lint fails; assert no push, task stays in `DEVELOPING`, event recorded.

---

## W5. Review-Fix loop

### W5.1 [x] Reviewer role + system prompt
Outline:
- New `roles` row `reviewer` (already partially supported by the codebase — verify routing).
- New default reviewer prompt in `config.yaml` and `prompts/reviewer.txt`. Prompt instructs the agent to produce a structured manifest (not freeform).

Files:
- `agent-orchestrator/config.yaml` (new prompt)
- `agent-orchestrator/db/roles.go` (seed `reviewer`)
- `agent-orchestrator/db/migrations.go`

Verify: starting the server seeds the reviewer role row.

### W5.2 [x] Review schema + storage
Outline:
- Reviews are freeform markdown — no structured `suggestions_json`. Code suggestions, severity, file/line references all express naturally inside markdown (fenced diff blocks, inline references, headings). Agents and humans write the same shape.
- New table `task_reviews(id, task_id, author_type, author_role, author_id, status, body, branch_head_sha, created_at)`.
  - `author_type` ∈ {`user`, `agent`}; `author_role` is the agent role (`reviewer`, …) or `null` for users.
  - `status`: `APPROVED` | `REVISION_REQUESTED`. Always set — every review is a formal evaluation that drives a state transition. Informal back-and-forth lives in `task_comments` threaded under the review (plan 1 A1.3).
  - `body`: markdown.
  - `branch_head_sha`: the commit SHA the review was made against; surfaces in the UI as "Reviewed at abc1234".
- Authoring permissions: a `POST /tasks/{id}/review` is accepted from any user, or from an agent whose role is `reviewer`. Other agent roles must reply via `task_comments` (W5.4).
- API:
  - `POST /tasks/{id}/review` — creates a `task_reviews` row; server flips state: `APPROVED` → `AWAITING_MERGE`, `REVISION_REQUESTED` → `AWAITING_REVISION`.
  - `GET /tasks/{id}/reviews` — newest-first list.

Files:
- `agent-orchestrator/db/task_reviews.go` (new)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/server/handlers.go`
- `agent-orchestrator/api/types.go`
- `agent-orchestrator/workflow/state.go` (transition rules)

Verify: post a review as a user; assert state changes. Post a second review on the same task (after a new push); assert it appends as a new row and the state changes again. Attempt to post a review from a `dev`-role agent; assert 403.

### W5.3 [x] Reviewer agent execution
Outline:
- The reviewer claims a task in `AWAITING_REVIEW`, worktree/clones onto the task's branch, runs the project's test command, reads diffs against `main`, and writes a markdown review.
- LLM tool `post_review(status, body)`: posts the review via the API. The reviewer system prompt instructs the LLM to embed code suggestions as fenced markdown blocks with file paths in headings, and to mark severity inline (e.g. `**blocker:**`, `**nit:**`).
- A second tool `post_task_comment(body, review_id?)` is available to the reviewer (and to other roles via W5.4) so reviewers can ask follow-up questions or clarify their review without creating a new state transition.

Files:
- `agent-orchestrator/tools/review.go` (new — `post_review`)
- `agent-orchestrator/tools/task_comment.go` (from plan 1 A1.3; verify the `review_id` arg is wired)
- `agent-orchestrator/tools/executor.go` (register)
- `agent-orchestrator/agent/executor.go` (reviewer branch)
- `agent-orchestrator/config.yaml` (reviewer prompt — markdown output, severity convention)

Verify: e2e — agent in reviewer role processes a task and posts a markdown review containing at least one fenced code suggestion; `status=REVISION_REQUESTED`; state transitions.

### W5.4 [x] Dev pickup of AWAITING_REVISION + dev↔reviewer discussion
Outline:
- When a dev agent claims a task in `AWAITING_REVISION`, the server includes the latest review (markdown body + status + branch_head_sha) and any threaded replies in the claim response. `context.go` (W3.5) writes `.agent_context/last_review.md` (the full review body) and `.agent_context/review_thread.md` (the reply chain).
- Executor's prompt builder appends the review body and instructs the LLM to address each point inline. If the dev agent disagrees with a point, it must either propose an alternative in code OR post a reply comment via `post_task_comment(body, review_id=…)` explaining why. The reviewer can then respond by posting another comment (also tied to `review_id`). This is the discussion loop.
- Authoring permissions for replies: `task_comments` are open to any role (per plan 1 A1.3). The use case the user called out — devs answering reviews — is satisfied by the dev agent's standard tool access; no new role gate is needed.

Files:
- `agent-orchestrator/agent/context.go`
- `agent-orchestrator/agent/executor.go`
- `agent-orchestrator/api/types.go`

Verify: integration test where reviewer requests a revision; dev picks up; resulting commit message references the `review_id`; dev posts a reply comment disagreeing with one point; reviewer (in a follow-up round) sees the reply in the thread.

### W5.5 [x] Unified task feed in the UI
Outline:
- TaskDetail renders one chronological "Activity" section combining reviews and comments. No separate "Reviews tab".
- New endpoint `GET /api/tasks/{id}/feed` returning rows from `task_reviews` and `task_comments` ordered by `created_at`, each carrying a `kind: 'review' | 'comment'` discriminator. Implementation: `SELECT … FROM task_reviews UNION ALL SELECT … FROM task_comments ORDER BY created_at`. Items may be filtered (`?since=…`) and paginated.
- New component `TaskFeed.svelte`:
  - For `kind='review'`: prominent card with status badge (`APPROVED` / `REVISION_REQUESTED`), author label, "Reviewed at {sha}" line, markdown-rendered body, and a collapsible inline thread of comments whose `review_id` matches.
  - For `kind='comment'` with `review_id=null`: plain markdown bubble (same style as a chat message), labelled by author.
  - Reply box appears under each review for posting a comment with `review_id` pre-filled. Top-level "Add comment" box at the bottom for un-threaded comments.
  - Markdown rendering uses the existing `marked` + `DOMPurify` pipeline from `Chat.svelte`.
- The component also serves as the rendering target for plan 1 A1.3 — there is no second comment component.

Files:
- `agent-orchestrator/server/feed_handler.go` (new — the union query + handler)
- `agent-orchestrator/server/server.go` (route)
- `agent-orchestrator/api/types.go` (`FeedItem` type with kind discriminator)
- `agent-orchestrator/ui/src/components/TaskFeed.svelte` (new)
- `agent-orchestrator/ui/src/lib/api.js` (`getTaskFeed`, `postTaskReview`, `postTaskComment`)
- `agent-orchestrator/ui/src/pages/TaskDetail.svelte` (mount `TaskFeed`)

Verify: e2e — task has 1 review with `REVISION_REQUESTED` and 2 replies under it, plus 1 standalone comment. Feed renders in correct chronological order; replies are visually grouped under the review; posting a reply via the UI shows up in the next refresh; posting a standalone comment appears as a top-level bubble.

---

## W6. Merge orchestration

### W6.1 [x] Merge supervisor (server-side background goroutine)
Outline:
- Single goroutine, started by `server.Start`. Polls tasks in `AWAITING_MERGE` every N seconds (config `agents.merge_supervisor_interval_sec`, default 10).
- For each candidate, computes its changed-paths set via `git diff main...task/{id} --name-only` (go-git tree walking).
- Maintains an in-memory file-lock set: paths currently being merged by an in-flight merge agent. A candidate is releasable when none of its paths intersect the lock set.
- On release, the supervisor flips the task to `AWAITING_MERGE_RELEASED` (internal sub-state) so a merge agent can claim it; alternatively, marks a `merge_claim_token` on the task that the claim API checks. Choose during impl based on what's simpler.

Files:
- `agent-orchestrator/workflow/merge_supervisor.go` (new)
- `agent-orchestrator/workflow/scheduler.go` (merge-agent claim respects the supervisor)
- `agent-orchestrator/git/diff.go` (new — changed-paths helper)
- `agent-orchestrator/db/merge_queue.go` (new — persisted lock table for crash recovery)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/config/config.go` (interval setting)

Verify: unit tests for the path-overlap algorithm; integration test where two non-overlapping tasks merge in parallel and two overlapping tasks merge serially.

### W6.2 [x] Merge agent role + workflow
Outline:
- Merge agent claims a released task → state `MERGING`.
- Steps:
  1. Create integration worktree at `{root}/worktrees/{task_id}/` on a fresh branch off latest `main`.
  2. `git merge task/{id}`. If conflicts:
     - For text-only conflicts (no `<<<<<<<` markers leaking through the LLM's interpretation), the merge tool offers an LLM resolution. Tool: `resolve_merge_conflict(file_path, conflict_blob) -> resolved_blob`.
     - For complex conflicts (more than `N` files or any structural conflict the tool decides it can't handle), bail.
  3. Run the project's test suite. The same checklist items from W4.2 must pass on the merged state.
  4. Push `main` to the bare repo. Post-receive hook on `main` triggers transition to `COMPLETED`.
- On any failure (conflict not auto-resolvable, tests fail), the supervisor moves the task to `AWAITING_REVISION` and writes a synthetic `task_reviews` row with `status=REVISION_REQUESTED`, `critique="Merge failed: …"`, and a suggestion containing the merge-fail log.

Files:
- `agent-orchestrator/tools/merge.go` (new)
- `agent-orchestrator/tools/executor.go`
- `agent-orchestrator/agent/executor.go` (merge branch)
- `agent-orchestrator/workflow/merge_supervisor.go` (safety-bounce wiring)
- `agent-orchestrator/db/task_reviews.go` (synthetic-review helper)

Verify: integration test forces a conflict; assert task lands in `AWAITING_REVISION` with a synthetic review attached.

### W6.3 [x] Upstream push on COMPLETED
Outline: hooks W1.3 into the post-`COMPLETED` step. If `remote_url` configured, push `main` (and tags if applicable) to `upstream` in a goroutine; log failure but do not roll back.

Files:
- `agent-orchestrator/workflow/merge_supervisor.go`
- `agent-orchestrator/git/remote.go`

Verify: covered by W1.3's integration test extended to assert the upstream branch advances.

### W6.4 [x] UI: merge queue view
Outline:
- ProjectDetail.svelte: new collapsible "Merge queue" section listing tasks in `AWAITING_MERGE` / `MERGING`, with their changed-path summary and lock status.
- TaskDetail.svelte: shows merge-attempt history (from the synthetic reviews + `task_logs`).

Files:
- `agent-orchestrator/ui/src/pages/ProjectDetail.svelte`
- `agent-orchestrator/ui/src/pages/TaskDetail.svelte`
- `agent-orchestrator/ui/src/lib/api.js` (`getMergeQueue(projectId)`)
- `agent-orchestrator/server/handlers.go` (new endpoint)

Verify: e2e visits a project with two AWAITING_MERGE tasks; asserts both appear; one shows `merging`, one shows `queued`.

---

## W7. Visibility — lifecycle history, charts, events

### W7.1 [x] Persisted state-transition history
Outline:
- New table `task_state_transitions(id, task_id, from_state, to_state, actor_agent_id, reason, created_at)`.
- Every state transition (scheduler, hooks, supervisor) writes a row.
- TaskDetail renders these as a timeline.

Files:
- `agent-orchestrator/db/task_state_transitions.go` (new)
- `agent-orchestrator/db/migrations.go`
- `agent-orchestrator/db/tasks.go` (helper to emit a transition row alongside the existing task event)
- `agent-orchestrator/ui/src/pages/TaskDetail.svelte`

Verify: a task's full lifecycle yields ≥ 6 transition rows; rendered in order.

### W7.2 [x] New event types for charts
Outline:
- Extend `TASK_COLORS` in Tasks.svelte and the chart legend with:
  `task_submitted_for_review`, `task_review_posted`, `task_revision_started`, `task_merge_started`, `task_merge_failed`, `task_pushed_upstream`.

Files:
- `agent-orchestrator/db/task_logs.go` (constants)
- `agent-orchestrator/ui/src/pages/Tasks.svelte`

Verify: charts render new colours; A6 crawler still passes.

---

## W8. Testing

### W8.1 [x] Go unit tests for new packages
- `git/bare_test.go`, `git/worktree_test.go`, `git/diff_test.go`, `git/remote_test.go`, `git/httpserver_test.go`
- `workflow/merge_supervisor_test.go`, `workflow/portpool_test.go`
- `db/task_reviews_test.go`, `db/merge_queue_test.go`, `db/task_state_transitions_test.go`

Verify: `task test:server` green.

### W8.2 [x] Integration test — full lifecycle
Outline:
- One Go integration test (build-tag `integration`) that:
  1. Boots the server with a tmp `storage.root`.
  2. Creates a project; asserts bare repo exists.
  3. Starts a colocated dev agent (in-process), claims a `BACKLOG` task, makes a code change, commits, pushes; assert state → `AWAITING_REVIEW`.
  4. Starts a reviewer agent; posts a `REVISION_REQUESTED` manifest with one suggestion; assert state → `AWAITING_REVISION`.
  5. Dev agent picks up again; addresses the suggestion; pushes; assert `AWAITING_REVIEW`.
  6. Reviewer approves; state → `AWAITING_MERGE`.
  7. Merge agent runs; state → `COMPLETED`; bare repo's `main` advanced.

Files:
- `agent-orchestrator/integration/lifecycle_test.go` (new — gated by build tag)

Verify: `go test -tags=integration ./integration/...` passes.

### W8.3 [x] Playwright e2e additions
Outline:
- Extends plan 1's A6 suite:
  - `e2e/features/workflow-states.spec.js`: drive a project through every state via UI controls + simulated agents (the integration backend started in test mode).
  - `e2e/features/review-manifest.spec.js`: render a manifest with mixed severities; assert visuals.
  - `e2e/features/merge-queue.spec.js`: two parallel + two serial merges.
  - `e2e/regressions/old-states-removed.spec.js`: no occurrence of `planned|queued|in_progress|needs_review` in DOM after migration.
  - The interaction crawler fixture from A6 is regenerated for the new state vocabulary (`npm run e2e:update-fixture`).

Files:
- `agent-orchestrator/ui/tests/e2e/features/**` (new files above)
- `agent-orchestrator/ui/tests/e2e/regressions/old-states-removed.spec.js` (new)
- `agent-orchestrator/ui/tests/e2e/_crawler/expected-interactions.json` (regenerated)

Verify: `task test:e2e` green.

---

## W9. Documentation & config example

- [x] Update `SPEC.md` with the ADL (state machine, review manifest schema, merge logic, git transports).
- [x] Update `config.yaml` with commented `storage:`, `remote_url:` (project-level), `agents.port_pool_start`, `agents.merge_supervisor_interval_sec`, `agents.worktree_retention_failed_hours`.
- [x] Update `IMPLEMENTATION_ROADMAP.md` to reference both plan files in order.

Files:
- `SPEC.md`, `IMPLEMENTATION_ROADMAP.md`, `agent-orchestrator/config.yaml`

Verify: docs read coherently end-to-end; reviewer (you) signs off.

---

## X. Items in `2026-05-16-plan.md` reshaped by this work

These don't need to be redone; this plan replaces or extends them. Flagging so plan 1 isn't done twice.

- **A1.1 task dependencies** — semantics unchanged (soft warning). The new file-overlap merge-locks are orthogonal; both can coexist (deps = workflow ordering, locks = merge ordering).
- **A1.2 task checklist + iteration groups** — the new state machine already encodes iteration (every `AWAITING_REVIEW → AWAITING_REVISION → DEVELOPING` cycle). Recommend: keep the per-task editable checklist for fine-grained pre-submit gates (W4.2 uses it); deprecate "iteration groups" UI in favour of the state-transition timeline from W7.1.
- **A1.3 task comments** — keep, with an added nullable `review_id` FK so comments can thread under a review. Comments and reviews are both freeform markdown; they're rendered together in the unified feed (W5.5). The only structural differences are: reviews carry a `status` and drive state transitions; reviews are authored by users or `reviewer`-role agents only, while comments are open to any role. Discussion under a review (devs answering reviewer feedback, reviewer following up) happens as threaded comments.
- **A1.4 log additional task events** — extended in W7.2; the union of event types is the source of truth.
- **A2 task LLM panel** — still useful; this is where humans answer agent questions during `AWAITING_REVISION` etc.
- **A5 platform settings** — add the new settings introduced here (`storage.root`, `agents.merge_supervisor_interval_sec`, `agents.port_pool_start`, `agents.worktree_retention_failed_hours`) to the same Platform section.
- **A6 Playwright** — fixture regenerated (W8.3); same test layout, more specs.

## Y. Out of scope for this plan (follow-ups)

- Auth/authz for the embedded git HTTP server (token, mTLS, or reverse-proxy off-load).
- LFS / large-file support.
- Cross-region replication.
- Kubernetes deployment recipes (architecture supports it; manifests are not in this plan).
- Real-time collaboration on a worktree (two agents in the same worktree).

---

## Z. Suggested execution order

1. **W1** (storage + bare repos + smart-HTTP) — nothing else works without it.
2. **W2** (state machine) — touches every page; do it once, atomically.
3. **W3** (agent modes) — unlocks remote agents and validates W1.4.
4. **W4** (dev push → AWAITING_REVIEW) — the first end-to-end slice.
5. **W5** (review loop) — closes the dev↔review cycle.
6. **W6** (merge orchestration) — completes the lifecycle.
7. **W7** (visibility) — UI polish.
8. **W8** (tests) — interleaved with each phase, then exhaustively at the end.
9. **W9** (docs) — last.
