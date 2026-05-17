import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Tasks page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/tasks')
    await page.waitForTimeout(600)
  })

  test('renders Tasks heading', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /tasks/i })).toBeVisible()
  })

  test('shows status filter select', async ({ page }) => {
    // Native <select> is exposed as combobox in some Chromium versions and listbox
    // in others — target by element directly to avoid role ambiguity.
    await expect(page.locator('select').first()).toBeVisible()
  })

  test('shows New button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /new|\+/i })).toBeVisible()
  })

  test('shows Refresh button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /↻|refresh/i })).toBeVisible()
  })

  test('task list shows no console errors on render', async ({ page }) => {
    // Console errors caught by the fixture; if we reach here the page loaded cleanly.
    await expect(page.locator('body')).toBeVisible()
  })

  test('activity log section is present', async ({ page }) => {
    // The log header is below the task list; scroll it into view before asserting.
    const header = page.getByText(/task activity logs/i)
    await header.scrollIntoViewIfNeeded()
    await expect(header).toBeVisible()
  })
})
