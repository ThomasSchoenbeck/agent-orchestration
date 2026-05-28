<script>
  import { onMount, onDestroy } from 'svelte'
  import { listLogs, listChatLog, listSettings, deleteLogs } from '../lib/api.js'
  import { formatTimestamp } from '../lib/time.js'
  import { toasts, router } from '../lib/stores.js'
  import Skeleton from '../components/Skeleton.svelte'

  // ── State ─────────────────────────────────────────────────────────────────
  let chatLog         = $state([])
  let chatLogLimit    = $state(50)
  let logs            = $state([])
  let loading         = $state(false)
  let level           = $state('')
  let limit           = $state(200)
  let hiddenLevels    = $state(new Set())
  let chartLevelFilter = $state('')
  let bucketMinutes   = $state(60)
  let expandedRows    = $state(new Set())   // entry IDs with expanded metadata

  const LEVEL_COLORS = {
    debug: '#94a3b8',
    info:  '#60a5fa',
    warn:  '#facc15',
    error: '#ef4444',
  }

  const levelText = {
    debug: 'text-gray-500',
    info:  'text-blue-400',
    warn:  'text-yellow-400',
    error: 'text-red-400',
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function load() {
    loading = true
    try {
      const params = { limit }
      if (level) params.level = level
      const [res, cl] = await Promise.all([
        listLogs(params),
        listChatLog({ limit: chatLogLimit }),
      ])
      logs    = Array.isArray(res) ? res : (res.logs ?? [])
      chatLog = Array.isArray(cl)  ? cl  : []
    } catch (e) {
      toasts.error('Failed to load logs: ' + e.message)
    } finally {
      loading = false
    }
  }

  function toggleLevel(l) {
    const s = new Set(hiddenLevels)
    s.has(l) ? s.delete(l) : s.add(l)
    hiddenLevels = s
  }

  function clickChart(l) {
    chartLevelFilter = chartLevelFilter === l ? '' : l
  }

  function toggleRow(id) {
    const s = new Set(expandedRows)
    s.has(id) ? s.delete(id) : s.add(id)
    expandedRows = s
  }

  function hasContext(entry) {
    return entry.agent_id || entry.task_id || entry.project_id ||
      (entry.metadata && Object.keys(entry.metadata).length > 0)
  }

  if (typeof window !== 'undefined') {
    window._logsChartClick = clickChart
  }

  // ── Derived: visible logs ──────────────────────────────────────────────────
  let visibleLogs = $derived(logs.filter(l => {
    if (hiddenLevels.has(l.level)) return false
    if (chartLevelFilter && l.level !== chartLevelFilter) return false
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
      const ts = new Date(l.timestamp ?? l.created_at).getTime()
      if (ts < start || hiddenLevels.has(l.level)) continue
      const b = Math.floor((ts - start) / winMs)
      if (!bkts[b]) bkts[b] = {}
      bkts[b][l.level] = (bkts[b][l.level] || 0) + 1
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
      for (const [lvl, count] of Object.entries(bkts[k])) {
        const h = Math.max(2, (count / maxV) * (H - PAD*2))
        y -= h
        const col = LEVEL_COLORS[lvl] || '#6b7280'
        bars += `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${bw.toFixed(1)}" height="${h.toFixed(1)}" fill="${col}" opacity="0.85" rx="1" style="cursor:pointer" onclick="window._logsChartClick('${lvl}')"/>`
      }
    })
    return `<svg viewBox="0 0 ${W} ${H}" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">${bars}</svg>`
  }

  function renderDonut(logs) {
    const counts = {}
    for (const l of logs) {
      if (!hiddenLevels.has(l.level))
        counts[l.level] = (counts[l.level] || 0) + 1
    }
    const total = Object.values(counts).reduce((a,b)=>a+b,0)
    if (!total) return ''
    const R = 38, CX = 50, CY = 50
    let ang = -Math.PI / 2, slices = ''
    for (const [lvl, count] of Object.entries(counts)) {
      const a = (count / total) * 2 * Math.PI
      const x1 = CX + R * Math.cos(ang), y1 = CY + R * Math.sin(ang)
      const x2 = CX + R * Math.cos(ang + a), y2 = CY + R * Math.sin(ang + a)
      const col = LEVEL_COLORS[lvl] || '#6b7280'
      slices += `<path d="M ${CX} ${CY} L ${x1.toFixed(2)} ${y1.toFixed(2)} A ${R} ${R} 0 ${a > Math.PI ? 1 : 0} 1 ${x2.toFixed(2)} ${y2.toFixed(2)} Z" fill="${col}" opacity="0.85" style="cursor:pointer" onclick="window._logsChartClick('${lvl}')"/>`
      ang += a
    }
    return `<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">${slices}<circle cx="${CX}" cy="${CY}" r="${R * 0.55}" fill="#1f2937"/></svg>`
  }

  const formatTime = formatTimestamp

  // ── Lifecycle ──────────────────────────────────────────────────────────────
  let timer = null
  onMount(async () => {
    load()
    let intervalMs = 10_000
    try {
      const all = await listSettings()
      const s = all.find(s => s.key === 'platform.charts.autorefresh_ms')
      if (s) {
        const parsed = parseInt(s.value, 10)
        if (parsed > 0) intervalMs = parsed
      }
    } catch (_) { /* fall back to default */ }
    timer = setInterval(load, intervalMs)
  })
  onDestroy(() => clearInterval(timer))

  // ── Clear handlers ─────────────────────────────────────────────────────────
  async function handleClearSystemLogs() {
    if (!confirm('Delete all system logs? This cannot be undone.')) return
    try {
      const { deleted } = await deleteLogs()
      toasts.success(`Deleted ${deleted} system log entries`)
      load()
    } catch (e) {
      toasts.error('Failed to clear system logs: ' + e.message)
    }
  }

</script>

<div class="flex flex-col h-full overflow-hidden">

  <!-- ── Header ─────────────────────────────────────────────────────────────── -->
  <div class="shrink-0 px-6 py-3 border-b border-surface-600 flex items-center justify-between flex-wrap gap-2">
    <h1 class="text-lg font-semibold text-gray-100">System Logs</h1>
    <div class="flex items-center gap-2 flex-wrap text-xs text-gray-500">
      <span>Agent logs →</span>
      <button class="text-accent hover:underline" onclick={() => router.push('agents')}>Agents</button>
      <span class="ml-2">Task logs →</span>
      <button class="text-accent hover:underline" onclick={() => router.push('tasks')}>Tasks</button>
    </div>
  </div>

  <!-- ── Controls ───────────────────────────────────────────────────────────── -->
  <div class="shrink-0 px-6 py-2 border-b border-surface-600 flex items-center gap-2 flex-wrap">
    <select
      class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
      bind:value={level}
      onchange={load}
    >
      <option value="">All levels</option>
      {#each Object.keys(LEVEL_COLORS) as l}<option value={l}>{l}</option>{/each}
    </select>
    <select
      class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
      bind:value={limit}
      onchange={load}
    >
      <option value={50}>50</option>
      <option value={200}>200</option>
      <option value={500}>500</option>
    </select>
    {#if chartLevelFilter}
      <span class="flex items-center gap-1 px-2 py-0.5 bg-accent/20 text-accent text-xs rounded">
        {chartLevelFilter}
        <button onclick={() => chartLevelFilter = ''} class="hover:text-white">×</button>
      </span>
    {/if}
    <span class="ml-auto text-gray-600">{visibleLogs.length} entries · auto-refresh 10 s</span>
    <button class="text-gray-500 hover:text-gray-300" onclick={load}>↻</button>
    <button
      class="px-2 py-1 text-xs rounded bg-red-900/40 text-red-400 hover:bg-red-900/70 transition-colors"
      onclick={handleClearSystemLogs}
    >Clear logs</button>
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
        {@html renderTimeline(logs)}
      </div>
    </div>
    <!-- Donut -->
    <div class="w-[72px] shrink-0">
      <span class="text-xs text-gray-500 block mb-1">Levels</span>
      <div class="h-[72px] bg-surface-800 rounded overflow-hidden">
        {@html renderDonut(logs)}
      </div>
    </div>
    <!-- Legend -->
    <div class="shrink-0 flex flex-col gap-1 pt-4">
      {#each Object.entries(LEVEL_COLORS) as [lvl, color]}
        <button
          class="flex items-center gap-1.5 text-[10px] text-left transition-opacity {hiddenLevels.has(lvl)?'opacity-30':''}"
          onclick={() => toggleLevel(lvl)}
        >
          <span class="w-2 h-2 rounded-sm shrink-0" style="background:{color}"></span>
          <span class="text-gray-400">{lvl}</span>
        </button>
      {/each}
    </div>
  </div>

  <!-- ── Chat log ──────────────────────────────────────────────────────────── -->
  {#if chatLog.length > 0}
    <div class="shrink-0 border-b border-surface-600">
      <div class="px-6 py-2 flex items-center justify-between">
        <span class="text-xs font-semibold text-gray-400 uppercase tracking-wide">Chat Log</span>
        <div class="flex items-center gap-2">
          <select
            class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
            bind:value={chatLogLimit}
            onchange={load}
          >
            <option value={20}>20</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </select>
        </div>
      </div>
      <div class="max-h-40 overflow-y-auto">
        <table class="w-full font-mono text-xs">
          <thead class="sticky top-0 bg-surface-800 z-10">
            <tr>
              <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Time</th>
              <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Provider</th>
              <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Direction</th>
              <th class="text-left px-4 py-1.5 text-gray-500 font-medium">Preview</th>
            </tr>
          </thead>
          <tbody>
            {#each chatLog as entry (entry.timestamp + entry.direction + entry.preview)}
              <tr class="border-t border-surface-700 hover:bg-surface-700/20">
                <td class="px-4 py-1 text-gray-600 whitespace-nowrap">{formatTime(entry.timestamp)}</td>
                <td class="px-4 py-1 text-gray-400 truncate max-w-[6rem]">{entry.provider_name || '—'}</td>
                <td class="px-4 py-1">
                  <span class="text-[10px] px-1.5 py-0.5 rounded {entry.direction === 'user_to_llm' ? 'bg-blue-900/50 text-blue-300' : 'bg-purple-900/50 text-purple-300'}">
                    {entry.direction === 'user_to_llm' ? '→ LLM' : '← LLM'}
                  </span>
                </td>
                <td class="px-4 py-1 text-gray-400">{entry.preview}…</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}

  <!-- ── Log stream ─────────────────────────────────────────────────────────── -->
  <div class="flex-1 overflow-y-auto font-mono text-xs">
    {#if loading && logs.length === 0}
      <Skeleton rows={5} mode="table" />
    {:else if visibleLogs.length === 0}
      <p class="p-4 text-gray-400">No log entries match current filters.</p>
    {:else}
      <table class="w-full">
        <thead class="sticky top-0 bg-surface-800 z-10">
          <tr>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Time</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-16">Level</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Agent</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Task</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium">Message</th>
          </tr>
        </thead>
        <tbody>
          {#each visibleLogs as entry (entry.id ?? entry.timestamp)}
            {@const rowId   = entry.id ?? entry.timestamp}
            {@const expanded = expandedRows.has(rowId)}
            {@const meta     = entry.metadata && Object.keys(entry.metadata).length > 0}
            <tr
              class="border-t border-surface-700 hover:bg-surface-700/20 {hasContext(entry) ? 'cursor-pointer' : ''}"
              onclick={() => hasContext(entry) && toggleRow(rowId)}
            >
              <td class="px-4 py-1 text-gray-600 whitespace-nowrap">{formatTime(entry.timestamp ?? entry.created_at)}</td>
              <td class="px-4 py-1 {levelText[entry.level] ?? 'text-gray-400'}">{entry.level ?? 'info'}</td>
              <td class="px-4 py-1 text-gray-500 truncate max-w-[7rem]" title={entry.agent_id}>
                {entry.agent_id ? entry.agent_id.slice(0, 8) + '…' : '—'}
              </td>
              <td class="px-4 py-1 text-gray-500 truncate max-w-[7rem]" title={entry.task_id}>
                {entry.task_id ? entry.task_id.slice(0, 8) + '…' : '—'}
              </td>
              <td class="px-4 py-1 text-gray-300 break-all">
                {entry.message}
                {#if meta && !expanded}
                  <span class="ml-1 text-[10px] text-gray-600 italic">+meta</span>
                {/if}
              </td>
            </tr>
            {#if expanded}
              <tr class="border-t border-surface-700 bg-surface-900/60">
                <td colspan="5" class="px-4 py-2 text-[10px] text-gray-400">
                  <div class="flex flex-wrap gap-4">
                    {#if entry.agent_id}
                      <div><span class="text-gray-600">agent_id</span> {entry.agent_id}</div>
                    {/if}
                    {#if entry.task_id}
                      <div><span class="text-gray-600">task_id</span> {entry.task_id}</div>
                    {/if}
                    {#if entry.project_id}
                      <div><span class="text-gray-600">project_id</span> {entry.project_id}</div>
                    {/if}
                    {#if meta}
                      <pre class="text-gray-300 bg-surface-800 rounded px-2 py-1 overflow-auto max-w-full whitespace-pre-wrap">{JSON.stringify(entry.metadata, null, 2)}</pre>
                    {/if}
                  </div>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    {/if}
  </div>

</div>
