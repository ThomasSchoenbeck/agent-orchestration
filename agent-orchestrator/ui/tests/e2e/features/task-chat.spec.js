/**
 * A2 feature: Tasks page — LLM conversation panel
 *
 * When a task is selected, a "💬 Assistant" toggle button appears.
 * Clicking it opens the AssistantSidebar scoped to that task.
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('task assistant panel opens when task is selected', async ({ page }) => {
  await page.goto('/#/tasks')
  await page.waitForTimeout(600)

  // Select first task by clicking its row.
  const taskRow = page.locator('.bg-surface-800.rounded.border.cursor-pointer').first()
  const count = await taskRow.count()
  if (count === 0) {
    test.skip(true, 'No tasks — skipping task-chat test')
    return
  }

  await taskRow.click()
  await page.waitForTimeout(200)

  // "💬 Assistant" button should now be visible.
  const assistantBtn = page.getByRole('button', { name: /assistant/i })
  await expect(assistantBtn).toBeVisible()

  // Click to open sidebar.
  await assistantBtn.click()
  await page.waitForTimeout(300)

  // Sidebar heading should show "Task Assistant".
  await expect(page.getByText(/task assistant/i)).toBeVisible()
})
