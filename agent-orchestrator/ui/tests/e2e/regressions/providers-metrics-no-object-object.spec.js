/**
 * B1 regression: Providers view shows `by_project` metric as `[object Object]`
 *
 * Asserts that the providers page never renders the string "[object Object]"
 * in any visible text, which was the symptom of rendering an array/object
 * metric value by coercing it to string.
 */
import { test, expect } from '../_fixtures/console-trap.js'

test('providers page: metrics do not render as [object Object]', async ({ page }) => {
  await page.goto('/#/providers')
  await page.waitForTimeout(800)

  const bodyText = await page.locator('body').textContent()
  expect(bodyText).not.toContain('[object Object]')
  expect(bodyText).not.toContain('[object object]')
})
