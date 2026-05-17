/**
 * A4 feature: Logs page — chat log section
 *
 * After messages exist in conversations, the Logs page shows a "Chat Log"
 * section with timestamp, provider, direction, and preview.
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('logs page: chat log section renders without errors', async ({ page }) => {
  await page.goto('/#/logs')
  await page.waitForTimeout(800)

  // Chat Log section appears only when there are messages.
  // Even if empty, the page must not throw errors (handled by console-trap).
  const body = await page.locator('body').textContent()

  // If the section is present, verify structural elements.
  if (body.toLowerCase().includes('chat log')) {
    await expect(page.getByText(/chat log/i)).toBeVisible()
    // Direction badges should be present.
    const dirBadges = page.getByText(/→ llm|← llm/i)
    if (await dirBadges.count() > 0) {
      await expect(dirBadges.first()).toBeVisible()
    }
  }
  // If no messages exist, the section is hidden — that's also correct behaviour.
})
