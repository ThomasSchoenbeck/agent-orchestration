<script>
  import { onMount, onDestroy } from 'svelte'
  import { listTasks, listProjects, createTask, updateTask, deleteTask, getTaskTypes, getTaskRoles, listAllTaskLogs } from '../lib/api.js'
  import { toasts, router } from '../lib/stores.js'

  // ── Task list state ───────────────────────────────────────────────────────
  let tasks          = $state([])
  let projects       = $state([])
  let taskTypes      = $state([])
  let taskRoles      = $state([])
  let loading        = $state(false)
  let showForm       = $state(false)
  let filterStatus   = $state('')
  let filterProject  = $state('')
  let selectedTaskId = $state('')

  const statusColors = {
    pending:     'bg-gray-700 text-gray-300',
    planned:     'bg-blue-900 text-blue-300',
    queued:      'bg-yellow-900 text-yellow-300',
    in_progress: 'bg-orange-900 text-orange-300',
    completed:   'bg-green-900 text-green-300',
    failed:      'bg-red-900 text-red-300',
  }

  let form = $state({
    project_id:  '',
    type:        'implement',
    role:        'worker',
    title:       '',
    description: '',
    priority:    5,
  })

  // ── Log panel state ────────────────────────────────────────────────────────
  let taskLogs        = $state([])
  let logsLoading     = $state(false)
  let logEventFilter  = $state('')
  let logSearch       = $state('')
  let hiddenTypes     = $state(new Set())
  let chartTypeFilter = $state('')
  let bucketMinutes   = $state(60)

  const TASK_COLORS = {
    task_created:   '#34d399',
    task_updated:   '#60a5fa',
    task_queued:    '#facc15',
    task_claimed:   '#a78bfa',
    task_started:   '#fb923c',
    task_completed: '#22c55e',
    task_failed:    '#ef4444',
    task_retried:   '#f97316',
    task_cancelled: '#6b7280',
    task_timeout:   '#f43f5e',
    task_result:    '#38bdf8',
    task_error:     '#e879f9',
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function loadTasks() {
    loading = true
    try {
      const params = {}
      if (filterStatus)  params.status     = filterStatus
      if (filterProject) params.project_id = filterProject
      const [tr, pr, tt, tr2] = await Promise.all([
        listTasks(params), listProjects(), getTaskTypes(), getTaskRoles(),
      ])
      tasks     = Array.isArray(tr)  ? tr  : (tr.tasks    ?? [])
      projects  = Array.isArray(pr)  ? pr  : (pr.projects ?? [])
      taskTypes = Array.isArray(tt)  ? tt  : []
      taskRoles = Array.isArray(tr2) ? tr2 : []
    } catch (e) {
      toasts.error('Failed to load: ' + e.message)
    } finally {
      loading = false
    }
  }

  async function fetchLogs() {
    logsLoading = true
    try {
      const params = { limit: 500 }
      if (selectedTaskId) params.task_id    = selectedTaskId
      if (logEventFilter) params.event_type = logEventFilter
      if (logSearch)      params.search     = logSearch
      taskLogs = (await listAllTaskLogs(params)) ?? []
    } catch (e) {
      taskLogs = []
    } finally {
      logsLoading = false
    }
  }

  async function refreshAll() {
    await Promise.all([loadTasks(), fetchLogs()])
  }

  function taskTitle(t) {
    return t.payload?.title ?? t.type ?? t.id
  }

  function projectName(projectId) {
    return projects.find(p => p.id === projectId)?.name ?? 'Unknown'
  }

  function selectTask(id) {
    selectedTaskId = selectedTaskId === id ? '' : id
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

  if (typeof window !== 'undefined') {
    window._taskChartClick = clickChart
  }

  // ── Derived: filtered logs ─────────────────────────────────────────────────
  let visibleLogs = $derived(taskLogs.filter(l => {
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
        const col = TASK_COLORS[type] || '#6b7280'
        bars += `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${bw.toFixed(1)}" height="${h.toFixed(1)}" fill="${col}" opacity="0.85" rx="1" style="cursor:pointer" onclick="window._taskChartClick('${type}')"/>`
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
      const col = TASK_COLORS[type] || '#6b7280'
      slices += `<path d="M ${CX} ${CY} L ${x1.toFixed(2)} ${y1.toFixed(2)} A ${R} ${R} 0 ${a > Math.PI ? 1 : 0} 1 ${x2.toFixed(2)} ${y2.toFixed(2)} Z" fill="${col}" opacity="0.85" style="cursor:pointer" onclick="window._taskChartClick('${type}')"/>`
      ang += a
    }
    return `<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" class="w-full h-full">${slices}<circle cx="${CX}" cy="${CY}" r="${R * 0.55}" fill="#1f2937"/></svg>`
  }

  // ── Create ────────────────────────────────────────────────────────────────
  async function submit() {
    if (!form.type || !form.role) return
    if (!form.project_id) { toasts.error('Please select a project'); return }
    try {
      await createTask({
        project_id: form.project_id,
        type:       form.type.trim(),
        role:       form.role.trim(),
        priority:   Number(form.priority),
        payload:    { title: form.title.trim(), description: form.description.trim() },
      })
      toasts.success('Task created')
      form     = { project_id: '', type: '', role: '', title: '', description: '', priority: 5 }
      showForm = false
      await refreshAll()
    } catch (e) {
      toasts.error('Create failed: ' + e.message)
    }
  }

  async function setStatus(id, status) {
    try {
      await updateTask(id, { status })
      await loadTasks()
    } catch (e) {
      toasts.error('Update failed: ' + e.message)
    }
  }

  async function remove(id) {
    if (!confirm('Delete this task?')) return
    try {
      await deleteTask(id)
      toasts.success('Task deleted')
      if (selectedTaskId === id) { selectedTaskId = ''; fetchLogs() }
      await loadTasks()
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
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

  <!-- ── Task list (top, scrollable) ───────────────────────────────────────── -->
  <div class="shrink-0 max-h-80 overflow-y-auto border-b border-surface-600">
    <div class="flex items-center justify-between px-6 py-3 sticky top-0 bg-surface-900 z-10 flex-wrap gap-2">
      <h1 class="text-lg font-semibold text-gray-100">Tasks</h1>
      <div class="flex items-center gap-2 flex-wrap">
        <select
          class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
          bind:value={filterStatus}
          onchange={loadTasks}
        >
          <option value="">All statuses</option>
          {#each ['pending','planned','queued','in_progress','completed','failed'] as s}
            <option value={s}>{s}</option>
          {/each}
        </select>
        <select
          class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
          bind:value={filterProject}
          onchange={loadTasks}
        >
          <option value="">All projects</option>
          {#each projects as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
        {#if selectedTaskId}
          <span class="text-xs text-accent">Filtered to task</span>
          <button class="text-xs text-gray-500 hover:text-gray-300" onclick={() => { selectedTaskId = ''; fetchLogs() }}>× Clear</button>
        {/if}
        <button class="text-xs text-gray-500 hover:text-gray-300" onclick={refreshAll}>↻ Refresh</button>
        <button
          class="px-2 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors"
          onclick={() => showForm = !showForm}
        >{showForm ? 'Cancel' : '+ New'}</button>
      </div>
    </div>

    {#if showForm}
      <form
        class="mx-6 mb-3 p-3 bg-surface-800 rounded border border-surface-600 flex flex-col gap-2"
        onsubmit={(e) => { e.preventDefault(); submit() }}
      >
        <div class="grid grid-cols-2 gap-2">
          <select
            class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs text-gray-300 focus:outline-none focus:border-accent"
            bind:value={form.project_id} required
          >
            <option value="">Select project *</option>
            {#each projects as p}<option value={p.id}>{p.name}</option>{/each}
          </select>
          <input
            class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="Title"
            bind:value={form.title}
          />
          <select
            aria-label="Task type"
            class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs text-gray-300 focus:outline-none focus:border-accent"
            bind:value={form.type} required
          >
            {#each taskTypes as tt}<option value={tt.value} title={tt.description}>{tt.label}</option>{/each}
            {#if taskTypes.length === 0}<option value="implement">Implement</option>{/if}
          </select>
          <select
            aria-label="Role"
            class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs text-gray-300 focus:outline-none focus:border-accent"
            bind:value={form.role} required
          >
            {#each taskRoles as tr}<option value={tr.value} title={tr.description}>{tr.label}</option>{/each}
            {#if taskRoles.length === 0}<option value="worker">Worker</option>{/if}
          </select>
        </div>
        <textarea
          class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent resize-none"
          placeholder="Description"
          rows="2"
          bind:value={form.description}
        ></textarea>
        <div class="flex items-center gap-2">
          <label for="task-priority" class="text-xs text-gray-400 shrink-0">Priority</label>
          <input id="task-priority" type="range" min="1" max="10" bind:value={form.priority} class="flex-1" />
          <span class="text-xs text-gray-300 w-4 text-right">{form.priority}</span>
          <button type="submit" class="ml-2 px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors">Create</button>
        </div>
      </form>
    {/if}

    {#if loading && tasks.length === 0}
      <p class="px-6 pb-4 text-gray-500 text-sm">Loading…</p>
    {:else if tasks.length === 0}
      <p class="px-6 pb-4 text-gray-500 text-sm">No tasks found.</p>
    {:else}
      <div class="px-6 pb-4 flex flex-col gap-1.5">
        {#each tasks as t (t.id)}
          <div
            class="p-2.5 bg-surface-800 rounded border cursor-pointer transition-colors
              {selectedTaskId === t.id ? 'border-accent' : 'border-surface-600 hover:border-surface-500'}"
            onclick={() => selectTask(t.id)}
          >
            <div class="flex items-start justify-between gap-2">
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span class="text-xs font-medium text-gray-100 truncate">{taskTitle(t)}</span>
                  <span class="text-[10px] px-1.5 py-0.5 rounded-full {statusColors[t.status] || 'bg-gray-700 text-gray-300'}">{t.status}</span>
                  <span class="text-[10px] text-gray-500 font-mono">{t.type}</span>
                  <span class="text-[10px] text-gray-600">{t.role}</span>
                  {#if t.project_id}
                    <span class="text-[10px] text-gray-500 bg-surface-700 px-1 py-0.5 rounded">📁 {projectName(t.project_id)}</span>
                  {/if}
                </div>
                {#if t.payload?.description}
                  <p class="text-[10px] text-gray-400 mt-0.5 truncate">{t.payload.description}</p>
                {/if}
              </div>
              <div class="flex items-center gap-1.5 shrink-0">
                {#if t.status === 'pending' || t.status === 'failed'}
                  <button
                    class="text-[10px] text-yellow-400 hover:text-yellow-300"
                    onclick={(e) => { e.stopPropagation(); setStatus(t.id, 'queued') }}
                  >Queue</button>
                {/if}
                <button
                  class="text-[10px] text-blue-400 hover:text-blue-300"
                  onclick={(e) => { e.stopPropagation(); router.push('tasks', t.id) }}
                >View</button>
                <button
                  class="text-[10px] text-red-400 hover:text-red-300"
                  onclick={(e) => { e.stopPropagation(); remove(t.id) }}
                >Del</button>
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- ── Log panel header ───────────────────────────────────────────────────── -->
  <div class="shrink-0 px-6 py-2 bg-surface-800 border-b border-surface-600 flex items-center justify-between">
    <span class="text-xs font-semibold text-gray-400 uppercase tracking-wide">Task Activity Logs</span>
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
        {@html renderTimeline(taskLogs)}
      </div>
    </div>
    <!-- Donut -->
    <div class="w-[72px] shrink-0">
      <span class="text-xs text-gray-500 block mb-1">Types</span>
      <div class="h-[72px] bg-surface-800 rounded overflow-hidden">
        {@html renderDonut(taskLogs)}
      </div>
    </div>
    <!-- Legend -->
    <div class="shrink-0 flex flex-col gap-0.5 pt-4 max-h-24 overflow-y-auto w-32">
      {#each Object.entries(TASK_COLORS) as [type, color]}
        <button
          class="flex items-center gap-1.5 text-[10px] text-left transition-opacity {hiddenTypes.has(type)?'opacity-30':''}"
          onclick={() => toggleType(type)}
        >
          <span class="w-2 h-2 rounded-sm shrink-0" style="background:{color}"></span>
          <span class="text-gray-400 truncate">{type.replace('task_','')}</span>
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
      {#each Object.keys(TASK_COLORS) as t}<option value={t}>{t}</option>{/each}
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
    {#if logsLoading && taskLogs.length === 0}
      <p class="p-4 text-gray-500">Loading…</p>
    {:else if visibleLogs.length === 0}
      <p class="p-4 text-gray-500">No events match current filters.</p>
    {:else}
      <table class="w-full">
        <thead class="sticky top-0 bg-surface-800 z-10">
          <tr>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-28">Time</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-44">Event</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium w-24">Status</th>
            <th class="text-left px-4 py-1.5 text-gray-500 font-medium">Description</th>
          </tr>
        </thead>
        <tbody>
          {#each visibleLogs as l (l.id)}
            <tr class="border-t border-surface-700 hover:bg-surface-700/20">
              <td class="px-4 py-1 font-mono text-gray-500 whitespace-nowrap">
                {new Date(l.timestamp).toLocaleTimeString()}
              </td>
              <td class="px-4 py-1">
                <span
                  class="px-1.5 py-0.5 rounded text-[10px] font-medium"
                  style="background:{TASK_COLORS[l.event_type] || '#374151'}22;color:{TASK_COLORS[l.event_type] || '#9ca3af'}"
                >{l.event_type}</span>
              </td>
              <td class="px-4 py-1 font-mono text-gray-500 text-[10px]">
                {#if l.old_status && l.new_status}
                  {l.old_status} → {l.new_status}
                {:else if l.new_status}
                  {l.new_status}
                {/if}
              </td>
              <td class="px-4 py-1 text-gray-400 truncate max-w-xs">{l.description || ''}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>

</div>
