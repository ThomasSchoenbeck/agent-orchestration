<script>
  import { onMount } from 'svelte'
  import { getCostBreakdown, getCostFilterOptions, listAgents } from '../lib/api.js'
  import { router } from '../lib/stores.js'

  // ── Dimensions (drill-down) ────────────────────────────────────────────────
  const DIMENSIONS = [
    { value: 'source',     label: 'Agent vs Chat' },
    { value: 'agent_role', label: 'By task type (role)' },
    { value: 'agent_id',   label: 'By agent' },
    { value: 'task',       label: 'By task' },
    { value: 'chat',       label: 'By chat' },
    { value: 'model',      label: 'By model' },
    { value: 'provider',   label: 'By provider' },
    { value: 'day',        label: 'By day' },
  ]

  // ── State ──────────────────────────────────────────────────────────────────
  let groupBy   = $state('agent_id')
  let filters   = $state({ from: '', to: '', model: '', agent_role: '', source: '', provider: '' })
  let breakdown = $state([])
  let options   = $state({ models: [], agent_roles: [], sources: [], providers: [], min_date: '', max_date: '' })
  let loading   = $state(false)
  let error     = $state('')
  let agentNames = $state({}) // agent id → display name (cost breakdown shows names, ids in tooltip)

  // Display label for a row: agent rows show the agent name (id stays in the
  // tooltip + link), falling back to the id when the agent is unknown/deleted.
  function displayKey(b) {
    if (!b.key) return '(none)'
    if (groupBy === 'agent_id') return agentNames[b.key] ?? b.key
    return b.key
  }

  let breakdownMax = $derived(breakdown.reduce((mx, b) => Math.max(mx, b.cost ?? 0), 0))
  let totalCost    = $derived(breakdown.reduce((s, b) => s + (b.cost ?? 0), 0))
  let totalIn      = $derived(breakdown.reduce((s, b) => s + (b.input_tokens ?? 0), 0))
  let totalOut     = $derived(breakdown.reduce((s, b) => s + (b.output_tokens ?? 0), 0))
  let totalInCost  = $derived(breakdown.reduce((s, b) => s + (b.input_cost ?? 0), 0))
  let totalOutCost = $derived(breakdown.reduce((s, b) => s + (b.output_cost ?? 0), 0))
  let totalCount   = $derived(breakdown.reduce((s, b) => s + (b.count ?? 0), 0))

  function pct(b) {
    return breakdownMax > 0 ? Math.round((b.cost ?? 0) / breakdownMax * 100) : 0
  }

  // A link target for id-keyed dimensions, else null (plain text).
  function linkFor(b) {
    if (!b.key) return null
    if (groupBy === 'agent_id') return ['agents', b.key]
    if (groupBy === 'task')     return ['tasks', b.key]
    return null
  }

  async function reload() {
    loading = true
    error = ''
    try {
      const b = await getCostBreakdown(groupBy, filters)
      breakdown = Array.isArray(b) ? b : []
    } catch (e) {
      breakdown = []
      error = e?.message || 'Failed to load cost breakdown'
    } finally {
      loading = false
    }
  }

  function clearFilters() {
    filters = { from: '', to: '', model: '', agent_role: '', source: '', provider: '' }
    reload()
  }

  onMount(async () => {
    try {
      options = await getCostFilterOptions()
    } catch (_) { /* options are best-effort */ }
    try {
      const agents = await listAgents()
      const map = {}
      for (const a of (Array.isArray(agents) ? agents : [])) map[a.id] = a.name
      agentNames = map
    } catch (_) { /* name mapping is best-effort */ }
    reload()
  })
</script>

<div class="flex-1 overflow-y-auto p-6">
  <!-- Header -->
  <div class="flex items-center gap-3 mb-6">
    <button
      class="text-xs text-accent hover:underline"
      onclick={() => router.go('providers')}
    >← Providers</button>
    <h1 class="text-xl font-semibold text-gray-100">Cost detail</h1>
  </div>

  <!-- Filters -->
  <div class="flex flex-wrap items-end gap-3 mb-6 p-4 bg-surface-800 rounded border border-surface-600">
    <label class="flex flex-col gap-1 text-xs text-gray-400">
      From
      <input type="date" bind:value={filters.from} onchange={reload}
        min={options.min_date} max={options.max_date}
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-gray-200 focus:outline-none" />
    </label>
    <label class="flex flex-col gap-1 text-xs text-gray-400">
      To
      <input type="date" bind:value={filters.to} onchange={reload}
        min={options.min_date} max={options.max_date}
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-gray-200 focus:outline-none" />
    </label>
    <label class="flex flex-col gap-1 text-xs text-gray-400">
      Task type (role)
      <select bind:value={filters.agent_role} onchange={reload}
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-gray-200 focus:outline-none">
        <option value="">All</option>
        {#each options.agent_roles as r}<option value={r}>{r}</option>{/each}
      </select>
    </label>
    <label class="flex flex-col gap-1 text-xs text-gray-400">
      Model
      <select bind:value={filters.model} onchange={reload}
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-gray-200 focus:outline-none">
        <option value="">All</option>
        {#each options.models as m}<option value={m}>{m}</option>{/each}
      </select>
    </label>
    <label class="flex flex-col gap-1 text-xs text-gray-400">
      Source
      <select bind:value={filters.source} onchange={reload}
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-gray-200 focus:outline-none">
        <option value="">All</option>
        {#each options.sources as s}<option value={s}>{s}</option>{/each}
      </select>
    </label>
    <label class="flex flex-col gap-1 text-xs text-gray-400">
      Provider
      <select bind:value={filters.provider} onchange={reload}
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-gray-200 focus:outline-none">
        <option value="">All</option>
        {#each options.providers as p}<option value={p}>{p}</option>{/each}
      </select>
    </label>
    <button
      class="text-xs text-gray-400 hover:text-gray-200 px-2 py-1 border border-surface-600 rounded"
      onclick={clearFilters}
    >Clear filters</button>
  </div>

  <!-- Dimension switch + totals -->
  <div class="flex items-center justify-between mb-3">
    <label class="flex items-center gap-2 text-sm text-gray-400">
      Break down
      <select bind:value={groupBy} onchange={reload}
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none">
        {#each DIMENSIONS as d}<option value={d.value}>{d.label}</option>{/each}
      </select>
    </label>
    <div class="text-xs text-gray-400">
      Total <span class="text-gray-100 font-semibold">~${totalCost.toFixed(4)}</span>
      (in ~${totalInCost.toFixed(4)} / out ~${totalOutCost.toFixed(4)})
      · {totalIn.toLocaleString()} in / {totalOut.toLocaleString()} out tokens
      · {totalCount.toLocaleString()} calls
    </div>
  </div>

  <!-- Breakdown -->
  {#if error}
    <p class="text-xs text-red-400">{error}</p>
  {:else if loading}
    <p class="text-xs text-gray-500 italic">Loading…</p>
  {:else if breakdown.length === 0}
    <p class="text-xs text-gray-600 italic">No cost data for the selected filters.</p>
  {:else}
    <div class="flex flex-col gap-1.5">
      {#each breakdown as b}
        {@const link = linkFor(b)}
        <div class="flex items-center gap-2 text-xs">
          <div class="w-48 shrink-0 truncate font-mono text-gray-300" title={b.key}>
            {#if link}
              <button class="text-accent hover:underline" onclick={() => router.push(...link)}>{displayKey(b)}</button>
            {:else}
              {displayKey(b)}
            {/if}
          </div>
          <div class="flex-1 bg-surface-800 rounded h-4 overflow-hidden">
            <div class="bg-accent h-4 rounded" style="width: {pct(b)}%"></div>
          </div>
          <div class="w-24 shrink-0 text-right text-gray-500" title="input / output tokens">{(b.input_tokens ?? 0).toLocaleString()}/{(b.output_tokens ?? 0).toLocaleString()}</div>
          <div class="w-32 shrink-0 text-right text-gray-500" title="input / output cost">~${(b.input_cost ?? 0).toFixed(4)} / ~${(b.output_cost ?? 0).toFixed(4)}</div>
          <div class="w-20 shrink-0 text-right text-gray-200" title="total cost">~${(b.cost ?? 0).toFixed(4)}</div>
          <div class="w-12 shrink-0 text-right text-gray-500">{b.count}×</div>
        </div>
      {/each}
    </div>
  {/if}
</div>
