/**
 * Vitest global test setup — runs once before every test file.
 */
import '@testing-library/jest-dom'
import { cleanup } from '@testing-library/svelte'
import { afterEach, vi } from 'vitest'

// Automatically unmount components after every test
afterEach(() => cleanup())

// ── WebSocket mock ────────────────────────────────────────────────────────────
// Provides a controllable WebSocket double that tests can inspect and drive.
class MockWebSocket {
  static instances = []

  constructor(url) {
    this.url          = url
    this.readyState   = WebSocket.CONNECTING
    this.sent         = []
    this.onopen       = null
    this.onmessage    = null
    this.onclose      = null
    this.onerror      = null
    MockWebSocket.instances.push(this)
  }

  send(data) { this.sent.push(data) }
  close()    { this.readyState = WebSocket.CLOSED; this.onclose?.({ code: 1000 }) }

  // Test helpers — call from test code to simulate server events
  simulateOpen()            { this.readyState = WebSocket.OPEN;   this.onopen?.() }
  simulateMessage(data)     { this.onmessage?.({ data: typeof data === 'string' ? data : JSON.stringify(data) }) }
  simulateError(err = {})   { this.onerror?.(err) }
  simulateClose(code = 1000){ this.readyState = WebSocket.CLOSED; this.onclose?.({ code }) }
}

MockWebSocket.CONNECTING = 0
MockWebSocket.OPEN       = 1
MockWebSocket.CLOSING    = 2
MockWebSocket.CLOSED     = 3

// Expose constants on prototype too (some code checks instance.readyState === WebSocket.OPEN)
Object.assign(MockWebSocket.prototype, {
  CONNECTING: 0, OPEN: 1, CLOSING: 2, CLOSED: 3,
})

vi.stubGlobal('WebSocket', MockWebSocket)
export { MockWebSocket }

// ── fetch mock helper ─────────────────────────────────────────────────────────
// Re-usable factory so tests don't need to set up the multi-layer mock by hand.
export function mockFetch(body, { status = 200, ok = true } = {}) {
  return vi.fn().mockResolvedValue({
    ok,
    status,
    json: () => Promise.resolve(body),
  })
}
