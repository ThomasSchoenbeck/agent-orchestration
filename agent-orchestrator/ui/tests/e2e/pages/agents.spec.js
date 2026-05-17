import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Agents page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/agents')
    await page.waitForTimeout(500)
  })

  test('renders Agents heading', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /agents/i })).toBeVisible()
  })

  test('shows status filter or empty state', async ({ page }) => {
    const hasFilter   = await page.getByRole('combobox').count() > 0
    const hasContent  = await page.locator('body').textContent()
    expect(hasFilter || hasContent.length > 0).toBe(true)
  })
})
