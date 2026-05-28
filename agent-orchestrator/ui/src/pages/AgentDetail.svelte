<script>
  import { onMount, onDestroy } from 'svelte'
  import { getAgent, getAgentStats, getAgentLogs, listTasks } from '../lib/api.js'
  import { formatTimestamp } from '../lib/time.js'
  import { toasts, router } from '../lib/stores.js'
  import Skeleton from '../components/Skeleton.svelte'

  let { agentId } = $props()

  // ── State ─────────────────────────────────────────────────────────────────
  let agent    = $state(null)
  let stats    = $state(null)
  let logs     = $state([])
  let tasks    = $state([])
  let loading  = $state(false)
  let activeTab = $state('logs')  // 'logs' | 'tasks'

  const statusDot = {
    online:  'bg-green-400',
    offline: 'bg-gray-500',
    busy:    'bg-yellow-400',
    idle:    'bg-blue-400',
  }

  const statusText = {
    online:  'text-green-400',
    offline: 'text-gray-500',
    busy:    'text-yellow-400',
    idle:    'text-blue-400',
  }

  const taskStatusColor = {
    COMPLETED: 'text-green-400',
    FAILED:    'text-red-400',
    DEVELOPING:'text-yellow-400',
    BACKLOG:   'text-gray-500',
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function load() {
    loading = true
    try {
      const [a, s, l, t] = await Promise.all([
        getAgent(agentId).catch(() => null),
        getAgentStats(agentId).catch(() => null),
        getAgentLogs(agentId).catch(() => []),
        listTasks({ agent_id: agentId, limit: 100 }).catch(() => []),
      ])
      agent = a
      stats = s
      logs  = Array.isArray(l) ? l : (l?.logs ?? [])
      tasks = Array.isArray(t) ? t : (t?.tasks ?? [])
    } catch (e) {
      toasts.error('Failed to load agent: ' + e.message)
    } finally {
      loading = false
    }
  }

  // ── Helpers ───────────────────────────────────────────────────────────────
  function fmtDuration(ms) {
    if (!ms) return '—'
    const s = Math.floor(ms / 1000)
    if (s < 60) return `${s}s`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m}m ${s % 60}s`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }

  function fmtUptime(ms) {
    if (!ms) return '—'
    const days = Math.floor(ms / 86_400_000)
    const hrs  = Math.floor((ms % 86_400_000) / 3_600_000)
    const mins = Math.floor((ms % 3_600_000) / 60_000)
    if (days > 0) return `${days}d ${hrs}h ${mins}m`
    if (hrs  > 0) return `${hrs}h ${mins}m`
    return `${mins}m`
  }

  const LEVEL_COLORS = {
    debug: 'text-gray-500',
    info:  'text-blue-400',
    warn:  'text-yellow-400',
    error: 'text-red-400',
  }

  let timer = null
  onMount(() => {
    load()
    timer = setInterval(load, 10_000)
  })
  onDestroy(() => clearInterval(timer))
</script>

<div class="flex-1 overflow-y-auto p-6">

  <!-- Header -->
  <div class="flex items-center gap-3 mb-5">
    <button
      class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
      onclick={() => router.go('agents')}
    >← Agents</button>
    {#if agent}
      <span class="w-2 h-2 rounded-full shrink-0 {statusDot[agent.status] ?? 'bg-gray-500'}"></span>
      <h1 class="text-xl font-semibold text-gray-100">{agent.name}</h1>
      <span class="text-xs px-1.5 py-0.5 rounded bg-surface-700 text-gray-400">{agent.mode}</span>
      {#each (agent.roles ?? []) as role}
        <span class="text-xs px-1.5 py-0.5 rounded bg-surface-700 text-blue-400">{role}</span>
      {/each}
      <span class="text-xs {statusText[agent.status] ?? 'text-gray-500'} ml-auto">{agent.status}</span>
    {:else if loading}
      <span class="text-gray-500">Loading…</span>
    {:else}
      <h1 class="text-xl font-semibold text-gray-400">Agent not found</h1>
    {/if}
  </div>

  {#if loading && !agent}
    <Skeleton rows={4} />
  {:else if agent}

    <!-- Stats grid -->
    {#if stats}
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          <div class="text-xs text-gray-500 mb-1">Uptime</div>
          <div class="text-lg font-semibold text-gray-100">{fmtUptime(stats.uptime_ms)}</div>
        </div>
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          <div class="text-xs text-gray-500 mb-1">Total tasks</div>
          <div class="text-lg font-semibold text-gray-100">{stats.total_tasks ?? '—'}</div>
        </div>
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          <div class="text-xs text-gray-500 mb-1">Completed / Failed</div>
          <div class="text-lg font-semibold">
            <span class="text-green-400">{stats.completed_tasks ?? 0}</span>
            <span class="text-gray-500"> / </span>
            <span class="text-red-400">{stats.failed_tasks ?? 0}</span>
          </div>
        </div>
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          <div class="text-xs text-gray-500 mb-1">Total tokens</div>
          <div class="text-lg font-semibold text-gray-100">{(stats.total_tokens ?? 0).toLocaleString()}</div>
        </div>
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          <div class="text-xs text-gray-500 mb-1">Avg task duration</div>
          <div class="text-lg font-semibold text-gray-100">{fmtDuration(stats.avg_task_ms)}</div>
        </div>
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          <div class="text-xs text-gray-500 mb-1">Registered</div>
          <div class="text-sm font-medium text-gray-300">{formatTimestamp(stats.registered_at)}</div>
        </div>
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          <div class="text-xs text-gray-500 mb-1">Last heartbeat</div>
          <div class="text-sm font-medium text-gray-300">{formatTimestamp(stats.last_heartbeat)}</div>
        </div>
        {#if agent.current_task_id}
          <div class="p-3 bg-surface-800 rounded border border-surface-600">
            <div class="text-xs text-gray-500 mb-1">Current task</div>
            <div class="text-sm font-mono text-yellow-400 truncate">{agent.current_task_id.slice(0, 12)}…</div>
          </div>
        {/if}
      </div>
    {/if}

    <!-- Tabs -->
    <div class="flex gap-1 mb-4 border-b border-surface-600 pb-0">
      {#each [['logs','Logs',logs.length],['tasks','Tasks',tasks.length]] as [id, label, count]}
        <button
          class="px-4 py-2 text-sm transition-colors border-b-2 -mb-px
                 {activeTab === id
                   ? 'border-accent text-accent'
                   : 'border-transparent text-gray-400 hover:text-gray-200'}"
          onclick={() => activeTab = id}
        >{label} <span class="text-xs text-gray-500">({count})</span></button>
      {/each}
    </div>

    <!-- Logs tab -->
    {#if activeTab === 'logs'}
      {#if logs.length === 0}
        <p class="text-gray-500 text-sm">No logs for this agent.</p>
      {:else}
        <div class="font-mono text-xs overflow-x-auto">
          <table class="w-full">
            <thead class="sticky top-0 bg-surface-800 z-10">
              <tr>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium w-28">Time</th>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium w-16">Level</th>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium">Message</th>
              </tr>
            </thead>
            <tbody>
              {#each logs as entry (entry.id ?? entry.timestamp)}
                <tr class="border-t border-surface-700 hover:bg-surface-700/20">
                  <td class="px-3 py-1 text-gray-600 whitespace-nowrap">{formatTimestamp(entry.timestamp ?? entry.created_at)}</td>
                  <td class="px-3 py-1 {LEVEL_COLORS[entry.level] ?? 'text-gray-400'}">{entry.level ?? '—'}</td>
                  <td class="px-3 py-1 text-gray-300 break-all">{entry.message ?? entry.event_type ?? ''}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {/if}

    <!-- Tasks tab -->
    {#if activeTab === 'tasks'}
      {#if tasks.length === 0}
        <p class="text-gray-500 text-sm">No tasks associated with this agent.</p>
      {:else}
        <div class="font-mono text-xs overflow-x-auto">
          <table class="w-full">
            <thead class="sticky top-0 bg-surface-800 z-10">
              <tr>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium w-28">ID</th>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium w-24">Type</th>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium w-20">Role</th>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium w-24">Status</th>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium w-20">Duration</th>
                <th class="text-left px-3 py-1.5 text-gray-500 font-medium">Created</th>
              </tr>
            </thead>
            <tbody>
              {#each tasks as task (task.id)}
                {@const durationMs = task.started_at && task.completed_at
                  ? new Date(task.completed_at) - new Date(task.started_at) : null}
                <tr class="border-t border-surface-700 hover:bg-surface-700/20">
                  <td class="px-3 py-1 text-gray-500">{task.id.slice(0, 8)}…</td>
                  <td class="px-3 py-1 text-gray-400">{task.type}</td>
                  <td class="px-3 py-1 text-gray-400">{task.role}</td>
                  <td class="px-3 py-1 {taskStatusColor[task.status] ?? 'text-gray-400'}">{task.status}</td>
                  <td class="px-3 py-1 text-gray-500">{fmtDuration(durationMs)}</td>
                  <td class="px-3 py-1 text-gray-600">{formatTimestamp(task.created_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {/if}

  {/if}
</div>
