<script>
  import { listBranches, readTree } from '../lib/api.js'

  let { projectId, onFileSelect } = $props()

  let branches    = $state([])
  let selectedRef = $state('main')
  // root nodes + per-path children loaded on expand
  let rootNodes   = $state([])
  let childMap    = $state({})   // path → TreeNode[]
  let expanded    = $state({})   // path → bool
  let activePath  = $state(null)
  let loading     = $state(false)
  let error       = $state(null)

  // Flat list of { node, depth } for all currently visible nodes.
  let visibleNodes = $derived(flatten(rootNodes, 0))

  function flatten(nodes, depth) {
    let result = []
    for (const n of nodes) {
      result.push({ node: n, depth })
      if (n.type === 'tree' && expanded[n.path] && childMap[n.path]) {
        result = result.concat(flatten(childMap[n.path], depth + 1))
      }
    }
    return result
  }

  async function loadBranches() {
    try {
      const b = await listBranches(projectId)
      branches = Array.isArray(b) ? b : []
      if (branches.length > 0 && !branches.includes(selectedRef)) {
        selectedRef = branches[0]
      }
    } catch (e) { error = e.message }
  }

  async function loadRoot() {
    loading   = true
    error     = null
    expanded  = {}
    childMap  = {}
    activePath = null
    try {
      const n = await readTree(projectId, selectedRef, '')
      rootNodes = Array.isArray(n) ? n : []
    } catch (e) { error = e.message }
    finally { loading = false }
  }

  async function toggleDir(node) {
    if (expanded[node.path]) {
      expanded = { ...expanded, [node.path]: false }
      return
    }
    if (!childMap[node.path]) {
      try {
        const sub = await readTree(projectId, selectedRef, node.path)
        childMap = { ...childMap, [node.path]: Array.isArray(sub) ? sub : [] }
      } catch (e) { error = e.message; return }
    }
    expanded = { ...expanded, [node.path]: true }
  }

  function selectFile(node) {
    activePath = node.path
    onFileSelect?.(node.path, selectedRef)
  }

  function handleClick(node) {
    if (node.type === 'tree') toggleDir(node)
    else selectFile(node)
  }

  function onBranchChange() { loadRoot() }

  $effect(() => {
    if (projectId) loadBranches().then(() => loadRoot())
  })
</script>

<div class="flex flex-col h-full text-sm select-none">
  <!-- Branch selector -->
  <div class="px-3 py-2 border-b border-surface-600 flex items-center gap-2 shrink-0">
    <span class="text-xs text-gray-500">Branch</span>
    <select
      class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs
             text-gray-300 focus:outline-none focus:border-accent"
      bind:value={selectedRef}
      onchange={onBranchChange}
    >
      {#each branches as b}
        <option value={b}>{b}</option>
      {/each}
      {#if branches.length === 0}
        <option value="main">main</option>
      {/if}
    </select>
  </div>

  <!-- Tree body -->
  <div class="flex-1 overflow-y-auto py-1">
    {#if loading}
      <p class="px-3 py-2 text-xs text-gray-500">Loading…</p>
    {:else if error}
      <p class="px-3 py-2 text-xs text-red-400">{error}</p>
    {:else if visibleNodes.length === 0}
      <p class="px-3 py-2 text-xs text-gray-500 italic">Empty repository</p>
    {:else}
      {#each visibleNodes as { node, depth }}
        {@const isActive = node.path === activePath}
        {@const isOpen   = node.type === 'tree' && expanded[node.path]}
        <button
          class="w-full text-left flex items-center gap-1 px-2 py-0.5 rounded
                 text-xs font-mono transition-colors
                 {isActive
                   ? 'bg-accent text-white'
                   : 'text-gray-300 hover:bg-surface-600'}"
          style="padding-left: {8 + depth * 14}px"
          onclick={() => handleClick(node)}
        >
          {#if node.type === 'tree'}
            <span class="w-3 shrink-0 text-gray-500">{isOpen ? '▾' : '▸'}</span>
            <span class="text-blue-300">📁</span>
          {:else}
            <span class="w-3 shrink-0"></span>
            <span>📄</span>
          {/if}
          <span class="truncate">{node.name}</span>
        </button>
      {/each}
    {/if}
  </div>
</div>
