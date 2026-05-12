<script>
  import { onMount } from 'svelte'
  import {
    listProviders, createProvider, updateProvider, deleteProvider,
    testProvider, seedProviders, getMetrics,
  } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'

  // ── State ─────────────────────────────────────────────────────────────────
  let providers   = $state([])
  let metrics     = $state(null)
  let loading     = $state(false)
  let showForm    = $state(false)
  let editingId   = $state(null)   // null = create, string = update
  let showKey     = $state(false)
  let testing     = $state({})     // providerid → boolean
  let testResults = $state({})     // providerid → {ok, latency_ms, error?}

  const PROVIDER_TYPES = [
    { value: 'openai_compatible', label: 'OpenAI compatible' },
    { value: 'anthropic',         label: 'Anthropic' },
    { value: 'azure',             label: 'Azure OpenAI' },
    { value: 'ollama',            label: 'Ollama (local)' },
  ]

  const emptyForm = () => ({
    name: '', type: 'openai_compatible', api_key: '',
    base_url: '', model_name: '', enabled: true, deployment: '',
  })
  let form = $state(emptyForm())

  // ── Derived ───────────────────────────────────────────────────────────────
  function typeLabel(t) {
    return PROVIDER_TYPES.find(pt => pt.value === t)?.label ?? t
  }

  function metricEntries(m) {
    if (!m || typeof m !== 'object') return []
    return Object.entries(m).filter(([, v]) => v !== null && v !== undefined)
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function load() {
    loading = true
    try {
      const [pr, mr] = await Promise.all([
        listProviders().catch(() => []),
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

  // ── Create / Update ───────────────────────────────────────────────────────
  function startCreate() {
    form      = emptyForm()
    editingId = null
    showKey   = false
    showForm  = true
  }

  function startEdit(p) {
    form = {
      name:       p.name,
      type:       p.type,
      api_key:    '',          // never pre-filled; blank = keep existing
      base_url:   p.base_url,
      model_name: p.model_name,
      enabled:    p.enabled,
      deployment: p.config?.deployment ?? '',
    }
    editingId = p.id
    showKey   = false
    showForm  = true
  }

  async function submit() {
    if (!form.name.trim() || !form.type) return
    const body = {
      name:       form.name.trim(),
      type:       form.type,
      base_url:   form.base_url.trim(),
      model_name: form.model_name.trim(),
      enabled:    form.enabled,
      config:     form.deployment ? { deployment: form.deployment.trim() } : {},
    }
    if (form.api_key.trim()) body.api_key = form.api_key.trim()

    try {
      if (editingId) {
        await updateProvider(editingId, body)
        toasts.success('Provider updated')
      } else {
        await createProvider(body)
        toasts.success('Provider created')
      }
      showForm  = false
      editingId = null
      await load()
    } catch (e) {
      toasts.error((editingId ? 'Update' : 'Create') + ' failed: ' + e.message)
    }
  }

  // ── Delete ────────────────────────────────────────────────────────────────
  async function remove(id) {
    if (!confirm('Delete this provider?')) return
    try {
      await deleteProvider(id)
      toasts.success('Provider deleted')
      await load()
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  // ── Enable / disable toggle ───────────────────────────────────────────────
  async function toggleEnabled(p) {
    const next = !p.enabled
    try {
      await updateProvider(p.id, { enabled: next })
      p.enabled = next // optimistic update; full reload follows
      await load()
    } catch (e) {
      toasts.error('Update failed: ' + e.message)
    }
  }

  // ── Test connection ───────────────────────────────────────────────────────
  async function runTest(id) {
    testing[id]     = true
    testResults[id] = null
    try {
      testResults[id] = await testProvider(id)
    } catch (e) {
      testResults[id] = { ok: false, error: e.message }
    } finally {
      testing[id] = false
    }
  }

  // ── Seed from config ──────────────────────────────────────────────────────
  async function runSeed() {
    try {
      const res = await seedProviders()
      toasts.success(`Seeded ${res.seeded ?? 0} new provider(s) from config`)
      await load()
    } catch (e) {
      toasts.error('Seed failed: ' + e.message)
    }
  }

  onMount(load)
</script>

<div class="flex-1 overflow-y-auto p-6">

  <!-- Header -->
  <div class="flex items-center justify-between mb-5">
    <h1 class="text-xl font-semibold text-gray-100">Providers</h1>
    <div class="flex items-center gap-2">
      <button
        class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
        onclick={runSeed}
      >↥ Import from config</button>
      <button
        class="text-xs text-gray-500 hover:text-gray-300 transition-colors"
        onclick={load}
      >↻ Refresh</button>
      <button
        class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
        onclick={startCreate}
      >{showForm && !editingId ? 'Cancel' : '+ Add Provider'}</button>
    </div>
  </div>

  <!-- Create / Edit form -->
  {#if showForm}
    <form
      class="mb-6 p-4 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3"
      onsubmit={(e) => { e.preventDefault(); submit() }}
    >
      <h2 class="text-sm font-semibold text-gray-300">
        {editingId ? 'Edit Provider' : 'New Provider'}
      </h2>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="provider-name" class="text-xs text-gray-500 mb-1 block">Name *</label>
          <input
            id="provider-name"
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="e.g. my-openai"
            bind:value={form.name}
            required
            readonly={!!editingId}
          />
        </div>
        <div>
          <label for="provider-type" class="text-xs text-gray-500 mb-1 block">Type *</label>
          <select
            id="provider-type"
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-300 focus:outline-none focus:border-accent"
            bind:value={form.type}
            required
          >
            {#each PROVIDER_TYPES as t}
              <option value={t.value}>{t.label}</option>
            {/each}
          </select>
        </div>
        <div>
          <label for="provider-api-key" class="text-xs text-gray-500 mb-1 block">
            API Key{editingId ? ' (leave blank to keep current)' : ''}
          </label>
          <div class="flex gap-1">
            <input
              id="provider-api-key"
              type={showKey ? 'text' : 'password'}
              class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
              placeholder={editingId ? '••••••••' : 'sk-…'}
              bind:value={form.api_key}
            />
            <button
              type="button"
              class="px-2 text-xs text-gray-400 hover:text-gray-200 border border-surface-500 rounded"
              onclick={() => showKey = !showKey}
            >{showKey ? 'Hide' : 'Show'}</button>
          </div>
        </div>
        <div>
          <label for="provider-model-name" class="text-xs text-gray-500 mb-1 block">Model name</label>
          <input
            id="provider-model-name"
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="e.g. gpt-4o"
            bind:value={form.model_name}
          />
        </div>
        <div class="col-span-2">
          <label for="provider-base-url" class="text-xs text-gray-500 mb-1 block">
            Base URL{form.type === 'ollama' ? ' *' : ' (optional)'}
          </label>
          <input
            id="provider-base-url"
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder={form.type === 'ollama' ? 'http://localhost:11434' : 'https://api.openai.com/v1'}
            bind:value={form.base_url}
          />
        </div>
        {#if form.type === 'azure'}
          <div class="col-span-2">
            <label for="provider-deployment" class="text-xs text-gray-500 mb-1 block">Azure deployment name *</label>
            <input
              id="provider-deployment"
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
              placeholder="my-gpt4-deployment"
              bind:value={form.deployment}
            />
          </div>
        {/if}
      </div>

      <label class="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
        <input type="checkbox" bind:checked={form.enabled} class="accent-accent" />
        Enabled
      </label>

      <div class="flex justify-end gap-2">
        <button
          type="button"
          class="px-3 py-1.5 text-sm text-gray-400 hover:text-gray-200 transition-colors"
          onclick={() => { showForm = false; editingId = null }}
        >Cancel</button>
        <button
          type="submit"
          class="px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
        >{editingId ? 'Update' : 'Create'}</button>
      </div>
    </form>
  {/if}

  <!-- Provider list -->
  {#if loading}
    <p class="text-gray-500 text-sm">Loading…</p>
  {:else if providers.length === 0}
    <p class="text-gray-500 text-sm mb-8">
      No providers configured. Add one above or
      <button class="underline hover:text-gray-300" onclick={runSeed}>import from config</button>.
    </p>
  {:else}
    <div class="flex flex-col gap-3 mb-8">
      {#each providers as p (p.id)}
        <div class="p-4 bg-surface-800 rounded border border-surface-600 hover:border-surface-500 transition-colors">
          <div class="flex items-start justify-between gap-4">
            <!-- Info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap mb-1">
                <span class="font-medium text-gray-100">{p.name}</span>
                <span class="text-xs px-1.5 py-0.5 rounded bg-surface-700 text-gray-400">
                  {typeLabel(p.type)}
                </span>
                {#if p.model_name}
                  <span class="text-xs text-gray-500 font-mono">{p.model_name}</span>
                {/if}
              </div>
              {#if p.base_url}
                <div class="text-xs text-gray-600 font-mono truncate">{p.base_url}</div>
              {/if}

              <!-- Test result -->
              {#if testing[p.id]}
                <div class="mt-2 text-xs text-gray-400">Testing…</div>
              {:else if testResults[p.id] != null}
                {#if testResults[p.id].ok}
                  <div class="mt-2 text-xs text-green-400">
                    ✓ {testResults[p.id].latency_ms}ms
                  </div>
                {:else}
                  <div class="mt-2 text-xs text-red-400 truncate">
                    ✗ {testResults[p.id].error ?? 'failed'}
                  </div>
                {/if}
              {/if}
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-3 shrink-0">
              <!-- Enabled toggle -->
              <label class="flex items-center gap-1.5 cursor-pointer text-xs text-gray-400 hover:text-gray-200">
                <input
                  type="checkbox"
                  checked={p.enabled}
                  onchange={() => toggleEnabled(p)}
                  class="accent-accent"
                />
                {p.enabled ? 'Enabled' : 'Disabled'}
              </label>

              <button
                class="text-xs text-blue-400 hover:text-blue-300 transition-colors"
                onclick={() => runTest(p.id)}
                disabled={testing[p.id]}
              >Test</button>
              <button
                class="text-xs text-gray-400 hover:text-gray-200 transition-colors"
                onclick={() => startEdit(p)}
              >Edit</button>
              <button
                class="text-xs text-red-400 hover:text-red-300 transition-colors"
                onclick={() => remove(p.id)}
              >Delete</button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Metrics -->
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
</div>
