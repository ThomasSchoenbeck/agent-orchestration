/**
 * Component tests for src/pages/Settings.svelte
 */
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Settings from '../pages/Settings.svelte'

// Minimal settings list returned by GET /api/settings
const SETTINGS = [
  { key: 'log.retention.agent.default_days',  value: '14', description: 'Agent default',  updated_at: new Date().toISOString() },
  { key: 'log.retention.task.default_days',   value: '30', description: 'Task default',   updated_at: new Date().toISOString() },
  { key: 'log.retention.system.default_days', value: '7',  description: 'System default', updated_at: new Date().toISOString() },
]

function stubFetch(settings = SETTINGS) {
  vi.stubGlobal('fetch', vi.fn((url, opts = {}) => {
    const body = (opts.method === 'PUT')
      ? { key: 'updated', value: 'ok' }
      : settings
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) })
  }))
}

afterEach(() => vi.unstubAllGlobals())

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Settings — rendering', () => {
  it('shows page heading', () => {
    stubFetch()
    render(Settings)
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  it('shows loading state initially', () => {
    stubFetch()
    render(Settings)
    // Loading text may appear briefly.
    expect(screen.getByText(/Loading/i)).toBeInTheDocument()
  })

  it('renders Agent Log Retention section', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('Agent Log Retention')).toBeInTheDocument()
    )
  })

  it('renders Task Log Retention section', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('Task Log Retention')).toBeInTheDocument()
    )
  })

  it('renders System Log Retention section', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('System Log Retention')).toBeInTheDocument()
    )
  })

  it('shows agent event types in table', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('agent_registered')).toBeInTheDocument()
    )
    expect(screen.getByText('agent_execute_complete')).toBeInTheDocument()
    expect(screen.getByText('agent_offline')).toBeInTheDocument()
  })

  it('shows task event types in table', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('task_created')).toBeInTheDocument()
    )
    expect(screen.getByText('task_completed')).toBeInTheDocument()
    expect(screen.getByText('task_failed')).toBeInTheDocument()
  })
})

// ── API calls ─────────────────────────────────────────────────────────────────
describe('Settings — API', () => {
  it('calls GET /api/settings on mount', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith('/api/settings', expect.anything())
    )
  })

  it('calls PUT /api/settings/:key when Save is clicked', async () => {
    // Multiple Save buttons exist; intercept them all.
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(SETTINGS) })
      .mockResolvedValue    ({ ok: true, status: 200, json: () => Promise.resolve(SETTINGS) })
    )
    const user = userEvent.setup()
    render(Settings)

    // Wait for sections to appear.
    await waitFor(() => screen.getByText('Agent Log Retention'))

    // Click the first Save button (agent default_days).
    const saveBtns = screen.getAllByRole('button', { name: /save/i })
    await user.click(saveBtns[0])

    await waitFor(() => {
      const putCall = fetch.mock.calls.find(([, opts]) => opts?.method === 'PUT')
      expect(putCall).toBeTruthy()
    })
  })
})

// ── Effective retention display ───────────────────────────────────────────────
describe('Settings — effective retention', () => {
  it('shows default days in effective column when no override', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() => screen.getByText('Agent Log Retention'))
    // Default retention should appear in the Effective column.
    const cells = screen.getAllByText(/14 \(default\)/)
    expect(cells.length).toBeGreaterThan(0)
  })

  it('shows task default days', async () => {
    stubFetch()
    render(Settings)
    await waitFor(() => screen.getByText('Task Log Retention'))
    const cells = screen.getAllByText(/30 \(default\)/)
    expect(cells.length).toBeGreaterThan(0)
  })
})
