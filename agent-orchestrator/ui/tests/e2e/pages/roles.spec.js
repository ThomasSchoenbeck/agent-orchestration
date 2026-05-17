import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Roles page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/roles')
    await page.waitForTimeout(500)
  })

  test('renders Roles heading', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /roles/i })).toBeVisible()
  })

  test('has New Role button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /new|\+/i })).toBeVisible()
  })

  test('page body is not empty', async ({ page }) => {
    const text = await page.locator('body').textContent()
    expect(text.trim().length).toBeGreaterThan(10)
  })
})
