# Plan — 2026-05-18 — Git Workflow Integration Tests

Progress markers: `[x]` done · `[ ]` pending · `[~]` in progress

---

## Gap analysis

All existing tests are either unit tests or pure HTTP API tests with no real git objects.
`newTestServer` has no `storage.root`, so the embedded git HTTP handler never touches disk.
The `integration/` directory does not exist.

The scenarios below require a real TCP listener (not just `httptest.ResponseRecorder`) so that
go-git HTTP clients can connect to `http://127.0.0.1:{port}/git/...`.

---

## Isolation and cleanup strategy

Every test must be independently repeatable and must leave no side-effects.

### Resource ownership rules

| Resource | How created | How cleaned up |
|---|---|---|
| SQLite DB | `db.Open(t.TempDir() + "/test.db")` | `t.Cleanup(func() { d.Close() })` |
| Storage root (bare repos, worktrees) | `root := t.TempDir()` | automatic — `t.TempDir()` is removed after test |
| HTTP test server | `ts := httptest.NewServer(srv)` | `t.Cleanup(func() { ts.Close() })` |
| Agent clone dir | `cloneDir := t.TempDir()` | automatic |
| go-git in-memory objects | stack-allocated | GC |

`t.TempDir()` is the single mechanism for all filesystem resources. Go's test runner removes
each `TempDir` after the owning test returns, even on failure.

### Test data ownership rules

- **Each test creates all its own data** — no shared fixtures, no `TestMain` setup that populates rows.
- Factory helpers (`makeProject`, `makeTask`, `registerAgent`, etc.) accept `t *testing.T` and
  register no cleanup of their own — the DB and storage root are already owned by the test.
- Tests that exercise a multi-step flow (e.g. push → review → merge) build the prerequisite state
  inline at the top of the test, not by calling other tests. This makes each test readable in isolation.
- Tests **do not share** a server instance. Each `t.Run` / top-level test that needs git calls
  `newGitTestServer(t)`, which creates a fresh DB + storage root + TCP listener.

### Parallelism

All integration tests call `t.Parallel()` at the top. Because each test owns completely separate
filesystem paths and DB files there is no shared mutable state, so they can run concurrently.

---

## IT-0: Test infrastructure

### IT-0.1 [x] Core helpers — `helpers_test.go`

**What to build:**

```
integration/helpers_test.go
```

Contains the following helpers. Every helper registers its own cleanup via `t.Cleanup` so callers
need not think about teardown.

```go
// newGitTestServer spins up a full server with a real TCP listener and a
// temporary storage root + DB. Returns the base URL.
func newGitTestServer(t *testing.T) (baseURL string, database *db.Database)

// apiDo sends a JSON request to baseURL+path and returns the response.
func apiDo(t, method, baseURL, path string, body any) *http.Response

// apiJSON is apiDo but also JSON-decodes the response body into out.
func apiJSON(t, method, baseURL, path string, body, out any) int // returns status code
```

Cleanup registered inside `newGitTestServer`:
```go
ts := httptest.NewServer(srv)
t.Cleanup(func() { ts.Close() })
t.Cleanup(func() { database.Close() })
```

Storage root is `t.TempDir()` — no explicit cleanup needed.

### IT-0.2 [x] Data factory helpers — `factories_test.go`

**What to build:**

```
integration/factories_test.go
```

Small, composable factories. Each returns only the IDs / objects the test needs.
No cleanup registration — owned resources (DB, storage) are cleaned up by the
test's own `t.Cleanup` calls registered in `newGitTestServer`.

```go
// makeProject POSTs to /api/projects and returns projectID + slug.
func makeProject(t, baseURL, name string) (id, slug string)

// makeTask POSTs to /api/tasks and returns taskID.
func makeTask(t, baseURL, projectID string) string

// registerAgent POSTs to /api/agents/register and returns agentID.
func registerAgent(t, baseURL, name string, roles []string) string

// claimTask POSTs /api/tasks/{id}/claim and asserts 200.
func claimTask(t, baseURL, taskID, agentID string)

// cloneRepo uses go-git PlainClone to clone repoURL into a new t.TempDir().
// Returns the go-git Repository and the clone path.
func cloneRepo(t *testing.T, repoURL string) (*gogit.Repository, string)

// commitAndPush creates a file at relPath in clonePath, commits it, and pushes
// to the named branch. Returns the commit hash string.
func commitAndPush(t, clonePath, relPath, content, branch string) string
```

**Verify:** compile-only at this stage; exercised by IT-1+.

---

## IT-1: Project creation initializes bare git repo on disk

### IT-1.1 [x] `POST /api/projects` writes `{storage.root}/repos/{id}.git`

**File:** `integration/project_git_test.go`

**Setup (per test):**
```
newGitTestServer → fresh DB + storage root + TCP listener
```

**Steps:**
1. `makeProject(t, baseURL, "myproject")`
2. Assert HTTP 201
3. Read `storageRoot + "/repos/" + projectID + ".git/HEAD"` from disk
4. Assert file exists
5. Assert contents = `"ref: refs/heads/main\n"`

**Cleanup:** `t.TempDir()` removes storage root; `t.Cleanup` closes DB + server.

**What can go wrong:** project handler not wiring `storage.RepoPath` correctly;
`InitBare` not called from the handler. Test will fail with "no such file".

---

## IT-2: Clone via embedded HTTP

### IT-2.1 [x] Empty-repo clone succeeds

**File:** `integration/clone_test.go`

**Setup (per test):**
```
newGitTestServer → makeProject → note repoURL = baseURL+"/git/"+slug+".git"
cloneDir = t.TempDir()
```

**Steps:**
1. `go-git PlainClone(cloneDir, false, &CloneOptions{URL: repoURL})`
2. Assert no error (or `git.ErrEmptyRemoteRepository` — bare repos with no commits
   are a valid edge case; test should accept either success or that specific error
   and assert the `.git/` directory was created)
3. Assert `cloneDir+"/.git/config"` exists

**Cleanup:** `t.TempDir()` cleans clone dir automatically.

**Note:** An empty bare repo (no commits) may cause go-git to return
`transport.ErrEmptyRemoteRepository`. The test should handle this: the important
assertion is that the HTTP conversation reached the server (not a connection error)
and that the server's git handler responded. Check that a second clone after a
first commit succeeds unconditionally.

---

## IT-3: Dev push triggers state transition

### IT-3.1 [x] Push to `task/{id}` sets `branch_head_sha` and → AWAITING_REVIEW

**File:** `integration/push_test.go`

**Setup (per test):**
```
newGitTestServer
makeProject("push-test")
makeTask(projectID)
registerAgent("worker-1", roles=["worker"])
claimTask(taskID, workerAgentID)           // → DEVELOPING
repo, cloneDir = cloneRepo(repoURL)
```

**Steps:**
1. `commitAndPush(cloneDir, "main.go", "package main", "task/"+taskID)`
2. `GET /api/tasks/{taskID}` → assert `status = AWAITING_REVIEW`
3. Assert `branch_head_sha` in response equals the hash returned by `commitAndPush`
4. Open bare repo with go-git; assert `refs/heads/task/{taskID}` resolves to same hash

**Cleanup:** all dirs via `t.TempDir()`; server via `t.Cleanup`.

---

## IT-4: Review cycle

### IT-4.1 [x] Reviewer claims, posts `changes_requested` → AWAITING_REVISION with SHA

**File:** `integration/review_test.go`

**Setup (per test) — built inline, not reusing IT-3 state:**
```
newGitTestServer
makeProject / makeTask / registerAgent("worker") / claimTask
commitAndPush(... "task/"+taskID)          // → AWAITING_REVIEW
registerAgent("reviewer-1", roles=["reviewer"])
claimTask(taskID, reviewerAgentID)         // → REVIEWING
```

**Steps:**
1. `POST /api/tasks/{taskID}/reviews` with `status=changes_requested, body="fix error handling"`
2. Assert `status = AWAITING_REVISION`
3. `GET /api/tasks/{taskID}/reviews` → assert `reviews[0].branch_head_sha` equals commit hash from push

**Cleanup:** standard — all via `t.TempDir` / `t.Cleanup`.

### IT-4.2 [x] Second push after revision updates `branch_head_sha` → AWAITING_REVIEW again

**File:** `integration/review_test.go` (second test function)

**Setup (per test) — fully independent, reproduces full prior state inline:**
```
newGitTestServer
makeProject / makeTask / worker claims / first push → AWAITING_REVIEW
reviewer claims / posts changes_requested → AWAITING_REVISION
worker re-claims → DEVELOPING
```

**Steps:**
1. `commitAndPush(cloneDir, "main.go", "package main // v2", "task/"+taskID)` — second commit
2. `GET /api/tasks/{taskID}` → assert `status = AWAITING_REVIEW`
3. Assert new `branch_head_sha` differs from the first commit hash

**Cleanup:** standard.

---

## IT-5: Review feed — threaded comments

### IT-5.1 [x] Comments thread under a review; standalone comment appears top-level

**File:** `integration/feed_test.go`

**Setup (per test):**
```
newGitTestServer
makeProject / makeTask / worker claims
commitAndPush → AWAITING_REVIEW
reviewer claims / POST /reviews (changes_requested) → get reviewID
```

**Steps:**
1. `POST /api/tasks/{taskID}/comments` with `review_id=reviewID, body="reply 1"`
2. `POST /api/tasks/{taskID}/comments` with `review_id=reviewID, body="reply 2"`
3. `POST /api/tasks/{taskID}/comments` with no `review_id`, `body="standalone"`
4. `GET /api/tasks/{taskID}/feed` → parse into `[]FeedItem`
5. Assert 4 items total (1 review + 2 threaded comments + 1 standalone)
6. Assert items are in ascending `created_at` order
7. Assert both reply items have `review_id == reviewID`
8. Assert standalone item has `review_id == ""`

**Cleanup:** standard.

---

## IT-6: Full lifecycle → COMPLETED, `main` branch advanced

### IT-6.1 [x] Dev push → approve → merge → COMPLETED; bare repo `main` has new file

**File:** `integration/merge_test.go`

**Setup (per test):**
```
newGitTestServer
makeProject / makeTask / worker claims
commitAndPush("app.go", "package app", "task/"+taskID) → AWAITING_REVIEW
reviewer claims / POST /reviews approved → AWAITING_MERGE
```

**Steps:**
1. Call `supervisor.TickOnce(ctx)` — releases the task for merge (no file-lock conflict)
2. Register merge agent, claim → MERGING
3. Merge agent calls `POST /api/tasks/{taskID}/merge-complete` (or equivalent) after
   merging `task/{id}` into `main` in the bare repo via go-git and pushing `main`
4. Assert `status = COMPLETED`
5. `cloneRepo(baseURL+"/git/"+slug+".git")` cloning `main`
6. Assert `cloneDir+"/app.go"` exists with content `"package app"`
7. Assert bare repo's `refs/heads/main` SHA ≠ zero hash

**What `TickOnce` is:** a new method on `MergeSupervisor`:
```go
// TickOnce runs one supervisor cycle synchronously. For testing only.
func (s *MergeSupervisor) TickOnce(ctx context.Context) error
```

**Cleanup:** standard.

---

## IT-7: Merge serialization — overlapping files

### IT-7.1 [x] Two tasks touching `shared.go` never merge simultaneously

**File:** `integration/merge_serial_test.go`

**Setup (per test) — all state built inline:**
```
newGitTestServer
makeProject
makeTask taskA / worker-A claims / pushes "shared.go" → AWAITING_REVIEW / approved → AWAITING_MERGE
makeTask taskB / worker-B claims / pushes "shared.go" → AWAITING_REVIEW / approved → AWAITING_MERGE
```

**Steps:**
1. `supervisor.TickOnce(ctx)` — assert exactly one task transitions to MERGING;
   assert the other stays AWAITING_MERGE
2. Record which task is first (call it `first`, `second`)
3. Merge agent completes `first` → COMPLETED
4. `supervisor.TickOnce(ctx)` — assert `second` now transitions to MERGING
5. Merge agent completes `second` → COMPLETED
6. Clone `main`; assert `shared.go` content reflects `second`'s commit (last writer wins)

**Key assertion:** between steps 1 and 3, fetch both task statuses and assert
`second.status == AWAITING_MERGE` (never MERGING at the same time as `first`).

**Cleanup:** standard.

---

## IT-8: Parallel merges — non-overlapping files

### IT-8.1 [x] Two tasks touching different files both reach MERGING simultaneously

**File:** `integration/merge_parallel_test.go`

**Setup (per test):**
```
newGitTestServer
makeProject
makeTask taskA / worker-A claims / pushes "a.go" → AWAITING_REVIEW / approved → AWAITING_MERGE
makeTask taskB / worker-B claims / pushes "b.go" → AWAITING_REVIEW / approved → AWAITING_MERGE
```

**Steps:**
1. `supervisor.TickOnce(ctx)` — assert both tasks transition to MERGING
2. Both merge agents complete in parallel (use `t.Parallel()` goroutines inside the test)
3. Assert both tasks → COMPLETED
4. Clone `main`; assert both `a.go` and `b.go` exist

**Cleanup:** standard.

---

## Summary: per-test resource checklist

Every test function must satisfy all of the following before its first assertion:

```
[ ] newGitTestServer called → registers Cleanup for server.Close() + db.Close()
[ ] all filesystem paths come from t.TempDir() (auto-cleaned)
[ ] all test data (projects, tasks, agents) created fresh within the test body
[ ] no reads from any resource created in a different test
```

---

## Sequencing

Build these in order — each task's verify step is a prerequisite for the next.

```
IT-0.1 helpers
IT-0.2 factories
IT-1.1 project → bare repo on disk
IT-2.1 clone via HTTP
IT-3.1 push → AWAITING_REVIEW + SHA
IT-4.1 review cycle (changes_requested)
IT-4.2 second push updates SHA
IT-5.1 feed threading           ← no git needed, can be done any time after IT-0
IT-6.1 full lifecycle → COMPLETED
IT-7.1 serial merge (overlapping files)
IT-8.1 parallel merge (non-overlapping files)
```

---

## Running the suite

```bash
# Run all integration tests
go test -tags=integration -v -count=1 ./integration/...

# Run a single scenario
go test -tags=integration -v -run TestPushTransitionsToAwaitingReview ./integration/...

# Run with race detector (safe because tests are fully isolated)
go test -tags=integration -race ./integration/...
```

`-count=1` bypasses the test cache, ensuring cleanup correctness is verified on every run.
`-race` is cheap here because all shared state lives in separate goroutines per test.

---

## Files to create / modify

| File | Status |
|------|--------|
| `agent-orchestrator/integration/helpers_test.go` | new |
| `agent-orchestrator/integration/factories_test.go` | new |
| `agent-orchestrator/integration/project_git_test.go` | new |
| `agent-orchestrator/integration/clone_test.go` | new |
| `agent-orchestrator/integration/push_test.go` | new |
| `agent-orchestrator/integration/review_test.go` | new |
| `agent-orchestrator/integration/feed_test.go` | new |
| `agent-orchestrator/integration/merge_test.go` | new |
| `agent-orchestrator/integration/merge_serial_test.go` | new |
| `agent-orchestrator/integration/merge_parallel_test.go` | new |
| `agent-orchestrator/workflow/merge_supervisor.go` | add `TickOnce(ctx)` |
| `agent-orchestrator/config/config.go` | confirm `Storage.Root` flows to git handler |
