/**
 * Component tests for src/pages/TaskDetail.svelte — B4: the task UI shows and
 * edits the review setup ("Any reviewer" vs a specific review role).
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import TaskDetail from '../pages/TaskDetail.svelte'

vi.mock('../lib/api.js', () => {
  const r = (v) => vi.fn().mockResolvedValue(v)
  return {
    getTask: vi.fn(),
    updateTask: vi.fn(),
    deleteTask: r({}),
    queueTask: r({}),
    unqueueTask: r({}),
    getProject: r({}),
    listTaskLogs: r([]),
    listTaskLinks: r([]),
    addTaskLink: r({}),
    removeTaskLink: r({}),
    listRequirements: r([]),
    listFeatures: r([]),
    listTaskDependencies: r([]),
    addTaskDependency: r({}),
    removeTaskDependency: r({}),
    listProjectTasks: r([]),
    listChecklistItems: r([]),
    createChecklistItem: r({}),
    updateChecklistItem: r({}),
    deleteChecklistItem: r({}),
    cloneChecklistIteration: r({}),
    listComments: r([]),
    createComment: r({}),
    deleteComment: r({}),
    listBranches: r([]),
    listCommits: r([]),
    readFile: r({}),
    deleteTaskLogs: r({}),
    getAgent: r(null),
    listLogs: r([]),
    getTaskCost: r(null),
    listPRs: r([]),
    createPR: r({ id: 'pr1' }),
    approvePR: r({}),
    rejectPR: r({}),
    getTaskRoles: vi.fn(),
  }
})

vi.mock('../lib/stores.js', () => ({
  toasts: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
  router: {
    go: vi.fn(),
    push: vi.fn(),
    subscribe: (fn) => { fn({ page: 'tasks', params: ['t1'] }); return () => {} },
  },
}))

import { getTask, getTaskRoles, updateTask, createPR } from '../lib/api.js'

const ROLES = [
  { id: 'w1', value: 'worker', label: 'Worker' },
  { id: 'rev-1', value: 'reviewer', label: 'Reviewer' },
]

const baseTask = (over = {}) => ({
  id: 't1', role: 'w1', review_role: '', status: 'DEVELOPING', priority: 5,
  payload: { title: 'Build auth', description: 'JWT flow' },
  project_id: '', assigned_agent_id: '',
  created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
  ...over,
})

beforeEach(() => {
  getTaskRoles.mockResolvedValue(ROLES)
  updateTask.mockResolvedValue(baseTask())
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, json: () => Promise.resolve([]) }))
})

afterEach(() => vi.unstubAllGlobals())

describe('TaskDetail — review setup display (B4)', () => {
  it('shows "Any reviewer" when review_role is unset', async () => {
    getTask.mockResolvedValue(baseTask({ review_role: '' }))
    render(TaskDetail, { props: { taskId: 't1' } })
    await waitFor(() =>
      expect(screen.getByTitle('Review setup')).toHaveTextContent('Any reviewer'),
    )
  })

  it('shows the review role when review_role is set', async () => {
    getTask.mockResolvedValue(baseTask({ review_role: 'rev-1' }))
    render(TaskDetail, { props: { taskId: 't1' } })
    await waitFor(() =>
      expect(screen.getByTitle('Review setup')).toHaveTextContent('reviewer'),
    )
  })
})

describe('TaskDetail — editing role/review (B4)', () => {
  it('saving sends role and review_role', async () => {
    getTask.mockResolvedValue(baseTask({ review_role: '' }))
    const user = userEvent.setup()
    render(TaskDetail, { props: { taskId: 't1' } })

    await waitFor(() => screen.getByRole('button', { name: 'Edit' }))
    await user.click(screen.getByRole('button', { name: 'Edit' }))

    await user.selectOptions(screen.getByRole('combobox', { name: 'Review role' }), 'rev-1')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(updateTask).toHaveBeenCalledWith(
        't1',
        expect.objectContaining({ role: 'w1', review_role: 'rev-1' }),
      )
    })
  })
})

describe('TaskDetail — Create PR button (reviewer-owned merge)', () => {
  it('shows Create PR for a task in review and calls createPR', async () => {
    getTask.mockResolvedValue(baseTask({ status: 'AWAITING_REVIEW' }))
    const user = userEvent.setup()
    render(TaskDetail, { props: { taskId: 't1' } })

    const btn = await screen.findByRole('button', { name: 'Create PR' })
    await user.click(btn)
    await waitFor(() => expect(createPR).toHaveBeenCalledWith('t1'))
  })

  it('does not show Create PR for a task not in review', async () => {
    getTask.mockResolvedValue(baseTask({ status: 'DEVELOPING' }))
    render(TaskDetail, { props: { taskId: 't1' } })
    await waitFor(() => screen.getByTitle('Review setup'))
    expect(screen.queryByRole('button', { name: 'Create PR' })).toBeNull()
  })
})
