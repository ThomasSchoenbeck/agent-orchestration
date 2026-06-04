/**
 * Component tests for src/App.svelte — sidebar navigation links.
 * Regression: nav items were <button onclick> and could not be right-clicked
 * or opened in a new tab. They are now real <a href> hash links.
 */
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import App from '../App.svelte'

function mockFetch(response = [], status = 200) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok:     status < 400,
    status,
    json:   () => Promise.resolve(response),
  }))
}

afterEach(() => vi.unstubAllGlobals())

describe('App — sidebar links', () => {
  it('renders nav items as anchors with hash hrefs (new-tab / right-click support)', () => {
    mockFetch([])
    render(App)

    const agents = screen.getByRole('link', { name: /Agents/ })
    expect(agents.getAttribute('href')).toBe('#/agents')

    const providers = screen.getByRole('link', { name: /Providers/ })
    expect(providers.getAttribute('href')).toBe('#/providers')

    const settings = screen.getByRole('link', { name: /Settings/ })
    expect(settings.getAttribute('href')).toBe('#/settings')
  })

  it('renders no <button> nav items in the sidebar', () => {
    mockFetch([])
    render(App)
    const nav = document.querySelector('nav')
    expect(nav.querySelector('button')).toBeNull()
  })
})
