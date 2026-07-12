<script>
  import { onMount, onDestroy } from "svelte"
  import { router, toasts } from "../lib/stores.js"
  import {
    getTask,
    updateTask,
    deleteTask,
    queueTask,
    unqueueTask,
    getProject,
    listTaskLogs,
    listTaskLinks,
    addTaskLink,
    removeTaskLink,
    listRequirements,
    listFeatures,
    listTaskDependencies,
    addTaskDependency,
    removeTaskDependency,
    listProjectTasks,
    listChecklistItems,
    createChecklistItem,
    updateChecklistItem,
    deleteChecklistItem,
    cloneChecklistIteration,
    listComments,
    createComment,
    deleteComment,
    listBranches,
    listCommits,
    readFile,
    deleteTaskLogs,
    getAgent,
    listLogs,
    getTaskCost,
    listPRs,
    createPR,
    approvePR,
    rejectPR,
    getTaskRoles,
    listAgentSessions,
  } from "../lib/api.js"
  import MarkdownEditor from "../components/MarkdownEditor.svelte"
  import FileTree from "../components/FileTree.svelte"
  import { formatTimestamp } from "../lib/time.js"
  import { roleLabel } from "../lib/roles.js"
  import Skeleton from "../components/Skeleton.svelte"

  let { taskId } = $props()

  // ── State ─────────────────────────────────────────────────────────────────
  let task = $state(null)
  let roleDefs = $state([])
  let project = $state(null)
  let assignedAgent = $state(null)
  let loading = $state(true)
  let taskCost = $state(null)
  let costExpanded = $state(false)
  let editing = $state(false)
  let taskLogs    = $state([])
  let agentExecLogs = $state([])   // from /api/logs?task_id=
  let agentSessions = $state([])   // from /api/agent-sessions?task_id= (checkpoints)
  let logsLoading = $state(false)
  let expandedLogs = $state(new Set())  // log IDs with expanded content

  function toggleLogExpand(id) {
    const s = new Set(expandedLogs)
    s.has(id) ? s.delete(id) : s.add(id)
    expandedLogs = s
  }

  // Merge task lifecycle events + agent execution logs into one timeline.
  let allEvents = $derived.by(() => {
    const events = [
      ...taskLogs.map(e => ({ ...e, _source: 'task' })),
      ...agentExecLogs.map(e => ({
        ...e,
        _source:     'agent',
        event_type:  e.level,
        description: e.message,
      })),
    ]
    events.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp))
    return events
  })

  // Subagent runs (Subagents feature): completed run_subagent invocations and the
  // summary each returned, surfaced from the agent exec logs' metadata so a run is
  // observable without reading server logs.
  let subagentRuns = $derived.by(() => {
    return agentExecLogs
      .map((e) => {
        let m = e.metadata
        if (typeof m === 'string') { try { m = JSON.parse(m) } catch (_) { m = null } }
        if (!m || m.source !== 'subagent' || !m.summary) return null
        return { id: e.id, ts: e.timestamp, skill: m.skill, instructions: m.instructions, summary: m.summary }
      })
      .filter(Boolean)
      .sort((a, b) => new Date(a.ts) - new Date(b.ts))
  })

  // Status history (Bug 10): a projection of status-change task-log events,
  // each carrying agent identity (resolved in the DB layer) and the branch from
  // metadata. Falls back to the state_transitions table when the log DB is off.
  let timeline = $derived.by(() => {
    const fromLogs = taskLogs
      .filter((e) => e.old_status || e.new_status)
      .map((e) => {
        let branch = ""
        try {
          const m = typeof e.metadata === "string" ? JSON.parse(e.metadata) : (e.metadata || {})
          branch = m?.branch || ""
        } catch (_) {}
        return {
          id: e.id, ts: e.timestamp,
          old_status: e.old_status, new_status: e.new_status,
          agent_name: e.agent_name, agent_id: e.agent_id,
          branch, description: e.description,
        }
      })
    if (fromLogs.length > 0) {
      return fromLogs.sort((a, b) => new Date(a.ts) - new Date(b.ts))
    }
    return transitions
      .map((tr) => ({
        id: tr.id, ts: tr.created_at,
        old_status: tr.from_state, new_status: tr.to_state,
        agent_name: "", agent_id: tr.actor_agent_id,
        branch: "", description: tr.reason,
      }))
      .sort((a, b) => new Date(a.ts) - new Date(b.ts))
  })
  let taskLinks = $state([])
  let projectReqs = $state([])
  let projectFeats = $state([])
  let deps = $state([])
  let projectTasks = $state([])
  let depSearch = $state("")
  let addingDep = $state(false)
  let checklist = $state([])
  let newItemLabel = $state("")
  let newItemGroup = $state("")
  let comments = $state([])
  let newComment = $state("")
  let postingComment = $state(false)
  let transitions = $state([])
  let reviews = $state([])
  let pullRequests = $state([])
  let prNotes = $state({}) // prId → decision note text
  let prBusy = $state(false)

  // ── Code panel ────────────────────────────────────────────────────────────
  let taskBranch        = $state('')   // the task's working branch (persisted task.branch)
  let taskBranchExists  = $state(false)
  let taskCommits       = $state([])
  let codeLoading       = $state(false)
  let selectedFilePath    = $state(null)
  let selectedFileContent = $state(null)

  // ── Event clearing ────────────────────────────────────────────────────────
  let clearingEvents = $state(false)

  async function clearEvents() {
    if (!confirm('Delete all events for this task?')) return
    clearingEvents = true
    try {
      await deleteTaskLogs(task.id)
    } finally {
      clearingEvents = false
      taskLogs = []
      agentExecLogs = []
    }
  }

  async function handleFileSelect(path) {
    selectedFilePath = path
    selectedFileContent = null
    try {
      const data = await readFile(task.project_id, taskBranch, path)
      selectedFileContent = data?.content ?? ''
    } catch {
      selectedFileContent = '(error loading file)'
    }
  }

  async function loadCodePanel(t) {
    if (!t.project_id) return
    // Branches are now human-readable (generated from the task type at claim);
    // fall back to the legacy task/<id> for tasks created before that change.
    taskBranch = t.branch || `task/${t.id}`
    codeLoading = true
    try {
      const [branches, commits] = await Promise.all([
        listBranches(t.project_id).catch(() => []),
        listCommits(t.project_id, taskBranch).catch(() => []),
      ])
      taskBranchExists = Array.isArray(branches) && branches.includes(taskBranch)
      taskCommits = Array.isArray(commits) ? commits : []
    } finally {
      codeLoading = false
    }
  }

  // Edit buffer
  let editBuf = $state({})

  // ── Helpers ───────────────────────────────────────────────────────────────
  const statusColors = {
    BACKLOG: "bg-blue-900 text-blue-300",
    DEVELOPING: "bg-orange-900 text-orange-300",
    AWAITING_REVIEW: "bg-yellow-900 text-yellow-300",
    REVIEWING: "bg-purple-900 text-purple-300",
    AWAITING_REVISION: "bg-rose-900 text-rose-300",
    AWAITING_MERGE: "bg-cyan-900 text-cyan-300",
    MERGING: "bg-indigo-900 text-indigo-300",
    COMPLETED: "bg-green-900 text-green-300",
    FAILED: "bg-red-900 text-red-300",
  }

  // Human-friendly status labels (Feature 2: MERGING is a merge review).
  const statusLabels = {
    MERGING: "MERGE REVIEW IN PROGRESS",
  }

  const formatDate = formatTimestamp

  // ── Data loading ──────────────────────────────────────────────────────────
  async function loadLogs(id) {
    if (taskLogs.length === 0) logsLoading = true
    try {
      const [data, execData] = await Promise.all([
        listTaskLogs(id),
        listLogs({ task_id: id, limit: 500 }),
      ])
      const incoming = Array.isArray(data) ? data : []
      if (JSON.stringify(incoming) !== JSON.stringify(taskLogs)) {
        taskLogs = incoming
      }
      agentExecLogs = Array.isArray(execData) ? execData : (execData?.logs ?? [])
    } catch (e) {
      // keep existing logs on transient errors
    } finally {
      logsLoading = false
    }
    // Session checkpoints (best-effort; absence is normal for short tasks).
    try {
      const sess = await listAgentSessions(id)
      agentSessions = Array.isArray(sess) ? sess : []
    } catch (e) { /* ignore */ }
  }

  async function pollTask() {
    try {
      const t = await getTask(taskId)
      if (t && t.updated_at !== task?.updated_at) {
        task = t
      }
    } catch (_) {}
    loadLogs(taskId)
  }

  async function loadAll() {
    loading = true
    try {
      // Fetch core task first; secondary endpoints (new in this build) are
      // fetched with individual fallbacks so a missing/unimplemented endpoint
      // doesn't prevent the page from rendering.
      const [t, links, depList, checklistData, commentData, transitionData, reviewData] =
        await Promise.all([
          getTask(taskId),
          listTaskLinks(taskId).catch(() => []),
          listTaskDependencies(taskId).catch(() => []),
          listChecklistItems(taskId).catch(() => []),
          listComments(taskId).catch(() => []),
          fetch(`/api/tasks/${taskId}/transitions`)
            .then((r) => (r.ok ? r.json() : []))
            .catch(() => []),
          fetch(`/api/tasks/${taskId}/reviews`)
            .then((r) => (r.ok ? r.json() : []))
            .catch(() => []),
        ])
      task = t
      roleDefs = await getTaskRoles().catch(() => [])
      transitions = Array.isArray(transitionData) ? transitionData : []
      reviews = Array.isArray(reviewData) ? reviewData : []
      pullRequests = await listPRs(taskId).catch(() => [])
      taskLinks = links ?? []
      deps = depList ?? []
      checklist = checklistData ?? []
      comments = commentData ?? []
      if (t.assigned_agent_id) {
        assignedAgent = await getAgent(t.assigned_agent_id).catch(() => null)
      }
      if (t.project_id) {
        const [proj, reqs, feats, allTasks] = await Promise.all([
          getProject(t.project_id),
          listRequirements(t.project_id),
          listFeatures(t.project_id),
          listProjectTasks(t.project_id),
        ])
        project = proj
        projectReqs = reqs ?? []
        projectFeats = feats ?? []
        projectTasks = (Array.isArray(allTasks) ? allTasks : (allTasks?.tasks ?? [])).filter(
          (t2) => t2.id !== taskId,
        )
      }
      taskCost = await getTaskCost(taskId).catch(() => null)
      loadLogs(taskId)
      loadCodePanel(t)
    } catch (e) {
      toasts.error("Failed to load task: " + e.message)
    } finally {
      loading = false
    }
  }

  // ── Pull requests (Feature 2) ──────────────────────────────────────────────
  async function createPullRequest() {
    prBusy = true
    try {
      await createPR(taskId)
      toasts.success("Pull request created")
      await loadAll()
    } catch (e) {
      toasts.error("Create PR failed: " + e.message)
    } finally {
      prBusy = false
    }
  }

  async function decidePR(prId, verdict) {
    prBusy = true
    try {
      const note = prNotes[prId] || ""
      if (verdict === "approve") {
        await approvePR(taskId, prId, note)
        toasts.success("Pull request approved — merging")
      } else {
        await rejectPR(taskId, prId, note)
        toasts.success("Pull request rejected — returned for revision")
      }
      await loadAll()
    } catch (e) {
      toasts.error("PR decision failed: " + e.message)
    } finally {
      prBusy = false
    }
  }

  // ── Task editing ──────────────────────────────────────────────────────────
  // ── Dependencies ──────────────────────────────────────────────────────────
  let depSearchResults = $derived(
    depSearch.trim().length < 1
      ? []
      : projectTasks
          .filter((t2) => !deps.some((d) => d.depends_on_id === t2.id))
          .filter((t2) =>
            (t2.payload?.title ?? t2.type ?? t2.id).toLowerCase().includes(depSearch.toLowerCase()),
          )
          .slice(0, 8),
  )

  async function addDep(dependsOnId) {
    try {
      const dep = await addTaskDependency(taskId, dependsOnId)
      deps = [...deps, dep]
      depSearch = ""
      addingDep = false
    } catch (e) {
      toasts.error("Could not add dependency: " + e.message)
    }
  }

  async function removeDep(dependsOnId) {
    try {
      await removeTaskDependency(taskId, dependsOnId)
      deps = deps.filter((d) => d.depends_on_id !== dependsOnId)
    } catch (e) {
      toasts.error("Could not remove dependency: " + e.message)
    }
  }

  // ── Checklist ──────────────────────────────────────────────────────────────
  let checklistGroups = $derived.by(() => {
    const groups = {}
    for (const item of checklist) {
      const g = item.group_label || ""
      if (!groups[g]) groups[g] = []
      groups[g].push(item)
    }
    return Object.entries(groups).sort(([a], [b]) => a.localeCompare(b))
  })

  const ITEM_STATUSES = ["pending", "in_progress", "passed", "failed", "skipped"]
  const ITEM_STATUS_COLORS = {
    pending: "text-gray-400",
    in_progress: "text-orange-400",
    passed: "text-green-400",
    failed: "text-red-400",
    skipped: "text-gray-600",
  }

  async function cycleItemStatus(item) {
    const next = ITEM_STATUSES[(ITEM_STATUSES.indexOf(item.status) + 1) % ITEM_STATUSES.length]
    try {
      const updated = await updateChecklistItem(taskId, item.id, { status: next })
      checklist = checklist.map((i) => (i.id === item.id ? updated : i))
    } catch (e) {
      toasts.error("Could not update item: " + e.message)
    }
  }

  async function addChecklistItem() {
    if (!newItemLabel.trim()) return
    try {
      const item = await createChecklistItem(taskId, {
        label: newItemLabel.trim(),
        group_label: newItemGroup.trim(),
        position: checklist.filter((i) => i.group_label === newItemGroup.trim()).length,
        status: "pending",
      })
      checklist = [...checklist, item]
      newItemLabel = ""
    } catch (e) {
      toasts.error("Could not add item: " + e.message)
    }
  }

  async function removeChecklistItem(id) {
    try {
      await deleteChecklistItem(taskId, id)
      checklist = checklist.filter((i) => i.id !== id)
    } catch (e) {
      toasts.error("Could not delete item: " + e.message)
    }
  }

  async function newIteration() {
    try {
      const { group_label } = await cloneChecklistIteration(taskId)
      const fresh = await listChecklistItems(taskId)
      checklist = fresh ?? []
      if (group_label) toasts.success(`New iteration: "${group_label}"`)
    } catch (e) {
      toasts.error("Could not create iteration: " + e.message)
    }
  }

  // ── Comments ────────────────────────────────────────────────────────────────
  async function postComment() {
    if (!newComment.trim()) return
    postingComment = true
    try {
      const c = await createComment(taskId, { body: newComment.trim() })
      comments = [...comments, c]
      newComment = ""
    } catch (e) {
      toasts.error("Could not post comment: " + e.message)
    } finally {
      postingComment = false
    }
  }

  async function removeComment(id) {
    try {
      await deleteComment(taskId, id)
      comments = comments.filter((c) => c.id !== id)
    } catch (e) {
      toasts.error("Could not delete comment: " + e.message)
    }
  }

  // isConflictComment flags the comments an agent posts when it cannot land a
  // merge (unresolved conflicts, branch-update or push failure) so the UI can
  // highlight them.
  function isConflictComment(c) {
    if (!c || c.author_type !== "agent" || typeof c.body !== "string") return false
    return (
      c.body.startsWith("Merge into main hit conflicts") ||
      c.body.startsWith("Branch update failed:") ||
      c.body.startsWith("Failed to push merged branch:")
    )
  }

  function startEdit() {
    editBuf = {
      title: task.payload?.title ?? "",
      description: task.payload?.description ?? "",
      priority: task.priority,
      status: task.status,
      role: task.role ?? "",
      reviewRole: task.review_role ?? "",
      linkedReqIds: new Set(
        taskLinks.filter((l) => l.kind === "requirement").map((l) => l.target_id),
      ),
      linkedFeatIds: new Set(taskLinks.filter((l) => l.kind === "feature").map((l) => l.target_id)),
    }
    editing = true
  }

  function toggleReqLink(id) {
    const next = new Set(editBuf.linkedReqIds)
    next.has(id) ? next.delete(id) : next.add(id)
    editBuf = { ...editBuf, linkedReqIds: next }
  }

  function toggleFeatLink(id) {
    const next = new Set(editBuf.linkedFeatIds)
    next.has(id) ? next.delete(id) : next.add(id)
    editBuf = { ...editBuf, linkedFeatIds: next }
  }

  async function saveTask() {
    try {
      const updated = await updateTask(taskId, {
        priority: Number(editBuf.priority),
        status: editBuf.status,
        role: editBuf.role,
        review_role: editBuf.reviewRole,
        payload: {
          title: editBuf.title.trim(),
          description: editBuf.description.trim(),
        },
      })
      task = updated

      // sync requirement/feature links
      const origReqIds = new Set(
        taskLinks.filter((l) => l.kind === "requirement").map((l) => l.target_id),
      )
      const origFeatIds = new Set(
        taskLinks.filter((l) => l.kind === "feature").map((l) => l.target_id),
      )
      const ops = []
      for (const id of editBuf.linkedReqIds)
        if (!origReqIds.has(id)) ops.push(addTaskLink(taskId, "requirement", id))
      for (const id of origReqIds)
        if (!editBuf.linkedReqIds.has(id)) ops.push(removeTaskLink(taskId, "requirement", id))
      for (const id of editBuf.linkedFeatIds)
        if (!origFeatIds.has(id)) ops.push(addTaskLink(taskId, "feature", id))
      for (const id of origFeatIds)
        if (!editBuf.linkedFeatIds.has(id)) ops.push(removeTaskLink(taskId, "feature", id))
      if (ops.length) await Promise.all(ops)
      taskLinks = await listTaskLinks(taskId)

      editing = false
      toasts.success("Task saved")
      loadLogs(taskId)
    } catch (e) {
      toasts.error("Save failed: " + e.message)
    }
  }

  async function handleDelete() {
    if (!confirm("Delete this task?")) return
    try {
      await deleteTask(taskId)
      toasts.success("Task deleted")
      router.go("tasks")
    } catch (e) {
      toasts.error("Delete failed: " + e.message)
    }
  }

  async function handleQueue() {
    try {
      const updated = await queueTask(taskId)
      task = updated
      toasts.success("Task queued")
      loadLogs(taskId)
    } catch (e) {
      toasts.error("Queue failed: " + e.message)
    }
  }

  async function handleUnqueue() {
    try {
      const updated = await unqueueTask(taskId)
      task = updated
      toasts.success("Task unqueued")
      loadLogs(taskId)
    } catch (e) {
      toasts.error("Unqueue failed: " + e.message)
    }
  }

  let logsTimer
  onMount(() => {
    loadAll()
    logsTimer = setInterval(pollTask, 5_000)
  })
  onDestroy(() => clearInterval(logsTimer))
</script>

<div class="flex-1 overflow-y-auto p-6 max-w-4xl mx-auto w-full">
  <!-- Breadcrumb -->
  <nav class="mb-5 text-sm text-gray-500 flex items-center gap-2 flex-wrap">
    <button class="hover:text-gray-300 transition-colors" onclick={() => router.go("projects")}
      >Projects</button
    >
    <span>›</span>
    {#if project}
      <button
        class="hover:text-gray-300 transition-colors"
        onclick={() => router.push("projects", project.id)}>{project.name}</button
      >
      <span>›</span>
    {/if}
    <span class="text-gray-300">Tasks</span>
    <span>›</span>
    <span class="text-gray-300">{task?.payload?.title ?? "…"}</span>
  </nav>

  {#if loading}
    <Skeleton rows={4} />
  {:else if !task}
    <p class="text-gray-400 text-sm">Task not found.</p>
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
                {#each ["BACKLOG", "DEVELOPING", "AWAITING_REVIEW", "REVIEWING", "AWAITING_REVISION", "AWAITING_MERGE", "MERGING", "COMPLETED", "FAILED"] as s}
                  <option value={s}>{s}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="task-priority" class="text-xs text-gray-500 mb-1 block">Priority</label>
              <input
                id="task-priority"
                type="number"
                min="1"
                max="10"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-200 focus:outline-none focus:border-accent"
                bind:value={editBuf.priority}
              />
            </div>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label for="task-role" class="text-xs text-gray-500 mb-1 block">Role</label>
              <select
                id="task-role"
                aria-label="Role"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-300 focus:outline-none focus:border-accent"
                bind:value={editBuf.role}
              >
                {#each roleDefs as r}
                  <option value={r.id}>{r.label}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="task-review-role" class="text-xs text-gray-500 mb-1 block">Review</label>
              <select
                id="task-review-role"
                aria-label="Review role"
                class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2
                       text-sm text-gray-300 focus:outline-none focus:border-accent"
                bind:value={editBuf.reviewRole}
              >
                <option value="">Any reviewer</option>
                {#each roleDefs as r}
                  <option value={r.id}>{r.label}</option>
                {/each}
              </select>
            </div>
          </div>

          {#if projectReqs.length > 0 || projectFeats.length > 0}
            <div class="grid grid-cols-2 gap-3">
              {#if projectReqs.length > 0}
                <div>
                  <label class="text-xs text-gray-500 mb-1 block">Linked Requirements</label>
                  <div
                    class="bg-surface-700 border border-surface-500 rounded p-2 max-h-32 overflow-y-auto flex flex-col gap-1"
                  >
                    {#each projectReqs as req (req.id)}
                      <label
                        class="flex items-center gap-2 text-xs cursor-pointer hover:text-gray-200 text-gray-300"
                      >
                        <input
                          type="checkbox"
                          checked={editBuf.linkedReqIds?.has(req.id)}
                          onchange={() => toggleReqLink(req.id)}
                          class="accent-accent"
                        />
                        {req.title}
                      </label>
                    {/each}
                  </div>
                </div>
              {/if}
              {#if projectFeats.length > 0}
                <div>
                  <label class="text-xs text-gray-500 mb-1 block">Linked Features</label>
                  <div
                    class="bg-surface-700 border border-surface-500 rounded p-2 max-h-32 overflow-y-auto flex flex-col gap-1"
                  >
                    {#each projectFeats as feat (feat.id)}
                      <label
                        class="flex items-center gap-2 text-xs cursor-pointer hover:text-gray-200 text-gray-300"
                      >
                        <input
                          type="checkbox"
                          checked={editBuf.linkedFeatIds?.has(feat.id)}
                          onchange={() => toggleFeatLink(feat.id)}
                          class="accent-accent"
                        />
                        {feat.title}
                      </label>
                    {/each}
                  </div>
                </div>
              {/if}
            </div>
          {/if}

          <div class="flex justify-end gap-2">
            <button
              class="px-3 py-1.5 text-sm text-gray-400 hover:text-gray-200 transition-colors"
              onclick={() => (editing = false)}>Cancel</button
            >
            <button
              class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
              onclick={saveTask}>Save</button
            >
          </div>
        </div>
      {:else}
        <!-- Read view -->
        <div class="flex items-start justify-between gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-3 mb-2 flex-wrap">
              <h1 class="text-2xl font-semibold text-gray-100">
                {task.payload?.title ?? "Untitled"}
              </h1>
              <span
                class="text-xs px-2 py-0.5 rounded-full
                {statusColors[task.status] || 'bg-gray-700 text-gray-300'}"
              >
                {statusLabels[task.status] ?? task.status}
              </span>
              <button
                class="text-xs text-accent hover:underline"
                onclick={() => router.push('sessions', task.id)}
              >Sessions &amp; memory →</button>
            </div>

            {#if task.payload?.description}
              <!-- Render description as markdown (readonly) -->
              <div class="mb-4">
                <MarkdownEditor value={task.payload.description} readonly={true} minHeight="0px" />
              </div>
            {:else}
              <p class="text-sm text-gray-400 italic mb-4">No description.</p>
            {/if}

            <!-- Cost chip -->
            {#if taskCost && (taskCost.total_tokens > 0 || taskCost.cost_usd > 0)}
              <button
                class="mb-3 inline-flex items-center gap-1.5 text-xs px-2 py-1 rounded
                       bg-surface-700 text-gray-400 hover:text-gray-200 hover:bg-surface-600 transition-colors"
                onclick={() => costExpanded = !costExpanded}
                title="Token usage and estimated cost"
              >
                💰 {taskCost.cost_usd > 0 ? '~$' + taskCost.cost_usd.toFixed(5) : ''}
                · {(taskCost.total_tokens / 1000).toFixed(1)}k tokens
                {costExpanded ? '▲' : '▼'}
              </button>
              {#if costExpanded}
                <div class="mb-3 p-2 bg-surface-800 rounded border border-surface-600 text-xs text-gray-400 grid grid-cols-3 gap-2">
                  <div><span class="text-gray-500">Input</span> {taskCost.input_tokens.toLocaleString()} tok</div>
                  <div><span class="text-gray-500">Output</span> {taskCost.output_tokens.toLocaleString()} tok</div>
                  <div><span class="text-gray-500">Rounds</span> {taskCost.rounds}</div>
                </div>
              {/if}
            {/if}

            <!-- Metadata -->
            <div class="flex flex-wrap gap-4 text-xs text-gray-500">
              <span title="Type">📋 {task.type}</span>
              <span title="Role">👤 {roleLabel(task.role, roleDefs)}</span>
              <span title="Review setup"
                >🔍 {task.review_role ? roleLabel(task.review_role, roleDefs) : "Any reviewer"}</span
              >
              <span title="Priority">⭐ {task.priority ?? "—"}</span>
              {#if project}
                <span title="Project">📁 {project.name}</span>
              {/if}
              <span class="text-gray-600">ID: {task.id}</span>
            </div>

            <div class="flex flex-wrap gap-4 text-xs text-gray-600 mt-3 font-mono">
              <span>Created: {formatDate(task.created_at)}</span>
              <span>Updated: {formatDate(task.updated_at)}</span>
            </div>

            {#if taskLinks.length > 0}
              <div class="flex flex-wrap gap-3 mt-3 text-xs">
                {#if taskLinks.some((l) => l.kind === "requirement")}
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="text-gray-500">Reqs:</span>
                    {#each taskLinks.filter((l) => l.kind === "requirement") as lnk (lnk.target_id)}
                      <span class="px-2 py-0.5 bg-blue-900/40 text-blue-300 rounded-full">
                        {projectReqs.find((r) => r.id === lnk.target_id)?.title ??
                          lnk.target_id.slice(0, 8)}
                      </span>
                    {/each}
                  </div>
                {/if}
                {#if taskLinks.some((l) => l.kind === "feature")}
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="text-gray-500">Features:</span>
                    {#each taskLinks.filter((l) => l.kind === "feature") as lnk (lnk.target_id)}
                      <span class="px-2 py-0.5 bg-purple-900/40 text-purple-300 rounded-full">
                        {projectFeats.find((f) => f.id === lnk.target_id)?.title ??
                          lnk.target_id.slice(0, 8)}
                      </span>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}
          </div>

          <button
            class="px-3 py-1.5 text-sm border border-surface-500 text-gray-400
                   hover:border-accent hover:text-gray-200 rounded transition-colors shrink-0"
            onclick={startEdit}>Edit</button
          >
        </div>
      {/if}
    </div>

    <!-- ── Dependencies ─────────────────────────────────────────────────────── -->
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-gray-200">
          Dependencies
          {#if deps.length > 0}
            <span class="ml-1.5 text-xs font-normal text-gray-500">({deps.length})</span>
          {/if}
        </h3>
        {#if projectTasks.length > 0}
          <button
            class="text-xs text-accent hover:text-accent-hover transition-colors"
            onclick={() => {
              addingDep = !addingDep
              depSearch = ""
            }}>{addingDep ? "Cancel" : "+ Add"}</button
          >
        {/if}
      </div>

      {#if addingDep}
        <div class="mb-3 relative">
          <input
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-1.5 text-xs
                   text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="Search tasks…"
            bind:value={depSearch}
            autofocus
          />
          {#if depSearchResults.length > 0}
            <div
              class="absolute z-20 top-full left-0 right-0 mt-1 bg-surface-700 border border-surface-500 rounded shadow-lg max-h-40 overflow-y-auto"
            >
              {#each depSearchResults as t2 (t2.id)}
                <button
                  class="w-full text-left px-3 py-1.5 text-xs text-gray-300 hover:bg-surface-600 flex items-center gap-2"
                  onclick={() => addDep(t2.id)}
                >
                  <span class="flex-1 truncate">{t2.payload?.title ?? t2.type ?? t2.id}</span>
                  <span class="text-[10px] text-gray-500 shrink-0">{t2.status}</span>
                </button>
              {/each}
            </div>
          {:else if depSearch.length > 0}
            <div
              class="absolute z-20 top-full left-0 right-0 mt-1 bg-surface-700 border border-surface-500 rounded shadow-lg px-3 py-2 text-xs text-gray-500"
            >
              No matching tasks
            </div>
          {/if}
        </div>
      {/if}

      {#if deps.length === 0}
        <p class="text-xs text-gray-400">
          No dependencies. Add one to get soft warnings when this task is claimed while deps are
          incomplete.
        </p>
      {:else}
        <div class="flex flex-col gap-2">
          {#each deps as dep (dep.depends_on_id)}
            {@const done = dep.depends_on_status === "completed"}
            <div
              class="flex items-center gap-2 p-2 rounded bg-surface-700/60 border
              {done ? 'border-green-900/40' : 'border-yellow-900/40'}"
            >
              <span
                class="text-[10px] w-2 h-2 rounded-full shrink-0 {done
                  ? 'bg-green-500'
                  : 'bg-yellow-500'}"
              ></span>
              <span class="flex-1 text-xs text-gray-300 truncate">{dep.depends_on_title}</span>
              <span class="text-[10px] text-gray-500 shrink-0">{dep.depends_on_status}</span>
              <button
                class="text-[10px] text-red-400 hover:text-red-300 shrink-0"
                onclick={() => removeDep(dep.depends_on_id)}
                aria-label="Remove dependency">×</button
              >
            </div>
          {/each}
        </div>
        {#if deps.some((d) => d.depends_on_status !== "completed")}
          <p class="mt-2 text-xs text-yellow-600">
            ⚠ Some dependencies are not completed — claiming this task will emit a warning.
          </p>
        {/if}
      {/if}
    </div>

    <!-- ── Checklist ────────────────────────────────────────────────────────── -->
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600">
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-sm font-semibold text-gray-200">
          Progress Checklist
          {#if checklist.length > 0}
            {@const done = checklist.filter(
              (i) => i.status === "passed" || i.status === "skipped",
            ).length}
            <span class="ml-1.5 text-xs font-normal text-gray-500">({done}/{checklist.length})</span
            >
          {/if}
        </h3>
        {#if checklist.length > 0}
          <button
            class="text-xs text-gray-500 hover:text-accent transition-colors"
            onclick={newIteration}
            title="Clone latest group with reset status">+ New iteration</button
          >
        {/if}
      </div>

      {#if checklist.length === 0}
        <p class="text-xs text-gray-400 mb-3">No checklist items yet.</p>
      {:else}
        {#each checklistGroups as [groupLabel, items] (groupLabel)}
          <div class="mb-3">
            {#if groupLabel}
              <p class="text-[10px] font-semibold text-gray-400 uppercase tracking-wide mb-1">
                {groupLabel}
              </p>
            {/if}
            <div class="flex flex-col gap-1">
              {#each items as item (item.id)}
                <div class="flex items-center gap-2 group">
                  <button
                    class="shrink-0 text-[10px] px-1.5 py-0.5 rounded font-medium transition-colors
                           {ITEM_STATUS_COLORS[item.status] || 'text-gray-400'} hover:opacity-80"
                    onclick={() => cycleItemStatus(item)}
                    title="Click to cycle status">{item.status}</button
                  >
                  <span
                    class="flex-1 text-xs text-gray-300 {item.status === 'skipped'
                      ? 'line-through opacity-50'
                      : ''}">{item.label}</span
                  >
                  <button
                    class="text-[10px] text-red-400 hover:text-red-300 opacity-0 group-hover:opacity-100 transition-opacity"
                    onclick={() => removeChecklistItem(item.id)}
                    aria-label="Remove checklist item">×</button
                  >
                </div>
              {/each}
            </div>
          </div>
        {/each}
      {/if}

      <!-- Add item row -->
      <div class="flex gap-2 mt-2">
        <input
          class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs
                 text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="New item label…"
          bind:value={newItemLabel}
          onkeydown={(e) => e.key === "Enter" && addChecklistItem()}
        />
        <input
          class="w-28 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs
                 text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="Group (opt)"
          bind:value={newItemGroup}
          onkeydown={(e) => e.key === "Enter" && addChecklistItem()}
        />
        <button
          class="px-2 py-1 bg-surface-600 hover:bg-surface-500 text-xs text-gray-300 rounded transition-colors"
          onclick={addChecklistItem}>Add</button
        >
      </div>
    </div>

    <!-- ── Comments ─────────────────────────────────────────────────────────── -->
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600">
      <h3 class="text-sm font-semibold text-gray-200 mb-3">
        Comments
        {#if comments.length > 0}
          <span class="ml-1.5 text-xs font-normal text-gray-500">({comments.length})</span>
        {/if}
      </h3>

      {#if comments.length === 0}
        <p class="text-xs text-gray-400 mb-3">No comments yet.</p>
      {:else}
        <div class="flex flex-col gap-3 mb-4">
          {#each comments as c (c.id)}
            {@const conflict = isConflictComment(c)}
            <div class="group flex gap-3">
              <!-- Avatar dot -->
              <div
                class="shrink-0 w-6 h-6 rounded-full flex items-center justify-center text-[10px] font-bold mt-0.5
                {c.author_type === 'agent'
                  ? 'bg-purple-900 text-purple-300'
                  : 'bg-surface-600 text-gray-400'}"
              >
                {c.author_type === "agent" ? (c.author_role?.[0]?.toUpperCase() ?? "A") : "U"}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 mb-1">
                  <span
                    class="text-xs font-medium {c.author_type === 'agent'
                      ? 'text-purple-300'
                      : 'text-gray-300'}"
                  >
                    {c.author_type === "agent"
                      ? c.author_name || c.author_role || "agent"
                      : c.author_name || "you"}
                  </span>
                  {#if c.author_type === "agent" && c.author_name && c.author_role}
                    <span class="text-[10px] text-gray-600">{c.author_role}</span>
                  {/if}
                  {#if conflict}
                    <span
                      class="text-[10px] px-1.5 py-0.5 rounded font-medium uppercase tracking-wide bg-rose-900 text-rose-300"
                    >⚠ Merge conflict</span>
                  {/if}
                  <span class="text-[10px] text-gray-600 font-mono">
                    {new Date(c.created_at).toLocaleString([], {
                      month: "short",
                      day: "numeric",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </span>
                  <button
                    class="ml-auto text-[10px] text-red-400 hover:text-red-300 opacity-0 group-hover:opacity-100 transition-opacity"
                    onclick={() => removeComment(c.id)}
                    aria-label="Delete comment">×</button
                  >
                </div>
                <div
                  class="text-xs whitespace-pre-wrap break-words {conflict
                    ? 'text-rose-200 border border-rose-800 bg-rose-950 rounded p-2'
                    : 'text-gray-300'}"
                >{c.body}</div>
              </div>
            </div>
          {/each}
        </div>
      {/if}

      <!-- New comment input -->
      <div class="flex flex-col gap-2">
        <textarea
          class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-xs
                 text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent resize-none"
          placeholder="Add a comment… (supports Markdown)"
          rows="3"
          bind:value={newComment}
          onkeydown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) postComment()
          }}
        ></textarea>
        <div class="flex justify-end">
          <button
            class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs rounded
                   transition-colors disabled:opacity-40"
            disabled={!newComment.trim() || postingComment}
            onclick={postComment}>{postingComment ? "Posting…" : "Comment"}</button
          >
        </div>
      </div>
    </div>

    <!-- ── Agent / timestamps / result / events ─────────────────────────────── -->
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3">
      {#if task.assigned_agent_id}
        <div class="flex items-center gap-2 text-sm">
          <span class="text-gray-400">Agent:</span>
          {#if assignedAgent?.name}
            <span class="text-gray-100">{assignedAgent.name}</span>
            <span class="text-xs text-gray-500 font-mono">{task.assigned_agent_id}</span>
          {:else}
            <span class="font-mono text-accent">{task.assigned_agent_id}</span>
          {/if}
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
          <pre
            class="bg-surface-800 rounded p-3 text-xs text-gray-300 overflow-x-auto whitespace-pre-wrap">{JSON.stringify(
              task.result,
              null,
              2,
            )}</pre>
        </div>
      {/if}

      <!-- ── Status history timeline (Bug 10) ───────────────────────── -->
      {#if timeline.length > 0}
        <div class="mt-6">
          <h3 class="text-sm font-semibold text-gray-300 mb-3">Status history</h3>
          <ol class="relative border-l border-surface-600 ml-3 flex flex-col gap-4">
            {#each timeline as ev (ev.id)}
              <li class="ml-4 text-xs">
                <span
                  class="absolute -left-1.5 mt-1 w-3 h-3 rounded-full border border-surface-600
                             {ev.new_status === 'COMPLETED'
                    ? 'bg-green-600'
                    : ev.new_status === 'FAILED'
                      ? 'bg-red-600'
                      : ev.new_status?.startsWith('AWAITING')
                        ? 'bg-yellow-600'
                        : 'bg-accent'}"
                >
                </span>
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-gray-500 font-mono">{formatTimestamp(ev.ts)}</span>
                  <span class="text-gray-400">
                    {#if ev.old_status}<span class="text-gray-500">{ev.old_status}</span> →
                    {/if}
                    <span class="font-semibold text-gray-200">{ev.new_status || ev.event_type}</span>
                  </span>
                  {#if ev.agent_name || ev.agent_id}
                    <span class="text-gray-500">
                      {ev.agent_name || "agent"}
                      {#if ev.agent_id}<span class="text-gray-600 font-mono">({ev.agent_id.slice(0, 8)})</span>{/if}
                    </span>
                  {/if}
                  {#if ev.branch}
                    <code class="text-[10px] font-mono px-1 rounded bg-surface-700 text-blue-300">{ev.branch}</code>
                  {/if}
                  {#if ev.description}
                    <span class="text-gray-600 italic">{ev.description}</span>
                  {/if}
                </div>
              </li>
            {/each}
          </ol>
        </div>
      {/if}

      <!-- ── Pull requests (Feature 2) ─────────────────────────────── -->
      {#if !pullRequests.some((p) => p.status === "open") && (task?.status === "AWAITING_REVIEW" || task?.status === "REVIEWING" || task?.status === "AWAITING_MERGE")}
        <div class="mt-6 flex items-center gap-2">
          <button
            class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
            disabled={prBusy}
            onclick={createPullRequest}
          >Create PR</button>
          <span class="text-xs text-gray-500">Open a pull request and move the task to the merge gate.</span>
        </div>
      {/if}
      {#if pullRequests.length > 0}
        <div class="mt-6">
          <h3 class="text-sm font-semibold text-gray-300 mb-3">Pull Requests</h3>
          <div class="flex flex-col gap-4">
            {#each pullRequests as pr (pr.id)}
              {@const decided = pr.status === "merged" || pr.status === "rejected"}
              <div
                class="rounded border p-3
                {pr.status === 'merged'
                  ? 'border-green-700 bg-green-950'
                  : pr.status === 'rejected'
                    ? 'border-rose-700 bg-rose-950'
                    : 'border-cyan-700 bg-cyan-950'}"
              >
                <div class="flex items-center gap-2 mb-2">
                  <span class="text-sm font-medium text-gray-200">{pr.title || "Pull request"}</span>
                  <span
                    class="text-[10px] px-1.5 py-0.5 rounded font-medium uppercase tracking-wide
                    {pr.status === 'merged'
                      ? 'bg-green-900 text-green-300'
                      : pr.status === 'rejected'
                        ? 'bg-rose-900 text-rose-300'
                        : 'bg-cyan-900 text-cyan-300'}"
                  >{pr.status}</span>
                  <span class="text-xs text-gray-500 font-mono ml-auto">{pr.branch} → {pr.base}</span>
                </div>
                {#if pr.author_name || pr.author_id}
                  <div class="text-xs text-gray-500 mb-1">
                    Opened by {pr.author_name || pr.author_id}
                  </div>
                {/if}
                {#if pr.body}
                  <div class="text-xs text-gray-300 whitespace-pre-wrap font-mono mb-2">{pr.body}</div>
                {/if}
                {#if pr.decision_body}
                  <div class="text-xs text-gray-400 border-t border-surface-600 mt-2 pt-2">
                    <span class="text-gray-500">Decision{pr.decider_id ? ` (${pr.decider_id})` : ""}:</span>
                    <span class="whitespace-pre-wrap font-mono"> {pr.decision_body}</span>
                  </div>
                {/if}

                {#if !decided}
                  <div class="mt-3 flex flex-col gap-2">
                    <textarea
                      class="w-full text-xs bg-surface-900 border border-surface-600 rounded p-2 text-gray-200"
                      rows="2"
                      placeholder="Decision notes (optional)"
                      bind:value={prNotes[pr.id]}
                    ></textarea>
                    <div class="flex gap-2">
                      <button
                        class="px-3 py-1 text-xs rounded bg-green-800 text-green-100 hover:bg-green-700 disabled:opacity-50"
                        onclick={() => decidePR(pr.id, "approve")}
                        disabled={prBusy}
                      >Approve &amp; merge</button>
                      <button
                        class="px-3 py-1 text-xs rounded bg-rose-800 text-rose-100 hover:bg-rose-700 disabled:opacity-50"
                        onclick={() => decidePR(pr.id, "reject")}
                        disabled={prBusy}
                      >Reject</button>
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <!-- ── Code reviews ──────────────────────────────────────────── -->
      {#if reviews.length > 0}
        <div class="mt-6">
          <h3 class="text-sm font-semibold text-gray-300 mb-3">Code Reviews</h3>
          <div class="flex flex-col gap-4">
            {#each reviews as rev (rev.id)}
              <div
                class="rounded border
                {rev.status === 'approved'
                  ? 'border-green-700 bg-green-950'
                  : rev.status === 'changes_requested'
                    ? 'border-yellow-700 bg-yellow-950'
                    : 'border-rose-700 bg-rose-950'} p-3"
              >
                <div class="flex items-center gap-2 mb-2">
                  <span
                    class="text-xs font-medium
                    {rev.status === 'approved'
                      ? 'text-green-300'
                      : rev.status === 'changes_requested'
                        ? 'text-yellow-300'
                        : 'text-rose-300'}"
                  >
                    {rev.status.replace("_", " ").toUpperCase()}
                  </span>
                  <span class="text-xs text-gray-500">{rev.author_role || rev.author_type}</span>
                  <span class="text-xs text-gray-600 font-mono ml-auto">
                    {new Date(rev.created_at).toLocaleString([], {
                      month: "short",
                      day: "numeric",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </span>
                </div>
                <div class="text-xs text-gray-300 whitespace-pre-wrap font-mono">{rev.body}</div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if subagentRuns.length > 0}
        <div class="mt-6">
          <h3 class="text-sm font-semibold text-gray-300 mb-3">Subagent runs</h3>
          <div class="flex flex-col gap-2">
            {#each subagentRuns as run (run.id)}
              <div class="p-3 bg-surface-800 rounded border border-surface-600">
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-[10px] px-1.5 py-0.5 rounded bg-purple-900/50 text-purple-300">subagent</span>
                  <span class="text-sm font-mono text-gray-200">{run.skill}</span>
                </div>
                {#if run.instructions}
                  <p class="text-xs text-gray-500 mb-1"><span class="text-gray-400">ask:</span> {run.instructions}</p>
                {/if}
                <p class="text-xs text-gray-300 whitespace-pre-wrap">{run.summary}</p>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if agentSessions.length > 0}
        <div class="mt-6">
          <h3 class="text-sm font-semibold text-gray-300 mb-3">Session checkpoints</h3>
          <div class="flex flex-col gap-2">
            {#each agentSessions as sess (sess.id)}
              <div class="p-3 bg-surface-800 rounded border border-surface-600">
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-[10px] px-1.5 py-0.5 rounded bg-amber-900/50 text-amber-300">checkpoint</span>
                  <span class="text-xs text-gray-500">round {sess.round} · {formatTimestamp(sess.created_at)}</span>
                </div>
                <p class="text-xs text-gray-300 whitespace-pre-wrap">{sess.summary}</p>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <div class="mt-6">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-gray-300">Task Events</h3>
          {#if allEvents.length > 0}
            <button
              class="px-2 py-0.5 text-xs rounded border border-red-800 text-red-400 hover:bg-red-900 transition-colors disabled:opacity-50"
              onclick={clearEvents}
              disabled={clearingEvents}
            >{clearingEvents ? 'Clearing…' : 'Clear events'}</button>
          {/if}
        </div>
        {#if logsLoading}
          <p class="text-xs text-gray-400">Loading events…</p>
        {:else if allEvents.length === 0}
          <p class="text-xs text-gray-400">No events yet.</p>
        {:else}
          <div class="flex flex-col gap-1">
            {#each allEvents as log (log.id)}
              {@const isAgent   = log._source === 'agent'}
              {@const meta      = isAgent && log.metadata && typeof log.metadata === 'object' && Object.keys(log.metadata).length > 0}
              {@const expanded  = expandedLogs.has(log.id)}
              {@const label     = log.event_type ?? log.level ?? 'info'}
              {@const isError   = label.includes('fail') || label.includes('error')}
              {@const isLLM     = isAgent && (log.message?.startsWith('LLM') || log.message?.startsWith('tool call'))}
              {@const actor     = log.agent_name || (log.agent_id ? log.agent_id.slice(0, 8) : '')}
              <div class="flex flex-col gap-0.5">
                <div class="flex items-start gap-2 text-xs py-0.5">
                  <span class="shrink-0 text-gray-600 font-mono w-36 pt-px">
                    {formatTimestamp(log.timestamp)}
                  </span>
                  <span class="shrink-0 w-24 truncate text-gray-500 pt-px" title={log.agent_id ?? ''}>
                    {actor || '—'}
                  </span>
                  <span class="shrink-0 px-1.5 py-0.5 rounded text-[10px] font-medium
                    {isError
                      ? 'bg-red-900 text-red-300'
                      : label.includes('complet') || label.includes('info')
                        ? isLLM ? 'bg-purple-900/50 text-purple-300' : 'bg-surface-700 text-gray-400'
                        : 'bg-surface-700 text-gray-400'}"
                  >{label}</span>
                  {#if log.old_status && log.new_status}
                    <span class="text-gray-500 pt-px">{log.old_status} → {log.new_status}</span>
                  {/if}
                  <span class="text-gray-300 flex-1 break-words pt-px">{log.description ?? ''}</span>
                  {#if meta}
                    <button
                      class="shrink-0 text-[10px] text-accent hover:text-accent-hover transition-colors"
                      onclick={() => toggleLogExpand(log.id)}
                    >{expanded ? 'Less ↑' : 'Read more ↓'}</button>
                  {/if}
                </div>
                {#if meta && expanded}
                  <div class="ml-36 pl-2 border-l-2 border-surface-600 flex flex-col gap-2 py-1">
                    {#each Object.entries(log.metadata) as [key, val]}
                      {#if val !== null && val !== undefined && val !== ''}
                        <div>
                          <div class="text-[10px] text-gray-500 uppercase tracking-wide mb-0.5">{key}</div>
                          <pre class="text-xs text-gray-300 bg-surface-800 rounded p-2 overflow-x-auto whitespace-pre-wrap break-words font-mono">{typeof val === 'string' ? val : JSON.stringify(val, null, 2)}</pre>
                        </div>
                      {/if}
                    {/each}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <!-- ── Code panel ────────────────────────────────────────────────────── -->
    {#if task.project_id}
      <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600">
        <h3 class="text-sm font-semibold text-gray-300 mb-3">Code</h3>

        <!-- Branch label -->
        <div class="flex items-center gap-2 mb-4">
          <span class="text-xs text-gray-500">Branch</span>
          <code class="text-xs font-mono px-2 py-0.5 rounded bg-surface-700 text-blue-300">
            {taskBranch}
          </code>
          {#if !codeLoading && !taskBranchExists}
            <span class="text-xs text-gray-600 italic">not yet pushed</span>
          {/if}
        </div>

        {#if codeLoading}
          <p class="text-xs text-gray-500">Loading…</p>
        {:else}
          <div class="flex gap-4">
            <!-- File tree (fixed 220px) -->
            <div class="shrink-0 border border-surface-600 rounded overflow-hidden" style="width:220px;height:280px">
              <FileTree projectId={task.project_id} ref={taskBranch} onFileSelect={handleFileSelect} />
            </div>

            <!-- File content viewer -->
            {#if selectedFilePath}
              <div class="flex-1 min-w-0 flex flex-col overflow-hidden">
                <div class="text-xs text-gray-500 mb-1 font-mono truncate">{selectedFilePath}</div>
                <pre class="text-xs font-mono bg-surface-700 rounded p-2 overflow-auto flex-1" style="max-height:264px">{selectedFileContent ?? 'Loading…'}</pre>
              </div>
            {/if}

            <!-- Commit log -->
            <div class="w-56 shrink-0 flex flex-col gap-1 overflow-y-auto" style="max-height:280px">
              {#if taskCommits.length === 0}
                <p class="text-xs text-gray-600 italic">No commits yet</p>
              {:else}
                {#each taskCommits as c}
                  <div class="flex flex-col gap-0.5 p-2 rounded bg-surface-700 text-xs">
                    <div class="flex items-center gap-2">
                      <code class="font-mono text-accent">{c.short_sha}</code>
                      <span class="text-gray-500 truncate">{c.author_name}</span>
                    </div>
                    <span class="text-gray-300 truncate">{c.message}</span>
                    <span class="text-gray-600">{new Date(c.date).toLocaleString()}</span>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        {/if}
      </div>
    {/if}

    <!-- ── Queue controls ─────────────────────────────────────────────────── -->
    <div class="p-5 bg-surface-800 rounded border border-surface-600">
      <h3 class="text-sm font-semibold text-gray-200 mb-3">Actions</h3>
      <div class="flex gap-2 flex-wrap">
        {#if task.status === "BACKLOG"}
          <button
            class="px-3 py-1.5 text-sm border border-orange-600 text-orange-400
                   hover:bg-orange-900 hover:border-orange-500 rounded transition-colors"
            onclick={handleUnqueue}>Unqueue</button
          >
        {:else if task.status !== "COMPLETED"}
          <button
            class="px-3 py-1.5 text-sm border border-yellow-600 text-yellow-400
                   hover:bg-yellow-900 hover:border-yellow-500 rounded transition-colors"
            onclick={handleQueue}>Queue</button
          >
        {/if}
        <button
          class="px-3 py-1.5 text-sm border border-red-600 text-red-400
                 hover:bg-red-900 hover:border-red-500 rounded transition-colors"
          onclick={handleDelete}>Delete</button
        >
      </div>
    </div>
  {/if}
</div>
