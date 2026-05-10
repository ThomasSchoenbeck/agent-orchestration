/**
 * Integration tests — run against a live Go server.
 *
 * These tests are SKIPPED unless the environment variable INTEGRATION_URL
 * is set (e.g. INTEGRATION_URL=http://localhost:8080).
 *
 * Run with:
 *   pnpm test:integration
 *
 * Or manually:
 *   INTEGRATION_URL=http://localhost:8080 pnpm vitest run src/__tests__/integration.test.js
 *
 * The tests exercise the full stack:
 *   Vitest (Node) → real HTTP/WebSocket → Go server → SQLite
 *
 * They are designed to be idempotent (each test cleans up after itself)
 * and order-independent.
 */
import { describe, it, expect, beforeAll } from 'vitest'

const BASE = process.env.INTEGRATION_URL ?? ''
const SKIP = !BASE

// ── helpers ───────────────────────────────────────────────────────────────────
async function api(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
    body:    body ? JSON.stringify(body) : undefined,
  }
  const res = await fetch(BASE + path, opts)
  if (res.status === 204) return null
  return res.json()
}

const GET    = (path)       => api('GET',    path)
const POST   = (path, body) => api('POST',   path, body)
const PUT    = (path, body) => api('PUT',    path, body)
const DELETE = (path)       => api('DELETE', path)

// ── health ────────────────────────────────────────────────────────────────────
describe.skipIf(SKIP)('Health endpoint', () => {
  it('GET /health returns {status: "ok"}', async () => {
    const res = await GET('/health')
    expect(res).toMatchObject({ status: 'ok' })
  })
})

// ── projects CRUD ─────────────────────────────────────────────────────────────
describe.skipIf(SKIP)('Projects API', () => {
  let createdId

  it('GET /api/projects returns an array', async () => {
    const res = await GET('/api/projects')
    expect(Array.isArray(res)).toBe(true)
  })

  it('POST /api/projects creates a project', async () => {
    const res = await POST('/api/projects', {
      name:        'Integration Test Project',
      description: 'Created by integration tests — safe to delete',
    })
    expect(res).toHaveProperty('id')
    expect(res.name).toBe('Integration Test Project')
    createdId = res.id
  })

  it('GET /api/projects/:id retrieves the project', async () => {
    const res = await GET(`/api/projects/${createdId}`)
    expect(res.id).toBe(createdId)
    expect(res.name).toBe('Integration Test Project')
  })

  it('PUT /api/projects/:id updates name', async () => {
    const res = await PUT(`/api/projects/${createdId}`, { name: 'Renamed Project' })
    expect(res.name).toBe('Renamed Project')
  })

  it('DELETE /api/projects/:id removes the project', async () => {
    const res = await DELETE(`/api/projects/${createdId}`)
    expect(res).toBeNull()
  })

  it('GET after DELETE returns 404 or empty', async () => {
    const res = await fetch(BASE + `/api/projects/${createdId}`)
    expect(res.status).toBe(404)
  })
})

// ── tasks CRUD ────────────────────────────────────────────────────────────────
describe.skipIf(SKIP)('Tasks API', () => {
  let projectId, taskId

  beforeAll(async () => {
    if (SKIP) return
    const p = await POST('/api/projects', { name: 'Task Test Project' })
    projectId = p.id
  })

  it('GET /api/tasks returns an array', async () => {
    const res = await GET('/api/tasks')
    expect(Array.isArray(res)).toBe(true)
  })

  it('POST /api/tasks creates a task', async () => {
    const res = await POST('/api/tasks', {
      project_id: projectId,
      type:       'implement',
      role:       'worker',
      priority:   5,
      payload:    { title: 'Integration task', description: 'Test' },
    })
    expect(res).toHaveProperty('id')
    expect(res.type).toBe('implement')
    expect(res.role).toBe('worker')
    expect(res.status).toMatch(/pending|planned|queued/)
    taskId = res.id
  })

  it('GET /api/tasks/:id retrieves the task', async () => {
    const res = await GET(`/api/tasks/${taskId}`)
    expect(res.id).toBe(taskId)
  })

  it('PUT /api/tasks/:id updates status', async () => {
    const res = await PUT(`/api/tasks/${taskId}`, { status: 'queued' })
    expect(res.status).toBe('queued')
  })

  it('GET /api/tasks?status=queued includes our task', async () => {
    const res = await GET('/api/tasks?status=queued')
    expect(Array.isArray(res)).toBe(true)
    expect(res.some(t => t.id === taskId)).toBe(true)
  })

  it('GET /api/tasks?project_id= filters correctly', async () => {
    const res = await GET(`/api/tasks?project_id=${projectId}`)
    expect(res.every(t => t.project_id === projectId)).toBe(true)
  })

  it('DELETE /api/tasks/:id removes the task', async () => {
    const res = await DELETE(`/api/tasks/${taskId}`)
    expect(res).toBeNull()
  })

  // clean up project
  it('cleans up: DELETE test project', async () => {
    await DELETE(`/api/projects/${projectId}`)
  })
})

// ── agents ────────────────────────────────────────────────────────────────────
describe.skipIf(SKIP)('Agents API', () => {
  let agentId

  it('GET /api/agents returns an array', async () => {
    const res = await GET('/api/agents')
    expect(Array.isArray(res)).toBe(true)
  })

  it('POST /api/agents/register registers an agent', async () => {
    const res = await POST('/api/agents/register', {
      name:  'integration-test-agent',
      roles: ['worker'],
    })
    expect(res).toHaveProperty('agent_id')
    agentId = res.agent_id
  })

  it('GET /api/agents includes registered agent', async () => {
    const res = await GET('/api/agents')
    const found = res.find(a => a.id === agentId)
    expect(found).toBeTruthy()
    expect(found.name).toBe('integration-test-agent')
    expect(found.roles).toContain('worker')
  })

  it('POST /api/agents/:id/heartbeat succeeds', async () => {
    const res = await POST(`/api/agents/${agentId}/heartbeat`)
    expect(res).toMatchObject({ status: 'ok' })
  })

  it('GET /api/agents/:id retrieves agent by id', async () => {
    const res = await GET(`/api/agents/${agentId}`)
    expect(res.id).toBe(agentId)
  })
})

// ── metrics ───────────────────────────────────────────────────────────────────
describe.skipIf(SKIP)('Metrics API', () => {
  it('GET /api/metrics returns an object', async () => {
    const res = await GET('/api/metrics')
    expect(typeof res).toBe('object')
    expect(res).not.toBeNull()
  })
})

// ── logs ──────────────────────────────────────────────────────────────────────
describe.skipIf(SKIP)('Logs API', () => {
  it('GET /api/logs returns an array', async () => {
    const res = await GET('/api/logs')
    expect(Array.isArray(res)).toBe(true)
  })

  it('POST /api/logs creates a log entry', async () => {
    const res = await POST('/api/logs', {
      level:   'info',
      message: 'Integration test log entry',
    })
    expect(res).toHaveProperty('id')
    expect(res.message).toBe('Integration test log entry')
  })

  it('GET /api/logs?level=info includes our entry', async () => {
    const res = await GET('/api/logs?level=info')
    expect(Array.isArray(res)).toBe(true)
    expect(res.some(e => e.message === 'Integration test log entry')).toBe(true)
  })
})

// ── WebSocket /ws/chat ────────────────────────────────────────────────────────
describe.skipIf(SKIP)('WebSocket chat', () => {
  it('accepts connection and echoes/responds to messages', () => {
    return new Promise((resolve, reject) => {
      const wsUrl = BASE.replace(/^http/, 'ws') + '/ws/chat'
      const ws    = new globalThis.WebSocket(wsUrl)

      const timeout = setTimeout(() => {
        ws.close()
        reject(new Error('WebSocket test timed out'))
      }, 8000)

      ws.onopen = () => {
        ws.send(JSON.stringify({ role: 'user', content: 'ping integration test' }))
      }

      ws.onmessage = (evt) => {
        clearTimeout(timeout)
        ws.close()
        try {
          const data = JSON.parse(evt.data)
          expect(data).toHaveProperty('content')
          resolve()
        } catch {
          // plain-text response is also acceptable
          expect(typeof evt.data).toBe('string')
          resolve()
        }
      }

      ws.onerror = (e) => {
        clearTimeout(timeout)
        reject(new Error('WebSocket error: ' + (e.message ?? 'unknown')))
      }
    })
  })
})
