# Test Review & Coverage Plan — 2026-06-14

Goal: (A) audit every existing test for actual usefulness, and (B) fill gaps so **all files reach ≥80% coverage** where feasible.

Progress markers: `[x]` = done · `[ ]` = pending · `[~]` = in progress

Baseline coverage is recorded in `tasks_2026-06-14.md`. Run commands:
- Backend: `cd agent-orchestrator && go test -count=1 -cover ./...`
- Frontend: `cd agent-orchestrator/ui && pnpm test`

---

## Task summary

### Phase A — Review existing tests for usefulness
- [ ] A1 — Audit backend `agent` + `tools` tests
- [ ] A2 — Audit backend `db` tests
- [ ] A3 — Audit backend `server` + `router` tests
- [ ] A4 — Audit backend `git`, `llm`, `workflow`, `storage`, `logging` tests
- [ ] A5 — Audit `integration` tests
- [ ] A6 — Audit frontend component/page/lib tests
- [ ] A7 — Consolidate audit findings, delete/fix weak tests

### Phase B — Fill coverage gaps to ≥80%
- [~] B1 — `server` package (41.1% → 53.1%, paused; remainder is integration-bound — see test_quality_audit_2026-06-14.md)
- [ ] B2 — root/`main` (6.1% → 80%)
- [x] B3 — `db` package (60.3% → **82.0%**, done) — 6 batches; also surfaced/fixed the `CloneChecklistIteration` deadlock
- [x] B4 — `llm` package (60.2% → **82.5%**, done) — 2 batches (openai.go httptest suite; ollama stream/embed, registry, resilience wrappers)
- [ ] B5 — `workflow` package (61.5% → 80%)
- [ ] B6 — `agent` package (65.3% → 80%)
- [ ] B7 — `git` package (69.9% → 80%)
- [ ] B8 — `tools` package (72.4% → 80%)
- [ ] B9 — `storage` (79.8% → 80%) + top-ups: `config`, `logging`, `branchname`
- [ ] B10 — Frontend 0%-coverage files
- [ ] B11 — Frontend low-coverage pages
- [ ] B12 — Frontend low-coverage components + libs
- [ ] B13 — Final verification: full suite + coverage gate

---

## Phase A — Review existing tests for usefulness

For each test reviewed, classify it as **keep**, **fix**, or **delete**, against these criteria:

- Does it assert real behavior, or just that a function returns no error / a mock was called?
- Is the assertion tautological (asserting the value you just set, or re-implementing the function under test)?
- Does it test through real code paths, or is so much mocked that nothing real runs?
- Does it cover meaningful branches (errors, edge cases), not just the happy path?
- Is it flaky (time/sleep-dependent, ordering-dependent, real network)?
- Is it redundant with another test?

Record findings inline in the test files as `// REVIEW: ...` notes during the pass, then act on them in A7.

### [ ] A1 — Audit `agent` + `tools` tests
Outline: read each test, classify, note weak/tautological/over-mocked cases. Pay attention to the resilience/circuit-breaker tests (often assert mock call counts only) and executor internal tests.
Files: `agent/*_test.go` (13 files), `tools/*_test.go` (12 files).

### [ ] A2 — Audit `db` tests
Outline: DB tests are prone to "insert then read back the same row" tautologies. Verify each asserts derived behavior (constraints, state transitions, routing, scoping), not just round-trips.
Files: `db/*_test.go` (24 files).

### [ ] A3 — Audit `server` + `router` tests
Outline: handler tests often assert only HTTP 200. Verify response bodies, error codes, and side effects are checked. `server` is the lowest-coverage package, so reviewing existing tests here also informs B1.
Files: `server/*_test.go` (26 files), `router/router_test.go`.

### [ ] A4 — Audit `git`, `llm`, `workflow`, `storage`, `logging` tests
Outline: git tests touch the filesystem — confirm they assert repo state, not just absence of error. llm tests should assert request shaping + retry/circuit behavior, not just mock returns. workflow supervisor tests are timing-sensitive — flag any sleeps/races.
Files: `git/*_test.go` (11), `llm/*_test.go` (8), `workflow/*_test.go` (7), `storage/*_test.go` (2), `logging/*_test.go` (3).

### [ ] A5 — Audit `integration` tests
Outline: integration package reports "no statements" (coverage measured elsewhere). Confirm these exercise full flows end-to-end and assert observable outcomes; flag any that silently skip.
Files: `integration/*_test.go` (19 files).

### [ ] A6 — Audit frontend tests
Outline: Svelte tests often render and assert "component exists" without exercising events/stores/API calls. Verify each asserts rendered output changes in response to user interaction and that API/ws mocks return realistic shapes. Check the 31 skipped tests — decide fix or remove.
Files: `ui/src/__tests__/*.test.js` (21 files).

### [ ] A7 — Consolidate findings and act
Outline: collect all `// REVIEW:` notes into a short findings section appended to this file; delete redundant/tautological tests, fix assertion-light ones. Re-run both suites; confirm green and that coverage did not drop from deletions (replace coverage lost to deleted tests in Phase B).
Files: any flagged in A1–A6.

---

## Phase B — Fill coverage gaps to ≥80%

Priority order is lowest-coverage-first. For each package, write tests for the listed source files that currently have **no dedicated test** or weak coverage. Aim for happy-path + at least one error/edge branch per exported function.

### [ ] B1 — `server` (41.1% → 80%) — highest priority
Outline: most handlers have no dedicated test. Add table-driven HTTP handler tests (request → status + body + side effect) using the existing test server/helpers.
Source files lacking dedicated tests:
- `server/agent_templates_handler.go`
- `server/agentcontext.go`
- `server/checklist_handler.go`
- `server/conversations_handler.go`
- `server/llm_handler.go`
- `server/meta.go`
- `server/project_chat_handler.go`
- `server/requirements_handler.go`
- `server/reviews.go`
- `server/roles_handler.go`
- `server/settings_handler.go`
- `server/static.go`
- `server/task_chat_handler.go`
- `server/server.go` (wiring/route registration smoke test)

### [ ] B2 — root / `main` (6.1% → 80%)
Outline: extract testable logic out of `main()` if needed (flag parsing, config load, server bootstrap) and test it; keep `main()` thin. Confirm `cmd/main.go` vs `main.go` responsibilities.
Files: `main.go`, `main_test.go`, possibly `cmd/main.go`.

### [ ] B3 — `db` (60.3% → 80%)
Outline: add tests for the data-access files below — assert constraints, ordering, scoping, and error paths, not just round-trips.
Source files lacking dedicated tests:
- `db/agent_logs.go`
- `db/checklist_templates.go`
- `db/context.go`
- `db/conversations.go`
- `db/project_features.go`
- `db/project_requirements.go`
- `db/projects.go`
- `db/providers.go`
- `db/task_checklist.go`
- `db/task_comments.go`
- `db/task_dependencies.go`
- `db/task_logs.go`
- `db/task_project_links.go`
- `db/migrations.go` (apply-from-empty + idempotency test)

### [ ] B4 — `llm` (60.2% → 80%)
Outline: add `openai_test.go` (request shaping, streaming/parse, error mapping). Extend provider/retry/circuit-breaker coverage for failure branches.
Files: `llm/openai.go` (no test), `llm/provider.go`, `llm/retry.go`, `llm/circuit_breaker.go`.

### [ ] B5 — `workflow` (61.5% → 80%)
Outline: supervisors have tests but low branch coverage. Add cases for cancellation, error propagation, retention pruning edges, port-pool exhaustion. Avoid sleeps — use injectable clocks/channels.
Files: `workflow/agent_supervisor.go`, `merge_supervisor.go`, `queue_supervisor.go`, `scheduler.go`, `retention.go`, `portpool.go`, `state.go`.

### [ ] B6 — `agent` (65.3% → 80%)
Outline: add tests for currently untested source.
Files lacking dedicated tests: `agent/git_tls.go`, `agent/subagent.go`. Extend `executor.go`/`resilience.go` error-branch coverage.

### [ ] B7 — `git` (69.9% → 80%)
Outline: add `bare.go` test (init/clone bare repo) and cover error branches in merge/squash/remote.
Files: `git/bare.go` (no test), `git/merge.go`, `git/squash.go`, `git/remote.go`.

### [ ] B8 — `tools` (72.4% → 80%)
Outline: add tests for untested source files.
Files lacking dedicated tests: `tools/capabilities.go`, `tools/executor.go`, `tools/task_comment.go`.

### [ ] B9 — `storage` + top-ups
Outline: `storage` (79.8%) needs a small top-up on `paths.go`/`context.go` error branches. Optionally lift `config` (86.5%), `logging` (84.0%), `branchname` (90.0%) — already above target, only touch if A-phase deletions drop them.
Files: `storage/paths.go`, `storage/context.go`.

### [ ] B10 — Frontend 0%-coverage files
Outline: write tests from scratch for components/pages with no coverage. `ProjectDetail.svelte` (~1286 lines) is large — split into focused test files by feature area.
Files:
- `ui/src/components/AssistantSidebar.svelte`
- `ui/src/components/CodeEditor.svelte`
- `ui/src/components/DiffViewer.svelte`
- `ui/src/pages/ProjectDetail.svelte`
- `ui/src/pages/Skills.svelte`
- `ui/src/main.js` (smoke/mount test)

### [ ] B11 — Frontend low-coverage pages
Outline: extend existing page tests to drive interactions (filters, form submit, error states, ws updates) until each hits ≥80% stmts.
Files (current → target 80):
- `ui/src/pages/TaskDetail.svelte` (43.34)
- `ui/src/pages/Logs.svelte` (53.84)
- `ui/src/pages/Settings.svelte` (61.2)
- `ui/src/pages/Providers.svelte` (61.84)
- `ui/src/pages/Chat.svelte` (62.69)
- `ui/src/pages/Roles.svelte` (76.92)
- `ui/src/pages/Tasks.svelte` (76.28)
- `ui/src/pages/Agents.svelte` (78.69)

### [ ] B12 — Frontend low-coverage components + libs
Outline: cover branches/events.
Files (current → target 80):
- `ui/src/App.svelte` (28.57)
- `ui/src/components/MarkdownEditor.svelte` (50)
- `ui/src/components/AgentTemplatesPanel.svelte` (52.24)
- `ui/src/lib/time.js` (60)
- `ui/src/lib/api.js` (67.21)

### [ ] B13 — Final verification
Outline: run full backend + frontend suites; confirm every package ≥80% (or documented exception) and all green. Update `tasks_2026-06-14.md` with the new coverage numbers.
Verify:
- `cd agent-orchestrator && go test -count=1 -cover ./...` — all packages ≥80%
- `cd agent-orchestrator/ui && pnpm test` — all files ≥80% stmts, no unexpected skips
