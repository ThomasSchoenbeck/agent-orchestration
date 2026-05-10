<script>
  import { onMount } from 'svelte'
  import { listProviders, getMetrics } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'

  // ── Svelte 5 runes ────────────────────────────────────────────────────────
  let providers = $state([])
  let metrics   = $state(null)
  let loading   = $state(false)

  async function load() {
    loading = true
    try {
      const [pr, mr] = await Promise.all([
        listProviders().catch(() => null),
        getMetrics().catch(() => null),
      ])
      providers = Array.isArray(pr) ? pr : (pr?.providers ?? [])
      metrics   = mr
    } catch (e) {
      toasts.error('Failed to load: ' + e.message)
    } finally {
      loading = false
    }
  }

  // Flatten metrics object to [label, value] pairs for display
  function metricEntries(m) {
    if (!m || typeof m !== 'object') return []
    return Object.entries(m).filter(([, v]) => v !== null && v !== undefined)
  }

  onMount(load)
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-xl font-semibold text-gray-100">Providers &amp; Metrics</h1>
    <button
      class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
      onclick={load}
    >↻ Refresh</button>
  </div>

  {#if loading}
    <p class="text-gray-500 text-sm">Loading…</p>
  {:else}

    <!-- Provider list -->
    <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-3">LLM Providers</h2>
    {#if providers.length === 0}
      <p class="text-gray-500 text-sm mb-8">No providers configured.</p>
    {:else}
      <div class="grid gap-2 mb-8">
        {#each providers as p (p.id ?? p.name)}
          <div class="p-3 bg-surface-800 rounded border border-surface-600 flex items-center gap-3">
            <span class="w-2 h-2 rounded-full bg-green-400"></span>
            <span class="text-sm font-medium text-gray-200">{p.name}</span>
            <span class="text-xs text-gray-500">{p.type ?? ''}</span>
            {#if p.model_name}
              <span class="ml-auto text-xs text-gray-500">{p.model_name}</span>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    <!-- Metrics summary -->
    {#if metrics && metricEntries(metrics).length > 0}
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-3">Metrics</h2>
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {#each metricEntries(metrics) as [key, val]}
          <div class="p-3 bg-surface-800 rounded border border-surface-600">
            <div class="text-xs text-gray-500 mb-1 capitalize">{key.replace(/_/g, ' ')}</div>
            <div class="text-lg font-semibold text-gray-100">
              {typeof val === 'number' ? val.toLocaleString() : String(val)}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
