<script>
  import { onMount } from 'svelte'
  import { router } from '../lib/stores.js'
  import { toasts } from '../lib/stores.js'
  import {
    getProject, updateProject,
    listProjectTasks, createTask, updateTask, deleteTask,
    getTaskTypes, getTaskRoles,
  } from '../lib/api.js'
  import MarkdownEditor from '../components/MarkdownEditor.svelte'

  let { projectId } = $props()

  // ── State ─────────────────────────────────────────────────────────────────
  let project      = $state(null)
  let tasks        = $state([])
  let taskTypes    = $state([])
  let taskRoles    = $state([])
  let loading      = $state(true)
  let editing      = $state(false)
  let showTaskForm = $state(false)
  let filterStatus = $state('')

  // Edit buffer — populated when editing starts
  let editBuf = $state({})

  // New task form
  let taskForm = $state({ type: 'implement', role: 'worker', title: '', description: '', priority: 5 })

  // ── Helpers ───────────────────────────────────────────────────────────────
  const statusColors = {
    planned:     'bg-blue-900 text-blue-300',
    queued:      'bg-yellow-900 text-yellow-300',
    in_progress: 'bg-orange-900 text-orange-300',
    needs_review:'bg-purple-900 text-purple-300',
    completed:   'bg-green-900 text-green-300',
    failed:      'bg-red-900 text-red-300',
  }

  const projectStatusOptions = ['planned', 'in_progress', 'completed', 'failed']

  function taskTitle(t) {
    return t.payload?.title ?? t.type ?? t.id
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function loadAll() {
    loading = true
    try {
      const [p, tt, tr] = await Promise.all([
        getProject(projectId),
        getTaskTypes(),
        getTaskRoles(),
      ])
      project   = p
      taskTypes = Array.isArray(tt) ? tt : []
      taskRoles = Array.isArray(tr) ? tr : []
      await loadTasks()
    } catch (e) {
      toasts.error('Failed to load project: ' + e.message)
    } finally {
      loading = false
    }
  }

  async function loadTasks() {
    try {
      const params = {}
      if (filterStatus) params.status = filterStatus
      const res = await listProjectTasks(projectId, params)
      tasks = Array.isArray(res) ? res : []
    } catch (e) {
      toasts.error('Failed to load tasks: ' + e.message)
    }
  }

  // ── Project editing ───────────────────────────────────────────────────────
  function startEdit() {
    editBuf = {
      name:        project.name,
      description: project.description,
      repo_path:   project.repo_path,
      git_url:     project.git_url,
      status:      project.status,
    }
    editing = true
  }

  async function saveProject() {
    try {
      const updated = await updateProject(projectId, {
        name:        editBuf.name,
        description: editBuf.description,
        repo_path:   editBuf.repo_path,
        git_url:     editBuf.git_url,
        status:      editBuf.status,
      })
      project = updated
      editing = false
      toasts.success('Project saved')
    } catch (e) {
      toasts.error('Save failed: ' + e.message)
    }
  }

  // ── Task actions ──────────────────────────────────────────────────────────
  async function submitTask() {
    if (!taskForm.title.trim()) { toasts.error('Task title is required'); return }
    try {
      await createTask({
        project_id: projectId,
        type:       taskForm.type,
        role:       taskForm.role,
        priority:   Number(taskForm.priority),
        payload:    { title: taskForm.title.trim(), description: taskForm.description.trim() },
      })
      toasts.success('Task created')
      taskForm     = { type: 'implement', role: 'worker', title: '', description: '', priority: 5 }
      showTaskForm = false
      await loadTasks()
    } catch (e) {
      toasts.error('Create failed: ' + e.message)
    }
  }

  async function setTaskStatus(id, status) {
    try {
      await updateTask(id, { status })
      await loadTasks()
    } catch (e) {
      toasts.error('Update failed: ' + e.message)
    }
  }

  async function removeTask(id) {
    if (!confirm('Delete this task?')) return
    try {
      await deleteTask(id)
      toasts.success('Task deleted')
      await loadTasks()
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  onMount(loadAll)
</script>

<div class="flex-1 overflow-y-auto p-6 max-w-5xl mx-auto w-full">

  <!-- Breadcrumb -->
  <nav class="mb-5 text-sm text-gray-500 flex items-center gap-2">
    <button
      class="hover:text-gray-300 transition-colors"
      onclick={() => router.go('projects')}
    >Projects</button>
    <span>›</span>
    <span class="text-gray-300">{project?.name ?? '…'}</span>
  </nav>

  {#if loading}
    <p class="text-gray-500 text-sm">Loading…</p>

  {:else if !project}
    <p class="text-gray-500 text-sm">Project not found.</p>

  {:else}
    <!-- ── Project header ─────────────────────────────────────────────────── -->
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600">
      {#if editing}
        <!-- Edit form -->
        <div class="flex flex-col gap-4">
          <div class="flex gap-3">
            <input
              class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2
                     text-sm text-gray-200 focus:outline-none focus:border-accent"
              placeholder="Project name *"
              bind:value={editBuf.name}
            />
            <select
              class="bg-surface-700 border border-surface-500 rounded px-3 py-2
                     text-sm text-gray-300 focus:outline-none focus:border-accent"
              bind:value={editBuf.status}
            >
              {#each projectStatusOptions as s}
                <option value={s}>{s}</option>
              {/each}
            </select>
          </div>

          <div>
            <label class="text-xs text-gray-500 mb-1 block">Description</label>
            <MarkdownEditor bind:value={editBuf.description} minHeight="140px" />
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label for="project-repo-path" class="text-xs text-gray-500 mb-1 block">Local path</label>
              <input
                id="project-repo-path"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-200 font-mono focus:outline-none focus:border-accent"
                placeholder="/path/to/project"
                bind:value={editBuf.repo_path}
              />
            </div>
            <div>
              <label for="project-git-url" class="text-xs text-gray-500 mb-1 block">Git remote URL</label>
              <input
                id="project-git-url"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-200 font-mono focus:outline-none focus:border-accent"
                placeholder="https://github.com/user/repo.git"
                bind:value={editBuf.git_url}
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
              onclick={saveProject}
            >Save</button>
          </div>
        </div>

      {:else}
        <!-- Read view -->
        <div class="flex items-start justify-between gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-3 mb-2 flex-wrap">
              <h1 class="text-xl font-semibold text-gray-100">{project.name}</h1>
              <span class="text-xs px-2 py-0.5 rounded-full
                {statusColors[project.status] || 'bg-gray-700 text-gray-300'}">
                {project.status}
              </span>
            </div>

            {#if project.description}
              <!-- Render description as markdown (readonly) -->
              <div class="mb-3">
                <MarkdownEditor value={project.description} readonly={true} minHeight="0px" />
              </div>
            {:else}
              <p class="text-sm text-gray-500 italic mb-3">No description.</p>
            {/if}

            <div class="flex gap-4 flex-wrap text-xs text-gray-500 font-mono">
              {#if project.repo_path}
                <span title="Local path">📁 {project.repo_path}</span>
              {/if}
              {#if project.git_url}
                <a
                  href={project.git_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="hover:text-accent transition-colors"
                  title="Git remote"
                >🔗 {project.git_url}</a>
              {/if}
              <span class="text-gray-600">ID: {project.id}</span>
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

    <!-- ── Tasks panel ────────────────────────────────────────────────────── -->
    <div>
      <div class="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 class="text-base font-semibold text-gray-200">Tasks
          <span class="ml-1 text-xs font-normal text-gray-500">({tasks.length})</span>
        </h2>
        <div class="flex items-center gap-2">
          <select
            class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs
                   text-gray-300 focus:outline-none focus:border-accent"
            bind:value={filterStatus}
            onchange={loadTasks}
          >
            <option value="">All statuses</option>
            {#each ['planned','queued','in_progress','needs_review','completed','failed'] as s}
              <option value={s}>{s}</option>
            {/each}
          </select>
          <button
            class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors"
            onclick={() => showTaskForm = !showTaskForm}
          >{showTaskForm ? 'Cancel' : '+ Task'}</button>
        </div>
      </div>

      <!-- New task form -->
      {#if showTaskForm}
        <form
          class="mb-4 p-4 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3"
          onsubmit={(e) => { e.preventDefault(); submitTask() }}
        >
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label for="task-type" class="text-xs text-gray-500 mb-1 block">Type</label>
              <select
                id="task-type"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-300 focus:outline-none focus:border-accent"
                bind:value={taskForm.type}
              >
                {#each taskTypes as tt}
                  <option value={tt.value} title={tt.description}>{tt.label}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="task-role" class="text-xs text-gray-500 mb-1 block">Role</label>
              <select
                id="task-role"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-300 focus:outline-none focus:border-accent"
                bind:value={taskForm.role}
              >
                {#each taskRoles as tr}
                  <option value={tr.value} title={tr.description}>{tr.label}</option>
                {/each}
              </select>
            </div>
          </div>

          <input
            class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="Title *"
            bind:value={taskForm.title}
            required
          />

          <div>
            <label class="text-xs text-gray-500 mb-1 block">Description</label>
            <MarkdownEditor bind:value={taskForm.description} minHeight="100px" placeholder="Task description…" />
          </div>

          <div class="flex items-center gap-3">
            <label for="task-priority" class="text-xs text-gray-400 shrink-0">Priority</label>
            <input id="task-priority" type="range" min="1" max="10" bind:value={taskForm.priority} class="flex-1" />
            <span class="text-xs text-gray-300 w-4 text-right">{taskForm.priority}</span>
          </div>

          <button
            type="submit"
            class="self-end px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
          >Create task</button>
        </form>
      {/if}

      <!-- Task list -->
      {#if tasks.length === 0}
        <p class="text-gray-500 text-sm py-4">No tasks yet. Add the first one above.</p>
      {:else}
        <div class="flex flex-col gap-2">
          {#each tasks as t (t.id)}
            <div class="p-3 bg-surface-800 rounded border border-surface-600 hover:border-surface-500 transition-colors">
              <div class="flex items-start justify-between gap-3">
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-sm font-medium text-gray-100 truncate">{taskTitle(t)}</span>
                    <span class="text-xs px-2 py-0.5 rounded-full {statusColors[t.status] || 'bg-gray-700 text-gray-300'}">
                      {t.status}
                    </span>
                    <span class="text-xs text-gray-500 bg-surface-700 px-1.5 py-0.5 rounded">{t.type}</span>
                    <span class="text-xs text-gray-600">{t.role}</span>
                  </div>
                  {#if t.payload?.description}
                    <p class="text-xs text-gray-400 mt-1 line-clamp-2">{t.payload.description}</p>
                  {/if}
                </div>
                <div class="flex items-center gap-2 shrink-0">
                  {#if t.status === 'planned' || t.status === 'failed'}
                    <button
                      class="text-xs text-yellow-400 hover:text-yellow-300 transition-colors"
                      onclick={() => setTaskStatus(t.id, 'queued')}
                    >Queue</button>
                  {/if}
                  <button
                    class="text-xs text-red-400 hover:text-red-300 transition-colors"
                    onclick={() => removeTask(t.id)}
                  >Del</button>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>
