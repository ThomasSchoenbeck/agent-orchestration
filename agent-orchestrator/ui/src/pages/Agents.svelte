<script>
  import { onMount, onDestroy } from 'svelte'
  import { listAgents, listRoles, listAgentLogs, deleteAgentLogs,
           updateAgent, stopAgent, resetAgent, getSkillsMeta } from '../lib/api.js'
  import { formatTimestamp } from '../lib/time.js'
  import { toasts, router } from '../lib/stores.js'
  import Skeleton from '../components/Skeleton.svelte'
  import AgentTemplatesPanel from '../components/AgentTemplatesPanel.svelte'

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

  // ── Resizable top region (agents + templates) ───────────────────────────────
  const TOP_MIN = 140
  function loadTopHeight() {
    try {
      const v = Number(localStorage.getItem('agentsTopHeight'))
      if (v >= TOP_MIN) return v
    } catch { /* ignore */ }
    return 288
  }
  let topHeight = $state(loadTopHeight())
  let resizeStartY = 0
  let resizeStartH = 0
  function startResize(e) {
    e.preventDefault()
    resizeStartY = e.clientY
    resizeStartH = topHeight
    window.addEventListener('mousemove', onResize)
    window.addEventListener('mouseup', stopResize)
  }
  function onResize(e) {
    const max = Math.max(TOP_MIN, window.innerHeight - 160)
    topHeight = Math.min(max, Math.max(TOP_MIN, resizeStartH + (e.clientY - resizeStartY)))
  }
  function stopResize() {
    window.removeEventListener('mousemove', onResize)
    window.removeEventListener('mouseup', stopResize)
    try { localStorage.setItem('agentsTopHeight', String(topHeight)) } catch { /* ignore */ }
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

  // ── Lifecycle controls (Feature 7) ──────────────────────────────────────────
  let availSkills    = $state([])
  let editingAgentId = $state('')
  let editBuf        = $state({ roles: '', skills: '' })

  const arrEq = (a = [], b = []) => a.length === b.length && a.every((x, i) => x === b[i])
  const isOverridden = (a) => !arrEq(a.roles, a.start_roles) || !arrEq(a.skills, a.start_skills)
  const splitCsv = (s) => (s || '').split(/[\s,]+/).map(x => x.trim()).filter(Boolean)

  function startEditAgent(a) {
    editingAgentId = a.id
    editBuf = { roles: (a.roles || []).join(', '), skills: (a.skills || []).join(', ') }
  }
  async function saveAgentLive(a) {
    try {
      await updateAgent(a.id, { roles: splitCsv(editBuf.roles), skills: splitCsv(editBuf.skills) })
      toasts.success('Live config updated (effective next poll)')
      editingAgentId = ''
      await loadAgents()
    } catch (e) { toasts.error('Update failed: ' + e.message) }
  }
  async function doStop(a) {
    if (!confirm(`Stop agent "${a.name}"? It will finish its current task and go offline.`)) return
    try { await stopAgent(a.id); toasts.success('Stop requested'); await loadAgents() }
    catch (e) { toasts.error('Stop failed: ' + e.message) }
  }
  async function doReset(a) {
    try { await resetAgent(a.id); toasts.success('Live config reset to start params'); await loadAgents() }
    catch (e) { toasts.error('Reset failed: ' + e.message) }
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

  // ── Clear handler ─────────────────────────────────────────────────────────
  async function handleClearAgentLogs() {
    if (!confirm('Delete all agent logs? This cannot be undone.')) return
    try {
      const { deleted } = await deleteAgentLogs()
      toasts.success(`Deleted ${deleted} agent log entries`)
      agentLogs = []
    } catch (e) {
      toasts.error('Failed to clear agent logs: ' + e.message)
    }
  }

  // ── Lifecycle ──────────────────────────────────────────────────────────────
  let timer = null
  onMount(() => {
    refreshAll()
    // Load skills fire-and-forget so the refresh interval is registered
    // synchronously (tests advance fake timers immediately after mount).
    getSkillsMeta().then((s) => { availSkills = s || [] }).catch(() => {})
    timer = setInterval(refreshAll, 5_000)
  })
  onDestroy(() => {
    clearInterval(timer)
    window.removeEventListener('mousemove', onResize)
    window.removeEventListener('mouseup', stopResize)
  })
</script>

<div class="flex flex-col h-full overflow-hidden">

  <!-- ── Top: agents (left) + templates (right) ────────────────────────────── -->
  <div class="shrink-0 flex border-b border-surface-600" style="height: {topHeight}px" data-testid="agents-top">

  <!-- Agents column -->
  <div class="flex-1 overflow-y-auto border-r border-surface-600 min-w-0">
    <div class="flex items-center justify-between px-6 py-3 sticky top-0 bg-surface-900 z-10">
      <h1 class="text-lg font-semibold text-gray-100">Agents</h1>
      <div class="flex items-center gap-2">
        {#if selectedAgentId}
          <span class="text-xs text-accent">Filtered to agent</span>
          <button class="text-xs text-gray-500 hover:text-gray-300" onclick={() => { selectedAgentId = ''; fetchLogs() }}>× Clear</button>
        {/if}
        <button class="text-xs text-gray-500 hover:text-gray-300" onclick={refreshAll}>↻ Refresh</button>
        <button
          class="text-xs px-2 py-1 rounded bg-red-900/40 text-red-400 hover:bg-red-900/70 transition-colors"
          onclick={handleClearAgentLogs}
        >Clear logs</button>
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
              <button
                class="ml-auto text-xs text-gray-500 hover:text-accent transition-colors"
                title="View agent detail"
                onclick={(e) => { e.stopPropagation(); router.push('agents', a.id) }}
              >Detail →</button>
            </div>
            <div class="flex flex-wrap gap-1 items-center">
              {#each (a.roles ?? []) as role}
                {@const def = resolveRole(role)}
                <span class="text-xs px-1.5 py-0.5 rounded-full {def ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}">{role}</span>
              {/each}
              {#each (a.skills ?? []) as sk}
                <span class="text-xs px-1.5 py-0.5 rounded-full bg-teal-900 text-teal-300">{sk}</span>
              {/each}
              {#if isOverridden(a)}
                <span class="text-[10px] px-1.5 py-0.5 rounded-full bg-amber-900 text-amber-300"
                  title="Live config differs from start params: roles [{(a.start_roles||[]).join(', ')}] skills [{(a.start_skills||[]).join(', ')}]">runtime override</span>
              {/if}
              {#if a.desired_state === 'stop' && a.status !== 'offline'}
                <span class="text-[10px] px-1.5 py-0.5 rounded-full bg-rose-900 text-rose-300">stopping</span>
              {/if}
            </div>

            <!-- Lifecycle controls (Feature 7) -->
            <div class="flex items-center gap-2 mt-2" onclick={(e) => e.stopPropagation()}>
              <button class="text-[11px] text-blue-400 hover:text-blue-300" onclick={() => startEditAgent(a)}>Edit live</button>
              {#if isOverridden(a)}
                <button class="text-[11px] text-amber-400 hover:text-amber-300" onclick={() => doReset(a)}>Reset to start</button>
              {/if}
              {#if a.status !== 'offline'}
                <button class="text-[11px] text-rose-400 hover:text-rose-300" onclick={() => doStop(a)}>Stop</button>
              {/if}
            </div>

            {#if editingAgentId === a.id}
              <div class="mt-2 flex flex-col gap-2" onclick={(e) => e.stopPropagation()}>
                <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs" placeholder="live roles (comma-separated)" bind:value={editBuf.roles} />
                <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs" placeholder="live skills (comma-separated)" bind:value={editBuf.skills} />
                {#if availSkills.length > 0}
                  <span class="text-[10px] text-gray-500">skills: {availSkills.map(s => s.value).join(', ')}</span>
                {/if}
                <div class="flex gap-2">
                  <button class="text-[11px] text-gray-400 hover:text-gray-200" onclick={() => editingAgentId = ''}>Cancel</button>
                  <button class="text-[11px] px-2 py-0.5 rounded bg-accent text-white" onclick={() => saveAgentLive(a)}>Save</button>
                </div>
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
  <!-- Templates column (Bug 8: managed templates merged into the Agents page) -->
  <div class="w-96 shrink-0 overflow-y-auto">
    <AgentTemplatesPanel {agents} />
  </div>
  </div>

  <!-- Resizable divider (Bug 4): drag to grow/shrink the agents+templates region -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="shrink-0 h-1.5 cursor-row-resize bg-surface-700 hover:bg-accent transition-colors"
    role="separator"
    aria-orientation="horizontal"
    aria-label="Resize agents panel"
    onmousedown={startResize}
  ></div>

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
                {formatTimestamp(l.timestamp)}
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
