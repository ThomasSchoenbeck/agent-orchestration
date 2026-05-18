/**
 * Unit tests for src/lib/stores.js
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { get } from 'svelte/store'

// Reset module between tests so the singleton store is re-created fresh
// with a clean window.location.hash.
beforeEach(() => {
  window.location.hash = ''
  vi.resetModules()
})

afterEach(() => {
  window.location.hash = ''
  vi.unstubAllGlobals()
})

// ── Router ────────────────────────────────────────────────────────────────────
describe('router store', () => {
  it('defaults to "projects" when hash is empty', async () => {
    window.location.hash = ''
    const { router } = await import('../lib/stores.js')
    expect(get(router)).toMatchObject({ page: 'projects', params: [] })
  })

  it('reads page from hash on init', async () => {
    window.location.hash = '#/tasks'
    const { router } = await import('../lib/stores.js')
    expect(get(router)).toMatchObject({ page: 'tasks', params: [] })
  })

  it('router.go() sets the hash and updates the store', async () => {
    const { router } = await import('../lib/stores.js')
    router.go('agents')
    expect(window.location.hash).toBe('#/agents')
  })

  it('updates store when hashchange fires', async () => {
    const { router } = await import('../lib/stores.js')
    window.location.hash = '#/logs'
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    expect(get(router)).toMatchObject({ page: 'logs' })
  })
})

// ── Toasts ────────────────────────────────────────────────────────────────────
describe('toasts store', () => {
  it('starts empty', async () => {
    const { toasts } = await import('../lib/stores.js')
    expect(get(toasts)).toHaveLength(0)
  })

  it('toasts.add() appends a toast', async () => {
    const { toasts } = await import('../lib/stores.js')
    toasts.add('Hello', 'info')
    const list = get(toasts)
    expect(list).toHaveLength(1)
    expect(list[0].message).toBe('Hello')
    expect(list[0].type).toBe('info')
  })

  it('toasts.success/error/info are shorthand aliases', async () => {
    const { toasts } = await import('../lib/stores.js')
    toasts.success('ok')
    toasts.error('fail')
    toasts.info('note')
    const list = get(toasts)
    expect(list.find(t => t.type === 'success')).toBeTruthy()
    expect(list.find(t => t.type === 'error')).toBeTruthy()
    expect(list.find(t => t.type === 'info')).toBeTruthy()
  })

  it('toasts.remove() removes by id', async () => {
    const { toasts } = await import('../lib/stores.js')
    toasts.add('A')
    toasts.add('B')
    const [first] = get(toasts)
    toasts.remove(first.id)
    expect(get(toasts).find(t => t.id === first.id)).toBeUndefined()
    expect(get(toasts)).toHaveLength(1)
  })

  it('toasts are auto-removed after duration', async () => {
    vi.useFakeTimers()
    const { toasts } = await import('../lib/stores.js')
    toasts.add('temp', 'info', 500)
    expect(get(toasts)).toHaveLength(1)
    vi.advanceTimersByTime(600)
    expect(get(toasts)).toHaveLength(0)
    vi.useRealTimers()
  })

  it('error toasts are persistent (no auto-dismiss)', async () => {
    vi.useFakeTimers()
    const { toasts } = await import('../lib/stores.js')
    toasts.error('something broke')
    expect(get(toasts)).toHaveLength(1)
    expect(get(toasts)[0].persistent).toBe(true)
    vi.advanceTimersByTime(10000)
    expect(get(toasts)).toHaveLength(1)
    vi.useRealTimers()
  })

  it('success toasts auto-dismiss after 5s', async () => {
    vi.useFakeTimers()
    const { toasts } = await import('../lib/stores.js')
    toasts.success('done')
    expect(get(toasts)).toHaveLength(1)
    vi.advanceTimersByTime(4999)
    expect(get(toasts)).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(get(toasts)).toHaveLength(0)
    vi.useRealTimers()
  })

  it('info toasts auto-dismiss after 4s', async () => {
    vi.useFakeTimers()
    const { toasts } = await import('../lib/stores.js')
    toasts.info('heads up')
    vi.advanceTimersByTime(4000)
    expect(get(toasts)).toHaveLength(0)
    vi.useRealTimers()
  })

  it('caps visible toasts at 3, dropping oldest', async () => {
    const { toasts } = await import('../lib/stores.js')
    toasts.error('e1')
    toasts.error('e2')
    toasts.error('e3')
    toasts.error('e4')
    const list = get(toasts)
    expect(list).toHaveLength(3)
    expect(list.map(t => t.message)).toEqual(['e2', 'e3', 'e4'])
  })
})

// ── Loading counter ───────────────────────────────────────────────────────────
describe('loading store', () => {
  it('false when counter is 0', async () => {
    const { loading } = await import('../lib/stores.js')
    expect(get(loading)).toBe(false)
  })

  it('true while counter > 0, false after decrement', async () => {
    const { loading, startLoading, stopLoading } = await import('../lib/stores.js')
    startLoading()
    expect(get(loading)).toBe(true)
    startLoading()
    stopLoading()
    expect(get(loading)).toBe(true)
    stopLoading()
    expect(get(loading)).toBe(false)
  })

  it('never goes negative', async () => {
    const { loading, stopLoading } = await import('../lib/stores.js')
    stopLoading()
    stopLoading()
    expect(get(loading)).toBe(false)
  })
})
