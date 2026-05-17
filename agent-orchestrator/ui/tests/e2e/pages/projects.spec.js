import { test, expect } from '../_fixtures/console-trap.js'

test.describe('Projects page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/#/projects')
    await page.waitForTimeout(500)
  })

  test('renders page heading', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /projects/i })).toBeVisible()
  })

  test('has a New Project button', async ({ page }) => {
    // The button toggles between '+ New Project' and 'Cancel'; match either state.
    await expect(page.getByRole('button', { name: /new project|cancel/i }).first()).toBeVisible()
  })

  test('shows empty state or project list', async ({ page }) => {
    // Wait for loading to finish — the loading indicator disappears once the API responds.
    await expect(page.getByText(/Loading/)).not.toBeVisible({ timeout: 5000 }).catch(() => {})
    // Either the empty-state paragraph or at least one project card (div[role="button"]) must exist.
    const emptyState  = page.getByText(/no projects yet/i)
    const projectCard = page.locator('div[role="button"]').first()
    await expect(emptyState.or(projectCard)).toBeVisible({ timeout: 3000 })
  })

  test('can open new project form', async ({ page }) => {
    await page.getByRole('button', { name: /new|create|\+/i }).first().click()
    await expect(page.getByPlaceholder(/name/i)).toBeVisible()
  })
})
