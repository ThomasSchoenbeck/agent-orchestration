<script>
  import { onMount } from 'svelte'
  import { listLogs } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'

  // ── Svelte 5 runes ────────────────────────────────────────────────────────
  let logs    = $state([])
  let loading = $state(false)
  let level   = $state('')
  let limit   = $state(100)

  const levelColors = {
    debug: 'text-gray-500',
    info:  'text-blue-400',
    warn:  'text-yellow-400',
    error: 'text-red-400',
  }

  async function load() {
    loading = true
    try {
      const params = { limit }
      if (level) params.level = level
      const res = await listLogs(params)
      logs      = Array.isArray(res) ? res : (res.logs ?? [])
    } catch (e) {
      toasts.error('Failed to load logs: ' + e.message)
    } finally {
      loading = false
    }
  }

  function formatTime(ts) {
    if (!ts) return ''
    try { return new Date(ts).toLocaleTimeString() } catch { return ts }
  }

  onMount(load)
</script>

<div class="flex-1 overflow-hidden flex flex-col p-6">
  <!-- Controls -->
  <div class="flex items-center justify-between mb-4 shrink-0">
    <h1 class="text-xl font-semibold text-gray-100">Logs</h1>
    <div class="flex items-center gap-3">
      <select
        class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-sm text-gray-300
               focus:outline-none focus:border-accent"
        bind:value={level}
        onchange={load}
      >
        <option value="">All levels</option>
        <option value="debug">debug</option>
        <option value="info">info</option>
        <option value="warn">warn</option>
        <option value="error">error</option>
      </select>
      <select
        class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-sm text-gray-300
               focus:outline-none focus:border-accent"
        bind:value={limit}
        onchange={load}
      >
        <option value={50}>50</option>
        <option value={100}>100</option>
        <option value={500}>500</option>
      </select>
      <button
        class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
        onclick={load}
      >↻</button>
    </div>
  </div>

  <!-- Log stream -->
  <div class="flex-1 overflow-y-auto bg-surface-800 rounded border border-surface-600 p-3 font-mono text-xs">
    {#if loading}
      <p class="text-gray-500">Loading…</p>
    {:else if logs.length === 0}
      <p class="text-gray-500">No log entries found.</p>
    {:else}
      {#each logs as entry (entry.id ?? entry.timestamp)}
        <div class="flex gap-3 py-0.5 border-b border-surface-700 last:border-0">
          <span class="text-gray-600 shrink-0 w-28">{formatTime(entry.timestamp)}</span>
          <span class="shrink-0 w-12 {levelColors[entry.level] ?? 'text-gray-400'}">{entry.level ?? 'info'}</span>
          {#if entry.agent_name || entry.agent_id}
            <span class="text-gray-500 shrink-0">[{entry.agent_name ?? entry.agent_id}]</span>
          {/if}
          <span class="text-gray-300 break-all">{entry.message}</span>
        </div>
      {/each}
    {/if}
  </div>
</div>
