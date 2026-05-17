import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Logs page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/logs')
    await page.waitForTimeout(600)
  })

  test('renders System Logs heading', async ({ page }) => {
    await expect(page.getByText(/system logs/i)).toBeVisible()
  })

  test('has level filter select', async ({ page }) => {
    await expect(page.getByRole('combobox').first()).toBeVisible()
  })

  test('has refresh button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /↻/i })).toBeVisible()
  })

  test('timeline chart section renders', async ({ page }) => {
    await expect(page.getByText(/timeline/i)).toBeVisible()
  })
})
