import { test, expect } from '../_fixtures/console-trap.js'

// End-to-end for the reviewer-owned merge UI: a human opens a PR on a task that
// is in review (Create PR), the task moves to the merge gate, and the human
// merges it (Approve & merge) until the PR reaches a decided state.
//
// Everything is seeded over the HTTP API (no git client). The feature branch is
// created as a fresh branch, so the merge itself may land or be rejected — this
// test asserts the *buttons* drive the server end-to-end (Create PR → merge gate
// → PR decided), not the git merge result (which the Go integration tests cover).

async function seedTaskInReview(request) {
  const name = 'pr-e2e-' + Date.now()
  const slug = name.toLowerCase()

  const pRes = await request.post('/api/projects', { data: { name, slug } })
  expect(pRes.ok()).toBeTruthy()
  const project = await pRes.json()

  const irRes = await request.post(`/api/projects/${project.id}/init-repo`)
  expect(irRes.ok()).toBeTruthy()

  const tRes = await request.post('/api/tasks', {
    data: { project_id: project.id, role: 'worker', priority: 5, payload: { title: 'PR e2e task' } },
  })
  expect(tRes.ok()).toBeTruthy()
  const task = await tRes.json()

  // Seed a commit on the task's fallback branch so the PR has something to merge.
  await request.post(`/api/projects/${project.id}/file`, {
    data: { branch: `task/${task.id}`, path: 'app.go', content: 'package app\n', message: 'work' },
  })

  // Move the task into review so the Create PR button shows.
  const uRes = await request.put(`/api/tasks/${task.id}`, { data: { status: 'AWAITING_REVIEW' } })
  expect(uRes.ok()).toBeTruthy()

  return task.id
}

test.describe('Pull request — Create PR and merge', () => {
  test('Create PR opens the merge gate, then Approve & merge decides the PR', async ({ page, request }) => {
    const taskId = await seedTaskInReview(request)

    await page.goto(`/#/tasks/${taskId}`)

    // 1. Create PR is offered for a task in review.
    const createBtn = page.getByRole('button', { name: 'Create PR' })
    await expect(createBtn).toBeVisible()
    await createBtn.click()

    // 2. The task is now at the merge gate — Approve & merge appears.
    const mergeBtn = page.getByRole('button', { name: /approve.*merge/i })
    await expect(mergeBtn).toBeVisible({ timeout: 10_000 })

    // 3. Merging decides the PR (button goes away once the PR is merged/rejected).
    await mergeBtn.click()
    await expect(mergeBtn).toBeHidden({ timeout: 10_000 })
  })
})
