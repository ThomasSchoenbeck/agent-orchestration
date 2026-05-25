<script>
  import { getFileDiff, getBranchDiff } from '../lib/api.js'

  let { projectId, baseRef = 'main', headRef = '' } = $props()

  let patches     = $state([])   // FilePatch[] from BranchDiff
  let selectedPath = $state(null)
  let singleDiff  = $state(null) // string from FileDiff
  let loading     = $state(false)
  let error       = $state(null)

  // Lines parsed for the selected diff.
  let diffLines = $derived(parseDiff(singleDiff))

  function parseDiff(raw) {
    if (!raw) return []
    return raw.split('\n').map(line => {
      if (line.startsWith('+') && !line.startsWith('+++')) return { type: 'add',    text: line }
      if (line.startsWith('-') && !line.startsWith('---')) return { type: 'remove', text: line }
      if (line.startsWith('@@'))                           return { type: 'hunk',   text: line }
      return { type: 'context', text: line }
    })
  }

  async function loadBranchDiff() {
    if (!headRef || !baseRef) return
    loading = true
    error   = null
    patches = []
    selectedPath = null
    singleDiff   = null
    try {
      const p = await getBranchDiff(projectId, baseRef, headRef)
      patches = Array.isArray(p) ? p : []
      if (patches.length > 0) await loadFileDiff(patches[0].path)
    } catch (e) { error = e.message }
    finally { loading = false }
  }

  async function loadFileDiff(filePath) {
    selectedPath = filePath
    singleDiff   = null
    try {
      const resp = await getFileDiff(projectId, baseRef, headRef, filePath)
      singleDiff = resp?.diff ?? ''
    } catch (e) { error = e.message }
  }

  $effect(() => {
    if (projectId && baseRef && headRef) loadBranchDiff()
  })

  const statusColor = { added: 'text-green-400', modified: 'text-yellow-400', deleted: 'text-red-400' }
  const statusLabel = { added: 'A', modified: 'M', deleted: 'D' }
</script>

<div class="flex h-full text-sm overflow-hidden">
  <!-- File sidebar -->
  {#if patches.length > 0}
    <div class="w-56 shrink-0 border-r border-surface-600 overflow-y-auto py-2">
      {#each patches as p}
        <button
          class="w-full text-left flex items-center gap-2 px-3 py-1 text-xs font-mono
                 transition-colors truncate
                 {selectedPath === p.path
                   ? 'bg-accent text-white'
                   : 'text-gray-300 hover:bg-surface-600'}"
          onclick={() => loadFileDiff(p.path)}
        >
          <span class="shrink-0 font-bold {statusColor[p.status] ?? ''}">{statusLabel[p.status] ?? '?'}</span>
          <span class="truncate">{p.path}</span>
        </button>
      {/each}
    </div>
  {/if}

  <!-- Diff content -->
  <div class="flex-1 overflow-auto">
    {#if loading}
      <div class="flex items-center justify-center h-full text-xs text-gray-500">Loading…</div>
    {:else if error}
      <div class="flex items-center justify-center h-full text-xs text-red-400">{error}</div>
    {:else if !headRef}
      <div class="flex items-center justify-center h-full text-xs text-gray-500">Select a head branch to compare.</div>
    {:else if patches.length === 0}
      <div class="flex items-center justify-center h-full text-xs text-gray-500">No changes between {baseRef} and {headRef}.</div>
    {:else if !singleDiff}
      <div class="flex items-center justify-center h-full text-xs text-gray-500">Select a file.</div>
    {:else}
      <pre class="p-4 text-xs font-mono leading-5 whitespace-pre-wrap">{#each diffLines as l}<span
          class={l.type === 'add'    ? 'block bg-green-950 text-green-300'
               : l.type === 'remove' ? 'block bg-red-950 text-red-300'
               : l.type === 'hunk'   ? 'block text-blue-400'
               :                       'block text-gray-400'}
        >{l.text}</span>{/each}</pre>
    {/if}
  </div>
</div>
