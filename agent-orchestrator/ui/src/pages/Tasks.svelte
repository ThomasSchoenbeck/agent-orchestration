<script>
  import { onMount, onDestroy } from 'svelte'
  import { listTasks, listProjects, createTask, updateTask, deleteTask, queueTask, getTaskTypes, getTaskRoles, listAllTaskLogs, listSettings, listRequirements, listFeatures, addTaskLink } from '../lib/api.js'
  import { toasts, router } from '../lib/stores.js'
  import AssistantSidebar from '../components/AssistantSidebar.svelte'
  import Skeleton from '../components/Skeleton.svelte'

  // ── Task list state ───────────────────────────────────────────────────────
  let tasks          = $state([])
  let projects       = $state([])
  let taskTypes      = $state([])
  let taskRoles      = $state([])
  let loading        = $state(false)
  let showForm       = $state(false)
  let filterStatus        = $state('')
  let filterProject       = $state('')
  let filterRequirement   = $state('')
  let filterFeature       = $state('')
  let filterReqOptions    = $state([])
  let filterFeatOptions   = $state([])
  let selectedTaskId      = $state('')
  let showSidebar         = $state(false)

  const statusColors = {
    BACKLOG:           'bg-blue-900 text-blue-300',
    DEVELOPING:        'bg-orange-900 text-orange-300',
    AWAITING_REVIEW:   'bg-yellow-900 text-yellow-300',
    REVIEWING:         'bg-purple-900 text-purple-300',
    AWAITING_REVISION: 'bg-rose-900 text-rose-300',
    AWAITING_MERGE:    'bg-cyan-900 text-cyan-300',
    MERGING:           'bg-indigo-900 text-indigo-300',
    COMPLETED:         'bg-green-900 text-green-300',
    FAILED:            'bg-red-900 text-red-300',
  }

  let form = $state({
    project_id:  '',
    type:        'implement',
    role:        'worker',
    title:       '',
    description: '',
    priority:    5,
  })
  let formAvailReqs   = $state([])
  let formAvailFeats  = $state([])
  let formLinkedReqs  = $state(new Set())
  let formLinkedFeats = $state(new Set())

  // ── Log panel state ────────────────────────────────────────────────────────
  let taskLogs        = $state([])
  let logsLoading     = $state(false)
  let logEventFilter  = $state('')
  let logSearch       = $state('')
  let hiddenTypes     = $state(new Set())
  let chartTypeFilter = $state(new Set())
  let bucketMinutes   = $state(60)

  const TASK_COLORS = {
    task_created:              '#34d399',
    task_updated:              '#60a5fa',
    task_queued:               '#facc15',
    task_claimed:              '#a78bfa',
    task_started:              '#fb923c',
    task_completed:            '#22c55e',
    task_failed:               '#ef4444',
    task_retried:              '#f97316',
    task_cancelled:            '#6b7280',
    task_timeout:              '#f43f5e',
    task_result:               '#38bdf8',
    task_error:                '#e879f9',
    task_dependency_warning:   '#fbbf24',
    task_dependency_added:     '#a3e635',
    task_dependency_removed:   '#fb923c',
    task_checklist_changed:    '#67e8f9',
    task_comment_added:        '#c084fc',
    task_link_added:           '#86efac',
    task_link_removed:         '#fca5a5',
    // W7.2 — new lifecycle events
    task_submitted_for_review: '#f59e0b',
    task_review_posted:        '#8b5cf6',
    task_revision_started:     '#ec4899',
    task_merge_started:        '#06b6d4',
    task_merge_failed:         '#dc2626',
    task_pushed_upstream:      '#10b981',
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function loadFilterOptions(projectId) {
    if (!projectId) {
      filterReqOptions = []; filterFeatOptions = []
      filterRequirement = ''; filterFeature = ''
      return
    }
    try {
      const [reqs, feats] = await Promise.all([
        listRequirements(projectId),
        listFeatures(projectId),
      ])
      filterReqOptions  = reqs ?? []
      filterFeatOptions = feats ?? []
    } catch (_) {
      filterReqOptions = []; filterFeatOptions = []
    }
  }

  async function loadFormProject(projectId) {
    formLinkedReqs = new Set(); formLinkedFeats = new Set()
    if (!projectId) { formAvailReqs = []; formAvailFeats = []; return }
    try {
      const [reqs, feats] = await Promise.all([
        listRequirements(projectId),
        listFeatures(projectId),
      ])
      formAvailReqs  = reqs ?? []
      formAvailFeats = feats ?? []
    } catch (_) {
      formAvailReqs = []; formAvailFeats = []
    }
  }

  function toggleFormReq(id) {
    const next = new Set(formLinkedReqs)
    next.has(id) ? next.delete(id) : next.add(id)
    formLinkedReqs = next
  }

  function toggleFormFeat(id) {
    const next = new Set(formLinkedFeats)
    next.has(id) ? next.delete(id) : next.add(id)
    formLinkedFeats = next
  }

  async function onProjectFilterChange() {
    filterRequirement = ''; filterFeature = ''
    await loadFilterOptions(filterProject)
    loadTasks()
  }

  async function loadTasks() {
    loading = true
    try {
      const params = {}
      if (filterStatus)      params.status         = filterStatus
      if (filterProject)     params.project_id     = filterProject
      if (filterRequirement) params.requirement_id = filterRequirement
      if (filterFeature)     params.feature_id     = filterFeature
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
    if (!selectedTaskId) showSidebar = false
    fetchLogs()
  }

  function toggleType(t) {
    const s = new Set(hiddenTypes)
    s.has(t) ? s.delete(t) : s.add(t)
    hiddenTypes = s
  }

  function clickChart(t) {
    const next = new Set(chartTypeFilter)
    if (next.has(t)) next.delete(t)
    else next.add(t)
    chartTypeFilter = next
  }

  if (typeof window !== 'undefined') {
    window._taskChartClick = clickChart
  }

  // ── Derived: filtered logs ─────────────────────────────────────────────────
  let visibleLogs = $derived(taskLogs.filter(l => {
    if (chartTypeFilter.size > 0 && !chartTypeFilter.has(l.event_type)) return false
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
      const created = await createTask({
        project_id: form.project_id,
        type:       form.type.trim(),
        role:       form.role.trim(),
        priority:   Number(form.priority),
        payload:    { title: form.title.trim(), description: form.description.trim() },
      })
      const ops = []
      for (const id of formLinkedReqs)  ops.push(addTaskLink(created.id, 'requirement', id))
      for (const id of formLinkedFeats) ops.push(addTaskLink(created.id, 'feature', id))
      if (ops.length) await Promise.all(ops)
      toasts.success('Task created')
      form            = { project_id: '', type: '', role: '', title: '', description: '', priority: 5 }
      formLinkedReqs  = new Set()
      formLinkedFeats = new Set()
      formAvailReqs   = []
      formAvailFeats  = []
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

  async function queueTaskAction(id) {
    try {
      await queueTask(id)
      await loadTasks()
    } catch (e) {
      toasts.error('Queue failed: ' + e.message)
    }
  }

  async function remove(id) {
    if (!confirm('Delete this task?')) return
    try {
      await deleteTask(id)
      toasts.success('Task deleted')
      if (selectedTaskId === id) { selectedTaskId = ''; showSidebar = false; fetchLogs() }
      await loadTasks()
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  // ── Lifecycle ──────────────────────────────────────────────────────────────
  let timer = null
  onMount(async () => {
    refreshAll()
    let intervalMs = 5_000
    try {
      const all = await listSettings()
      const s = all.find(s => s.key === 'platform.charts.autorefresh_ms')
      if (s) {
        const parsed = parseInt(s.value, 10)
        if (parsed > 0) intervalMs = parsed
      }
    } catch (_) { /* fall back to default */ }
    timer = setInterval(refreshAll, intervalMs)
  })
  onDestroy(() => clearInterval(timer))
</script>

<div class="flex h-full overflow-hidden">
<div class="flex-1 flex flex-col overflow-hidden min-w-0">

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
          {#each ['BACKLOG','DEVELOPING','AWAITING_REVIEW','REVIEWING','AWAITING_REVISION','AWAITING_MERGE','MERGING','COMPLETED','FAILED'] as s}
            <option value={s}>{s}</option>
          {/each}
        </select>
        <select
          class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
          bind:value={filterProject}
          onchange={onProjectFilterChange}
        >
          <option value="">All projects</option>
          {#each projects as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
        {#if filterReqOptions.length > 0}
          <select
            class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
            bind:value={filterRequirement}
            onchange={loadTasks}
          >
            <option value="">All requirements</option>
            {#each filterReqOptions as r}<option value={r.id}>{r.title}</option>{/each}
          </select>
        {/if}
        {#if filterFeatOptions.length > 0}
          <select
            class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
            bind:value={filterFeature}
            onchange={loadTasks}
          >
            <option value="">All features</option>
            {#each filterFeatOptions as f}<option value={f.id}>{f.title}</option>{/each}
          </select>
        {/if}
        {#if selectedTaskId}
          <span class="text-xs text-accent">Filtered to task</span>
          <button class="text-xs text-gray-500 hover:text-gray-300" onclick={() => { selectedTaskId = ''; showSidebar = false; fetchLogs() }}>× Clear</button>
          <button
            class="text-xs px-2 py-1 rounded transition-colors {showSidebar ? 'bg-accent text-white' : 'bg-surface-700 text-gray-400 hover:bg-surface-600'}"
            onclick={() => showSidebar = !showSidebar}
          >💬 Assistant</button>
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
            bind:value={form.project_id}
            onchange={() => loadFormProject(form.project_id)}
            required
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
        {#if formAvailReqs.length > 0 || formAvailFeats.length > 0}
          <div class="grid grid-cols-2 gap-2">
            {#if formAvailReqs.length > 0}
              <div>
                <span class="text-xs text-gray-500 mb-1 block">Link Requirements</span>
                <div class="bg-surface-700 border border-surface-500 rounded p-2 max-h-24 overflow-y-auto flex flex-col gap-0.5">
                  {#each formAvailReqs as req (req.id)}
                    <label class="flex items-center gap-1.5 text-xs cursor-pointer hover:text-gray-200 text-gray-300">
                      <input type="checkbox" checked={formLinkedReqs.has(req.id)} onchange={() => toggleFormReq(req.id)} class="accent-accent" />
                      {req.title}
                    </label>
                  {/each}
                </div>
              </div>
            {/if}
            {#if formAvailFeats.length > 0}
              <div>
                <span class="text-xs text-gray-500 mb-1 block">Link Features</span>
                <div class="bg-surface-700 border border-surface-500 rounded p-2 max-h-24 overflow-y-auto flex flex-col gap-0.5">
                  {#each formAvailFeats as feat (feat.id)}
                    <label class="flex items-center gap-1.5 text-xs cursor-pointer hover:text-gray-200 text-gray-300">
                      <input type="checkbox" checked={formLinkedFeats.has(feat.id)} onchange={() => toggleFormFeat(feat.id)} class="accent-accent" />
                      {feat.title}
                    </label>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        {/if}
        <div class="flex items-center gap-2">
          <label for="task-priority" class="text-xs text-gray-400 shrink-0">Priority</label>
          <input id="task-priority" type="range" min="1" max="10" bind:value={form.priority} class="flex-1" />
          <span class="text-xs text-gray-300 w-4 text-right">{form.priority}</span>
          <button type="submit" class="ml-2 px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors">Create</button>
        </div>
      </form>
    {/if}

    {#if loading && tasks.length === 0}
      <Skeleton rows={4} />
    {:else if tasks.length === 0}
      <p class="px-6 pb-4 text-gray-400 text-sm">No tasks found.</p>
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
                  <button
                    class="text-xs font-medium text-gray-100 truncate hover:text-accent text-left"
                    onclick={(e) => { e.stopPropagation(); router.push('tasks', t.id) }}
                  >{taskTitle(t)}</button>
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
                {#if t.status === 'FAILED' || t.status === 'COMPLETED'}
                  <button
                    class="text-[10px] text-yellow-400 hover:text-yellow-300"
                    onclick={(e) => { e.stopPropagation(); queueTaskAction(t.id) }}
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
    {#each chartTypeFilter as filterType (filterType)}
      <span class="flex items-center gap-1 px-2 py-0.5 bg-accent/20 text-accent text-xs rounded">
        {filterType}
        <button onclick={() => { const next = new Set(chartTypeFilter); next.delete(filterType); chartTypeFilter = next }} class="hover:text-white" aria-label={`Remove ${filterType} filter`}>×</button>
      </span>
    {/each}
  </div>

  <!-- ── Log list ────────────────────────────────────────────────────────────── -->
  <div class="flex-1 overflow-y-auto text-xs">
    {#if logsLoading && taskLogs.length === 0}
      <Skeleton rows={4} mode="table" />
    {:else if visibleLogs.length === 0}
      <p class="p-4 text-gray-400">No events match current filters.</p>
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

{#if showSidebar && selectedTaskId}
  <AssistantSidebar scope={{ kind: 'task', id: selectedTaskId }} />
{/if}

</div>
