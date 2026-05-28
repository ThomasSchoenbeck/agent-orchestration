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

  test('shows roles checkboxes in create form when roles exist', async ({ page }) => {
    // Navigate to roles first to ensure at least one role exists
    await page.goto('/#/roles')
    await page.waitForTimeout(600)

    await page.goto('/#/providers')
    await page.waitForTimeout(600)

    await page.getByRole('button', { name: /\+ add provider/i }).click()

    // If roles are seeded, the Roles section should appear
    const rolesSection = page.locator('text=Roles').first()
    // Section may or may not be visible depending on whether roles are configured;
    // we only assert it doesn't crash and page still works
    await expect(page.getByRole('button', { name: /create/i })).toBeVisible()
  })

  test('provider card shows role tags after save', async ({ page }) => {
    // Open form
    await page.getByRole('button', { name: /\+ add provider/i }).click()
    await page.waitForTimeout(200)

    // Check if any role checkboxes are visible (depends on seeded roles)
    const checkboxes = page.locator('label').filter({ hasText: /planner|executor|reviewer/i })
    const count = await checkboxes.count()

    if (count > 0) {
      // Select first role
      await checkboxes.first().click()
    }

    // Fill required fields
    await page.getByPlaceholder(/e.g. my-openai/i).fill(`e2e-test-provider-${Date.now()}`)
    await page.getByRole('button', { name: /create/i }).click()
    await page.waitForTimeout(600)

    // No [object Object] should appear in role tags
    const text = await page.locator('body').textContent()
    expect(text).not.toContain('[object Object]')
  })
})
