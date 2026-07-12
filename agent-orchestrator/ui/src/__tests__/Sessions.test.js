/**
 * Component tests for src/pages/Sessions.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Sessions from '../pages/Sessions.svelte'

vi.mock('../lib/api.js', () => ({
  listAgentSessions:   vi.fn(),
  getTaskMemory:       vi.fn(),
  listPreparedPrompts: vi.fn(),
}))

vi.mock('../lib/time.js', () => ({
  formatTimestamp: (ts) => ts ? String(ts) : '—',
}))

vi.mock('../lib/stores.js', () => ({
  toasts: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
  router: { go: vi.fn(), push: vi.fn(), subscribe: (fn) => { fn({ page: 'sessions', params: ['t1'] }); return () => {} } },
}))

import { listAgentSessions, getTaskMemory, listPreparedPrompts } from '../lib/api.js'

const SESSIONS = [
  { id: 's1', kind: 'main', title: 'Main session', status: 'done', cost: 0.12, round: 0, parent_id: '', created_at: '2026-07-11T00:00:00Z' },
  { id: 's2', kind: 'work', title: 'code_subtask', status: 'done', cost: 0.05, round: 1, parent_id: 's1', created_at: '2026-07-11T00:01:00Z' },
]

const MEMORY = {
  content: {
    summary: 'Implemented feature X',
    progress: ['seeded memory', 'delegated to code_subtask'],
    decisions: [], findings: [], open_questions: [],
  },
}

// Prompt is >80 chars so the collapsed preview (first 80) differs from the full
// text — the full string then appears only in the expanded <pre>.
const FULL_PROMPT = 'You are a careful worker agent. Delegate exploration to investigate_codebase before writing any code, then verify with tests.'
const PROMPTS = [
  { id: 'p1', session_id: 's1', round: 0, prompt: FULL_PROMPT, created_at: '2026-07-11T00:00:30Z' },
]

beforeEach(() => {
  listAgentSessions.mockResolvedValue(SESSIONS)
  getTaskMemory.mockResolvedValue(MEMORY)
  listPreparedPrompts.mockResolvedValue(PROMPTS)
})

describe('Sessions — rendering', () => {
  it('renders the session tree (main + nested subagent)', async () => {
    render(Sessions, { props: { taskId: 't1' } })
    await waitFor(() => expect(screen.getByText('Main session')).toBeInTheDocument())
    expect(screen.getByText('code_subtask')).toBeInTheDocument()
  })

  it('renders the memory summary and progress items', async () => {
    render(Sessions, { props: { taskId: 't1' } })
    await waitFor(() => expect(screen.getByText('Implemented feature X')).toBeInTheDocument())
    expect(screen.getByText('delegated to code_subtask')).toBeInTheDocument()
  })

  it('expands a synthesized prompt to show its full text', async () => {
    render(Sessions, { props: { taskId: 't1' } })
    const user = userEvent.setup()
    // The prompt row is a button (the session-tree "round 0" chip is a plain span).
    const promptBtn = await screen.findByRole('button', { name: /round 0/ })

    await user.click(promptBtn)
    await waitFor(() => expect(screen.getByText(FULL_PROMPT)).toBeInTheDocument())
  })
})

describe('Sessions — empty states', () => {
  it('shows empty messages when nothing is recorded', async () => {
    listAgentSessions.mockResolvedValue([])
    getTaskMemory.mockResolvedValue({})
    listPreparedPrompts.mockResolvedValue([])
    render(Sessions, { props: { taskId: 't1' } })
    await waitFor(() =>
      expect(screen.getByText(/No sessions recorded/)).toBeInTheDocument()
    )
    expect(screen.getByText(/No memory recorded/)).toBeInTheDocument()
    expect(screen.getByText(/No synthesized prompts/)).toBeInTheDocument()
  })
})
