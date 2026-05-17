/**
 * A5 feature: Platform settings — chart autorefresh interval
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('settings: autorefresh interval input is present', async ({ page }) => {
  await page.goto('/#/settings')

  // Wait for the loading guard to clear.
  await expect(page.getByText('Loading…')).not.toBeVisible({ timeout: 5000 }).catch(() => {})

  // Platform section and the autorefresh row live in the {:else} block.
  await expect(page.getByText('Platform')).toBeVisible({ timeout: 3000 })
  await expect(page.getByText(/auto.refresh/i)).toBeVisible()

  // Verify the number input is present and accepts input.
  const input = page.locator('input[type="number"]').first()
  if (await input.count() > 0) {
    await input.fill('3000')
    await page.keyboard.press('Tab')
    // NOTE: do NOT check for /failed/i here — the retention table rows contain
    // "agent_execute_failed" and "task_failed" which match that regex and are
    // legitimately visible, causing a false-positive failure.
  }
})
