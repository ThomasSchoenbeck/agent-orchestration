import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Chat page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/chat')
    await page.waitForTimeout(600)
  })

  test('renders Chat heading or conversation list', async ({ page }) => {
    const hasHeading = await page.getByRole('heading', { name: /chat/i }).count() > 0
    const hasList    = await page.getByText(/no conversations|new/i).count() > 0
    expect(hasHeading || hasList).toBe(true)
  })

  test('has a New conversation button', async ({ page }) => {
    await expect(page.getByRole('button', { name: /new|\+/i })).toBeVisible()
  })

  test('WebSocket connected indicator is visible', async ({ page }) => {
    // Either "Connected" or "Reconnecting" — both are valid initial states.
    await expect(
      page.getByText(/connected|reconnecting/i)
    ).toBeVisible({ timeout: 5000 })
  })
})
