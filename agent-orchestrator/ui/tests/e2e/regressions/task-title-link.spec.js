/**
 * B4 regression: clicking the task title does not open task detail
 *
 * The row's onclick was intercepting the title click and selecting the row
 * instead of navigating. Fix: title is a separate button with stopPropagation.
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('task title click navigates to task detail', async ({ page }) => {
  await page.goto('/#/tasks')
  await page.waitForTimeout(600)

  // Only run if there are tasks to click.
  const firstTaskTitle = page.locator('.text-gray-100.font-medium, button.text-gray-100').first()
  const count = await firstTaskTitle.count()
  if (count === 0) {
    test.skip(true, 'No tasks present — skipping title-link test')
    return
  }

  await firstTaskTitle.click()

  // After click, hash should contain a task ID segment (not just #/tasks).
  await page.waitForTimeout(300)
  const hash = await page.evaluate(() => window.location.hash)
  expect(hash).toMatch(/^#\/tasks\/.+/)
})
