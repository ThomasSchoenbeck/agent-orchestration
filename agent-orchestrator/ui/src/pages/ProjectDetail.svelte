<script>
  import { onMount } from 'svelte'
  import { router } from '../lib/stores.js'
  import { toasts } from '../lib/stores.js'
  import {
    getProject, updateProject,
    listProjectTasks, createTask, updateTask, deleteTask, queueTask, unqueueTask,
    getTaskRoles,
    listRequirements, createRequirement, updateRequirement, deleteRequirement,
    listFeatures, createFeature, updateFeature, deleteFeature,
    initRepo, listBranches, deleteBranch, commitFiles, listCommits,
    resyncProjectScope,
  } from '../lib/api.js'
  import { roleLabel } from '../lib/roles.js'
  import MarkdownEditor from '../components/MarkdownEditor.svelte'
  import AssistantSidebar from '../components/AssistantSidebar.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import FileTree from '../components/FileTree.svelte'
  import CodeEditor from '../components/CodeEditor.svelte'
  import DiffViewer from '../components/DiffViewer.svelte'

  let { projectId } = $props()

  // ── State ─────────────────────────────────────────────────────────────────
  let project      = $state(null)
  let tasks        = $state([])
  let taskRoles    = $state([])
  let loading      = $state(true)
  let editing      = $state(false)
  let showTaskForm = $state(false)
  let filterStatus = $state('')
  let showAssistant = $state(false)

  // ── Repo health ───────────────────────────────────────────────────────────
  let repoMissing   = $state(false)
  let initingRepo   = $state(false)

  async function checkRepo() {
    try {
      await listBranches(projectId)
      repoMissing = false
    } catch (_) {
      repoMissing = true
    }
  }

  async function handleInitRepo() {
    initingRepo = true
    try {
      await initRepo(projectId)
      repoMissing = false
      toasts.success('Repository initialised')
    } catch (e) {
      toasts.error('Init failed: ' + e.message)
    } finally {
      initingRepo = false
    }
  }

  // ── Tab state ─────────────────────────────────────────────────────────────
  let activeTab     = $state('overview')   // 'overview' | 'files' | 'diff'
  let activeFilePath = $state(null)
  let activeFileRef  = $state('main')
  let diffBaseRef    = $state('main')
  let diffHeadRef    = $state('')

  // ── Files tab: branch list + commit log ───────────────────────────────────
  let fileBranches      = $state([])
  let selectedBranch    = $state('main')
  let branchCommits     = $state([])
  let branchesLoading   = $state(false)
  let commitsLoading    = $state(false)

  async function loadFileBranches() {
    branchesLoading = true
    try {
      const b = await listBranches(projectId)
      fileBranches = Array.isArray(b) ? b : []
      if (fileBranches.length && !fileBranches.includes(selectedBranch)) {
        selectedBranch = fileBranches[0]
      }
    } catch (_) {}
    finally { branchesLoading = false }
    await loadBranchCommits()
  }

  async function loadBranchCommits() {
    commitsLoading = true
    try {
      const c = await listCommits(projectId, selectedBranch)
      branchCommits = Array.isArray(c) ? c : []
    } catch (_) { branchCommits = [] }
    finally { commitsLoading = false }
  }

  async function selectBranch(branch) {
    selectedBranch = branch
    activeFilePath = null
    activeFileRef  = branch
    diffHeadRef    = branch
    await loadBranchCommits()
  }

  async function handleDeleteBranch(branch) {
    if (branch === 'main') return
    if (!confirm(`Delete branch "${branch}"? This cannot be undone.`)) return
    try {
      await deleteBranch(projectId, branch)
      toasts.success(`Deleted branch ${branch}`)
      if (selectedBranch === branch) selectedBranch = 'main'
      await loadFileBranches()
    } catch (e) {
      toasts.error('Failed to delete branch: ' + e.message)
    }
  }

  function onFileSelect(path, ref) {
    activeFilePath = path
    activeFileRef  = ref
  }

  // ── Multi-file staging ────────────────────────────────────────────────────
  // stagedFiles: path → { content, ref }
  let stagedFiles    = $state({})
  let stageCommitMsg = $state('')
  let stageSaving    = $state(false)

  function onStageFile(path, content) {
    stagedFiles = { ...stagedFiles, [path]: { content, ref: activeFileRef } }
  }

  function unstageFile(path) {
    const next = { ...stagedFiles }
    delete next[path]
    stagedFiles = next
  }

  async function commitAllStaged() {
    if (!stageCommitMsg.trim() || Object.keys(stagedFiles).length === 0) return
    stageSaving = true
    try {
      // All staged files must share the same branch; use the ref of the first.
      const entries = Object.entries(stagedFiles)
      const branch  = entries[0][1].ref
      await commitFiles(projectId, {
        branch,
        message: stageCommitMsg.trim(),
        files: entries.map(([path, { content }]) => ({ path, content })),
      })
      stagedFiles    = {}
      stageCommitMsg = ''
      toasts.success('Committed ' + entries.length + ' file(s)')
    } catch (e) {
      toasts.error('Commit failed: ' + e.message)
    } finally {
      stageSaving = false
    }
  }

  // Requirements / features state
  let requirements       = $state([])
  let features           = $state([])
  let reqsOpen           = $state(true)
  let featsOpen          = $state(true)
  let reqStatusFilter    = $state([])
  let featStatusFilter   = $state([])
  let newReqTitle        = $state('')
  let newFeatTitle       = $state('')
  let editingReqId       = $state(null)
  let editingFeatId      = $state(null)
  let reqBuf             = $state({})
  let featBuf            = $state({})

  const REQ_STATUSES  = ['proposed', 'accepted', 'satisfied', 'needs_review', 'implemented', 'obsolete']
  const FEAT_STATUSES = ['planned', 'in_progress', 'done', 'needs_review', 'dropped']

  const REQ_STATUS_COLORS = {
    proposed:     'bg-blue-900 text-blue-300',
    accepted:     'bg-green-900 text-green-300',
    satisfied:    'bg-emerald-900 text-emerald-300',
    needs_review: 'bg-amber-900 text-amber-300',
    implemented:  'bg-emerald-900 text-emerald-300',
    obsolete:     'bg-gray-700 text-gray-400',
  }
  const FEAT_STATUS_COLORS = {
    planned:      'bg-blue-900 text-blue-300',
    in_progress:  'bg-orange-900 text-orange-300',
    done:         'bg-green-900 text-green-300',
    needs_review: 'bg-amber-900 text-amber-300',
    dropped:      'bg-gray-700 text-gray-400',
  }

  let filteredReqs  = $derived(
    reqStatusFilter.length === 0 ? requirements : requirements.filter(r => reqStatusFilter.includes(r.status))
  )
  let filteredFeats = $derived(
    featStatusFilter.length === 0 ? features : features.filter(f => featStatusFilter.includes(f.status))
  )

  // Edit buffer — populated when editing starts
  let editBuf = $state({})

  // Load sidebar visibility from localStorage
  function loadSidebarState() {
    if (projectId) {
      const key = `sidebar_${projectId}`
      const stored = localStorage.getItem(key)
      showAssistant = stored === 'true'
    }
  }

  function toggleSidebar() {
    showAssistant = !showAssistant
    if (projectId) {
      localStorage.setItem(`sidebar_${projectId}`, String(showAssistant))
    }
  }

  function applyAssistantToDescription(content) {
    if (editing) {
      editBuf.description = content
    } else {
      startEdit()
      editBuf.description = content
    }
  }

  // New task form
  let taskForm = $state({ role: 'worker', review_role: '', title: '', description: '', priority: 5 })

  // ── Helpers ───────────────────────────────────────────────────────────────
  const statusColors = {
    BACKLOG:           'bg-blue-900 text-blue-300',
    UNQUEUED:          'bg-gray-700 text-gray-300',
    DEVELOPING:        'bg-orange-900 text-orange-300',
    AWAITING_REVIEW:   'bg-yellow-900 text-yellow-300',
    REVIEWING:         'bg-purple-900 text-purple-300',
    AWAITING_REVISION: 'bg-rose-900 text-rose-300',
    AWAITING_MERGE:    'bg-cyan-900 text-cyan-300',
    MERGING:           'bg-indigo-900 text-indigo-300',
    COMPLETED:         'bg-green-900 text-green-300',
    FAILED:            'bg-red-900 text-red-300',
    active:            'bg-teal-900 text-teal-300',
    complete:          'bg-green-900 text-green-300',
  }

  const projectStatusOptions = ['active', 'planned', 'in_progress', 'complete', 'completed', 'failed']

  function taskTitle(t) {
    return t.payload?.title ?? t.type ?? t.id
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function loadAll() {
    loading = true
    try {
      const [p, tr] = await Promise.all([
        getProject(projectId),
        getTaskRoles(),
      ])
      project   = p
      taskRoles = Array.isArray(tr) ? tr : []
      await Promise.all([loadTasks(), loadRequirements(), loadFeatures(), checkRepo()])
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

  async function loadRequirements() {
    try { requirements = await listRequirements(projectId) } catch (_) {}
  }

  async function loadFeatures() {
    try { features = await listFeatures(projectId) } catch (_) {}
  }

  async function addRequirement() {
    if (!newReqTitle.trim()) return
    try {
      const r = await createRequirement(projectId, { title: newReqTitle.trim(), position: requirements.length })
      requirements = [...requirements, r]
      newReqTitle = ''
    } catch (e) { toasts.error('Create failed: ' + e.message) }
  }

  async function saveRequirement(req) {
    try {
      const updated = await updateRequirement(projectId, req.id, reqBuf)
      requirements = requirements.map(r => r.id === req.id ? { ...r, ...updated } : r)
      editingReqId = null
    } catch (e) { toasts.error('Save failed: ' + e.message) }
  }

  async function removeRequirement(id) {
    if (!confirm('Delete this requirement?')) return
    try {
      await deleteRequirement(projectId, id)
      requirements = requirements.filter(r => r.id !== id)
    } catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  async function patchReqStatus(req, status) {
    try {
      const updated = await updateRequirement(projectId, req.id, { status })
      requirements = requirements.map(r => r.id === req.id ? { ...r, ...updated } : r)
    } catch (e) { toasts.error('Update failed: ' + e.message) }
  }

  async function addFeature() {
    if (!newFeatTitle.trim()) return
    try {
      const f = await createFeature(projectId, { title: newFeatTitle.trim(), position: features.length })
      features = [...features, f]
      newFeatTitle = ''
    } catch (e) { toasts.error('Create failed: ' + e.message) }
  }

  async function saveFeature(feat) {
    try {
      const updated = await updateFeature(projectId, feat.id, featBuf)
      features = features.map(f => f.id === feat.id ? { ...f, ...updated } : f)
      editingFeatId = null
    } catch (e) { toasts.error('Save failed: ' + e.message) }
  }

  async function removeFeature(id) {
    if (!confirm('Delete this feature?')) return
    try {
      await deleteFeature(projectId, id)
      features = features.filter(f => f.id !== id)
    } catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  async function patchFeatStatus(feat, status) {
    try {
      const updated = await updateFeature(projectId, feat.id, { status })
      features = features.map(f => f.id === feat.id ? { ...f, ...updated } : f)
    } catch (e) { toasts.error('Update failed: ' + e.message) }
  }

  // ── Scope re-sync (Feature 5) ─────────────────────────────────────────────
  let resyncing = $state(false)
  async function requestScopeResync() {
    resyncing = true
    try {
      // The server owns the re-sync instructions (orchestrator.resync_prompt
      // setting) and enqueues the orchestrator task.
      await resyncProjectScope(projectId)
      toasts.success('Scope re-sync requested — an orchestrator will reconcile it')
      await loadTasks()
    } catch (e) {
      toasts.error('Could not request re-sync: ' + e.message)
    } finally {
      resyncing = false
    }
  }

  // ── Project editing ───────────────────────────────────────────────────────
  function startEdit() {
    editBuf = {
      name:                  project.name,
      description:           project.description,
      repo_path:             project.repo_path,
      git_url:               project.git_url,
      remote_url:            project.remote_url || '',
      remote_credentials_ref: project.remote_credentials_ref || '',
      coding_rules:          project.coding_rules || '',
      status:                project.status,
      auto_queue:            !!project.auto_queue,
      max_open_tasks:        project.max_open_tasks ?? 0,
    }
    editing = true
  }

  async function saveProject() {
    try {
      const updated = await updateProject(projectId, {
        name:                  editBuf.name,
        description:           editBuf.description,
        repo_path:             editBuf.repo_path,
        git_url:               editBuf.git_url,
        remote_url:            editBuf.remote_url || undefined,
        remote_credentials_ref: editBuf.remote_credentials_ref || undefined,
        coding_rules:          editBuf.coding_rules || undefined,
        status:                editBuf.status,
        auto_queue:            editBuf.auto_queue,
        max_open_tasks:        Number(editBuf.max_open_tasks) || 0,
      })
      project = updated
      editing = false
      toasts.success('Project saved')
    } catch (e) {
      toasts.error('Save failed: ' + e.message)
    }
  }

  // Re-arm a completed project for exactly one beyond-scope improvement round.
  async function reEnableAutoQueue() {
    try {
      const updated = await updateProject(projectId, {
        auto_queue: true, status: 'active', plan_rounds: 0,
      })
      project = updated
      toasts.success('Auto-queue re-enabled — one improvement round will run')
    } catch (e) {
      toasts.error('Could not re-enable auto-queue: ' + e.message)
    }
  }

  // ── Task actions ──────────────────────────────────────────────────────────
  async function submitTask() {
    if (!taskForm.title.trim()) { toasts.error('Task title is required'); return }
    try {
      await createTask({
        project_id: projectId,
        role:       taskForm.role,
        ...(taskForm.review_role ? { review_role: taskForm.review_role } : {}),
        priority:   Number(taskForm.priority),
        payload:    { title: taskForm.title.trim(), description: taskForm.description.trim() },
      })
      toasts.success('Task created')
      taskForm     = { role: 'worker', review_role: '', title: '', description: '', priority: 5 }
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

  async function changeTaskRole(id, role) {
    try {
      await updateTask(id, { role })
      await loadTasks()
    } catch (e) {
      toasts.error('Role change failed: ' + e.message)
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

  async function unqueueTaskAction(id) {
    try {
      await unqueueTask(id)
      await loadTasks()
    } catch (e) {
      toasts.error('Unqueue failed: ' + e.message)
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

  onMount(() => {
    loadSidebarState()
    loadAll()
  })
</script>

<div class="flex-1 flex overflow-hidden">
  <!-- Main content -->
  <div class="flex-1 flex flex-col overflow-hidden {showAssistant ? 'max-w-4xl' : 'max-w-5xl'} mx-auto w-full">
  <!-- Scrollable wrapper for overview; fills height for Files/Diff -->
  <div class="{activeTab === 'overview' ? 'overflow-y-auto' : 'overflow-hidden flex flex-col flex-1'} p-6 flex flex-col">
    <!-- Breadcrumb -->
    <nav class="mb-5 text-sm text-gray-500 flex items-center gap-2 justify-between">
      <div class="flex items-center gap-2">
        <button
          class="hover:text-gray-300 transition-colors"
          onclick={() => router.go('projects')}
        >Projects</button>
        <span>›</span>
        <span class="text-gray-300">{project?.name ?? '…'}</span>
      </div>
      <button
        class="text-xs px-2 py-1 border border-surface-500 text-gray-400 hover:border-accent hover:text-gray-200 rounded transition-colors"
        onclick={toggleSidebar}
        title="Toggle assistant sidebar"
      >💬 {showAssistant ? 'Hide' : 'Show'}</button>
    </nav>

  {#if loading}
    <Skeleton rows={3} />

  {:else if !project}
    <p class="text-gray-400 text-sm">Project not found.</p>

  {:else}
    <!-- ── Tab bar ───────────────────────────────────────────────────────── -->
    <div class="flex gap-1 mb-4 border-b border-surface-600 shrink-0">
      {#each [['overview','Overview'],['files','Files'],['diff','Diff']] as [id, label]}
        <button
          class="px-4 py-2 text-sm transition-colors border-b-2 -mb-px
                 {activeTab === id
                   ? 'border-accent text-gray-100'
                   : 'border-transparent text-gray-500 hover:text-gray-300'}"
          onclick={() => {
            activeTab = id
            if (id === 'files') loadFileBranches()
            if (id === 'diff' && selectedBranch !== 'main') diffHeadRef = selectedBranch
          }}
        >{label}</button>
      {/each}
    </div>

    <!-- ── Repo missing banner ───────────────────────────────────────────── -->
    {#if repoMissing}
      <div class="mb-4 flex items-center gap-3 px-4 py-3 bg-yellow-950 border border-yellow-700 rounded text-sm">
        <span class="text-yellow-400">⚠</span>
        <span class="text-yellow-200 flex-1">The git repository for this project is missing or could not be opened.</span>
        <button
          class="px-3 py-1 bg-yellow-700 hover:bg-yellow-600 text-yellow-100 text-xs rounded
                 transition-colors disabled:opacity-50"
          disabled={initingRepo}
          onclick={handleInitRepo}
        >{initingRepo ? 'Initialising…' : 'Initialise repository'}</button>
      </div>
    {/if}

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

          <!-- Auto-queue (Feature 4) -->
          <div class="flex items-center gap-4 flex-wrap">
            <label class="flex items-center gap-2 text-sm text-gray-300">
              <input type="checkbox" bind:checked={editBuf.auto_queue} />
              Auto-queue (keep backlog full until complete)
            </label>
            <label class="flex items-center gap-2 text-xs text-gray-500">
              Max open tasks
              <input
                type="number" min="0"
                class="w-20 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200"
                bind:value={editBuf.max_open_tasks}
              />
              <span class="text-gray-600">(0 = unlimited)</span>
            </label>
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
              <label for="project-git-url" class="text-xs text-gray-500 mb-1 block">Git remote URL (legacy)</label>
              <input
                id="project-git-url"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-200 font-mono focus:outline-none focus:border-accent"
                placeholder="https://github.com/user/repo.git"
                bind:value={editBuf.git_url}
              />
            </div>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label for="project-remote-url" class="text-xs text-gray-500 mb-1 block">Upstream remote URL</label>
              <input
                id="project-remote-url"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-200 font-mono focus:outline-none focus:border-accent"
                placeholder="https://github.com/org/repo.git"
                bind:value={editBuf.remote_url}
              />
            </div>
            <div>
              <label for="project-credentials-ref" class="text-xs text-gray-500 mb-1 block">Credentials env var / key</label>
              <input
                id="project-credentials-ref"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-200 font-mono focus:outline-none focus:border-accent"
                placeholder="GITHUB_TOKEN"
                bind:value={editBuf.remote_credentials_ref}
              />
            </div>
          </div>

          <div>
            <label for="project-coding-rules" class="text-xs text-gray-500 mb-1 block">Coding rules</label>
            <textarea
              id="project-coding-rules"
              rows="4"
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                     text-sm text-gray-200 font-mono focus:outline-none focus:border-accent resize-y"
              placeholder="Freeform coding conventions written to .agent_context/ for each task…"
              bind:value={editBuf.coding_rules}
            ></textarea>
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
              {#if project.auto_queue}
                <span class="text-xs px-2 py-0.5 rounded-full bg-teal-900 text-teal-300" title="Backlog auto-replenishes until complete">
                  auto-queue{project.plan_rounds ? ` · round ${project.plan_rounds}` : ''}
                </span>
              {/if}
              {#if project.status === 'complete'}
                <button
                  class="text-xs px-2 py-0.5 rounded border border-teal-700 text-teal-300 hover:bg-teal-900"
                  onclick={reEnableAutoQueue}
                >Re-enable auto-queue</button>
              {/if}
            </div>

            {#if project.description}
              <!-- Render description as markdown (readonly) -->
              <div class="mb-3">
                <MarkdownEditor value={project.description} readonly={true} minHeight="0px" />
              </div>
            {:else}
              <p class="text-sm text-gray-400 italic mb-3">No description.</p>
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
              {#if project.remote_url}
                <a
                  href={project.remote_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="hover:text-accent transition-colors"
                  title="Upstream remote"
                >⬆ {project.remote_url}</a>
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

    <!-- ── Overview: requirements / features / tasks ────────────────────── -->
    {#if activeTab === 'overview'}

    <!-- ── Scope-dirty banner (Feature 5) ────────────────────────────────── -->
    {#if project?.scope_dirty}
      <div class="mb-4 flex items-center justify-between gap-3 rounded border border-amber-700 bg-amber-950 px-3 py-2">
        <span class="text-xs text-amber-200">
          The description changed — requirements and features may be out of date.
        </span>
        <button
          class="shrink-0 px-3 py-1 text-xs rounded bg-amber-800 text-amber-100 hover:bg-amber-700 disabled:opacity-50"
          onclick={requestScopeResync}
          disabled={resyncing}
        >{resyncing ? 'Requesting…' : 'Re-sync scope'}</button>
      </div>
    {/if}

    <!-- ── Requirements ──────────────────────────────────────────────────── -->
    <div class="mb-4">
      <div class="flex items-center justify-between mb-2 gap-2 flex-wrap">
        <button
          class="flex items-center gap-1.5 text-base font-semibold text-gray-200 hover:text-gray-100"
          onclick={() => reqsOpen = !reqsOpen}
        >
          <span class="text-xs text-gray-500">{reqsOpen ? '▾' : '▸'}</span>
          Requirements
          <span class="text-xs font-normal text-gray-500">({requirements.length})</span>
        </button>
        <div class="flex items-center gap-1.5 flex-wrap">
          {#each REQ_STATUSES as s}
            <button
              class="text-[10px] px-1.5 py-0.5 rounded-full border transition-colors
                {reqStatusFilter.includes(s)
                  ? (REQ_STATUS_COLORS[s] || 'bg-surface-600 text-gray-300') + ' border-transparent'
                  : 'border-surface-600 text-gray-500 hover:text-gray-300'}"
              onclick={() => {
                if (reqStatusFilter.includes(s)) reqStatusFilter = reqStatusFilter.filter(x => x !== s)
                else reqStatusFilter = [...reqStatusFilter, s]
              }}
            >{s}</button>
          {/each}
        </div>
      </div>

      {#if reqsOpen}
        <div class="flex flex-col gap-1.5 mb-2">
          {#each filteredReqs as req (req.id)}
            <div class="p-2.5 bg-surface-800 rounded border border-surface-600">
              {#if editingReqId === req.id}
                <div class="flex flex-col gap-2">
                  <input
                    class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 focus:outline-none focus:border-accent"
                    bind:value={reqBuf.title}
                  />
                  <textarea
                    class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none focus:border-accent resize-none"
                    rows="3"
                    placeholder="Description (markdown)"
                    bind:value={reqBuf.body}
                  ></textarea>
                  <div class="flex items-center gap-2">
                    <select
                      class="bg-surface-700 border border-surface-500 rounded px-2 py-0.5 text-xs text-gray-300 focus:outline-none"
                      bind:value={reqBuf.status}
                    >
                      {#each REQ_STATUSES as s}<option value={s}>{s}</option>{/each}
                    </select>
                    <button class="px-2 py-0.5 bg-accent hover:bg-accent-hover text-white text-xs rounded" onclick={() => saveRequirement(req)}>Save</button>
                    <button class="px-2 py-0.5 bg-surface-600 hover:bg-surface-500 text-gray-300 text-xs rounded" onclick={() => editingReqId = null}>Cancel</button>
                  </div>
                </div>
              {:else}
                <div class="flex items-start justify-between gap-2">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      <span class="text-sm text-gray-200 font-medium">{req.title}</span>
                      <select
                        class="bg-transparent border-0 text-[10px] px-1 py-0 rounded-full cursor-pointer focus:outline-none {REQ_STATUS_COLORS[req.status] || 'text-gray-400'}"
                        value={req.status}
                        onchange={(e) => patchReqStatus(req, e.currentTarget.value)}
                      >
                        {#each REQ_STATUSES as s}<option value={s}>{s}</option>{/each}
                      </select>
                      {#if req.linked_tasks > 0}
                        <span class="text-[10px] text-gray-500 bg-surface-700 px-1 py-0.5 rounded">{req.linked_tasks} task{req.linked_tasks !== 1 ? 's' : ''}</span>
                      {/if}
                    </div>
                    {#if req.body}
                      <p class="text-xs text-gray-400 mt-0.5 line-clamp-2">{req.body}</p>
                    {/if}
                  </div>
                  <div class="flex items-center gap-1.5 shrink-0">
                    <button class="text-[10px] text-blue-400 hover:text-blue-300" onclick={() => { editingReqId = req.id; reqBuf = { title: req.title, body: req.body, status: req.status } }}>Edit</button>
                    <button class="text-[10px] text-red-400 hover:text-red-300" onclick={() => removeRequirement(req.id)}>Del</button>
                  </div>
                </div>
              {/if}
            </div>
          {:else}
            {#if filteredReqs.length === 0 && requirements.length === 0}
              <p class="text-xs text-gray-400 py-2">No requirements yet. Use requirements to define what the project must do.</p>
            {:else if filteredReqs.length === 0}
              <p class="text-xs text-gray-400 py-2">No requirements match the current status filter.</p>
            {/if}
          {/each}
        </div>

        <div class="flex items-center gap-2">
          <input
            class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 placeholder-gray-600 focus:outline-none focus:border-accent"
            placeholder="New requirement title…"
            bind:value={newReqTitle}
            onkeydown={(e) => e.key === 'Enter' && addRequirement()}
          />
          <button
            class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
            disabled={!newReqTitle.trim()}
            onclick={addRequirement}
          >+ Add</button>
        </div>
      {/if}
    </div>

    <!-- ── Features ───────────────────────────────────────────────────────── -->
    <div class="mb-4">
      <div class="flex items-center justify-between mb-2 gap-2 flex-wrap">
        <button
          class="flex items-center gap-1.5 text-base font-semibold text-gray-200 hover:text-gray-100"
          onclick={() => featsOpen = !featsOpen}
        >
          <span class="text-xs text-gray-500">{featsOpen ? '▾' : '▸'}</span>
          Features
          <span class="text-xs font-normal text-gray-500">({features.length})</span>
        </button>
        <div class="flex items-center gap-1.5 flex-wrap">
          {#each FEAT_STATUSES as s}
            <button
              class="text-[10px] px-1.5 py-0.5 rounded-full border transition-colors
                {featStatusFilter.includes(s)
                  ? (FEAT_STATUS_COLORS[s] || 'bg-surface-600 text-gray-300') + ' border-transparent'
                  : 'border-surface-600 text-gray-500 hover:text-gray-300'}"
              onclick={() => {
                if (featStatusFilter.includes(s)) featStatusFilter = featStatusFilter.filter(x => x !== s)
                else featStatusFilter = [...featStatusFilter, s]
              }}
            >{s}</button>
          {/each}
        </div>
      </div>

      {#if featsOpen}
        <div class="flex flex-col gap-1.5 mb-2">
          {#each filteredFeats as feat (feat.id)}
            <div class="p-2.5 bg-surface-800 rounded border border-surface-600">
              {#if editingFeatId === feat.id}
                <div class="flex flex-col gap-2">
                  <input
                    class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 focus:outline-none focus:border-accent"
                    bind:value={featBuf.title}
                  />
                  <textarea
                    class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none focus:border-accent resize-none"
                    rows="3"
                    placeholder="Description (markdown)"
                    bind:value={featBuf.body}
                  ></textarea>
                  <div class="flex items-center gap-2">
                    <select
                      class="bg-surface-700 border border-surface-500 rounded px-2 py-0.5 text-xs text-gray-300 focus:outline-none"
                      bind:value={featBuf.status}
                    >
                      {#each FEAT_STATUSES as s}<option value={s}>{s}</option>{/each}
                    </select>
                    <button class="px-2 py-0.5 bg-accent hover:bg-accent-hover text-white text-xs rounded" onclick={() => saveFeature(feat)}>Save</button>
                    <button class="px-2 py-0.5 bg-surface-600 hover:bg-surface-500 text-gray-300 text-xs rounded" onclick={() => editingFeatId = null}>Cancel</button>
                  </div>
                </div>
              {:else}
                <div class="flex items-start justify-between gap-2">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      <span class="text-sm text-gray-200 font-medium">{feat.title}</span>
                      <select
                        class="bg-transparent border-0 text-[10px] px-1 py-0 rounded-full cursor-pointer focus:outline-none {FEAT_STATUS_COLORS[feat.status] || 'text-gray-400'}"
                        value={feat.status}
                        onchange={(e) => patchFeatStatus(feat, e.currentTarget.value)}
                      >
                        {#each FEAT_STATUSES as s}<option value={s}>{s}</option>{/each}
                      </select>
                      {#if feat.linked_tasks > 0}
                        <span class="text-[10px] text-gray-500 bg-surface-700 px-1 py-0.5 rounded">{feat.linked_tasks} task{feat.linked_tasks !== 1 ? 's' : ''}</span>
                      {/if}
                    </div>
                    {#if feat.body}
                      <p class="text-xs text-gray-400 mt-0.5 line-clamp-2">{feat.body}</p>
                    {/if}
                  </div>
                  <div class="flex items-center gap-1.5 shrink-0">
                    <button class="text-[10px] text-blue-400 hover:text-blue-300" onclick={() => { editingFeatId = feat.id; featBuf = { title: feat.title, body: feat.body, status: feat.status } }}>Edit</button>
                    <button class="text-[10px] text-red-400 hover:text-red-300" onclick={() => removeFeature(feat.id)}>Del</button>
                  </div>
                </div>
              {/if}
            </div>
          {:else}
            {#if filteredFeats.length === 0 && features.length === 0}
              <p class="text-xs text-gray-400 py-2">No features yet. Use features to track what the project ships.</p>
            {:else if filteredFeats.length === 0}
              <p class="text-xs text-gray-400 py-2">No features match the current status filter.</p>
            {/if}
          {/each}
        </div>

        <div class="flex items-center gap-2">
          <input
            class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 placeholder-gray-600 focus:outline-none focus:border-accent"
            placeholder="New feature title…"
            bind:value={newFeatTitle}
            onkeydown={(e) => e.key === 'Enter' && addFeature()}
          />
          <button
            class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
            disabled={!newFeatTitle.trim()}
            onclick={addFeature}
          >+ Add</button>
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
            {#each ['BACKLOG','DEVELOPING','AWAITING_REVIEW','REVIEWING','AWAITING_REVISION','AWAITING_MERGE','MERGING','COMPLETED','FAILED'] as s}
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
            <div>
              <label for="task-review-role" class="text-xs text-gray-500 mb-1 block">Review role</label>
              <select
                id="task-review-role"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-300 focus:outline-none focus:border-accent"
                bind:value={taskForm.review_role}
              >
                <option value="">Auto (default reviewer)</option>
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
        <p class="text-gray-400 text-sm py-4">No tasks yet. Add the first one above.</p>
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
                    <span class="text-xs text-gray-500 bg-surface-700 px-1.5 py-0.5 rounded">{t.type}</span>
                    <span class="text-xs text-gray-600">{roleLabel(t.role, taskRoles)}</span>
                  </div>
                  {#if t.payload?.description}
                    <p class="text-xs text-gray-400 mt-1 line-clamp-2">{t.payload.description}</p>
                  {/if}
                </div>
                <div class="flex items-center gap-2 shrink-0">
                  <select
                    class="text-xs bg-surface-700 border border-surface-600 rounded px-1 py-0.5 text-gray-300 focus:outline-none focus:border-accent"
                    value={roleLabel(t.role, taskRoles)}
                    onclick={(e) => e.stopPropagation()}
                    onchange={(e) => { e.stopPropagation(); changeTaskRole(t.id, e.currentTarget.value) }}
                    title="Change role"
                  >
                    {#each taskRoles as tr}
                      <option value={tr.value}>{tr.label || tr.value}</option>
                    {/each}
                  </select>
                  {#if t.status === 'BACKLOG'}
                    <button
                      class="text-xs text-orange-400 hover:text-orange-300 transition-colors"
                      onclick={(e) => { e.stopPropagation(); unqueueTaskAction(t.id) }}
                    >Unqueue</button>
                  {:else if t.status === 'UNQUEUED' || t.status === 'FAILED'}
                    <button
                      class="text-xs text-yellow-400 hover:text-yellow-300 transition-colors"
                      onclick={(e) => { e.stopPropagation(); queueTaskAction(t.id) }}
                    >Queue</button>
                  {/if}
                  <button
                    class="text-xs text-red-400 hover:text-red-300 transition-colors"
                    onclick={(e) => { e.stopPropagation(); removeTask(t.id) }}
                  >Del</button>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    {/if}
    <!-- ── Files tab ─────────────────────────────────────────────────────── -->
    {#if activeTab === 'files'}
      <div class="flex-1 overflow-hidden flex flex-col gap-0">
        <div class="flex-1 overflow-hidden flex gap-0 border border-surface-600 rounded-t">

          <!-- Branch list (left column, fixed width) -->
          <div class="w-36 shrink-0 border-r border-surface-600 flex flex-col bg-surface-800 overflow-y-auto">
            <div class="px-2 py-1.5 text-xs text-gray-500 uppercase tracking-wide border-b border-surface-600 shrink-0">Branches</div>
            {#if branchesLoading}
              <p class="px-2 py-2 text-xs text-gray-600">Loading…</p>
            {:else if fileBranches.length === 0}
              <p class="px-2 py-2 text-xs text-gray-600 italic">No branches</p>
            {:else}
              {#each fileBranches as b}
                <div class="flex items-center group transition-colors
                            {selectedBranch === b ? 'bg-accent' : 'hover:bg-surface-700'}">
                  <button
                    class="flex-1 min-w-0 text-left px-2 py-1.5 text-xs font-mono truncate
                           {selectedBranch === b ? 'text-white' : 'text-gray-300'}"
                    title={b}
                    onclick={() => selectBranch(b)}
                  >{b.startsWith('task/') && b.length > 14 ? 'task/' + b.slice(5, 13) + '…' : b}</button>
                  {#if b !== 'main'}
                    <button
                      class="px-1.5 py-1.5 text-xs text-gray-500 hover:text-red-400 opacity-0 group-hover:opacity-100 shrink-0"
                      title={'Delete branch ' + b}
                      onclick={() => handleDeleteBranch(b)}
                    >✕</button>
                  {/if}
                </div>
              {/each}
            {/if}
          </div>

          <!-- File tree (middle column) -->
          <div class="w-52 shrink-0 border-r border-surface-600 overflow-hidden flex flex-col bg-surface-800">
            <FileTree {projectId} ref={selectedBranch} onFileSelect={onFileSelect} />
          </div>

          <!-- Code editor (right, fills remaining space) -->
          <div class="flex-1 overflow-hidden flex flex-col bg-surface-900">
            <CodeEditor {projectId} ref={activeFileRef} path={activeFilePath} onStage={onStageFile} />
          </div>
        </div>

        <!-- Commit log strip below the three panels -->
        <div class="border border-t-0 border-surface-600 bg-surface-800 shrink-0" style="max-height:160px;overflow-y:auto">
          <div class="px-3 py-1.5 text-xs text-gray-500 uppercase tracking-wide border-b border-surface-600 sticky top-0 bg-surface-800">
            Commits — <code class="font-mono text-blue-300">{selectedBranch}</code>
          </div>
          {#if commitsLoading}
            <p class="px-3 py-2 text-xs text-gray-600">Loading…</p>
          {:else if branchCommits.length === 0}
            <p class="px-3 py-2 text-xs text-gray-600 italic">No commits on this branch yet.</p>
          {:else}
            <div class="flex flex-col">
              {#each branchCommits as c}
                <div class="flex items-center gap-3 px-3 py-1.5 text-xs border-b border-surface-700 hover:bg-surface-700 transition-colors">
                  <code class="font-mono text-accent shrink-0">{c.short_sha}</code>
                  <span class="text-gray-300 truncate flex-1">{c.message}</span>
                  <span class="text-gray-500 shrink-0">{c.author_name}</span>
                  <span class="text-gray-600 shrink-0">{new Date(c.date).toLocaleDateString()}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Staging panel -->
        {#if Object.keys(stagedFiles).length > 0}
          <div class="border border-t-0 border-surface-600 rounded-b bg-surface-800 p-3 flex flex-col gap-2 shrink-0">
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-gray-400 uppercase tracking-wide">
                Staged ({Object.keys(stagedFiles).length})
              </span>
            </div>
            <div class="flex flex-wrap gap-1">
              {#each Object.keys(stagedFiles) as p}
                <span class="flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-blue-900/30 text-blue-300 font-mono">
                  {p}
                  <button
                    class="text-gray-500 hover:text-red-400 transition-colors leading-none"
                    onclick={() => unstageFile(p)}
                    title="Unstage"
                  >✕</button>
                </span>
              {/each}
            </div>
            <div class="flex items-center gap-2">
              <input
                class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-1.5 text-xs
                       text-gray-200 focus:outline-none focus:border-accent"
                placeholder="Commit message…"
                bind:value={stageCommitMsg}
                onkeydown={(e) => e.key === 'Enter' && commitAllStaged()}
              />
              <button
                class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs rounded
                       disabled:opacity-40 transition-colors"
                disabled={!stageCommitMsg.trim() || stageSaving}
                onclick={commitAllStaged}
              >{stageSaving ? 'Committing…' : 'Commit all'}</button>
            </div>
          </div>
        {/if}
      </div>
    {/if}

    <!-- ── Diff tab ───────────────────────────────────────────────────────── -->
    {#if activeTab === 'diff'}
      <div class="flex-1 overflow-hidden flex flex-col gap-3">
        <div class="flex items-center gap-3 shrink-0">
          <label class="text-xs text-gray-500">Base</label>
          <input
            class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs
                   text-gray-300 focus:outline-none focus:border-accent w-32"
            placeholder="main"
            bind:value={diffBaseRef}
          />
          <span class="text-gray-500 text-xs">→</span>
          <label class="text-xs text-gray-500">Head</label>
          <input
            class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs
                   text-gray-300 focus:outline-none focus:border-accent w-32"
            placeholder="feature-branch"
            bind:value={diffHeadRef}
          />
        </div>
        <div class="flex-1 overflow-hidden border border-surface-600 rounded bg-surface-900">
          <DiffViewer {projectId} baseRef={diffBaseRef} headRef={diffHeadRef} />
        </div>
      </div>
    {/if}

  {/if}
  </div>
  </div>

  <!-- Assistant sidebar -->
  {#if showAssistant && project}
    <AssistantSidebar projectId={project.id} onApplyToDescription={applyAssistantToDescription} />
  {/if}
</div>
