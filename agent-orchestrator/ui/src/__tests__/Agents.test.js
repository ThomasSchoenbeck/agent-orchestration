/**
 * Component tests for src/pages/Agents.svelte
 * Updated for the new split-layout with agent log panel.
 */
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { tick } from 'svelte'
import Agents from '../pages/Agents.svelte'
import { get } from 'svelte/store'
import { toasts } from '../lib/stores.js'

describe('Agents — load error handling', () => {
  it('shows an error toast when agents fail to load', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('boom')))
    render(Agents)
    await waitFor(() => {
      const errs = get(toasts).filter(
        (t) => t.type === 'error' && /Failed to load agents/.test(t.message)
      )
      expect(errs.length).toBeGreaterThan(0)
    })
  })
})

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
function stubFetch(agents = AGENTS, roles = ROLES, logs = AGENT_LOGS, templates = []) {
  vi.stubGlobal('fetch', vi.fn((url) => {
    let data = agents
    if (url.includes('/api/meta/'))                data = []
    else if (url.includes('/api/agent-templates')) data = templates
    else if (url.includes('/api/agent-logs'))      data = logs
    else if (url.includes('/api/roles'))           data = roles
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(data) })
  }))
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
  try { localStorage.clear() } catch { /* ignore */ }
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

// ── Templates panel (Bug 7 + 8: Managed page merged into Agents) ──────────────
describe('Agents — templates panel', () => {
  const TPL = [
    { id: 't1', name: 'reviewer-pool', roles: ['reviewer'], skills: [], replicas: 2, autostart: false, enabled: true },
  ]

  it('renders the Templates panel with template names', async () => {
    stubFetch(AGENTS, ROLES, AGENT_LOGS, TPL)
    render(Agents)
    await waitFor(() => {
      expect(screen.getByText('Templates')).toBeInTheDocument()
      expect(screen.getByText('reviewer-pool')).toBeInTheDocument()
    })
  })

  it('opens an edit form pre-filled with current values when Edit is clicked', async () => {
    stubFetch(AGENTS, ROLES, AGENT_LOGS, TPL)
    const user = userEvent.setup()
    render(Agents)
    await waitFor(() => expect(screen.getByText('reviewer-pool')).toBeInTheDocument())

    await user.click(screen.getByText('Edit'))
    expect(screen.getByDisplayValue('reviewer-pool')).toBeInTheDocument()        // name input
    expect(screen.getByRole('button', { name: 'Remove reviewer' })).toBeInTheDocument() // roles chip
  })

  it('PATCHes the template on Save', async () => {
    stubFetch(AGENTS, ROLES, AGENT_LOGS, TPL)
    const user = userEvent.setup()
    render(Agents)
    await waitFor(() => expect(screen.getByText('reviewer-pool')).toBeInTheDocument())

    await user.click(screen.getByText('Edit'))
    const name = screen.getByDisplayValue('reviewer-pool')
    await user.clear(name)
    await user.type(name, 'reviewer-team')
    await user.click(screen.getByText('Save'))

    await waitFor(() => {
      const patched = fetch.mock.calls.find(
        ([url, opts]) => url.includes('/api/agent-templates/t1') && opts?.method === 'PATCH'
      )
      expect(patched).toBeTruthy()
    })
  })
})

// ── Stopping badge (Bug: stale "stopping" on not-running agents) ───────────────
describe('Agents — stopping badge', () => {
  it('hides the "stopping" badge for an offline agent left with desired_state=stop', async () => {
    stubFetch([
      { id: 'a1', name: 'stopped-agent', roles: ['worker'], status: 'offline', desired_state: 'stop' },
    ])
    render(Agents)
    await waitFor(() => expect(screen.getByText('stopped-agent')).toBeInTheDocument())
    expect(screen.queryByText('stopping')).toBeNull()
  })

  it('shows the "stopping" badge for a running agent asked to stop', async () => {
    stubFetch([
      { id: 'a1', name: 'draining-agent', roles: ['worker'], status: 'online', desired_state: 'stop' },
    ])
    render(Agents)
    await waitFor(() => expect(screen.getByText('draining-agent')).toBeInTheDocument())
    expect(screen.getByText('stopping')).toBeInTheDocument()
  })
})

// ── Resizable top region (Bug 4) ──────────────────────────────────────────────
describe('Agents — resizable top region', () => {
  it('renders a resize divider', () => {
    stubFetch()
    const { container } = render(Agents)
    expect(
      container.querySelector('[role="separator"][aria-label="Resize agents panel"]')
    ).toBeInTheDocument()
  })

  it('grows the agents region when the divider is dragged down', async () => {
    localStorage.clear()
    stubFetch()
    const { container } = render(Agents)
    const sep = container.querySelector('[role="separator"]')
    const top = container.querySelector('[data-testid="agents-top"]')
    expect(top.getAttribute('style')).toContain('height: 288px')

    sep.dispatchEvent(new MouseEvent('mousedown', { clientY: 300, bubbles: true }))
    window.dispatchEvent(new MouseEvent('mousemove', { clientY: 400, bubbles: true }))
    window.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    await tick()

    expect(top.getAttribute('style')).toContain('height: 388px')
  })
})
