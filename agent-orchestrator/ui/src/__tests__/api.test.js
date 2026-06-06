/**
 * Unit tests for src/lib/api.js
 * Every test stubs globalThis.fetch and asserts the wrapper calls the
 * correct URL, method, and body, and handles error responses.
 */
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  listProjects, getProject, createProject, updateProject, deleteProject,
  listTasks,    getTask,    createTask,    updateTask,    deleteTask,
  listAgents,   getAgent,
  listProviders,
  listLogs,
  getMetrics,
  llmChat,
  listAgentLogs,
  listAllTaskLogs,
  listSettings,
  updateSetting,
  listBranches,
  deleteBranch,
  getCostBreakdown,
} from '../lib/api.js'

// ── helpers ───────────────────────────────────────────────────────────────────
function stubFetch(body, opts = {}) {
  const { status = 200, ok = true } = opts
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok,
    status,
    json: () => Promise.resolve(body),
  }))
}

function stubFetchError(message = 'Server error') {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok:     false,
    status: 500,
    json:   () => Promise.resolve({ error: message }),
  }))
}

afterEach(() => vi.unstubAllGlobals())

// ── Projects ──────────────────────────────────────────────────────────────────
describe('listProjects', () => {
  it('GET /api/projects', async () => {
    stubFetch([{ id: '1', name: 'Proj' }])
    const res = await listProjects()
    expect(fetch).toHaveBeenCalledWith('/api/projects', expect.objectContaining({ method: 'GET' }))
    expect(res).toEqual([{ id: '1', name: 'Proj' }])
  })

  it('throws on non-ok response', async () => {
    stubFetchError('not found')
    await expect(listProjects()).rejects.toThrow('not found')
  })
})

describe('getProject', () => {
  it('GET /api/projects/:id', async () => {
    stubFetch({ id: 'abc', name: 'X' })
    await getProject('abc')
    expect(fetch).toHaveBeenCalledWith('/api/projects/abc', expect.anything())
  })
})

describe('createProject', () => {
  it('POST /api/projects with body', async () => {
    stubFetch({ id: '2', name: 'New' }, { status: 201 })
    await createProject({ name: 'New', description: 'desc' })
    const [url, opts] = fetch.mock.calls[0]
    expect(url).toBe('/api/projects')
    expect(opts.method).toBe('POST')
    expect(JSON.parse(opts.body)).toEqual({ name: 'New', description: 'desc' })
  })
})

describe('updateProject', () => {
  it('PUT /api/projects/:id', async () => {
    stubFetch({ id: '1' })
    await updateProject('1', { name: 'Updated' })
    const [url, opts] = fetch.mock.calls[0]
    expect(url).toBe('/api/projects/1')
    expect(opts.method).toBe('PUT')
  })
})

describe('deleteProject', () => {
  it('DELETE /api/projects/:id — returns null on 204', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 204 }))
    const res = await deleteProject('1')
    expect(res).toBeNull()
    expect(fetch).toHaveBeenCalledWith('/api/projects/1', expect.objectContaining({ method: 'DELETE' }))
  })
})

// ── Tasks ─────────────────────────────────────────────────────────────────────
describe('listTasks', () => {
  it('GET /api/tasks without params', async () => {
    stubFetch([])
    await listTasks()
    expect(fetch).toHaveBeenCalledWith('/api/tasks', expect.anything())
  })

  it('GET /api/tasks?status=pending&project_id=p1', async () => {
    stubFetch([])
    await listTasks({ status: 'pending', project_id: 'p1' })
    const [url] = fetch.mock.calls[0]
    expect(url).toContain('status=pending')
    expect(url).toContain('project_id=p1')
  })
})

describe('createTask', () => {
  it('POST /api/tasks with required fields', async () => {
    stubFetch({ id: 't1' }, { status: 201 })
    await createTask({ project_id: 'p1', type: 'implement', role: 'worker', priority: 5, payload: {} })
    const [url, opts] = fetch.mock.calls[0]
    expect(url).toBe('/api/tasks')
    expect(opts.method).toBe('POST')
    const body = JSON.parse(opts.body)
    expect(body.type).toBe('implement')
    expect(body.role).toBe('worker')
  })
})

describe('updateTask', () => {
  it('PUT /api/tasks/:id', async () => {
    stubFetch({ id: 't1', status: 'queued' })
    await updateTask('t1', { status: 'queued' })
    expect(fetch).toHaveBeenCalledWith('/api/tasks/t1', expect.objectContaining({ method: 'PUT' }))
  })
})

describe('deleteTask', () => {
  it('DELETE /api/tasks/:id', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 204 }))
    await deleteTask('t1')
    expect(fetch).toHaveBeenCalledWith('/api/tasks/t1', expect.objectContaining({ method: 'DELETE' }))
  })
})

// ── Agents ────────────────────────────────────────────────────────────────────
describe('listAgents', () => {
  it('GET /api/agents', async () => {
    stubFetch([{ id: 'a1', name: 'worker', status: 'online' }])
    const res = await listAgents()
    expect(res).toHaveLength(1)
    expect(fetch).toHaveBeenCalledWith('/api/agents', expect.anything())
  })
})

describe('getAgent', () => {
  it('GET /api/agents/:id', async () => {
    stubFetch({ id: 'a1' })
    await getAgent('a1')
    expect(fetch).toHaveBeenCalledWith('/api/agents/a1', expect.anything())
  })
})

// ── Providers ─────────────────────────────────────────────────────────────────
describe('listProviders', () => {
  it('GET /api/providers', async () => {
    stubFetch([{ id: 'p1', name: 'openai', type: 'openai' }])
    await listProviders()
    expect(fetch).toHaveBeenCalledWith('/api/providers', expect.anything())
  })
})

// ── Logs ──────────────────────────────────────────────────────────────────────
describe('listLogs', () => {
  it('GET /api/logs without params', async () => {
    stubFetch([])
    await listLogs()
    expect(fetch).toHaveBeenCalledWith('/api/logs', expect.anything())
  })

  it('GET /api/logs?level=error&limit=50', async () => {
    stubFetch([])
    await listLogs({ level: 'error', limit: 50 })
    const [url] = fetch.mock.calls[0]
    expect(url).toContain('level=error')
    expect(url).toContain('limit=50')
  })
})

// ── Metrics ───────────────────────────────────────────────────────────────────
describe('getMetrics', () => {
  it('GET /api/metrics', async () => {
    stubFetch({ total_tasks: 5, completed: 3 })
    const res = await getMetrics()
    expect(res.total_tasks).toBe(5)
    expect(fetch).toHaveBeenCalledWith('/api/metrics', expect.anything())
  })
})

// ── LLM chat ──────────────────────────────────────────────────────────────────
describe('llmChat', () => {
  it('POST /api/llm/chat', async () => {
    stubFetch({ role: 'assistant', content: 'Hello' })
    await llmChat({ role: 'orchestrator', messages: [{ role: 'user', content: 'Hi' }] })
    const [url, opts] = fetch.mock.calls[0]
    expect(url).toBe('/api/llm/chat')
    expect(opts.method).toBe('POST')
    const body = JSON.parse(opts.body)
    expect(body.role).toBe('orchestrator')
  })
})

// ── Agent logs ────────────────────────────────────────────────────────────────
describe('listAgentLogs', () => {
  it('GET /api/agent-logs with no params', async () => {
    stubFetch([])
    await listAgentLogs()
    const [url] = fetch.mock.calls[0]
    expect(url).toBe('/api/agent-logs')
  })

  it('appends agent_id param when provided', async () => {
    stubFetch([])
    await listAgentLogs({ agent_id: 'a1' })
    const [url] = fetch.mock.calls[0]
    expect(url).toContain('agent_id=a1')
  })

  it('appends event_type param when provided', async () => {
    stubFetch([])
    await listAgentLogs({ event_type: 'agent_registered', limit: 100 })
    const [url] = fetch.mock.calls[0]
    expect(url).toContain('event_type=agent_registered')
    expect(url).toContain('limit=100')
  })
})

// ── Task logs (collection) ────────────────────────────────────────────────────
describe('listAllTaskLogs', () => {
  it('GET /api/task-logs with no params', async () => {
    stubFetch([])
    await listAllTaskLogs()
    const [url] = fetch.mock.calls[0]
    expect(url).toBe('/api/task-logs')
  })

  it('appends task_id param when provided', async () => {
    stubFetch([])
    await listAllTaskLogs({ task_id: 't1', limit: 50 })
    const [url] = fetch.mock.calls[0]
    expect(url).toContain('task_id=t1')
    expect(url).toContain('limit=50')
  })
})

// ── Settings ──────────────────────────────────────────────────────────────────
describe('listSettings', () => {
  it('GET /api/settings', async () => {
    stubFetch([{ key: 'foo', value: '1', description: '' }])
    const result = await listSettings()
    expect(fetch).toHaveBeenCalledWith('/api/settings', expect.anything())
    expect(Array.isArray(result)).toBe(true)
  })

  it('throws on non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      text: () => Promise.resolve('Internal Server Error'),
    }))
    await expect(listSettings()).rejects.toThrow()
  })
})

describe('updateSetting', () => {
  it('PUT /api/settings/:key with value', async () => {
    stubFetch({ key: 'log.retention.agent.default_days', value: '30' })
    await updateSetting('log.retention.agent.default_days', '30')
    const [url, opts] = fetch.mock.calls[0]
    expect(url).toContain('/api/settings/log.retention.agent.default_days')
    expect(opts.method).toBe('PUT')
    const body = JSON.parse(opts.body)
    expect(body.value).toBe('30')
  })

  it('encodes key in URL', async () => {
    stubFetch({ key: 'a.b', value: 'x' })
    await updateSetting('a.b', 'x')
    const [url] = fetch.mock.calls[0]
    expect(url).toContain('/api/settings/a.b')
  })

  it('throws on non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      text: () => Promise.resolve('not found'),
    }))
    await expect(updateSetting('missing', '1')).rejects.toThrow()
  })
})

describe('branches', () => {
  it('GET /api/projects/:id/branches', async () => {
    stubFetch(['main', 'task/abc'])
    const res = await listBranches('p1')
    expect(fetch).toHaveBeenCalledWith('/api/projects/p1/branches', expect.anything())
    expect(res).toEqual(['main', 'task/abc'])
  })

  it('DELETE /api/projects/:id/branches?name= (url-encoded)', async () => {
    stubFetch(null, { status: 204 })
    await deleteBranch('p1', 'task/abc')
    const [url, opts] = fetch.mock.calls[0]
    expect(url).toBe('/api/projects/p1/branches?name=task%2Fabc')
    expect(opts.method).toBe('DELETE')
  })
})

describe('cost breakdown', () => {
  it('GET /api/metrics/costs?group_by=', async () => {
    stubFetch([{ key: 'agent', cost: 0.1, count: 2 }])
    const res = await getCostBreakdown('agent_role')
    expect(fetch).toHaveBeenCalledWith('/api/metrics/costs?group_by=agent_role', expect.anything())
    expect(res[0].key).toBe('agent')
  })
})
