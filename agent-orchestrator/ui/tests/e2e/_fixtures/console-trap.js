/**
 * console-trap.js — Playwright fixture that fails any test on uncaught
 * console errors or unhandled promise rejections.
 */
import { test as base, expect } from '@playwright/test'

export const test = base.extend({
  page: async ({ page }, use) => {
    const errors = []
    page.on('console', msg => {
      if (msg.type() === 'error') {
        const text = msg.text()
        // Ignore benign browser messages, WebSocket noise, and HTTP errors from
        // fetch calls that the app already handles gracefully (try/catch in api.js).
        // "Failed to load resource" is the browser's standard console entry for
        // any non-2xx response; it is not a JS error and does not indicate a bug.
        if (
          text.includes('WebSocket') ||
          text.includes('net::ERR_') ||
          text.includes('favicon') ||
          text.includes('Failed to load resource')
        ) return
        errors.push(text)
      }
    })
    page.on('pageerror', err => errors.push(err.message))

    await use(page)

    if (errors.length > 0) {
      throw new Error(
        `Page produced ${errors.length} console error(s):\n` +
        errors.map(e => `  • ${e}`).join('\n')
      )
    }
  },
})

export { expect }
