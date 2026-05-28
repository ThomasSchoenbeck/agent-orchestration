/**
 * Component tests for src/pages/Chat.svelte
 * WebSocket is provided by MockWebSocket from setup.js.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { MockWebSocket, mockFetch } from './setup.js'
import Chat from '../pages/Chat.svelte'

const MOCK_CONVERSATION = {
  id: 'conv1', title: 'Test Conversation', provider_id: 'provider1',
  created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z',
}

const MOCK_PROVIDER = {
  id: 'provider1', name: 'Test Provider', enabled: true,
}

beforeEach(() => {
  MockWebSocket.instances = []
  // Mock API calls: listConversations returns one conversation, listProviders returns one provider,
  // getConversation returns the conversation with empty messages
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve([MOCK_CONVERSATION]) })
    .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve([MOCK_PROVIDER]) })
    .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve({ conversation: MOCK_CONVERSATION, messages: [] }) })
    .mockResolvedValue({ ok: true, status: 200, json: () => Promise.resolve({}) })
  )
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

  it('shows empty-state prompt before any messages', async () => {
    render(Chat)
    await waitFor(() => screen.getByText(/Start a conversation with the orchestrator/i))
    expect(screen.getByText(/Start a conversation with the orchestrator/i)).toBeInTheDocument()
  })

  it('Send button is disabled when disconnected', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    const btn = screen.getByRole('button', { name: /send/i })
    expect(btn).toBeDisabled()
  })

  it('Send button is disabled when input is empty', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const btn = screen.getByRole('button', { name: /send/i })
    expect(btn).toBeDisabled()
  })

  it('Send button is enabled when connected and input has text', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const editor = screen.getByRole('textbox', { name: /Message/i })
    editor.innerHTML = 'Hello'
    fireEvent.input(editor)
    await waitFor(() => expect(screen.getByRole('button', { name: /send/i })).not.toBeDisabled())
  })
})

// ── Sending messages ──────────────────────────────────────────────────────────
describe('Chat — sending messages', () => {
  it('sends user message to the WebSocket', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const editor = screen.getByRole('textbox', { name: /Message/i })
    editor.innerHTML = 'Hello world'
    fireEvent.input(editor)
    const btn = screen.getByRole('button', { name: /send/i })
    await waitFor(() => expect(btn).not.toBeDisabled())
    fireEvent.click(btn)

    await waitFor(() => expect(getSocket().sent).toHaveLength(1))
    // Parse sent data (it's a stringified JSON object)
    const sent = getSocket().sent[0]
    const parsed = typeof sent === 'string' ? JSON.parse(sent) : sent
    expect(parsed).toMatchObject({
      role: 'user', content: 'Hello world',
    })
  })

  it('appends user message to the chat log', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const editor = screen.getByRole('textbox', { name: /Message/i })
    editor.innerHTML = 'Ping'
    fireEvent.input(editor)
    const btn = screen.getByRole('button', { name: /send/i })
    await waitFor(() => expect(btn).not.toBeDisabled())
    fireEvent.click(btn)
    await waitFor(() => expect(screen.getByText('Ping')).toBeInTheDocument())
  })

  it('clears the input after sending', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const editor = screen.getByRole('textbox', { name: /Message/i })
    editor.innerHTML = 'Ping'
    fireEvent.input(editor)
    const btn = screen.getByRole('button', { name: /send/i })
    await waitFor(() => expect(btn).not.toBeDisabled())
    fireEvent.click(btn)
    await waitFor(() => expect(editor.innerHTML).toBe(''))
  })

  it('sends via Enter key', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const editor = screen.getByRole('textbox', { name: /Message/i })
    editor.innerHTML = 'Hi'
    fireEvent.input(editor)
    await waitFor(() => expect(screen.getByRole('button', { name: /send/i })).not.toBeDisabled())
    fireEvent.keyDown(editor, { key: 'Enter', shiftKey: false })
    await waitFor(() => expect(getSocket().sent).toHaveLength(1))
  })

  it('does NOT send via Shift+Enter (newline)', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const editor = screen.getByRole('textbox', { name: /Message/i })
    editor.innerHTML = 'line1'
    fireEvent.input(editor)
    await waitFor(() => expect(screen.getByRole('button', { name: /send/i })).not.toBeDisabled())
    fireEvent.keyDown(editor, { key: 'Enter', shiftKey: true })
    expect(getSocket().sent).toHaveLength(0)
  })
})

// ── Receiving messages ────────────────────────────────────────────────────────
describe('Chat — receiving messages', () => {
  it('displays assistant message from JSON payload', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    getSocket().simulateMessage({ role: 'assistant', content: 'Hello there!' })
    await waitFor(() =>
      expect(screen.getByText('Hello there!')).toBeInTheDocument()
    )
  })

  it('displays assistant message from plain-string response', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    // Plain string — not JSON
    getSocket().onmessage({ data: 'plain response' })
    await waitFor(() =>
      expect(screen.getByText(/plain response/i)).toBeInTheDocument()
    )
  })

  it('clears sending indicator when response arrives', async () => {
    render(Chat)
    await waitFor(() => screen.getByRole('textbox', { name: /Message/i }))
    getSocket().simulateOpen()
    await waitFor(() => screen.getByText('Connected'))
    const editor = screen.getByRole('textbox', { name: /Message/i })
    editor.innerHTML = 'Hi'
    fireEvent.input(editor)
    await waitFor(() => expect(screen.getByRole('button', { name: /send/i })).not.toBeDisabled())
    fireEvent.keyDown(editor, { key: 'Enter', shiftKey: false })
    // "…" typing indicator should appear (might be in a span or other element)
    await waitFor(() => {
      const elements = screen.queryAllByText(/…/)
      expect(elements.length).toBeGreaterThan(0)
    })
    // Simulate response
    getSocket().simulateMessage({ role: 'assistant', content: 'Hi back' })
    // Wait for the sending indicator to disappear and the response to appear
    await waitFor(() => {
      expect(screen.getByText(/Hi back/)).toBeInTheDocument()
      const indicatorElements = screen.queryAllByText(/…/)
      console.log(indicatorElements)

      expect(indicatorElements).toHaveLength(0)
    }, { timeout: 3000 })
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
