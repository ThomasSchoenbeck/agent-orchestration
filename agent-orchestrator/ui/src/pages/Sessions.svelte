<script>
  import { onMount } from 'svelte'
  import { listAgentSessions, getTaskMemory, listPreparedPrompts } from '../lib/api.js'
  import { toasts, router } from '../lib/stores.js'
  import { formatTimestamp } from '../lib/time.js'
  import Skeleton from '../components/Skeleton.svelte'

  let { taskId } = $props()

  let sessions = $state([])
  let memory   = $state(null)      // TaskMemory or {}
  let prompts  = $state([])
  let loading  = $state(false)
  let expanded = $state(new Set()) // prompt ids expanded

  const KIND_COLOR = {
    main:        'text-blue-400',
    discovery:   'text-teal-400',
    work:        'text-green-400',
    prompt_prep: 'text-purple-400',
    task_status: 'text-amber-400',
  }
  const STATUS_COLOR = {
    running:   'text-yellow-400',
    done:      'text-green-400',
    failed:    'text-red-400',
    timed_out: 'text-orange-400',
  }

  // Build a depth-annotated, DFS-ordered list from the flat session rows so the
  // main → subagent hierarchy renders as an indented tree (roots first).
  let tree = $derived.by(() => {
    const byParent = new Map()
    const ids = new Set(sessions.map(s => s.id))
    for (const s of sessions) {
      const pid = s.parent_id && ids.has(s.parent_id) ? s.parent_id : ''
      if (!byParent.has(pid)) byParent.set(pid, [])
      byParent.get(pid).push(s)
    }
    const out = []
    const walk = (pid, depth) => {
      for (const s of (byParent.get(pid) ?? [])) {
        out.push({ ...s, depth })
        walk(s.id, depth + 1)
      }
    }
    walk('', 0)
    return out
  })

  // Prepared prompts grouped by session, each sorted by round.
  let promptsBySession = $derived.by(() => {
    const m = new Map()
    for (const p of prompts) {
      if (!m.has(p.session_id)) m.set(p.session_id, [])
      m.get(p.session_id).push(p)
    }
    for (const arr of m.values()) arr.sort((a, b) => (a.round ?? 0) - (b.round ?? 0))
    return m
  })

  let memoryContent = $derived(memory?.content ?? null)
  let hasMemory = $derived.by(() => {
    const c = memoryContent
    if (!c) return false
    return !!(c.summary || (c.progress?.length) || (c.decisions?.length) ||
              (c.findings?.length) || (c.open_questions?.length))
  })

  function toggle(id) {
    const s = new Set(expanded)
    s.has(id) ? s.delete(id) : s.add(id)
    expanded = s
  }

  async function load() {
    loading = true
    try {
      const [ss, mem, pp] = await Promise.all([
        listAgentSessions(taskId).catch(() => []),
        getTaskMemory(taskId).catch(() => ({})),
        listPreparedPrompts(taskId).catch(() => []),
      ])
      sessions = Array.isArray(ss) ? ss : []
      memory   = mem ?? {}
      prompts  = Array.isArray(pp) ? pp : []
    } catch (e) {
      toasts.error('Failed to load sessions: ' + e.message)
    } finally {
      loading = false
    }
  }

  onMount(load)
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center gap-3 mb-5">
    <button class="text-xs text-gray-500 hover:text-gray-300" onclick={() => router.push('tasks', taskId)}>← Task</button>
    <h1 class="text-xl font-semibold text-gray-100">Sessions &amp; Memory</h1>
    <button class="text-xs text-gray-500 hover:text-gray-300 ml-auto" onclick={load}>↻ Refresh</button>
  </div>

  {#if loading && sessions.length === 0}
    <Skeleton rows={4} />
  {:else}
    <!-- Session tree -->
    <section class="mb-8">
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-3">Session tree</h2>
      {#if tree.length === 0}
        <p class="text-sm text-gray-500">No sessions recorded for this task yet.</p>
      {:else}
        <div class="flex flex-col gap-1.5">
          {#each tree as s (s.id)}
            <div class="p-3 bg-surface-800 rounded border border-surface-600" style="margin-left: {s.depth * 1.5}rem">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-xs font-mono {KIND_COLOR[s.kind] ?? 'text-gray-400'}">{s.kind || 'main'}</span>
                {#if s.title}<span class="text-sm text-gray-200">{s.title}</span>{/if}
                <span class="text-xs {STATUS_COLOR[s.status] ?? 'text-gray-500'}">{s.status || '—'}</span>
                {#if s.round != null}<span class="text-[10px] px-1.5 py-0.5 rounded bg-surface-700 text-gray-400">round {s.round}</span>{/if}
                <span class="text-xs text-gray-500 ml-auto">~${(s.cost ?? 0).toFixed(4)}</span>
              </div>
              {#if s.summary}
                <p class="text-xs text-gray-400 mt-1 whitespace-pre-wrap">{s.summary}</p>
              {/if}
              <div class="text-[10px] text-gray-600 mt-1 font-mono">{formatTimestamp(s.created_at)}</div>
            </div>
          {/each}
        </div>
      {/if}
    </section>

    <!-- Memory panel -->
    <section class="mb-8">
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-3">Task memory</h2>
      {#if !hasMemory}
        <p class="text-sm text-gray-500">No memory recorded for this task yet.</p>
      {:else}
        <div class="p-4 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3">
          {#if memoryContent.summary}
            <div>
              <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Summary</div>
              <p class="text-sm text-gray-300 whitespace-pre-wrap">{memoryContent.summary}</p>
            </div>
          {/if}
          {#each [['Progress', memoryContent.progress], ['Decisions', memoryContent.decisions], ['Findings', memoryContent.findings], ['Open questions', memoryContent.open_questions]] as [label, items]}
            {#if items && items.length > 0}
              <div>
                <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">{label}</div>
                <ul class="list-disc list-inside text-sm text-gray-300 flex flex-col gap-0.5">
                  {#each items as it}<li class="whitespace-pre-wrap">{it}</li>{/each}
                </ul>
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    </section>

    <!-- Prompt-prep inspector -->
    <section>
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-3">Synthesized prompts</h2>
      {#if prompts.length === 0}
        <p class="text-sm text-gray-500">No synthesized prompts recorded for this task yet.</p>
      {:else}
        <div class="flex flex-col gap-3">
          {#each [...promptsBySession] as [sessionId, rows] (sessionId)}
            <div class="p-3 bg-surface-800 rounded border border-surface-600">
              <div class="text-xs font-mono text-gray-500 mb-2">session {sessionId.slice(0, 8)}… · {rows.length} round(s)</div>
              <div class="flex flex-col gap-1">
                {#each rows as p (p.id)}
                  <div class="border-t border-surface-700 pt-1">
                    <button
                      class="w-full text-left text-xs text-gray-300 hover:text-gray-100 flex items-center gap-2"
                      onclick={() => toggle(p.id)}
                    >
                      <span class="text-gray-500">{expanded.has(p.id) ? '▼' : '▶'}</span>
                      <span>round {p.round}</span>
                      <span class="text-gray-600 truncate">{p.prompt.slice(0, 80)}</span>
                    </button>
                    {#if expanded.has(p.id)}
                      <pre class="text-xs text-gray-300 bg-surface-900 rounded p-2 mt-1 overflow-auto max-h-64 whitespace-pre-wrap">{p.prompt}</pre>
                    {/if}
                  </div>
                {/each}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </section>
  {/if}
</div>
