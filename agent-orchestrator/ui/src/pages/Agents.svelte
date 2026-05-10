<script>
  import { onMount, onDestroy } from 'svelte'
  import { listAgents } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'

  // ── Svelte 5 runes ────────────────────────────────────────────────────────
  let agents   = $state([])
  let loading  = $state(false)
  let interval = $state(null)

  const statusDot = {
    online:  'bg-green-400',
    offline: 'bg-gray-500',
    busy:    'bg-yellow-400',
    idle:    'bg-blue-400',
  }

  async function load() {
    loading = true
    try {
      const res = await listAgents()
      agents   = Array.isArray(res) ? res : (res.agents ?? [])
    } catch (e) {
      toasts.error('Failed to load agents: ' + e.message)
    } finally {
      loading = false
    }
  }

  onMount(() => {
    load()
    interval = setInterval(load, 10_000)
  })
  onDestroy(() => clearInterval(interval))
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-xl font-semibold text-gray-100">Agents</h1>
    <button
      class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
      onclick={load}
    >↻ Refresh</button>
  </div>

  {#if loading && agents.length === 0}
    <p class="text-gray-500 text-sm">Loading…</p>
  {:else if agents.length === 0}
    <p class="text-gray-500 text-sm">No agents registered.</p>
  {:else}
    <div class="grid gap-3">
      {#each agents as a (a.id)}
        <div class="p-4 bg-surface-800 rounded border border-surface-600">
          <div class="flex items-center gap-3 mb-2">
            <span class="w-2.5 h-2.5 rounded-full shrink-0 {statusDot[a.status] || 'bg-gray-500'}"></span>
            <span class="font-medium text-gray-100">{a.name}</span>
            <span class="text-xs text-gray-500 capitalize">{a.status}</span>
          </div>
          <div class="flex flex-wrap gap-1 mb-2">
            {#each (a.roles ?? []) as role}
              <span class="text-xs px-2 py-0.5 bg-surface-700 text-gray-300 rounded-full">{role}</span>
            {/each}
          </div>
          <div class="text-xs text-gray-600 font-mono">{a.id}</div>
          {#if a.last_heartbeat && a.last_heartbeat !== '0001-01-01T00:00:00Z'}
            <div class="text-xs text-gray-600 mt-1">
              Last seen: {new Date(a.last_heartbeat).toLocaleString()}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
