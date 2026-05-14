/**
 * Component tests for src/pages/Agents.svelte
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Agents from '../pages/Agents.svelte'

const AGENTS = [
  {
    id: 'a1', name: 'worker-1',
    roles: ['worker', 'reviewer'],
    status: 'online',
    last_heartbeat: new Date().toISOString(),
  },
  {
    id: 'a2', name: 'planner-1',
    roles: ['orchestrator'],
    status: 'offline',
    last_heartbeat: '0001-01-01T00:00:00Z',
  },
]

const ROLES = [
  { id: 'r1', name: 'worker', label: 'Worker', enabled: true },
  { id: 'r2', name: 'reviewer', label: 'Reviewer', enabled: true },
  { id: 'r3', name: 'orchestrator', label: 'Orchestrator', enabled: true },
]

function stubFetch(agents = AGENTS, roles = ROLES) {
  vi.stubGlobal('fetch', vi.fn((url) => {
    let data = agents
    if (url === '/api/roles') {
      data = roles
    }
    return Promise.resolve({
      ok: true, status: 200,
      json: () => Promise.resolve(data),
    })
  }))
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()   // always restore real timers (guarding the polling test)
})

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Agents — rendering', () => {
  it('shows page heading', () => {
    stubFetch()
    render(Agents)
    expect(screen.getByText('Agents')).toBeInTheDocument()
  })

  it('shows empty state when no agents', async () => {
    stubFetch([], ROLES)
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText(/No agents registered/i)).toBeInTheDocument()
    )
  })

  it('renders an agent card for each agent', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('worker-1')).toBeInTheDocument()
      expect(screen.getByText('planner-1')).toBeInTheDocument()
    })
  })

  it('shows role badges', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('worker')).toBeInTheDocument()
      expect(screen.getByText('reviewer')).toBeInTheDocument()
      expect(screen.getByText('orchestrator')).toBeInTheDocument()
    })
  })

  it('shows status text for each agent', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('online')).toBeInTheDocument()
      expect(screen.getByText('offline')).toBeInTheDocument()
    })
  })

  it('does NOT show last-seen for zero-time heartbeat', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => screen.getByText('planner-1'))
    // Year 0001 heartbeat should be hidden
    expect(screen.queryByText(/Last seen: 1\/1\/1/i)).not.toBeInTheDocument()
  })

  it('shows last-seen for valid heartbeat', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() =>
      expect(screen.getByText(/Last seen:/i)).toBeInTheDocument()
    )
  })

  it('shows resolved role definitions', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      // Should show role labels from resolved definitions
      expect(screen.getByText('Worker')).toBeInTheDocument()
      expect(screen.getByText('Reviewer')).toBeInTheDocument()
      expect(screen.getByText('Orchestrator')).toBeInTheDocument()
    })
  })

  it('shows warning for undefined roles', async () => {
    // Create agents with a role that doesn't exist in role definitions
    const agentsWithUndefined = [
      { ...AGENTS[0], roles: ['worker', 'nonexistent'] },
    ]
    stubFetch(agentsWithUndefined, ROLES)
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('Worker')).toBeInTheDocument()
      // Match the warning badge with flexible text matching (icon + text in same element)
      expect(screen.getByText(/no definition/i)).toBeInTheDocument()
    })
  })
})

// ── API calls ─────────────────────────────────────────────────────────────────
describe('Agents — API', () => {
  it('calls GET /api/agents and /api/roles on mount', async () => {
    stubFetch()
    render(Agents)
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/agents', expect.anything())
      expect(fetch).toHaveBeenCalledWith('/api/roles', expect.anything())
    })
  })

  it('Refresh button triggers reload', async () => {
    stubFetch()
    const user = userEvent.setup()
    render(Agents)
    await waitFor(() => screen.getByText('worker-1'))
    await user.click(screen.getByText('↻ Refresh'))
    // Initial load: 2 calls (agents + roles), refresh: 2 more = 4 total
    expect(fetch).toHaveBeenCalledTimes(4)
  })

  it('sets up polling interval on mount', async () => {
    vi.useFakeTimers()
    stubFetch()
    render(Agents)
    // Initial load: 2 calls (agents + roles) even with fake timers via microtasks
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
    // Advance past the 10-second polling interval.
    vi.advanceTimersByTime(10_500)
    // Polling triggers another load: 2 more calls = 4 total
    expect(fetch).toHaveBeenCalledTimes(4)
    // vi.useRealTimers() is called in afterEach, no need to repeat here.
  })
})

// ── accepts wrapped response ──────────────────────────────────────────────────
describe('Agents — response shapes', () => {
  it('handles bare array response', async () => {
    stubFetch(AGENTS)
    render(Agents)
    await waitFor(() => expect(screen.getByText('worker-1')).toBeInTheDocument())
  })

  it('handles {agents: [...]} wrapped response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve({ agents: AGENTS }),
    }))
    render(Agents)
    await waitFor(() => expect(screen.getByText('worker-1')).toBeInTheDocument())
  })
})
