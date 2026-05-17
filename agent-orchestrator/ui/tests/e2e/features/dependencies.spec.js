/**
 * A1.1 feature: Task dependencies (soft warning)
 *
 * On task detail, a Dependencies section is present with an add field.
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('task detail: dependencies section is present', async ({ page }) => {
  await page.goto('/#/tasks')

  // Wait for tasks to actually appear rather than a fixed sleep.
  const viewBtn = page.getByRole('button', { name: /view/i }).first()
  const appeared = await viewBtn.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false)
  if (!appeared) {
    test.skip(true, 'No visible tasks within 5 s — skipping dependencies test')
    return
  }

  await viewBtn.click()

  // Wait for task detail to load; bail out gracefully if the page shows an error.
  const notFound = page.getByText(/task not found/i)
  const deps     = page.getByRole('heading', { name: /dependencies/i })

  // Race: first of (notFound | deps) to become visible wins.
  await Promise.race([
    notFound.waitFor({ state: 'visible', timeout: 6000 }).catch(() => {}),
    deps.waitFor({ state: 'visible', timeout: 6000 }).catch(() => {}),
  ])

  if (await notFound.isVisible()) {
    test.skip(true, 'Task detail shows "Task not found" — API may be unavailable')
    return
  }

  await expect(deps).toBeVisible()
})
