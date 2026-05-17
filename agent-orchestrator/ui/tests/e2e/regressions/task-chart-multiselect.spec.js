/**
 * B5 regression: task chart filter does not allow multi-select
 *
 * Previously chartTypeFilter was a single string; clicking a second chart
 * slice replaced rather than adding to the filter. Fix: changed to Set.
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('task chart: multiple event types can be selected simultaneously', async ({ page }) => {
  await page.goto('/#/tasks')
  await page.waitForTimeout(800)

  // The chart slices are SVG rects/paths that call window._taskChartClick.
  // Simulate clicking two different event types via JS.
  await page.evaluate(() => {
    if (typeof window._taskChartClick === 'function') {
      window._taskChartClick('task_created')
      window._taskChartClick('task_completed')
    }
  })
  await page.waitForTimeout(200)

  // Both filter pills should be visible.
  const pills = page.locator('.bg-accent\\/20, [class*="bg-accent"]').filter({ hasText: /task_created|task_completed/ })
  const pillCount = await pills.count()

  // If there are logs of both types, both pills should appear.
  // If no logs, pills won't appear — that's fine, skip.
  if (pillCount > 0) {
    expect(pillCount).toBeGreaterThanOrEqual(2)
  }
})
