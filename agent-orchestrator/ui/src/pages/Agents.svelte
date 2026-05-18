<script>
  import { onMount, onDestroy } from 'svelte'
  import { listAgents, listRoles, listAgentLogs } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'
  import Skeleton from '../components/Skeleton.svelte'

  // ── Agent list state ───────────────────────────────────────────────────────
  let agents         = $state([])
  let roles          = $state([])
  let loadingAgents  = $state(false)
  let selectedAgentId = $state('')

  const statusDot = {
    online:  'bg-green-400',
    offline: 'bg-gray-500',
    busy:    'bg-yellow-400',
    idle:    'bg-blue-400',
  }

  // ── Log panel state ────────────────────────────────────────────────────────
  let agentLogs      = $state([])
  let logsLoading    = $state(false)
  let logEventFilter = $state('')
  let logSearch      = $state('')
  let hiddenTypes    = $state(new Set())
  let chartTypeFilter = $state('')
  let bucketMinutes  = $state(60)

  const AGENT_COLORS = {
    agent_registered:        '#34d399',
    agent_poll_task_found:   '#60a5fa',
    agent_claim_attempt:     '#a78bfa',
    agent_claim_success:     '#4ade80',
    agent_claim_failed:      '#f87171',
    agent_execute_start:     '#fb923c',
    agent_llm_call:          '#38bdf8',
    agent_tool_call:         '#facc15',
    agent_tool_error:        '#f43f5e',
    agent_context_overflow:  '#e879f9',
    agent_reasoning_step:    '#94a3b8',
    agent_retry_backoff:     '#f97316',
    agent_human_approval_req:'#c084fc',
    agent_execute_complete:  '#22c55e',
    agent_execute_failed:    '#ef4444',
    agent_offline:           '#6b7280',
  }

  // ── Data loading ───────────────────────────────────────────────────────────
  async function loadAgents() {
    loadingAgents = true
    try {
      const [agentsRes, rolesRes] = await Promise.all([listAgents(), listRoles()])
      agents = Array.isArray(agentsRes) ? agentsRes : (agentsRes.agents ?? [])
      roles  = Array.isArray(rolesRes)  ? rolesRes  : (rolesRes.roles  ?? [])
    } catch (e) {
      toasts.error('Failed to load agents: ' + e.message)
    } finally {
      loadingAgents = false
    }
  }

  async function fetchLogs() {
    logsLoading = true
    try {
      const params = { limit: 500 }
      if (selectedAgentId) params.agent_id   = selectedAgentId
      if (logEventFilter)  params.event_type = logEventFilter
      if (logSearch)       params.search     = logSearch
      agentLogs = (await listAgentLogs(params)) ?? []
    } catch (e) {
      agentLogs = []
    } finally {
      logsLoading = false
    }
  }

  async function refreshAll() {
    await Promise.all([loadAgents(), fetchLogs()])
  }

  function resolveRole(roleName) {
    return roles.find(r => r.name === roleName)
  }

  function selectAgent(id) {
    selectedAgentId = selectedAgentId === id ? '' : id
    fetchLogs()
  }

  function toggleType(t) {
    const s = new Set(hiddenTypes)
    s.has(t) ? s.delete(t) : s.add(t)
    hiddenTypes = s
  }

  function clickChart(t) {
    chartTypeFilter = chartTypeFilter === t ? '' : t
  }

  // Make clickChart accessible from inline SVG onclick.
  if (typeof window !== 'undefined') {
    window._agentChartClick = clickChart
  }

  // ── Derived: filtered log list ─────────────────────────────────────────────
  let visibleLogs = $derived(agentLogs.filter(l => {
    if (chartTypeFilter && l.event_type !== chartTypeFilter) return false
    return true
  }))

  // ── Chart helpers ──────────────────────────────────────────────────────────
  function renderTimeline(logs) {
    if (!logs.length) return ''
    const now   = Date.now()
    const winMs = bucketMinutes * 60 * 1000
    const start = now - winMs * 12
    const bkts  = {}
    for (const l of logs) {
      const ts = new Date(l.timestamp).getTime()
      if (ts < start || hiddenTypes.has(l.event_type)) continue
      const b = Math.floor((ts - start) / winMs)
      if (!bkts[b]) bkts[b] = {}
      bkts[b][l.event_type] = (bkts[b][l.event_type] || 0) + 1
    }
    const keys = Object.keys(bkts).map(Number).sort((a,b)=>a-b)
    if (!keys.length) return ''
    const W = 500, H = 72, PAD = 2
    const maxV = Math.max(...keys.map(k => Object.values(bkts[k]).reduce((a,b)=>a+b,0)), 1)
    const bw   = Math.max(3, (W - PAD*2) / Math.max(keys.length, 1) - 1)
    let bars = ''
    keys.forEach((k, i) => {
      const x = PAD + i * (bw + 1)
      let y = H - PAD
      for (const [type, count] of Object.entries(bkts[k])) {
        const h = Math.max(2, (count / maxV) * (H - PAD*2))
        y -= h
        const col = AGENT_COLORS[type] || '#6b7280'
        bars += `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${bw.toFixed(1)}" height="${h.toFixed(1)}" fill="${col}" opacity="0.85" rx="1" style="cursor:pointer" onclick="window._agentChartClick('${type}')"/>`
      }
    })
    return `<svg viewBox="0 0 ${W} ${H}" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">${bars}</svg>`
  }

  function renderDonut(logs) {
    const counts = {}
    for (const l of logs) {
      if (!hiddenTypes.has(l.event_type))
        counts[l.event_type] = (counts[l.event_type] || 0) + 1
    }
    const total = Object.values(counts).reduce((a,b)=>a+b,0)
    if (!total) return ''
    const R = 38, CX = 50, CY = 50
    let ang = -Math.PI / 2, slices = ''
    for (const [type, count] of Object.entries(counts)) {
      const a = (count / total) * 2 * Math.PI
      const x1 = CX + R * Math.cos(ang), y1 = CY + R * Math.sin(ang)
      const x2 = CX + R * Math.cos(ang + a), y2 = CY + R * Math.sin(ang + a)
      const col = AGENT_COLORS[type] || '#6b7280'
      slices += `<path d="M ${CX} ${CY} L ${x1.toFixed(2)} ${y1.toFixed(2)} A ${R} ${R} 0 ${a > Math.PI ? 1 : 0} 1 ${x2.toFixed(2)} ${y2.toFixed(2)} Z" fill="${col}" opacity="0.85" style="cursor:pointer" onclick="window._agentChartClick('${type}')"/>`
      ang += a
    }
    return `<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">${slices}<circle cx="${CX}" cy="${CY}" r="${R * 0.55}" fill="#1f2937"/></svg>`
  }

  // ── Lifecycle ──────────────────────────────────────────────────────────────
  let timer = null
  onMount(() => {
    refreshAll()
    timer = setInterval(refreshAll, 5_000)
  })
  onDestroy(() => clearInterval(timer))
</script>

<div class="flex flex-col h-full overflow-hidden">

  <!-- ── Agent list (top, scrollable) ──────────────────────────────────────── -->
  <div class="shrink-0 max-h-72 overflow-y-auto border-b border-surface-600">
    <div class="flex items-center justify-between px-6 py-3 sticky top-0 bg-surface-900 z-10">
      <h1 class="text-lg font-semibold text-gray-100">Agents</h1>
      <div class="flex items-center gap-2">
        {#if selectedAgentId}
          <span class="text-xs text-accent">Filtered to agent</span>
          <button class="text-xs text-gray-500 hover:text-gray-300" onclick={() => { selectedAgentId = ''; fetchLogs() }}>× Clear</button>
        {/if}
        <button class="text-xs text-gray-500 hover:text-gray-300" onclick={refreshAll}>↻ Refresh</button>
      </div>
    </div>

    {#if loadingAgents && agents.length === 0}
      <Skeleton rows={3} />
    {:else if agents.length === 0}
      <p class="px-6 pb-4 text-gray-400 text-sm">No agents registered.</p>
    {:else}
      <div class="px-6 pb-4 grid gap-2">
        {#each agents as a (a.id)}
          <div
            class="p-3 bg-surface-800 rounded border cursor-pointer transition-colors
              {selectedAgentId === a.id ? 'border-accent' : 'border-surface-600 hover:border-surface-500'}"
            onclick={() => selectAgent(a.id)}
          >
            <div class="flex items-center gap-2 mb-1">
              <span class="w-2 h-2 rounded-full shrink-0 {statusDot[a.status] || 'bg-gray-500'}"></span>
              <span class="font-medium text-sm text-gray-100">{a.name}</span>
              <span class="text-xs text-gray-500 capitalize">{a.status}</span>
            </div>
            <div class="flex flex-wrap gap-1">
              {#each (a.roles ?? []) as role}
                {@const def = resolveRole(role)}
                <span class="text-xs px-1.5 py-0.5 rounded-full {def ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}">{role}</span>
              {/each}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- ── Log panel header ───────────────────────────────────────────────────── -->
  <div class="shrink-0 px-6 py-2 bg-surface-800 border-b border-surface-600 flex items-center justify-between">
    <span class="text-xs font-semibold text-gray-400 uppercase tracking-wide">Agent Activity Logs</span>
    <span class="text-xs text-gray-600">{visibleLogs.length} events · auto-refresh 5 s</span>
  </div>

  <!-- ── Charts row ─────────────────────────────────────────────────────────── -->
  <div class="shrink-0 px-6 py-3 border-b border-surface-600 flex gap-4 items-start">
    <!-- Timeline -->
    <div class="flex-1 min-w-0">
      <div class="flex items-center gap-2 mb-1">
        <span class="text-xs text-gray-500">Timeline</span>
        {#each [{l:'5 m',v:5},{l:'1 hr',v:60},{l:'1 day',v:1440}] as b}
          <button
            class="px-1.5 py-0.5 text-[10px] rounded transition-colors {bucketMinutes===b.v?'bg-accent text-white':'bg-surface-700 text-gray-400 hover:bg-surface-600'}"
            onclick={()=>bucketMinutes=b.v}>{b.l}</button>
        {/each}
      </div>
      <div class="h-[72px] bg-surface-800 rounded overflow-hidden">
        {@html renderTimeline(agentLogs)}
      </div>
    </div>
    <!-- Donut -->
    <div class="w-[72px] shrink-0">
      <span class="text-xs text-gray-500 block mb-1">Types</span>
      <div class="h-[72px] bg-surface-800 rounded overflow-hidden">
        {@html renderDonut(agentLogs)}
      </div>
    </div>
    <!-- Legend -->
    <div class="shrink-0 flex flex-col gap-0.5 pt-4 max-h-24 overflow-y-auto w-36">
      {#each Object.entries(AGENT_COLORS) as [type, color]}
        <button
          class="flex items-center gap-1.5 text-[10px] text-left transition-opacity {hiddenTypes.has(type)?'opacity-30':''}"
          onclick={() => toggleType(type)}
        >
          <span class="w-2 h-2 rounded-sm shrink-0" style="background:{color}"></span>
          <span class="text-gray-400 truncate">{type.replace('agent_','')}</span>
        </button>
      {/each}
    </div>
  </div>

  <!-- ── Filters ─────────────────────────────────────────────────────────────── -->
  <div class="shrink-0 px-6 py-2 border-b border-surface-600 flex gap-2 items-center flex-wrap">
    <select
      class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
      bind:value={logEventFilter}
      onchange={fetchLogs}
    >
      <option value="">All event types</option>
      {#each Object.keys(AGENT_COLORS) as t}<option value={t}>{t}</option>{/each}
    </select>
    <input
      class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 w-44 focus:outline-none"
      placeholder="Search description…"
      bind:value={logSearch}
      oninput={fetchLogs}
    />
    {#if chartTypeFilter}
      <span class="flex items-center gap-1 px-2 py-0.5 bg-accent/20 text-accent text-xs rounded">
        {chartTypeFilter}
        <button onclick={() => chartTypeFilter = ''} class="hover:text-white">×</button>
      </span>
    {/if}
  </div>

  <!-- ── Log list ────────────────────────────────────────────────────────────── -->
  <div class="flex-1 overflow-y-auto text-xs">
    {#if logsLoading && agentLogs.length === 0}
      <Skeleton rows={4} mode="table" />
    {:else if visibleLogs.length === 0}
      <p class="p-4 text-gray-400">No events match current filters.</p>
    {:else}
      <table class="w-full">
        <thead class="sticky top-0 bg-surface-800 z-10">
          <tr>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Time</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Agent</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-44">Event</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium">Description</th>
          </tr>
        </thead>
        <tbody>
          {#each visibleLogs as l (l.id)}
            <tr class="border-t border-surface-700 hover:bg-surface-700/20">
              <td class="px-4 py-1 font-mono text-gray-500 whitespace-nowrap">
                {new Date(l.timestamp).toLocaleTimeString()}
              </td>
              <td class="px-4 py-1 font-mono text-gray-400 truncate max-w-[7rem]">
                {l.agent_name || l.agent_id?.slice(0,8) || '—'}
              </td>
              <td class="px-4 py-1">
                <span
                  class="px-1.5 py-0.5 rounded text-[10px] font-medium"
                  style="background:{AGENT_COLORS[l.event_type] || '#374151'}22;color:{AGENT_COLORS[l.event_type] || '#9ca3af'}"
                >{l.event_type}</span>
              </td>
              <td class="px-4 py-1 text-gray-400 truncate max-w-xs">{l.description || ''}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>

</div>
