import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Settings page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/settings')
    await page.waitForTimeout(600)
  })

  test('renders Settings heading', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /settings/i })).toBeVisible()
  })

  test('has Platform section with autorefresh setting', async ({ page }) => {
    await expect(page.getByText(/platform/i)).toBeVisible()
    await expect(page.getByText(/autorefresh|auto.refresh/i)).toBeVisible()
  })

  test('has Checklist Templates section', async ({ page }) => {
    await expect(page.getByText(/checklist template/i)).toBeVisible()
  })
})
