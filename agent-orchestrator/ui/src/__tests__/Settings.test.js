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
import { get } from 'svelte/store'
import { toasts } from '../lib/stores.js'

describe('Settings — error handling', () => {
  it('shows an error toast when settings fail to load', async () => {
    listSettings.mockRejectedValue(new Error('boom'))
    render(Settings)
    await waitFor(() => {
      const errs = get(toasts).filter(
        (t) => t.type === 'error' && /Failed to load settings/.test(t.message)
      )
      expect(errs.length).toBeGreaterThan(0)
    })
  })
})

// ── Mock the api module ───────────────────────────────────────────────────────
vi.mock('../lib/api.js', () => ({
  listSettings:             vi.fn(),
  updateSetting:            vi.fn(),
  listChecklistTemplates:   vi.fn(),
  createChecklistTemplate:  vi.fn(),
  updateChecklistTemplate:  vi.fn(),
  deleteChecklistTemplate:  vi.fn(),
  getTaskTypes:             vi.fn(),
  createTaskType:           vi.fn(),
  updateTaskType:           vi.fn(),
  deleteTaskType:           vi.fn(),
}))

// Import after vi.mock so we get the mocked versions
import {
  listSettings,
  updateSetting,
  listChecklistTemplates,
  getTaskTypes,
  createTaskType,
} from '../lib/api.js'

const TASK_TYPES = [
  { id: 'tt-normal', key: 'normal', label: 'Normal', branch_template: 'feature/{slug}', is_default: true, sort_order: 0 },
  { id: 'tt-bug', key: 'bug', label: 'Bug', branch_template: 'bug/{slug}', is_default: false, sort_order: 1 },
]

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
  getTaskTypes.mockResolvedValue(TASK_TYPES)
  createTaskType.mockResolvedValue({ id: 'tt-new' })
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

describe('Settings — Task Types panel (B4/T9)', () => {
  it('lists task types with their branch templates', async () => {
    render(Settings)
    await waitFor(() => screen.getByText('Task Types'))
    expect(screen.getByText('Normal')).toBeInTheDocument()
    expect(screen.getByText('Bug')).toBeInTheDocument()
    expect(screen.getByText('feature/{slug}')).toBeInTheDocument()
  })

  it('creates a task type from the add form', async () => {
    const user = userEvent.setup()
    render(Settings)
    await waitFor(() => screen.getByText('Task Types'))

    await user.type(screen.getByPlaceholderText('key (e.g. bug)'), 'hotfix')
    await user.type(screen.getByPlaceholderText('Label'), 'Hotfix')
    // userEvent treats "{" as a special-key prefix; "{{" types a literal "{".
    await user.type(screen.getByPlaceholderText('bug/{slug}'), 'hotfix/{{slug}')
    await user.click(screen.getByText('Add task type'))

    await waitFor(() => {
      expect(createTaskType).toHaveBeenCalledWith(
        expect.objectContaining({ key: 'hotfix', label: 'Hotfix', branch_template: 'hotfix/{slug}' }),
      )
    })
  })
})
