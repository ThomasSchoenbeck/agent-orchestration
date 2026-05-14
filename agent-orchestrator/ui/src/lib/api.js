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
export const projectChat = (projectId, data) => post(`/api/projects/${projectId}/chat`, data)

// ── Tasks ────────────────────────────────────────────────────────────────────
export const listTasks        = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/tasks${qs ? '?' + qs : ''}`)
}
export const listProjectTasks = (projectId, params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/projects/${projectId}/tasks${qs ? '?' + qs : ''}`)
}
export const getTask    = (id)    => get(`/api/tasks/${id}`)
export const createTask = (data)  => post('/api/tasks', data)
export const updateTask = (id, d) => put(`/api/tasks/${id}`, d)
export const deleteTask = (id)    => del(`/api/tasks/${id}`)
export const unqueueTask = (id)  => post(`/api/tasks/${id}/unqueue`)
export const listTaskLogs = (taskId) => get(`/api/tasks/${taskId}/logs`)

// ── Agents ───────────────────────────────────────────────────────────────────
export const listAgents  = () => get('/api/agents')
export const getAgent    = (id) => get(`/api/agents/${id}`)

// ── Providers ────────────────────────────────────────────────────────────────
export const listProviders   = ()        => get('/api/providers')
export const getProvider     = (id)      => get(`/api/providers/${id}`)
export const createProvider  = (data)    => post('/api/providers', data)
export const updateProvider  = (id, d)   => put(`/api/providers/${id}`, d)
export const deleteProvider  = (id)      => del(`/api/providers/${id}`)
export const testProvider    = (id)      => post(`/api/providers/${id}/test`)
export const seedProviders   = ()        => post('/api/providers/seed')

// ── Roles ────────────────────────────────────────────────────────────────────
export const listRoles        = ()        => get('/api/roles')
export const getRole          = (id)      => get(`/api/roles/${id}`)
export const createRole       = (data)    => post('/api/roles', data)
export const updateRole       = (id, d)   => put(`/api/roles/${id}`, d)
export const deleteRole       = (id)      => del(`/api/roles/${id}`)
export const seedRoles        = ()        => post('/api/roles/seed')
export const previewRolePrompt = (id, vars) => post(`/api/roles/${id}/preview-prompt`, vars)

// ── Logs ─────────────────────────────────────────────────────────────────────
export const listLogs = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/logs${qs ? '?' + qs : ''}`)
}

// ── Metrics ──────────────────────────────────────────────────────────────────
export const getMetrics = () => get('/api/metrics')

// ── Meta (enumerations) ──────────────────────────────────────────────────────
export const getTaskTypes = () => get('/api/meta/task-types')
export const getTaskRoles = () => get('/api/meta/task-roles')

// ── Conversations ───────────────────────────────────────────────────────────
export const listConversations = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/conversations${qs ? '?' + qs : ''}`)
}
export const getConversation = (id, messageLimit = 50) =>
  get(`/api/conversations/${id}?message_limit=${messageLimit}`)
export const createConversation = (data) => post('/api/conversations', data)
export const updateConversation = (id, d) => put(`/api/conversations/${id}`, d)
export const deleteConversation = (id) => del(`/api/conversations/${id}`)
export const listMessages = (conversationId, params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/conversations/${conversationId}/messages${qs ? '?' + qs : ''}`)
}
export const addMessage = (conversationId, data) =>
  post(`/api/conversations/${conversationId}/messages`, data)

// ── LLM Chat (one-shot, non-WS) ──────────────────────────────────────────────
export const llmChat = (data) => post('/api/llm/chat', data)

// ── Agent logs ────────────────────────────────────────────────────────────────
export const listAgentLogs = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/agent-logs${qs ? '?' + qs : ''}`)
}

// ── Task logs (collection endpoint, distinct from per-task /api/tasks/:id/logs) ─
export const listAllTaskLogs = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/task-logs${qs ? '?' + qs : ''}`)
}

// ── Settings ─────────────────────────────────────────────────────────────────
export async function listSettings() {
  const r = await fetch('/api/settings', { method: 'GET' })
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}

export async function updateSetting(key, value) {
  const r = await fetch(`/api/settings/${encodeURIComponent(key)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value: String(value) }),
  })
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}
