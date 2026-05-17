import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Providers page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/providers')
    await page.waitForTimeout(600)
  })

  test('renders Providers heading', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /providers/i })).toBeVisible()
  })

  test('has New Provider button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /new|\+/i })).toBeVisible()
  })

  test('metrics section does not contain [object Object]', async ({ page }) => {
    const text = await page.locator('body').textContent()
    expect(text).not.toContain('[object Object]')
  })
})
