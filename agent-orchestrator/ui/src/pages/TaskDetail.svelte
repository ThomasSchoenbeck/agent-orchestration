<script>
  import { onMount } from 'svelte'
  import { router, toasts } from '../lib/stores.js'
  import {
    getTask, updateTask, deleteTask, unqueueTask,
    getProject, listTaskLogs,
  } from '../lib/api.js'
  import MarkdownEditor from '../components/MarkdownEditor.svelte'

  let { taskId } = $props()

  // ── State ─────────────────────────────────────────────────────────────────
  let task        = $state(null)
  let project     = $state(null)
  let loading     = $state(true)
  let editing     = $state(false)
  let taskLogs    = $state([])
  let logsLoading = $state(false)

  // Edit buffer
  let editBuf = $state({})

  // ── Helpers ───────────────────────────────────────────────────────────────
  const statusColors = {
    planned:     'bg-blue-900 text-blue-300',
    queued:      'bg-yellow-900 text-yellow-300',
    in_progress: 'bg-orange-900 text-orange-300',
    needs_review:'bg-purple-900 text-purple-300',
    completed:   'bg-green-900 text-green-300',
    failed:      'bg-red-900 text-red-300',
  }

  function formatDate(iso) {
    if (!iso) return '—'
    const d = new Date(iso)
    return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'})
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function loadLogs(id) {
    logsLoading = true
    try {
      const data = await listTaskLogs(id)
      taskLogs = Array.isArray(data) ? data : []
    } catch (e) {
      taskLogs = []
    } finally {
      logsLoading = false
    }
  }

  async function loadAll() {
    loading = true
    try {
      const t = await getTask(taskId)
      task = t
      if (t.project_id) {
        project = await getProject(t.project_id)
      }
      loadLogs(taskId)
    } catch (e) {
      toasts.error('Failed to load task: ' + e.message)
    } finally {
      loading = false
    }
  }

  // ── Task editing ──────────────────────────────────────────────────────────
  function startEdit() {
    editBuf = {
      title:       task.payload?.title ?? '',
      description: task.payload?.description ?? '',
      priority:    task.priority,
      status:      task.status,
    }
    editing = true
  }

  async function saveTask() {
    try {
      const updated = await updateTask(taskId, {
        priority: Number(editBuf.priority),
        status:   editBuf.status,
        payload:  {
          title:       editBuf.title.trim(),
          description: editBuf.description.trim(),
        },
      })
      task = updated
      editing = false
      toasts.success('Task saved')
    } catch (e) {
      toasts.error('Save failed: ' + e.message)
    }
  }

  async function handleUnqueue() {
    if (!confirm('Unqueue this task?')) return
    try {
      const updated = await unqueueTask(taskId)
      task = updated
      toasts.success('Task unqueued')
    } catch (e) {
      toasts.error('Unqueue failed: ' + e.message)
    }
  }

  async function handleDelete() {
    if (!confirm('Delete this task?')) return
    try {
      await deleteTask(taskId)
      toasts.success('Task deleted')
      router.go('tasks')
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  async function handleQueue() {
    try {
      const updated = await updateTask(taskId, { status: 'queued' })
      task = updated
      toasts.success('Task queued')
    } catch (e) {
      toasts.error('Queue failed: ' + e.message)
    }
  }

  onMount(loadAll)
</script>

<div class="flex-1 overflow-y-auto p-6 max-w-4xl mx-auto w-full">

  <!-- Breadcrumb -->
  <nav class="mb-5 text-sm text-gray-500 flex items-center gap-2 flex-wrap">
    <button
      class="hover:text-gray-300 transition-colors"
      onclick={() => router.go('projects')}
    >Projects</button>
    <span>›</span>
    {#if project}
      <button
        class="hover:text-gray-300 transition-colors"
        onclick={() => router.push('projects', project.id)}
      >{project.name}</button>
      <span>›</span>
    {/if}
    <span class="text-gray-300">Tasks</span>
    <span>›</span>
    <span class="text-gray-300">{task?.payload?.title ?? '…'}</span>
  </nav>

  {#if loading}
    <p class="text-gray-500 text-sm">Loading…</p>

  {:else if !task}
    <p class="text-gray-500 text-sm">Task not found.</p>

  {:else}
    <!-- ── Task header ────────────────────────────────────────────────────── -->
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600">
      {#if editing}
        <!-- Edit form -->
        <div class="flex flex-col gap-4">
          <input
            class="text-xl font-semibold bg-surface-700 border border-surface-500 rounded px-3 py-2
                   text-gray-100 focus:outline-none focus:border-accent"
            placeholder="Task title *"
            bind:value={editBuf.title}
            required
          />

          <div>
            <label class="text-xs text-gray-500 mb-1 block">Description</label>
            <MarkdownEditor bind:value={editBuf.description} minHeight="140px" />
          </div>

          <div class="grid grid-cols-3 gap-3">
            <div>
              <label for="task-status" class="text-xs text-gray-500 mb-1 block">Status</label>
              <select
                id="task-status"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-300 focus:outline-none focus:border-accent"
                bind:value={editBuf.status}
              >
                {#each ['planned','queued','in_progress','needs_review','completed','failed'] as s}
                  <option value={s}>{s}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="task-priority" class="text-xs text-gray-500 mb-1 block">Priority</label>
              <input
                id="task-priority"
                type="number"
                min="1" max="10"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-200 focus:outline-none focus:border-accent"
                bind:value={editBuf.priority}
              />
            </div>
          </div>

          <div class="flex justify-end gap-2">
            <button
              class="px-3 py-1.5 text-sm text-gray-400 hover:text-gray-200 transition-colors"
              onclick={() => editing = false}
            >Cancel</button>
            <button
              class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
              onclick={saveTask}
            >Save</button>
          </div>
        </div>

      {:else}
        <!-- Read view -->
        <div class="flex items-start justify-between gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-3 mb-2 flex-wrap">
              <h1 class="text-2xl font-semibold text-gray-100">{task.payload?.title ?? 'Untitled'}</h1>
              <span class="text-xs px-2 py-0.5 rounded-full
                {statusColors[task.status] || 'bg-gray-700 text-gray-300'}">
                {task.status}
              </span>
            </div>

            {#if task.payload?.description}
              <!-- Render description as markdown (readonly) -->
              <div class="mb-4">
                <MarkdownEditor value={task.payload.description} readonly={true} minHeight="0px" />
              </div>
            {:else}
              <p class="text-sm text-gray-500 italic mb-4">No description.</p>
            {/if}

            <!-- Metadata -->
            <div class="flex flex-wrap gap-4 text-xs text-gray-500">
              <span title="Type">📋 {task.type}</span>
              <span title="Role">👤 {task.role}</span>
              <span title="Priority">⭐ {task.priority ?? '—'}</span>
              {#if project}
                <span title="Project">📁 {project.name}</span>
              {/if}
              <span class="text-gray-600">ID: {task.id}</span>
            </div>

            <div class="flex flex-wrap gap-4 text-xs text-gray-600 mt-3 font-mono">
              <span>Created: {formatDate(task.created_at)}</span>
              <span>Updated: {formatDate(task.updated_at)}</span>
            </div>
          </div>

          <button
            class="px-3 py-1.5 text-sm border border-surface-500 text-gray-400
                   hover:border-accent hover:text-gray-200 rounded transition-colors shrink-0"
            onclick={startEdit}
          >Edit</button>
        </div>
      {/if}
    </div>

    <!-- ── Agent / timestamps / result / events ─────────────────────────────── -->
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3">

      {#if task.assigned_agent_id}
        <div class="flex items-center gap-2 text-sm">
          <span class="text-gray-400">Agent:</span>
          <span class="font-mono text-accent">{task.assigned_agent_id}</span>
        </div>
      {/if}

      {#if task.started_at}
        <div class="flex gap-4 text-xs text-gray-500">
          <span>Started: {new Date(task.started_at).toLocaleString()}</span>
          {#if task.completed_at}
            <span>Completed: {new Date(task.completed_at).toLocaleString()}</span>
          {/if}
        </div>
      {/if}

      {#if task.result && Object.keys(task.result).length > 0}
        <div class="mt-4">
          <h3 class="text-sm font-semibold text-gray-300 mb-2">Result</h3>
          <pre class="bg-surface-800 rounded p-3 text-xs text-gray-300 overflow-x-auto whitespace-pre-wrap">{JSON.stringify(task.result, null, 2)}</pre>
        </div>
      {/if}

      <div class="mt-6">
        <h3 class="text-sm font-semibold text-gray-300 mb-3">Task Events</h3>
        {#if logsLoading}
          <p class="text-xs text-gray-500">Loading events…</p>
        {:else if taskLogs.length === 0}
          <p class="text-xs text-gray-500">No events yet.</p>
        {:else}
          <div class="flex flex-col gap-2">
            {#each taskLogs as log (log.id)}
              <div class="flex items-start gap-3 text-xs">
                <span class="shrink-0 text-gray-500 font-mono w-36">
                  {new Date(log.timestamp).toLocaleTimeString()}
                </span>
                <span class="shrink-0 px-1.5 py-0.5 rounded text-[10px] font-medium
                  {log.event_type.includes('failed') || log.event_type.includes('error')
                    ? 'bg-red-900 text-red-300'
                    : log.event_type.includes('complet')
                      ? 'bg-green-900 text-green-300'
                      : 'bg-surface-700 text-gray-400'}">
                  {log.event_type}
                </span>
                {#if log.old_status && log.new_status}
                  <span class="text-gray-500">{log.old_status} → {log.new_status}</span>
                {/if}
                {#if log.agent_id}
                  <span class="text-gray-600 font-mono truncate">{log.agent_id.slice(0,8)}</span>
                {/if}
                {#if log.description}
                  <span class="text-gray-400 truncate">{log.description}</span>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>

    </div>

    <!-- ── Queue controls ─────────────────────────────────────────────────── -->
    <div class="p-5 bg-surface-800 rounded border border-surface-600">
      <h3 class="text-sm font-semibold text-gray-200 mb-3">Actions</h3>
      <div class="flex gap-2 flex-wrap">
        {#if task.status === 'planned' || task.status === 'failed'}
          <button
            class="px-3 py-1.5 text-sm border border-yellow-600 text-yellow-400
                   hover:bg-yellow-900 hover:border-yellow-500 rounded transition-colors"
            onclick={handleQueue}
          >Queue</button>
        {/if}
        {#if task.status === 'queued' || task.status === 'planned'}
          <button
            class="px-3 py-1.5 text-sm border border-orange-600 text-orange-400
                   hover:bg-orange-900 hover:border-orange-500 rounded transition-colors"
            onclick={handleUnqueue}
          >Unqueue</button>
        {/if}
        <button
          class="px-3 py-1.5 text-sm border border-red-600 text-red-400
                 hover:bg-red-900 hover:border-red-500 rounded transition-colors"
          onclick={handleDelete}
        >Delete</button>
      </div>
    </div>
  {/if}
</div>
