/**
 * interaction-coverage.spec.js
 *
 * Visits every route in the app and inventories every interactive element
 * (buttons, links, inputs, selects, textareas). Compares against a checked-in
 * fixture to catch orphan elements or missing UI.
 *
 * To update the fixture after intentional changes:
 *   pnpm e2e:update-fixture
 */
import { test, expect } from '@playwright/test'
import { readFileSync, writeFileSync, existsSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const FIXTURE_PATH = join(__dirname, 'expected-interactions.json')
const UPDATE = !!process.env.PLAYWRIGHT_UPDATE_FIXTURE

/** Routes to crawl. Order matters for sequential execution. */
const ROUTES = [
  { name: 'projects',  hash: '#/projects' },
  { name: 'tasks',     hash: '#/tasks' },
  { name: 'agents',    hash: '#/agents' },
  { name: 'chat',      hash: '#/chat' },
  { name: 'logs',      hash: '#/logs' },
  { name: 'settings',  hash: '#/settings' },
  { name: 'providers', hash: '#/providers' },
  { name: 'roles',     hash: '#/roles' },
]

/**
 * Returns a sorted, deduplicated list of interaction keys for the current page.
 * Each key is: `<tag>|<ariaLabel or text or type or placeholder>`.
 */
async function collectInteractions(page) {
  return page.evaluate(() => {
    const keys = new Set()
    const TAGS = ['a', 'button', 'input', 'select', 'textarea']
    for (const tag of TAGS) {
      for (const el of document.querySelectorAll(tag)) {
        // Skip elements with zero dimensions (hidden / display:none).
        const rect = el.getBoundingClientRect()
        if (rect.width === 0 && rect.height === 0) continue
        const label = (
          el.getAttribute('aria-label') ||
          el.textContent?.trim() ||
          el.getAttribute('placeholder') ||
          el.getAttribute('type') ||
          ''
        ).trim().slice(0, 80)
        keys.add(`${tag}|${label}`)
      }
    }
    return [...keys].sort()
  })
}

test('interaction coverage — all pages', async ({ page }) => {
  const discovered = {}

  for (const route of ROUTES) {
    await page.goto('/' + route.hash)
    // Wait for Svelte to hydrate and API calls to settle.
    await page.waitForTimeout(800)
    discovered[route.name] = await collectInteractions(page)
  }

  if (UPDATE) {
    writeFileSync(FIXTURE_PATH, JSON.stringify(discovered, null, 2) + '\n')
    console.log('✓ Fixture written to', FIXTURE_PATH)
    return
  }

  if (!existsSync(FIXTURE_PATH)) {
    throw new Error(
      'No interaction fixture found.\n' +
      'Run: pnpm e2e:update-fixture   to generate it.'
    )
  }

  const fixture = JSON.parse(readFileSync(FIXTURE_PATH, 'utf8'))
  const failures = []

  for (const route of ROUTES) {
    const live     = new Set(discovered[route.name] ?? [])
    const expected = new Set(fixture[route.name] ?? [])

    // Skip routes whose fixture entry is empty — not yet inventoried.
    if (expected.size === 0) continue

    const extra   = [...live].filter(k => !expected.has(k))
    const missing = [...expected].filter(k => !live.has(k))

    if (extra.length) {
      failures.push(
        `[${route.name}] NEW interactions not in fixture — add them or remove from UI:\n` +
        extra.map(k => `    + ${k}`).join('\n')
      )
    }
    if (missing.length) {
      failures.push(
        `[${route.name}] MISSING interactions still in fixture — remove from fixture or restore in UI:\n` +
        missing.map(k => `    - ${k}`).join('\n')
      )
    }
  }

  if (failures.length) {
    throw new Error('Interaction coverage failures:\n\n' + failures.join('\n\n'))
  }
})
