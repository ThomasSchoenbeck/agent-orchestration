<script>
  import { onMount } from 'svelte'
  import { listTasks, listProjects, createTask, updateTask, deleteTask, getTaskTypes, getTaskRoles } from '../lib/api.js'
  import { toasts, router } from '../lib/stores.js'

  // ── Svelte 5 runes ────────────────────────────────────────────────────────
  let tasks         = $state([])
  let projects      = $state([])
  let taskTypes     = $state([])
  let taskRoles     = $state([])
  let loading       = $state(false)
  let showForm      = $state(false)
  let filterStatus  = $state('')
  let filterProject = $state('')

  let form = $state({
    project_id:  '',
    type:        'implement',
    role:        'worker',
    title:       '',
    description: '',
    priority:    5,
  })

  const statusColors = {
    pending:     'bg-gray-700 text-gray-300',
    planned:     'bg-blue-900 text-blue-300',
    queued:      'bg-yellow-900 text-yellow-300',
    in_progress: 'bg-orange-900 text-orange-300',
    completed:   'bg-green-900 text-green-300',
    failed:      'bg-red-900 text-red-300',
  }

  function taskTitle(t) {
    return t.payload?.title ?? t.type ?? t.id
  }

  function projectName(projectId) {
    return projects.find(p => p.id === projectId)?.name ?? 'Unknown'
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function load() {
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
      await load()
    } catch (e) {
      toasts.error('Create failed: ' + e.message)
    }
  }

  // ── Status update ────────────────────────────────────────────────────────
  async function setStatus(id, status) {
    try {
      await updateTask(id, { status })
      await load()
    } catch (e) {
      toasts.error('Update failed: ' + e.message)
    }
  }

  // ── Delete ────────────────────────────────────────────────────────────────
  async function remove(id) {
    if (!confirm('Delete this task?')) return
    try {
      await deleteTask(id)
      toasts.success('Task deleted')
      await load()
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  onMount(load)
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center justify-between mb-4">
    <h1 class="text-xl font-semibold text-gray-100">Tasks</h1>
    <button
      class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
      onclick={() => showForm = !showForm}
    >
      {showForm ? 'Cancel' : '+ New Task'}
    </button>
  </div>

  <!-- Filters -->
  <div class="flex gap-3 mb-4 flex-wrap">
    <select
      class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-sm text-gray-300
             focus:outline-none focus:border-accent"
      bind:value={filterStatus}
      onchange={load}
    >
      <option value="">All statuses</option>
      {#each ['pending','planned','queued','in_progress','completed','failed'] as s}
        <option value={s}>{s}</option>
      {/each}
    </select>
    <select
      class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-sm text-gray-300
             focus:outline-none focus:border-accent"
      bind:value={filterProject}
      onchange={load}
    >
      <option value="">All projects</option>
      {#each projects as p}
        <option value={p.id}>{p.name}</option>
      {/each}
    </select>
    <button
      class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
      onclick={load}
    >↻ Refresh</button>
  </div>

  {#if showForm}
    <form
      class="mb-6 p-4 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3"
      onsubmit={(e) => { e.preventDefault(); submit() }}
    >
      <div class="grid grid-cols-2 gap-3">
        <select
          class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-300
                 focus:outline-none focus:border-accent"
          bind:value={form.project_id}
          required
        >
          <option value="">Select project *</option>
          {#each projects as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
        <input
          class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-200
                 placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="Title"
          bind:value={form.title}
        />
        <select
          aria-label="Task type"
          class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-300
                 focus:outline-none focus:border-accent"
          bind:value={form.type}
          required
        >
          {#each taskTypes as tt}
            <option value={tt.value} title={tt.description}>{tt.label}</option>
          {/each}
          {#if taskTypes.length === 0}
            <option value="implement">Implement</option>
          {/if}
        </select>
        <select
          aria-label="Role"
          class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-300
                 focus:outline-none focus:border-accent"
          bind:value={form.role}
          required
        >
          {#each taskRoles as tr}
            <option value={tr.value} title={tr.description}>{tr.label}</option>
          {/each}
          {#if taskRoles.length === 0}
            <option value="worker">Worker</option>
          {/if}
        </select>
      </div>
      <textarea
        class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-200
               placeholder-gray-500 focus:outline-none focus:border-accent resize-none"
        placeholder="Description"
        rows="3"
        bind:value={form.description}
      ></textarea>
      <div class="flex items-center gap-3">
        <label for="task-priority" class="text-xs text-gray-400 shrink-0">Priority</label>
        <input id="task-priority" type="range" min="1" max="10" bind:value={form.priority} class="flex-1" />
        <span class="text-xs text-gray-300 w-4 text-right">{form.priority}</span>
      </div>
      <button
        type="submit"
        class="self-end px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
      >
        Create
      </button>
    </form>
  {/if}

  {#if loading}
    <p class="text-gray-500 text-sm">Loading…</p>
  {:else if tasks.length === 0}
    <p class="text-gray-500 text-sm">No tasks found.</p>
  {:else}
    <div class="flex flex-col gap-2">
      {#each tasks as t (t.id)}
        <div
          class="p-3 bg-surface-800 rounded border border-surface-600 hover:border-surface-500 transition-colors cursor-pointer"
          onclick={() => router.push('tasks', t.id)}
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-sm font-medium text-gray-100 truncate">{taskTitle(t)}</span>
                <span class="text-xs px-2 py-0.5 rounded-full {statusColors[t.status] || 'bg-gray-700 text-gray-300'}">
                  {t.status}
                </span>
                <span class="text-xs text-gray-500 font-mono">{t.type}</span>
                <span class="text-xs text-gray-600">{t.role}</span>
                {#if t.project_id}
                  <span class="text-xs text-gray-500 bg-surface-700 px-1.5 py-0.5 rounded">📁 {projectName(t.project_id)}</span>
                {/if}
              </div>
              {#if t.payload?.description}
                <p class="text-xs text-gray-400 mt-1 line-clamp-2">{t.payload.description}</p>
              {/if}
            </div>
            <div class="flex items-center gap-2 shrink-0">
              {#if t.status === 'planned' || t.status === 'failed'}
                <button
                  class="text-xs text-yellow-400 hover:text-yellow-300 transition-colors"
                  onclick={(e) => { e.stopPropagation(); setStatus(t.id, 'queued') }}
                >Queue</button>
              {/if}
              <button
                class="text-xs text-red-400 hover:text-red-300 transition-colors"
                onclick={(e) => { e.stopPropagation(); remove(t.id) }}
              >Del</button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
