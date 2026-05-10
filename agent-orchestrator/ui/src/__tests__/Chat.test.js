/**
 * Component tests for src/pages/Chat.svelte
 * WebSocket is provided by MockWebSocket from setup.js.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { MockWebSocket } from './setup.js'
import Chat from '../pages/Chat.svelte'

beforeEach(() => {
  MockWebSocket.instances = []
})
afterEach(() => vi.clearAllMocks())

function getSocket() {
  return MockWebSocket.instances[0]
}

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Chat — rendering', () => {
  it('shows the Chat heading', () => {
    render(Chat)
    expect(screen.getByText('Chat')).toBeInTheDocument()
  })

  it('shows "Reconnecting…" before socket opens', () => {
    render(Chat)
    expect(screen.getByText(/Reconnecting/i)).toBeInTheDocument()
  })

  it('shows "Connected" after socket opens', async () => {
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() =>
      expect(screen.getByText('Connected')).toBeInTheDocument()
    )
  })

  it('shows back to "Reconnecting…" after socket closes', async () => {
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    getSocket().simulateClose()
    await waitFor(() =>
      expect(screen.getByText(/Reconnecting/i)).toBeInTheDocument()
    )
  })

  it('shows empty-state prompt before any messages', () => {
    render(Chat)
    expect(screen.getByText(/Send a message/i)).toBeInTheDocument()
  })

  it('Send button is disabled when disconnected', () => {
    render(Chat)
    const btn = screen.getByRole('button', { name: /send/i })
    expect(btn).toBeDisabled()
  })

  it('Send button is disabled when input is empty', async () => {
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const btn = screen.getByRole('button', { name: /send/i })
    expect(btn).toBeDisabled()
  })

  it('Send button is enabled when connected and input has text', async () => {
    const user = userEvent.setup()
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    await user.type(screen.getByPlaceholderText(/Message/i), 'Hello')
    expect(screen.getByRole('button', { name: /send/i })).not.toBeDisabled()
  })
})

// ── Sending messages ──────────────────────────────────────────────────────────
describe('Chat — sending messages', () => {
  it('sends user message to the WebSocket', async () => {
    const user = userEvent.setup()
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    await user.type(screen.getByPlaceholderText(/Message/i), 'Hello world')
    await user.click(screen.getByRole('button', { name: /send/i }))

    expect(getSocket().sent).toHaveLength(1)
    expect(JSON.parse(getSocket().sent[0])).toMatchObject({
      role: 'user', content: 'Hello world',
    })
  })

  it('appends user message to the chat log', async () => {
    const user = userEvent.setup()
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    await user.type(screen.getByPlaceholderText(/Message/i), 'Ping')
    await user.click(screen.getByRole('button', { name: /send/i }))
    await waitFor(() => expect(screen.getByText('Ping')).toBeInTheDocument())
  })

  it('clears the input after sending', async () => {
    const user = userEvent.setup()
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const textarea = screen.getByPlaceholderText(/Message/i)
    await user.type(textarea, 'Ping')
    await user.click(screen.getByRole('button', { name: /send/i }))
    await waitFor(() => expect(textarea).toHaveValue(''))
  })

  it('sends via Enter key', async () => {
    const user = userEvent.setup()
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    await user.type(screen.getByPlaceholderText(/Message/i), 'Hi{Enter}')
    expect(getSocket().sent).toHaveLength(1)
  })

  it('does NOT send via Shift+Enter (newline)', async () => {
    const user = userEvent.setup()
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    await user.type(screen.getByPlaceholderText(/Message/i), 'line1{Shift>}{Enter}{/Shift}line2')
    expect(getSocket().sent).toHaveLength(0)
  })
})

// ── Receiving messages ────────────────────────────────────────────────────────
describe('Chat — receiving messages', () => {
  it('displays assistant message from JSON payload', async () => {
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    getSocket().simulateMessage({ role: 'assistant', content: 'Hello there!' })
    await waitFor(() =>
      expect(screen.getByText('Hello there!')).toBeInTheDocument()
    )
  })

  it('displays assistant message from plain-string response', async () => {
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    // Plain string — not JSON
    getSocket().onmessage({ data: 'plain response' })
    await waitFor(() =>
      expect(screen.getByText(/plain response/i)).toBeInTheDocument()
    )
  })

  it('clears sending indicator when response arrives', async () => {
    const user = userEvent.setup()
    render(Chat)
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    await user.type(screen.getByPlaceholderText(/Message/i), 'Hi{Enter}')
    // "…" typing indicator should appear
    await waitFor(() => expect(screen.getByText('…')).toBeInTheDocument())
    // Simulate response
    getSocket().simulateMessage({ role: 'assistant', content: 'Hi back' })
    await waitFor(() =>
      expect(screen.queryByText('…')).not.toBeInTheDocument()
    )
  })
})

// ── WebSocket lifecycle ───────────────────────────────────────────────────────
describe('Chat — WebSocket lifecycle', () => {
  it('creates a WebSocket on mount', () => {
    render(Chat)
    expect(MockWebSocket.instances).toHaveLength(1)
  })

  it('closes the socket when component is destroyed', async () => {
    const { unmount } = render(Chat)
    const ws = getSocket()
    unmount()
    expect(ws.readyState).toBe(WebSocket.CLOSED)
  })
})
