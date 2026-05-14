/**
 * Component tests for src/pages/Agents.svelte
 * Updated for the new split-layout with agent log panel.
 */
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Agents from '../pages/Agents.svelte'

const AGENTS = [
  { id: 'a1', name: 'worker-1', roles: ['worker', 'reviewer'], status: 'online' },
  { id: 'a2', name: 'planner-1', roles: ['orchestrator'], status: 'offline' },
]

const ROLES = [
  { id: 'r1', name: 'worker',       label: 'Worker',       enabled: true },
  { id: 'r2', name: 'reviewer',     label: 'Reviewer',     enabled: true },
  { id: 'r3', name: 'orchestrator', label: 'Orchestrator', enabled: true },
]

const AGENT_LOGS = [
  {
    id: 'log1', agent_id: 'a1', agent_name: 'w1',
    event_type: 'agent_registered', description: 'Agent joined',
    timestamp: new Date().toISOString(),
  },
  {
    id: 'log2', agent_id: 'a1', agent_name: 'w1',
    event_type: 'agent_claim_success', description: 'Claimed task t1',
    timestamp: new Date().toISOString(),
  },
]

// Stub fetch: dispatch by URL prefix.
function stubFetch(agents = AGENTS, roles = ROLES, logs = AGENT_LOGS) {
  vi.stubGlobal('fetch', vi.fn((url) => {
    let data = agents
    if (url.includes('/api/roles'))       data = roles
    else if (url.includes('/api/agent-logs')) data = logs
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(data) })
  }))
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Agents — rendering', () => {
  it('shows page heading', () => {
    stubFetch()
    render(Agents)
    expect(screen.getByText('Agents')).toBeInTheDocument()
  })

  it('shows empty state when no agents', async () => {
    stubFetch([], ROLES, [])
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText(/No agents registered/i)).toBeInTheDocument()
    )
  })

  it('renders agent names', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('worker-1')).toBeInTheDocument()
      expect(screen.getByText('planner-1')).toBeInTheDocument()
    })
  })

  it('shows status text for each agent', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('online')).toBeInTheDocument()
      // "offline" also appears in the chart legend (agent_offline → "offline"), so use getAllByText
      expect(screen.getAllByText('offline').length).toBeGreaterThan(0)
    })
  })

  it('shows role badges', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getAllByText('worker').length).toBeGreaterThan(0)
      expect(screen.getByText('reviewer')).toBeInTheDocument()
      expect(screen.getByText('orchestrator')).toBeInTheDocument()
    })
  })
})

// ── Log panel ─────────────────────────────────────────────────────────────────
describe('Agents — log panel', () => {
  it('shows log panel header', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText('Agent Activity Logs')).toBeInTheDocument()
    )
  })

  it('shows Timeline and Types chart labels', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('Timeline')).toBeInTheDocument()
      expect(screen.getByText('Types')).toBeInTheDocument()
    })
  })

  it('shows event type filter dropdown', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() =>
      expect(screen.getByDisplayValue('All event types')).toBeInTheDocument()
    )
  })

  it('shows search input', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() =>
      expect(screen.getByPlaceholderText('Search description…')).toBeInTheDocument()
    )
  })

  it('renders log rows from API', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText('agent_registered')).toBeInTheDocument()
    )
    expect(screen.getByText('agent_claim_success')).toBeInTheDocument()
  })

  it('shows description column content', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText('Agent joined')).toBeInTheDocument()
    )
    expect(screen.getByText('Claimed task t1')).toBeInTheDocument()
  })

  it('shows event count in panel header', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText(/2 events/)).toBeInTheDocument()
    )
  })

  it('shows empty state when no logs', async () => {
    stubFetch(AGENTS, ROLES, [])
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText(/No events match current filters/i)).toBeInTheDocument()
    )
  })
})

// ── Agent selection (log scoping) ─────────────────────────────────────────────
describe('Agents — agent selection', () => {
  it('shows "Filtered to agent" chip when an agent is selected', async () => {
    const user = userEvent.setup()
    stubFetch()
    render(Agents)
    await waitFor(() => screen.getByText('worker-1'))

    // Click agent card to select it.
    await user.click(screen.getByText('worker-1'))
    await waitFor(() =>
      expect(screen.getByText('Filtered to agent')).toBeInTheDocument()
    )
  })

  it('shows × Clear button when filtered', async () => {
    const user = userEvent.setup()
    stubFetch()
    render(Agents)
    await waitFor(() => screen.getByText('worker-1'))
    await user.click(screen.getByText('worker-1'))
    await waitFor(() =>
      expect(screen.getByText('× Clear')).toBeInTheDocument()
    )
  })

  it('clears filter on × Clear click', async () => {
    const user = userEvent.setup()
    stubFetch()
    render(Agents)
    await waitFor(() => screen.getByText('worker-1'))
    await user.click(screen.getByText('worker-1'))
    await waitFor(() => screen.getByText('× Clear'))
    await user.click(screen.getByText('× Clear'))
    await waitFor(() =>
      expect(screen.queryByText('Filtered to agent')).not.toBeInTheDocument()
    )
  })
})

// ── API calls ─────────────────────────────────────────────────────────────────
describe('Agents — API', () => {
  it('calls GET /api/agents, /api/roles, /api/agent-logs on mount', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      const urls = fetch.mock.calls.map(([url]) => url)
      expect(urls.some(u => u.includes('/api/agents'))).toBe(true)
      expect(urls.some(u => u.includes('/api/roles'))).toBe(true)
      expect(urls.some(u => u.includes('/api/agent-logs'))).toBe(true)
    })
  })

  it('Refresh button triggers reload', async () => {
    stubFetch()
    const user = userEvent.setup()
    render(Agents)
    await waitFor(() => screen.getByText('worker-1'))
    const callsBefore = fetch.mock.calls.length
    await user.click(screen.getByText('↻ Refresh'))
    await waitFor(() =>
      expect(fetch.mock.calls.length).toBeGreaterThan(callsBefore)
    )
  })

  it('sets up auto-refresh interval on mount', async () => {
    vi.useFakeTimers()
    stubFetch()
    render(Agents)
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    const callsAfterMount = fetch.mock.calls.length

    vi.advanceTimersByTime(5_500)
    expect(fetch.mock.calls.length).toBeGreaterThan(callsAfterMount)
  })
})

// ── Chart buckets ─────────────────────────────────────────────────────────────
describe('Agents — timeline buckets', () => {
  it('shows bucket selector buttons', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('5 m')).toBeInTheDocument()
      expect(screen.getByText('1 hr')).toBeInTheDocument()
      expect(screen.getByText('1 day')).toBeInTheDocument()
    })
  })
})
