/**
 * Component tests for src/pages/Tasks.svelte
 * Updated for the new state vocabulary (BACKLOG, DEVELOPING, etc.)
 */
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Tasks from '../pages/Tasks.svelte'

const PROJECTS = [{ id: 'proj1', name: 'MyProject' }]
const TASK_TYPES = [
  { value: 'implement', label: 'Implement', description: 'Write code' },
  { value: 'review',    label: 'Review',    description: 'Review code' },
]
const TASK_ROLES = [
  { value: 'worker',   label: 'Worker',   description: 'Does the work' },
  { value: 'reviewer', label: 'Reviewer', description: 'Reviews the work' },
]
const TASKS = [
  {
    id: 't1', type: 'implement', role: 'worker', status: 'BACKLOG', priority: 5,
    payload: { title: 'Build auth', description: 'JWT flow' }, project_id: 'proj1',
  },
  {
    id: 't2', type: 'review', role: 'reviewer', status: 'COMPLETED', priority: 3,
    payload: { title: 'Code review', description: '' }, project_id: 'proj1',
  },
  {
    id: 't3', type: 'plan', role: 'orchestrator', status: 'FAILED', priority: 8,
    payload: {}, project_id: 'proj1',
  },
]
const TASK_LOGS = [
  {
    id: 'tl1', task_id: 't1', event_type: 'task_created',
    old_status: '', new_status: 'BACKLOG', description: 'Task was created',
    timestamp: new Date().toISOString(),
  },
  {
    id: 'tl2', task_id: 't1', event_type: 'task_claimed',
    old_status: 'BACKLOG', new_status: 'DEVELOPING', description: 'Agent claimed it',
    timestamp: new Date().toISOString(),
  },
]

function stubFetch(tasks = TASKS, projects = PROJECTS, types = TASK_TYPES, roles = TASK_ROLES, logs = TASK_LOGS) {
  vi.stubGlobal('fetch', vi.fn((url) => {
    let data
    if (url.includes('/api/task-logs'))    data = logs
    else if (url.includes('/api/projects')) data = projects
    else if (url.includes('task-types'))   data = types
    else if (url.includes('task-roles'))   data = roles
    else                                   data = tasks
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(data) })
  }))
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Tasks — rendering', () => {
  it('shows page heading', () => {
    stubFetch()
    render(Tasks)
    expect(screen.getByText('Tasks')).toBeInTheDocument()
  })

  it('renders task titles from API', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => {
      expect(screen.getByText('Build auth')).toBeInTheDocument()
      expect(screen.getByText('Code review')).toBeInTheDocument()
    })
  })

  it('shows status badge for each task', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => {
      expect(screen.getAllByText('BACKLOG',   { selector: 'span' }).length).toBeGreaterThan(0)
      expect(screen.getAllByText('COMPLETED', { selector: 'span' }).length).toBeGreaterThan(0)
      expect(screen.getAllByText('FAILED',    { selector: 'span' }).length).toBeGreaterThan(0)
    })
  })

  it('falls back to type when payload.title is missing', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => {
      const planEls = screen.getAllByText('plan')
      expect(planEls.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('shows Queue button for FAILED and COMPLETED tasks', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => {
      const queueBtns = screen.getAllByText('Queue')
      expect(queueBtns.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('shows View button for each task', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => {
      const viewBtns = screen.getAllByText('View')
      expect(viewBtns.length).toBe(3)
    })
  })
})

// ── Filters ───────────────────────────────────────────────────────────────────
describe('Tasks — filters', () => {
  it('renders status filter dropdown', () => {
    stubFetch()
    render(Tasks)
    expect(screen.getByDisplayValue('All statuses')).toBeInTheDocument()
  })

  it('status filter contains new state vocabulary', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => {
      // The dropdown options should include new states
      const select = screen.getByDisplayValue('All statuses')
      const options = Array.from(select.options).map(o => o.value)
      expect(options).toContain('BACKLOG')
      expect(options).toContain('DEVELOPING')
      expect(options).toContain('AWAITING_REVIEW')
      expect(options).toContain('AWAITING_REVISION')
      expect(options).not.toContain('planned')
      expect(options).not.toContain('in_progress')
      expect(options).not.toContain('needs_review')
    })
  })

  it('renders project filter with loaded projects', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => expect(screen.getByText('MyProject')).toBeInTheDocument())
  })
})

// ── Create form ───────────────────────────────────────────────────────────────
describe('Tasks — create form', () => {
  it('shows form when "+ New" is clicked', async () => {
    stubFetch()
    const user = userEvent.setup()
    render(Tasks)
    await user.click(screen.getByText('+ New'))
    expect(screen.getByRole('combobox', { name: /Task type/i })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /Role/i })).toBeInTheDocument()
  })

  it('posts task with all required fields', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockImplementation((url) => {
        let data
        if (url.includes('/api/task-logs'))    data = []
        else if (url.includes('/api/projects')) data = PROJECTS
        else if (url.includes('task-types'))   data = TASK_TYPES
        else if (url.includes('task-roles'))   data = TASK_ROLES
        else if (url.includes('/api/tasks') && !url.includes('task-logs')) data = TASKS
        else data = []
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(data) })
      })
      .mockResolvedValueOnce({ ok: true, status: 201, json: () => Promise.resolve({ id: 'tnew' }) })
    )

    const user = userEvent.setup()
    render(Tasks)
    await waitFor(() => screen.getByText('+ New'))
    await user.click(screen.getByText('+ New'))

    await user.selectOptions(screen.getByDisplayValue('Select project *'), 'proj1')
    await user.selectOptions(screen.getByRole('combobox', { name: /Task type/i }), 'implement')
    await user.selectOptions(screen.getByRole('combobox', { name: /Role/i }), 'worker')
    await user.type(screen.getByPlaceholderText('Title'), 'New task')
    await user.click(screen.getByText('Create'))

    await waitFor(() => {
      const postCall = fetch.mock.calls.find(([url, opts]) =>
        url === '/api/tasks' && opts?.method === 'POST'
      )
      expect(postCall).toBeTruthy()
      const body = JSON.parse(postCall[1].body)
      expect(body.type).toBe('implement')
      expect(body.role).toBe('worker')
      expect(body.project_id).toBe('proj1')
      expect(body.payload.title).toBe('New task')
    })
  })
})

// ── Queue action ──────────────────────────────────────────────────────────────
describe('Tasks — queue action', () => {
  it('POSTs to /queue endpoint when Queue is clicked', async () => {
    vi.stubGlobal('fetch', vi.fn((url, opts = {}) => {
      if (opts.method === 'POST' && url.includes('/queue')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ ...TASKS[2], status: 'BACKLOG' }) })
      }
      let data
      if (url.includes('/api/task-logs'))    data = TASK_LOGS
      else if (url.includes('/api/projects')) data = PROJECTS
      else if (url.includes('task-types'))   data = TASK_TYPES
      else if (url.includes('task-roles'))   data = TASK_ROLES
      else                                   data = TASKS
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(data) })
    }))

    const user = userEvent.setup()
    render(Tasks)
    await waitFor(() => screen.getAllByText('Queue'))
    await user.click(screen.getAllByText('Queue')[0])

    await waitFor(() => {
      const postCall = fetch.mock.calls.find(([url, opts]) =>
        url.includes('/api/tasks/') && url.includes('/queue') && opts?.method === 'POST'
      )
      expect(postCall).toBeTruthy()
    })
  })
})

// ── Log panel ─────────────────────────────────────────────────────────────────
describe('Tasks — log panel', () => {
  it('shows log panel header', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() =>
      expect(screen.getByText('Task Activity Logs')).toBeInTheDocument()
    )
  })

  it('shows Timeline and Types chart labels', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() => {
      expect(screen.getByText('Timeline')).toBeInTheDocument()
      expect(screen.getByText('Types')).toBeInTheDocument()
    })
  })

  it('shows event type filter dropdown', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() =>
      expect(screen.getByDisplayValue('All event types')).toBeInTheDocument()
    )
  })

  it('shows search input for logs', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() =>
      expect(screen.getByPlaceholderText('Search description…')).toBeInTheDocument()
    )
  })

  it('renders log event types from API', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() =>
      expect(screen.getByText('task_created')).toBeInTheDocument()
    )
    expect(screen.getByText('task_claimed')).toBeInTheDocument()
  })

  it('shows new-vocabulary status transition in log rows', async () => {
    stubFetch()
    render(Tasks)
    await waitFor(() =>
      expect(screen.getByText('BACKLOG → DEVELOPING')).toBeInTheDocument()
    )
  })

  it('shows empty state when no logs', async () => {
    stubFetch(TASKS, PROJECTS, TASK_TYPES, TASK_ROLES, [])
    render(Tasks)
    await waitFor(() =>
      expect(screen.getByText(/No events match current filters/i)).toBeInTheDocument()
    )
  })
})

// ── Task selection (log scoping) ──────────────────────────────────────────────
describe('Tasks — task selection', () => {
  it('shows "Filtered to task" chip when a task row is clicked', async () => {
    const user = userEvent.setup()
    stubFetch()
    render(Tasks)
    await waitFor(() => screen.getByText('Build auth'))

    const taskRow = screen.getByText('Build auth').closest('div[class*="bg-surface-800"]')
    if (taskRow) await user.click(taskRow)

    await waitFor(() =>
      expect(screen.getByText('Filtered to task')).toBeInTheDocument()
    )
  })
})

// ── Auto-refresh ──────────────────────────────────────────────────────────────
describe('Tasks — auto-refresh', () => {
  it('sets up 5-second polling interval', async () => {
    vi.useFakeTimers()
    stubFetch()
    render(Tasks)
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    const callsAfterMount = fetch.mock.calls.length

    await vi.advanceTimersByTimeAsync(5_500)
    expect(fetch.mock.calls.length).toBeGreaterThan(callsAfterMount)
  })
})
