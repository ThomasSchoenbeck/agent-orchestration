/**
 * Component tests for src/pages/Settings.svelte
 *
 * API module is mocked at the module level (vi.mock is hoisted) so the
 * component never touches fetch.  Responses resolve in one microtask tick,
 * making every waitFor near-instant.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Settings from '../pages/Settings.svelte'

// ── Mock the api module ───────────────────────────────────────────────────────
vi.mock('../lib/api.js', () => ({
  listSettings:             vi.fn(),
  updateSetting:            vi.fn(),
  listChecklistTemplates:   vi.fn(),
  createChecklistTemplate:  vi.fn(),
  updateChecklistTemplate:  vi.fn(),
  deleteChecklistTemplate:  vi.fn(),
}))

// Import after vi.mock so we get the mocked versions
import {
  listSettings,
  updateSetting,
  listChecklistTemplates,
} from '../lib/api.js'

// Minimal settings list
const SETTINGS = [
  { key: 'log.retention.agent.default_days',  value: '14', description: 'Agent default',  updated_at: new Date().toISOString() },
  { key: 'log.retention.task.default_days',   value: '30', description: 'Task default',   updated_at: new Date().toISOString() },
  { key: 'log.retention.system.default_days', value: '7',  description: 'System default', updated_at: new Date().toISOString() },
]

beforeEach(() => {
  listSettings.mockResolvedValue(SETTINGS)
  listChecklistTemplates.mockResolvedValue([])
  updateSetting.mockResolvedValue({ key: 'updated', value: 'ok' })
})

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Settings — rendering', () => {
  it('shows page heading', () => {
    render(Settings)
    expect(screen.getByText('Settings')).toBeInTheDocument()
  })

  it('shows loading state initially', () => {
    render(Settings)
    expect(screen.getByRole('generic', { name: /Loading/i })).toBeInTheDocument()
  })

  it('renders Agent Log Retention section', async () => {
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('Agent Log Retention')).toBeInTheDocument()
    )
  })

  it('renders Task Log Retention section', async () => {
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('Task Log Retention')).toBeInTheDocument()
    )
  })

  it('renders System Log Retention section', async () => {
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('System Log Retention')).toBeInTheDocument()
    )
  })

  it('shows agent event types in table', async () => {
    render(Settings)
    await waitFor(() =>
      expect(screen.getByText('agent_registered')).toBeInTheDocument()
    )
    expect(screen.getByText('agent_execute_complete')).toBeInTheDocument()
    expect(screen.getByText('agent_offline')).toBeInTheDocument()
  })

  it('shows task event types in table', async () => {
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
  it('calls listSettings on mount', async () => {
    render(Settings)
    await waitFor(() => expect(listSettings).toHaveBeenCalled())
  })

  it('calls updateSetting when Save is clicked', async () => {
    const user = userEvent.setup()
    render(Settings)

    await waitFor(() => screen.getByText('Agent Log Retention'))

    const saveBtns = screen.getAllByRole('button', { name: /save/i })
    await user.click(saveBtns[0])

    await waitFor(() => expect(updateSetting).toHaveBeenCalled())
  })
})

// ── Effective retention display ───────────────────────────────────────────────
describe('Settings — effective retention', () => {
  it('shows default days in effective column when no override', async () => {
    render(Settings)
    await waitFor(() => screen.getByText('Agent Log Retention'))
    const cells = screen.getAllByText(/14 \(default\)/)
    expect(cells.length).toBeGreaterThan(0)
  })

  it('shows task default days', async () => {
    render(Settings)
    await waitFor(() => screen.getByText('Task Log Retention'))
    const cells = screen.getAllByText(/30 \(default\)/)
    expect(cells.length).toBeGreaterThan(0)
  })
})
