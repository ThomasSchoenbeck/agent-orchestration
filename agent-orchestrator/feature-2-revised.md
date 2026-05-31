# Feature 2 (revised) — PR workflow: reviewer opens PR, deployer reviews and merges on approval

**Status:** `[ ]` pending
**Supersedes:** the original Feature 2 spec.
**Related:** Feature 3 (revised) — capabilities; Feature 4 — auto-queue.

---

## Goal

When a worker pushes its branch and the task is approved in review, the system drives an explicit, human-or-agent-gated PR workflow into `main`. **PRs are never auto-approved.** A `handles_merge` agent (the deployer) must perform its own review of the PR and explicitly approve before any merge happens; a human can do the same at any time via the UI.

## Lifecycle

```
worker pushes        → AWAITING_REVIEW
reviewer approves    → opens PR (status: open), task → AWAITING_MERGE
deployer claims      → MERGING   (the deployer is reviewing the PR; not yet merged)
  approve            → server merges branch into main, deletes branch,
                       PR → merged, task → COMPLETED
  reject             → PR → rejected, task → AWAITING_REVISION (branch kept)
human (any time)     → may approve/reject the PR via the UI, same effect
```

Two distinct review gates: the **work review** (`reviewer`, `handles_review`) judges whether the code is correct/complete; the **merge review** (`deployer`, `handles_merge`) judges whether the PR is safe to integrate and deploy. Both are real reviews with verdicts — neither is automatic.

## What to build

**Part A — PR model and persistence**
Add `PullRequest` to `db/models.go`:

```go
type PullRequest struct {
    ID         string    `json:"id"`
    TaskID     string    `json:"task_id"`
    ProjectID  string    `json:"project_id"`
    Branch     string    `json:"branch"`      // source: task/<id>
    Base       string    `json:"base"`        // always "main"
    Title      string    `json:"title"`
    Body       string    `json:"body"`        // reviewer's approval summary becomes the description
    Status     string    `json:"status"`      // open | approved | rejected | merged
    AuthorID   string    `json:"author_id"`   // reviewer agent that opened it
    AuthorName string    `json:"author_name"`
    DeciderID  string    `json:"decider_id"`  // deployer agent or human that approved/rejected
    DecisionBody string  `json:"decision_body"` // the merge reviewer's verdict notes
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

Migration: `CREATE TABLE pull_requests (...)`. New file `db/pull_requests.go`: `CreatePR`, `GetPR`, `ListPRsForTask`, `UpdatePRStatus`, `SetPRDecision`.

**Part B — Reviewer opens the PR on approval**
When the work reviewer approves (the `approved` verdict in the existing review flow), instead of transitioning straight to merge it:
1. Creates a `PullRequest` (status `open`) with the review summary as the body.
2. Transitions the task `REVIEWING → AWAITING_MERGE`.
3. Logs `pr_opened`.

Add `OpenPR(taskID, title, body string) error` to `agent/client.go`; the reviewer's executor calls it on approval.

**Part C — Deployer reviews the PR (no auto-approve)**
A `deployer` agent (capability `handles_merge`, per Feature 3) claims the `AWAITING_MERGE` task (`AWAITING_MERGE → MERGING`). It runs its own review against the PR — build/integration checks, conflict pre-check, deploy-readiness — then submits a decision:
- `POST /api/tasks/{id}/pull-requests/{prID}/approve` with decision notes → triggers merge (Part D).
- `POST /api/tasks/{id}/pull-requests/{prID}/reject` with decision notes → PR `rejected`, task `MERGING → AWAITING_REVISION`, branch kept.

Add `SubmitPRDecision(taskID, prID, verdict, body string) error` to `agent/client.go`. The executor detects an `AWAITING_MERGE`/`MERGING` task as a merge-review context (analogous to how `handles_review` tasks are detected) and calls it. If no deployer agent is online, the task simply waits in `AWAITING_MERGE` for a human decision — nothing auto-approves.

**Part D — Merge executed by the approve endpoint**
The approve handler is the single place that touches git, so a merge can only follow an explicit approval:
1. Open the bare repo.
2. Merge `task.Branch` into `main` (fast-forward if possible, merge commit otherwise). On conflict: do not complete — set PR `rejected` with the conflict log and task → `AWAITING_REVISION`.
3. Delete the source branch ref.
4. PR → `merged`; task `MERGING → COMPLETED`; log `pr_merged`.
5. If the project has an upstream `RemoteURL` and mirroring is enabled, push `main` upstream (non-blocking).

Add `git/merge.go`: `MergeBranch(repoPath, base, branch string) (sha string, err error)` and `DeleteBranch(repoPath, branch string) error`. Reuse the existing file-lock/conflict checks as a pre-merge guard.

**Part E — Agent workdir cleanup**
After every task — success, failure, review, or merge — the agent removes its local workspace. In `agent/agent.go` `executeTask`, add `defer os.RemoveAll(localPath)` immediately after `task.WorktreePath` is set, so cleanup is guaranteed on panic or early return.

**Part F — UI**
- PR panel in `TaskDetail.svelte`: open PRs with title, author (reviewer), status, the deployer/human decision notes, and approve/reject buttons. Humans can always decide here.
- `AWAITING_MERGE` and `MERGING` status badges (MERGING shown as "merge review in progress").
- `ui/src/lib/api.js` — `listPRs(taskID)`, `approvePR(taskID, prID, body)`, `rejectPR(taskID, prID, body)`.

## Reconciliation note (important)

This design **replaces any fully-automated background merge supervisor**. Merge is gated by an explicit approval decision from a `handles_merge` agent or a human — there is no actor that merges without a decision. Do not run an auto-merging supervisor alongside this; the approve endpoint is the sole merge path. (Conflict detection / file-lock logic is retained only as a pre-merge guard inside the approve handler, not as an autonomous merger.)

## Files to touch

- `db/models.go` — add `PullRequest`
- `db/pull_requests.go` — new: CRUD + decision setter
- `db/migrations.go` — `CREATE TABLE pull_requests`
- `git/merge.go` — new: `MergeBranch`, `DeleteBranch`
- `server/reviews.go` — on `approved`, open PR + transition to `AWAITING_MERGE` (instead of direct merge)
- `server/handlers.go` (or `server/pull_requests.go`) — PR list + approve/reject endpoints with merge logic
- `agent/client.go` — `OpenPR`, `SubmitPRDecision`
- `agent/executor.go` — reviewer opens PR on approval; deployer submits PR decision on merge-review tasks
- `agent/agent.go` — `defer os.RemoveAll(localPath)` cleanup
- `ui/src/pages/TaskDetail.svelte` — PR panel, badges
- `ui/src/lib/api.js` — PR helpers

## Tests

- `TestReviewApproval_OpensPRAndAwaitsMerge` — approved work review creates a PR (open) and moves task to `AWAITING_MERGE`
- `TestPR_NotAutoApproved` — PR sits `open` in `AWAITING_MERGE` with no deployer online; no merge occurs
- `TestDeployerClaimsAndApproves_Merges` — deployer claims, approves; branch merged into main, deleted, PR `merged`, task `COMPLETED`
- `TestDeployerRejects_ReturnsToRevision` — deployer rejects; task `AWAITING_REVISION`, branch kept, PR `rejected`
- `TestApprove_MergeConflict_ReturnsToRevision` — approval with conflicting branch → task `AWAITING_REVISION`, PR `rejected` with conflict log
- `TestHumanApprovePR_Merges` — human approve via endpoint merges identically
- `TestMergeBranch_FastForward` / `TestMergeBranch_MergeCommit` / `TestDeleteBranch` (`git/merge_test.go`)
- `TestAgentCleanup_RemovesWorkdir` / `TestAgentCleanup_OnFailure`
- Integration `TestFullWorkflow_E2E` — worker → push → reviewer approves (PR open) → deployer reviews + approves → merged, task `COMPLETED`, workdir gone
