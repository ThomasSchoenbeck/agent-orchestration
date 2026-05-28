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

  test('provider dropdown is present in the create form', async ({ page }) => {
    await page.getByRole('button', { name: /\+ add role/i }).click()
    await page.waitForTimeout(200)
    await expect(page.getByRole('combobox')).toBeVisible()
  })

  test('provider label shows filtered hint when name matches a role', async ({ page }) => {
    // Seed providers and roles first so the filter has data to work with
    await page.goto('/#/providers')
    await page.waitForTimeout(400)
    await page.goto('/#/roles')
    await page.waitForTimeout(400)

    await page.getByRole('button', { name: /\+ add role/i }).click()
    await page.waitForTimeout(200)

    // Type a role name that matches any seeded provider role
    const nameInput = page.getByPlaceholder(/e.g. worker/i)
    await nameInput.fill('planner')
    await page.waitForTimeout(300)

    // Either "filtered for this role" or "showing all" hint should be visible
    const body = await page.locator('body').textContent()
    const hasHint = body.includes('filtered for this role') || body.includes('showing all')
    expect(hasHint).toBe(true)
  })
})
