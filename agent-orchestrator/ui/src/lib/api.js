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
export const taskChat    = (taskId, data)    => post(`/api/tasks/${taskId}/chat`, data)

// ── Project git file/tree/diff ───────────────────────────────────────────────
export const initRepo      = (projectId)                => post(`/api/projects/${projectId}/init-repo`)
export const listBranches  = (projectId)                => get(`/api/projects/${projectId}/branches`)
export const readTree      = (projectId, ref, path='')  => {
  const qs = new URLSearchParams({ ref, path }).toString()
  return get(`/api/projects/${projectId}/tree?${qs}`)
}
export const readFile      = (projectId, ref, path)     => {
  const qs = new URLSearchParams({ ref, path }).toString()
  return get(`/api/projects/${projectId}/file?${qs}`)
}
export const listCommits   = (projectId, ref)           =>
  get(`/api/projects/${projectId}/commits?ref=${encodeURIComponent(ref)}`)
export const commitFile    = (projectId, data)           => put(`/api/projects/${projectId}/file`, data)
export const commitFiles   = (projectId, data)           => post(`/api/projects/${projectId}/files`, data)
export const getFileDiff   = (projectId, base, head, path) => {
  const qs = new URLSearchParams({ base, head, path }).toString()
  return get(`/api/projects/${projectId}/diff?${qs}`)
}
export const getBranchDiff = (projectId, base, head)    => {
  const qs = new URLSearchParams({ base, head }).toString()
  return get(`/api/projects/${projectId}/diff?${qs}`)
}

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
export const queueTask   = (id)  => post(`/api/tasks/${id}/queue`)
export const unqueueTask = (id)  => post(`/api/tasks/${id}/unqueue`)
export const listTaskLogs = (taskId) => get(`/api/tasks/${taskId}/logs`)

// ── Agents ───────────────────────────────────────────────────────────────────
export const listAgents  = () => get('/api/agents')
export const getAgent      = (id) => get(`/api/agents/${id}`)
export const getAgentStats = (id) => get(`/api/agents/${id}/stats`)
export const getAgentLogs  = (id) => get(`/api/agents/${id}/logs`)

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
export const deleteLogs = (before) => {
  const qs = before ? '?before=' + encodeURIComponent(before) : ''
  return request('DELETE', `/api/logs${qs}`)
}
export const deleteTaskLogs = (taskId) =>
  request('DELETE', `/api/tasks/${taskId}/logs`)

// ── Metrics ──────────────────────────────────────────────────────────────────
export const getMetrics = () => get('/api/metrics')
export const getTaskCost = (taskId) => get(`/api/tasks/${taskId}/cost`)

// ── Meta (enumerations) ──────────────────────────────────────────────────────
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

// ── Chat log ──────────────────────────────────────────────────────────────────
export const listChatLog = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/chat-log${qs ? '?' + qs : ''}`)
}

// ── Agent logs ────────────────────────────────────────────────────────────────
export const listAgentLogs = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/agent-logs${qs ? '?' + qs : ''}`)
}
export const deleteAgentLogs = (before) => {
  const qs = before ? '?before=' + encodeURIComponent(before) : ''
  return request('DELETE', `/api/agent-logs${qs}`)
}

// ── Task logs (collection endpoint, distinct from per-task /api/tasks/:id/logs) ─
export const listAllTaskLogs = (params = {}) => {
  const qs = new URLSearchParams(params).toString()
  return get(`/api/task-logs${qs ? '?' + qs : ''}`)
}
export const deleteAllTaskLogs = (before) => {
  const qs = before ? '?before=' + encodeURIComponent(before) : ''
  return request('DELETE', `/api/task-logs${qs}`)
}

// ── Requirements ─────────────────────────────────────────────────────────────
export const listRequirements  = (projectId)       => get(`/api/projects/${projectId}/requirements`)
export const createRequirement = (projectId, data) => post(`/api/projects/${projectId}/requirements`, data)
export const updateRequirement = (projectId, id, data) => request('PATCH', `/api/projects/${projectId}/requirements/${id}`, data)
export const deleteRequirement = (projectId, id)   => del(`/api/projects/${projectId}/requirements/${id}`)

// ── Features ──────────────────────────────────────────────────────────────────
export const listFeatures  = (projectId)       => get(`/api/projects/${projectId}/features`)
export const createFeature = (projectId, data) => post(`/api/projects/${projectId}/features`, data)
export const updateFeature = (projectId, id, data) => request('PATCH', `/api/projects/${projectId}/features/${id}`, data)
export const deleteFeature = (projectId, id)   => del(`/api/projects/${projectId}/features/${id}`)

// ── Comments ──────────────────────────────────────────────────────────────────
export const listComments   = (taskId, reviewId = '') =>
  get(`/api/tasks/${taskId}/comments${reviewId ? '?review_id=' + reviewId : ''}`)
export const createComment  = (taskId, data)         => post(`/api/tasks/${taskId}/comments`, data)
export const deleteComment  = (taskId, commentId)    => del(`/api/tasks/${taskId}/comments/${commentId}`)

// ── Checklist ─────────────────────────────────────────────────────────────────
export const listChecklistItems     = (taskId)         => get(`/api/tasks/${taskId}/checklist`)
export const createChecklistItem    = (taskId, data)   => post(`/api/tasks/${taskId}/checklist`, data)
export const updateChecklistItem    = (taskId, id, d)  => request('PATCH', `/api/tasks/${taskId}/checklist/${id}`, d)
export const deleteChecklistItem    = (taskId, id)     => del(`/api/tasks/${taskId}/checklist/${id}`)
export const cloneChecklistIteration = (taskId)        => post(`/api/tasks/${taskId}/checklist/iterations`)
export const listChecklistTemplates  = ()              => get('/api/checklist-templates')
export const createChecklistTemplate = (data)          => post('/api/checklist-templates', data)
export const updateChecklistTemplate = (id, d)         => put(`/api/checklist-templates/${id}`, d)
export const deleteChecklistTemplate = (id)            => del(`/api/checklist-templates/${id}`)

// ── Task dependencies ─────────────────────────────────────────────────────────
export const listTaskDependencies  = (taskId)              => get(`/api/tasks/${taskId}/dependencies`)
export const addTaskDependency     = (taskId, dependsOnId) => post(`/api/tasks/${taskId}/dependencies`, { depends_on_id: dependsOnId })
export const removeTaskDependency  = (taskId, dependsOnId) => request('DELETE', `/api/tasks/${taskId}/dependencies`, { depends_on_id: dependsOnId })

// ── Task links ────────────────────────────────────────────────────────────────
export const listTaskLinks  = (taskId)              => get(`/api/tasks/${taskId}/links`)
export const addTaskLink    = (taskId, kind, targetId) => post(`/api/tasks/${taskId}/links`, { kind, target_id: targetId })
export const removeTaskLink = (taskId, kind, targetId) => request('DELETE', `/api/tasks/${taskId}/links`, { kind, target_id: targetId })

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
