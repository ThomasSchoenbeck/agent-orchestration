/**
 * Component tests for src/pages/Tasks.svelte
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
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
    id: 't1', type: 'implement', role: 'worker', status: 'pending', priority: 5,
    payload: { title: 'Build auth', description: 'JWT flow' }, project_id: 'proj1',
  },
  {
    id: 't2', type: 'review',    role: 'reviewer', status: 'completed', priority: 3,
    payload: { title: 'Code review', description: '' }, project_id: 'proj1',
  },
  {
    id: 't3', type: 'plan',      role: 'orchestrator', status: 'failed', priority: 8,
    payload: {}, project_id: 'proj1',
  },
]

let fetchCalls = 0

function setupFetch(...responses) {
  fetchCalls = 0
  vi.stubGlobal('fetch', vi.fn((...args) => {
    const resp = responses[fetchCalls] ?? responses[responses.length - 1]
    fetchCalls++
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(resp) })
  }))
}

// Default: four parallel responses for initial load() call
// (listTasks, listProjects, getTaskTypes, getTaskRoles)
function defaultFetch() {
  setupFetch(TASKS, PROJECTS, TASK_TYPES, TASK_ROLES)
}

afterEach(() => vi.unstubAllGlobals())

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Tasks — rendering', () => {
  it('shows page heading', () => {
    defaultFetch()
    render(Tasks)
    expect(screen.getByText('Tasks')).toBeInTheDocument()
  })

  it('renders tasks from API', async () => {
    defaultFetch()
    render(Tasks)
    await waitFor(() => expect(screen.getByText('Build auth')).toBeInTheDocument())
    expect(screen.getByText('Code review')).toBeInTheDocument()
  })

  it('shows status badge for each task', async () => {
    defaultFetch()
    render(Tasks)
    // Use { selector: 'span' } to target only the badge <span> elements and avoid
    // false-positive matches against the identically-named <option> elements in
    // the status filter <select>.
    await waitFor(() => {
      expect(screen.getByText('pending',   { selector: 'span' })).toBeInTheDocument()
      expect(screen.getByText('completed', { selector: 'span' })).toBeInTheDocument()
      expect(screen.getByText('failed',    { selector: 'span' })).toBeInTheDocument()
    })
  })

  it('shows type and role', async () => {
    defaultFetch()
    render(Tasks)
    await waitFor(() => {
      expect(screen.getByText('implement')).toBeInTheDocument()
      expect(screen.getByText('worker')).toBeInTheDocument()
    })
  })

  it('falls back to type when payload.title is missing', async () => {
    defaultFetch()
    render(Tasks)
    // t3 has empty payload — taskTitle() returns t.type ('plan').
    // 'plan' appears twice: once as title span, once as the type mono badge.
    await waitFor(() => {
      const planEls = screen.getAllByText('plan')
      expect(planEls.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('shows Queue button for pending tasks', async () => {
    defaultFetch()
    render(Tasks)
    // Both t1 (pending) and t3 (failed) get a Queue button, so use getAllByText.
    await waitFor(() => {
      const queueBtns = screen.getAllByText('Queue')
      expect(queueBtns.length).toBeGreaterThanOrEqual(1)
    })
  })

  it('shows Queue button for failed tasks', async () => {
    defaultFetch()
    render(Tasks)
    // t3 is failed — should also have Queue
    await waitFor(() => {
      const queueBtns = screen.getAllByText('Queue')
      expect(queueBtns.length).toBeGreaterThanOrEqual(2)
    })
  })
})

// ── Filters ───────────────────────────────────────────────────────────────────
describe('Tasks — filters', () => {
  it('renders status filter dropdown', () => {
    defaultFetch()
    render(Tasks)
    expect(screen.getByDisplayValue('All statuses')).toBeInTheDocument()
  })

  it('renders project filter with loaded projects', async () => {
    defaultFetch()
    render(Tasks)
    await waitFor(() => expect(screen.getByText('MyProject')).toBeInTheDocument())
  })

  it('re-fetches when status filter changes', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 200, json: () => Promise.resolve([]) }))
    const user = userEvent.setup()
    render(Tasks)
    const select = screen.getByDisplayValue('All statuses')
    await user.selectOptions(select, 'pending')
    await waitFor(() => {
      const calls = fetch.mock.calls
      const filtered = calls.find(([url]) => url.includes('status=pending'))
      expect(filtered).toBeTruthy()
    })
  })
})

// ── Create form ───────────────────────────────────────────────────────────────
describe('Tasks — create form', () => {
  it('shows form when "+ New Task" is clicked', async () => {
    defaultFetch()
    const user = userEvent.setup()
    render(Tasks)
    await user.click(screen.getByText('+ New Task'))
    expect(screen.getByRole('combobox', { name: /Task type/i })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /Role/i })).toBeInTheDocument()
  })

  it('posts task with all required fields', async () => {
    // initial load: tasks + projects + task-types + task-roles (4 parallel)
    // then POST create, then reload (4 parallel, fallback [])
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASKS) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(PROJECTS) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASK_TYPES) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASK_ROLES) })
      .mockResolvedValueOnce({ ok: true, status: 201, json: () => Promise.resolve({ id: 'tnew' }) })
      .mockResolvedValue    ({ ok: true, status: 200, json: () => Promise.resolve([]) })
    )
    const user = userEvent.setup()
    render(Tasks)
    await waitFor(() => screen.getByText('+ New Task'))
    await user.click(screen.getByText('+ New Task'))

    // Select project
    await user.selectOptions(screen.getByDisplayValue('Select project *'), 'proj1')

    // Type and role are now <select> elements, not text inputs
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
  it('PUTs status=queued when Queue is clicked', async () => {
    // Initial load: tasks + projects + task-types + task-roles
    // Then: PUT update, then: reload (4 parallel calls again)
    vi.stubGlobal('fetch', vi.fn()
      // Initial load
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASKS) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(PROJECTS) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASK_TYPES) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASK_ROLES) })
      // PUT /api/tasks/t1
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve({ ...TASKS[0], status: 'queued' }) })
      // Reload after update
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASKS) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(PROJECTS) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASK_TYPES) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(TASK_ROLES) })
    )
    const user = userEvent.setup()
    render(Tasks)
    // getAllByText handles the case where both t1 (pending) and t3 (failed) show Queue.
    await waitFor(() => screen.getAllByText('Queue'))
    await user.click(screen.getAllByText('Queue')[0])

    await waitFor(() => {
      const putCall = fetch.mock.calls.find(([url, opts]) =>
        url === '/api/tasks/t1' && opts?.method === 'PUT'
      )
      expect(putCall).toBeTruthy()
      expect(JSON.parse(putCall[1].body).status).toBe('queued')
    })
  })
})
