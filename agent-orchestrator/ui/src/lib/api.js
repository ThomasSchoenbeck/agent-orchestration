/**
 * api.js — thin fetch wrapper for the Agent Orchestrator REST API.
 * All functions throw an Error with a human-readable message on failure.
 */

async function request(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  }
  if (body !== undefined) opts.body = JSON.stringify(body)

  const res = await fetch(path, opts)
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const j = await res.json()
      if (j.error) msg = j.error
    } catch (_) {}
    throw new Error(msg)
  }
  if (res.status === 204) return null
  return res.json()
}

const get  = (path)        => request('GET',    path)
const post = (path, body)  => request('POST',   path, body)
const put  = (path, body)  => request('PUT',    path, body)
const del  = (path)        => request('DELETE', path)

// ── Projects ────────────────────────────────────────────────────────────────
export const listProjects   = ()       => get('/api/projects')
export const getProject     = (id)     => get(`/api/projects/${id}`)
export const createProject  = (data)   => post('/api/projects', data)
export const updateProject  = (id, d)  => put(`/api/projects/${id}`, d)
export const deleteProject  = (id)     => del(`/api/projects/${id}`)

// ── Tasks ────────────────────────────────────────────────────────────────────
export const listTasks  = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/tasks${qs ? '?' + qs : ''}`)
}
export const getTask    = (id)    => get(`/api/tasks/${id}`)
export const createTask = (data)  => post('/api/tasks', data)
export const updateTask = (id, d) => put(`/api/tasks/${id}`, d)
export const deleteTask = (id)    => del(`/api/tasks/${id}`)

// ── Agents ───────────────────────────────────────────────────────────────────
export const listAgents  = () => get('/api/agents')
export const getAgent    = (id) => get(`/api/agents/${id}`)

// ── Providers ────────────────────────────────────────────────────────────────
export const listProviders = () => get('/api/providers')

// ── Logs ─────────────────────────────────────────────────────────────────────
export const listLogs = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/logs${qs ? '?' + qs : ''}`)
}

// ── Metrics ──────────────────────────────────────────────────────────────────
export const getMetrics = () => get('/api/metrics')

// ── LLM Chat (one-shot, non-WS) ──────────────────────────────────────────────
export const llmChat = (data) => post('/api/llm/chat', data)
