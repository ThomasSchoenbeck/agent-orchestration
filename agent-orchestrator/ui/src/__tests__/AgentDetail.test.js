/**
 * Component tests for src/pages/AgentDetail.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import AgentDetail from '../pages/AgentDetail.svelte'

vi.mock('../lib/api.js', () => ({
  getAgent:      vi.fn(),
  getAgentStats: vi.fn(),
  getAgentLogs:  vi.fn(),
  listTasks:     vi.fn(),
  getTaskRoles:  vi.fn(),
}))

vi.mock('../lib/time.js', () => ({
  formatTimestamp: (ts) => ts ? new Date(ts).toISOString().slice(0, 16) : '—',
}))

vi.mock('../lib/stores.js', () => ({
  toasts: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
  router: { go: vi.fn(), push: vi.fn(), subscribe: (fn) => { fn({ page: 'agents', params: ['a1'] }); return () => {} } },
}))

import { getAgent, getAgentStats, getAgentLogs, listTasks, getTaskRoles } from '../lib/api.js'

const AGENT = {
  id: 'a1', name: 'worker-1', status: 'online', mode: 'remote',
  roles: ['executor', 'planner'], current_task_id: null,
  registered_at: new Date(Date.now() - 3600_000).toISOString(),
  last_heartbeat: new Date().toISOString(),
}

const STATS = {
  uptime_ms: 3_600_000,
  total_tasks: 12,
  completed_tasks: 10,
  failed_tasks: 2,
  total_tokens: 45_000,
  avg_task_ms: 8_000,
  registered_at: AGENT.registered_at,
  last_heartbeat: AGENT.last_heartbeat,
}

const LOGS = [
  { id: 'l1', level: 'info',  message: 'task started',  timestamp: new Date().toISOString() },
  { id: 'l2', level: 'error', message: 'something went wrong', timestamp: new Date().toISOString() },
]

const TASKS = [
  { id: 'task-001', type: 'implement', role: 'executor', status: 'COMPLETED',
    created_at: new Date().toISOString(), started_at: new Date(Date.now() - 10_000).toISOString(),
    completed_at: new Date().toISOString() },
  { id: 'task-002', type: 'review', role: 'planner', status: 'FAILED',
    created_at: new Date().toISOString(), started_at: null, completed_at: null },
]

beforeEach(() => {
  getAgent.mockResolvedValue(AGENT)
  getAgentStats.mockResolvedValue(STATS)
  getAgentLogs.mockResolvedValue(LOGS)
  listTasks.mockResolvedValue(TASKS)
  getTaskRoles.mockResolvedValue([])
})

describe('AgentDetail — rendering', () => {
  it('shows agent name after load', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => expect(screen.getByText('worker-1')).toBeInTheDocument())
  })

  it('shows status', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => expect(screen.getByText('online')).toBeInTheDocument())
  })

  it('shows roles as tags', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => expect(screen.getByText('executor')).toBeInTheDocument())
    expect(screen.getByText('planner')).toBeInTheDocument()
  })
})

describe('AgentDetail — stats', () => {
  it('shows total tasks', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => expect(screen.getByText('12')).toBeInTheDocument())
  })

  it('shows total tokens', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => expect(screen.getByText(/45.?000/)).toBeInTheDocument())
  })

  it('shows uptime', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => expect(screen.getByText('1h 0m')).toBeInTheDocument())
  })
})

describe('AgentDetail — logs tab', () => {
  it('shows log messages in Logs tab', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => expect(screen.getByText('task started')).toBeInTheDocument())
    expect(screen.getByText('something went wrong')).toBeInTheDocument()
  })

  it('shows log level colors', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => screen.getByText('task started'))
    // info and error levels should both be present
    const levels = screen.getAllByText(/info|error/)
    expect(levels.length).toBeGreaterThanOrEqual(2)
  })
})

describe('AgentDetail — tasks tab', () => {
  it('shows task rows after switching to Tasks tab', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => screen.getByText('worker-1'))

    const user = userEvent.setup()
    const tasksTab = screen.getByRole('button', { name: /tasks/i })
    await user.click(tasksTab)

    await waitFor(() => expect(screen.getByText('implement')).toBeInTheDocument())
    expect(screen.getByText('review')).toBeInTheDocument()
  })

  it('shows task statuses', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    await waitFor(() => screen.getByText('worker-1'))

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /tasks/i }))

    await waitFor(() => expect(screen.getByText('COMPLETED')).toBeInTheDocument())
    expect(screen.getByText('FAILED')).toBeInTheDocument()
  })
})

describe('AgentDetail — back navigation', () => {
  it('has a back button', async () => {
    render(AgentDetail, { props: { agentId: 'a1' } })
    expect(screen.getByText('← Agents')).toBeInTheDocument()
  })
})
