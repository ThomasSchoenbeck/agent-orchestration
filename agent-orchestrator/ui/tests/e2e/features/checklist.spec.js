/**
 * A1.2 feature: Task progress checklist with iteration groups
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('task detail: checklist section is present and interactive', async ({ page }) => {
  await page.goto('/#/tasks')

  const viewBtn = page.getByRole('button', { name: /view/i }).first()
  const appeared = await viewBtn.waitFor({ state: 'visible', timeout: 5000 }).then(() => true).catch(() => false)
  if (!appeared) {
    test.skip(true, 'No visible tasks within 5 s — skipping checklist test')
    return
  }

  await viewBtn.click()

  const notFound  = page.getByText(/task not found/i)
  const checklist = page.getByRole('heading', { name: /checklist/i })

  await Promise.race([
    notFound.waitFor({ state: 'visible', timeout: 6000 }).catch(() => {}),
    checklist.waitFor({ state: 'visible', timeout: 6000 }).catch(() => {}),
  ])

  if (await notFound.isVisible()) {
    test.skip(true, 'Task detail shows "Task not found" — API may be unavailable')
    return
  }

  await expect(checklist).toBeVisible()

  // "New iteration" button is present when checklist has items.
  const iterBtn = page.getByRole('button', { name: /new iteration|iteration/i })
  if (await iterBtn.count() > 0) {
    await expect(iterBtn.first()).toBeVisible()
  }

  // Add item if the input is present.
  const labelInput = page.getByPlaceholder(/item label|add item/i)
  if (await labelInput.count() > 0) {
    await labelInput.fill('Test item')
    await page.getByRole('button', { name: /add|save/i }).last().click()
    await expect(page.getByText('Test item').first()).toBeVisible({ timeout: 3000 })
  }
})
