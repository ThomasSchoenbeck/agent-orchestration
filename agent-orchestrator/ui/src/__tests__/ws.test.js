/**
 * Unit tests for src/lib/ws.js
 * The global WebSocket is replaced by MockWebSocket from setup.js.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { MockWebSocket } from './setup.js'
import { createChatSocket } from '../lib/ws.js'

beforeEach(() => {
  MockWebSocket.instances = []
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('createChatSocket', () => {
  it('connects to /ws/chat on the current host', () => {
    createChatSocket({})
    expect(MockWebSocket.instances).toHaveLength(1)
    expect(MockWebSocket.instances[0].url).toMatch(/\/ws\/chat$/)
  })

  it('calls onOpen when connection opens', () => {
    const onOpen = vi.fn()
    createChatSocket({ onOpen })
    MockWebSocket.instances[0].simulateOpen()
    expect(onOpen).toHaveBeenCalledOnce()
  })

  it('calls onMessage with parsed JSON payload', () => {
    const onMessage = vi.fn()
    createChatSocket({ onMessage })
    MockWebSocket.instances[0].simulateOpen()
    MockWebSocket.instances[0].simulateMessage({ role: 'assistant', content: 'Hi' })
    expect(onMessage).toHaveBeenCalledWith({ role: 'assistant', content: 'Hi' })
  })

  it('calls onMessage with raw string when JSON parse fails', () => {
    const onMessage = vi.fn()
    createChatSocket({ onMessage })
    MockWebSocket.instances[0].simulateOpen()
    // Plain text that is not valid JSON
    MockWebSocket.instances[0].onmessage({ data: 'plain text' })
    expect(onMessage).toHaveBeenCalledWith({ role: 'assistant', content: 'plain text' })
  })

  it('calls onClose when connection closes', () => {
    const onClose = vi.fn()
    createChatSocket({ onClose })
    MockWebSocket.instances[0].simulateClose()
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('calls onError on socket error', () => {
    const onError = vi.fn()
    createChatSocket({ onError })
    MockWebSocket.instances[0].simulateError({ message: 'fail' })
    expect(onError).toHaveBeenCalledOnce()
  })

  it('send() serialises message to JSON and calls ws.send', () => {
    const sock = createChatSocket({})
    const ws   = MockWebSocket.instances[0]
    ws.simulateOpen()
    sock.send({ role: 'user', content: 'Hello' })
    expect(ws.sent).toHaveLength(1)
    expect(JSON.parse(ws.sent[0])).toEqual({ role: 'user', content: 'Hello' })
  })

  it('send() is a no-op when socket is not OPEN', () => {
    const sock = createChatSocket({})
    // socket starts CONNECTING, not OPEN
    sock.send({ role: 'user', content: 'Hello' })
    expect(MockWebSocket.instances[0].sent).toHaveLength(0)
  })

  it('auto-reconnects after 2 s when not explicitly closed', () => {
    createChatSocket({})
    MockWebSocket.instances[0].simulateClose()
    expect(MockWebSocket.instances).toHaveLength(1)
    vi.advanceTimersByTime(2100)
    expect(MockWebSocket.instances).toHaveLength(2)
  })

  it('does NOT reconnect after sock.close()', () => {
    const sock = createChatSocket({})
    sock.close()
    vi.advanceTimersByTime(3000)
    // Still only the one original instance
    expect(MockWebSocket.instances).toHaveLength(1)
  })

  it('ready getter returns true only when OPEN', () => {
    const sock = createChatSocket({})
    expect(sock.ready).toBe(false)
    MockWebSocket.instances[0].simulateOpen()
    expect(sock.ready).toBe(true)
    MockWebSocket.instances[0].simulateClose()
    expect(sock.ready).toBe(false)
  })
})
