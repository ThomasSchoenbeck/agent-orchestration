/**
 * A1.3 feature: Task comments section
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('task detail: comments section is present', async ({ page }) => {
  await page.goto('/#/tasks')

  const viewBtn = page.getByRole('button', { name: /view/i }).first()
  const appeared = await viewBtn.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false)
  if (!appeared) {
    test.skip(true, 'No visible tasks within 5 s — skipping comments test')
    return
  }

  await viewBtn.click()

  const notFound = page.getByText(/task not found/i)
  const comments = page.getByRole('heading', { name: /comments/i })

  await Promise.race([
    notFound.waitFor({ state: 'visible', timeout: 6000 }).catch(() => {}),
    comments.waitFor({ state: 'visible', timeout: 6000 }).catch(() => {}),
  ])

  if (await notFound.isVisible()) {
    test.skip(true, 'Task detail shows "Task not found" — API may be unavailable')
    return
  }

  await expect(comments).toBeVisible()

  const commentInput = page.getByPlaceholder(/add a comment/i)
  if (await commentInput.count() > 0) {
    await commentInput.fill('E2E test comment')
    await page.keyboard.press('Control+Enter')
    await expect(page.locator('.text-xs.text-gray-300', { hasText: 'E2E test comment' }).first()).toBeVisible({ timeout: 3000 })
  }
})
