/**
 * Component tests for src/pages/Logs.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Logs from '../pages/Logs.svelte'

vi.mock('../lib/api.js', () => ({
  listLogs:         vi.fn(),
  listChatLog:      vi.fn(),
  listSettings:     vi.fn(),
  deleteLogs:       vi.fn(),
  listAgentLogs:    vi.fn(),
  deleteAgentLogs:  vi.fn(),
  listAllTaskLogs:  vi.fn(),
  deleteAllTaskLogs: vi.fn(),
}))

vi.mock('../lib/time.js', () => ({
  formatTimestamp: (ts) => new Date(ts).toISOString(),
}))

import { listLogs, listChatLog, listSettings, listAgentLogs, listAllTaskLogs } from '../lib/api.js'
import { get } from 'svelte/store'
import { toasts } from '../lib/stores.js'

const LOGS = [
  {
    id: 'l1', level: 'info', message: 'agent started',
    agent_id: 'agent-abc-123', task_id: 'task-xyz-456',
    project_id: 'proj-1',
    metadata: { provider: 'gpt-4o', tokens: 100 },
    timestamp: new Date().toISOString(),
  },
  {
    id: 'l2', level: 'error', message: 'task failed',
    agent_id: 'agent-abc-123', task_id: 'task-xyz-456',
    project_id: null, metadata: null,
    timestamp: new Date().toISOString(),
  },
  {
    id: 'l3', level: 'debug', message: 'no context',
    agent_id: null, task_id: null, project_id: null, metadata: null,
    timestamp: new Date().toISOString(),
  },
]

beforeEach(() => {
  listLogs.mockResolvedValue(LOGS)
  listChatLog.mockResolvedValue([])
  listSettings.mockResolvedValue([])
  listAgentLogs.mockResolvedValue([])
  listAllTaskLogs.mockResolvedValue([])
})

describe('Logs — error handling', () => {
  it('adds an error toast when the initial system-log load fails', async () => {
    listLogs.mockRejectedValue(new Error('boom'))
    render(Logs)
    await waitFor(() => {
      const errs = get(toasts).filter(
        (t) => t.type === 'error' && /Failed to load logs/.test(t.message)
      )
      expect(errs.length).toBeGreaterThan(0)
    })
  })
})

describe('Logs — rendering', () => {
  it('shows page heading', () => {
    render(Logs)
    expect(screen.getByText('Logs')).toBeInTheDocument()
  })

  it('renders log messages after load', async () => {
    render(Logs)
    await waitFor(() =>
      expect(screen.getByText('agent started')).toBeInTheDocument()
    )
    expect(screen.getByText('task failed')).toBeInTheDocument()
  })

  it('renders Agent column header in system log table', async () => {
    render(Logs)
    await waitFor(() => screen.getByText('agent started'))
    // "Agent" appears as both tab label and column header; verify at least one is a <th>
    const ths = document.querySelectorAll('th')
    const agentTh = Array.from(ths).some(th => th.textContent.trim() === 'Agent')
    expect(agentTh).toBe(true)
  })

  it('renders Task column header in system log table', async () => {
    render(Logs)
    await waitFor(() => screen.getByText('agent started'))
    const ths = document.querySelectorAll('th')
    const taskTh = Array.from(ths).some(th => th.textContent.trim() === 'Task')
    expect(taskTh).toBe(true)
  })

  it('shows truncated agent_id in Agent column', async () => {
    render(Logs)
    await waitFor(() => screen.getByText('agent started'))
    // agent-abc-123 should be truncated to first 8 chars + ellipsis
    expect(screen.getAllByText('agent-ab…').length).toBeGreaterThan(0)
  })

  it('shows — for missing agent_id', async () => {
    render(Logs)
    await waitFor(() => screen.getByText('no context'))
    // The third row has no agent_id — should show —
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThan(0)
  })
})

describe('Logs — expandable context rows', () => {
  it('shows +meta hint when entry has metadata', async () => {
    render(Logs)
    await waitFor(() => screen.getByText('agent started'))
    expect(screen.getByText('+meta')).toBeInTheDocument()
  })

  it('expands context details on row click', async () => {
    render(Logs)
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('agent started'))
    // Click the row with metadata
    const row = screen.getByText('agent started').closest('tr')
    await user.click(row)

    await waitFor(() =>
      expect(screen.getByText('agent-abc-123')).toBeInTheDocument()
    )
    // metadata JSON should be rendered
    expect(screen.getByText(/gpt-4o/)).toBeInTheDocument()
  })

  it('collapses on second click', async () => {
    render(Logs)
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('agent started'))
    const row = screen.getByText('agent started').closest('tr')
    await user.click(row)
    await waitFor(() => screen.getByText('agent-abc-123'))

    await user.click(row)
    await waitFor(() =>
      expect(screen.queryByText('agent-abc-123')).not.toBeInTheDocument()
    )
  })
})

describe('Logs — server calls', () => {
  it('passes limit and system_only to listLogs', async () => {
    render(Logs)
    await waitFor(() =>
      expect(listLogs).toHaveBeenCalledWith(
        expect.objectContaining({ limit: 200, system_only: true })
      )
    )
  })
})
