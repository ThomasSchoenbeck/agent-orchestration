<script>
  import { onMount } from 'svelte'
  import {
    listProviders, createProvider, updateProvider, deleteProvider,
    testProvider, getMetrics, getCostBreakdown, listRoles,
  } from '../lib/api.js'
  import { toasts, router } from '../lib/stores.js'
  import Skeleton from '../components/Skeleton.svelte'

  // ── State ─────────────────────────────────────────────────────────────────
  let providers   = $state([])
  let roles       = $state([])
  let metrics     = $state(null)
  let loading     = $state(false)

  // ── Cost breakdown (F6) ──────────────────────────────────────────────────
  let breakdownGroup = $state('source')
  let breakdown      = $state([])
  let breakdownMax   = $derived(breakdown.reduce((mx, b) => Math.max(mx, b.cost ?? 0), 0))
  async function loadBreakdown() {
    try {
      const b = await getCostBreakdown(breakdownGroup)
      breakdown = Array.isArray(b) ? b : []
    } catch (_) { breakdown = [] }
  }
  function breakdownPct(b) {
    return breakdownMax > 0 ? Math.round((b.cost ?? 0) / breakdownMax * 100) : 0
  }
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

  const emptyModel = () => ({
    name: '', roles: '', input_per_million: '', output_per_million: '',
    text_tool_calls: false, fold_system_into_user: false,
    system_prefix: '', tool_allowlist: '',
  })

  const emptyForm = () => ({
    name: '', type: 'openai_compatible', api_key: '',
    base_url: '', model_name: '', enabled: true, deployment: '', text_tool_calls: false, fold_system_into_user: false, system_prefix: '', tool_allowlist: '',
    roles: [],
    models: [],  // per-model config rows
  })
  let form = $state(emptyForm())

  // ── Derived ───────────────────────────────────────────────────────────────
  function typeLabel(t) {
    return PROVIDER_TYPES.find(pt => pt.value === t)?.label ?? t
  }

  // Returns only scalar (number / string / boolean) metric entries for the grid display.
  function metricEntries(m) {
    if (!m || typeof m !== 'object') return []
    return Object.entries(m).filter(([, v]) =>
      v !== null && v !== undefined && !Array.isArray(v) && typeof v !== 'object'
    )
  }

  // Returns array/object metric entries (e.g. by_project, by_agent) for table display.
  function metricTableEntries(m) {
    if (!m || typeof m !== 'object') return []
    return Object.entries(m).filter(([, v]) => Array.isArray(v) && v.length > 0)
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  let rolesError = $state(null)

  async function loadRoles() {
    rolesError = null
    try {
      const rl = await listRoles()
      roles = Array.isArray(rl) ? rl : []
    } catch (e) {
      rolesError = e.message
    }
  }

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
      deployment:      p.config?.deployment ?? '',
      text_tool_calls:      p.config?.text_tool_calls ?? false,
      fold_system_into_user: p.config?.fold_system_into_user ?? false,
      system_prefix:         p.config?.system_prefix ?? '',
      tool_allowlist:        (p.config?.tool_allowlist ?? []).join(', '),
      roles:      p.roles ?? [],
      models:     (p.models ?? []).map(m => ({
        name:               m.name,
        roles:              (m.roles ?? []).join(', '),
        input_per_million:  String(m.input_per_million ?? ''),
        output_per_million: String(m.output_per_million ?? ''),
        text_tool_calls:    m.text_tool_calls ?? false,
        fold_system_into_user: m.fold_system_into_user ?? false,
        system_prefix:      m.system_prefix ?? '',
        tool_allowlist:     (m.tool_allowlist ?? []).join(', '),
      })),
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
      roles:      form.roles,
      config: {
        ...(form.deployment ? { deployment: form.deployment.trim() } : {}),
        ...(form.text_tool_calls ? { text_tool_calls: true } : {}),
        ...(form.fold_system_into_user ? { fold_system_into_user: true } : {}),
        ...(form.system_prefix ? { system_prefix: form.system_prefix } : {}),
        ...(form.tool_allowlist.trim() ? { tool_allowlist: form.tool_allowlist.split(',').map(s => s.trim()).filter(Boolean) } : {}),
      },
    }
    if (form.api_key.trim()) body.api_key = form.api_key.trim()
    body.models = form.models
      .filter(m => m.name.trim())
      .map(m => ({
        name:               m.name.trim(),
        roles:              m.roles.split(',').map(r => r.trim()).filter(Boolean),
        input_per_million:  parseFloat(m.input_per_million) || 0,
        output_per_million: parseFloat(m.output_per_million) || 0,
        ...(m.text_tool_calls        ? { text_tool_calls: true }              : {}),
        ...(m.fold_system_into_user  ? { fold_system_into_user: true }        : {}),
        ...(m.system_prefix.trim()   ? { system_prefix: m.system_prefix.trim() } : {}),
        ...(m.tool_allowlist.trim()  ? { tool_allowlist: m.tool_allowlist.split(',').map(s => s.trim()).filter(Boolean) } : {}),
      }))

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

  onMount(() => { load(); loadRoles(); loadBreakdown() })
</script>

<div class="flex-1 overflow-y-auto p-6">

  <!-- Header -->
  <div class="flex items-center justify-between mb-5">
    <h1 class="text-xl font-semibold text-gray-100">Providers</h1>
    <div class="flex items-center gap-2">
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

      <div class="mt-1 pt-2 border-t border-surface-700">
        <span class="text-xs text-gray-500 uppercase tracking-wide">
          Default behaviour
          <span class="text-gray-600 normal-case">(applied when a model row below does not override it)</span>
        </span>
      </div>

      <label class="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
        <input type="checkbox" bind:checked={form.text_tool_calls} class="accent-accent" />
        <span>
          Text tool calls
          <span class="text-xs text-gray-500 ml-1">(for models that don't support structured function calling)</span>
        </span>
      </label>

      <label class="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
        <input type="checkbox" bind:checked={form.fold_system_into_user} class="accent-accent" />
        <span>
          Fold system prompt into user message
          <span class="text-xs text-gray-500 ml-1">(for models without a system role, e.g. Gemma)</span>
        </span>
      </label>

      <div>
        <label for="provider-system-prefix" class="text-xs text-gray-500 mb-1 block">
          System prompt prefix
          <span class="text-gray-600 ml-1">(prepended before the system message, e.g. &lt;|think|&gt; for Gemma)</span>
        </label>
        <input
          id="provider-system-prefix"
          class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                 text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="<|think|>"
          bind:value={form.system_prefix}
        />
      </div>

      <div>
        <label for="provider-tool-allowlist" class="text-xs text-gray-500 mb-1 block">
          Tool allowlist
          <span class="text-gray-600 ml-1">(comma-separated — leave empty to send all tools; recommended for small models)</span>
        </label>
        <input
          id="provider-tool-allowlist"
          class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                 text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="write_file, read_file, list_files, apply_diff, run_tests"
          bind:value={form.tool_allowlist}
        />
      </div>

      <div>
        <label for="provider-roles" class="text-xs text-gray-500 mb-1 block">Roles</label>
        {#if rolesError}
          <p class="text-xs text-red-400">Failed to load roles: {rolesError}</p>
        {:else if roles.length === 0}
          <p class="text-xs text-gray-500">
            No roles defined yet —
            <a href="#/roles" class="text-accent hover:underline">create roles</a>
            first, then assign them here.
          </p>
        {:else}
          <select
            id="provider-roles"
            multiple
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 focus:outline-none focus:border-accent"
            size={Math.max(3, roles.length)}
            onchange={(e) => {
              form.roles = Array.from(e.target.selectedOptions).map(o => o.value)
            }}
          >
            {#each roles as role}
              <option value={role.name} selected={form.roles.includes(role.name)}>
                {role.label || role.name}
              </option>
            {/each}
          </select>
        {/if}
      </div>

      <!-- Models table: per-model role and pricing config -->
      <div>
        <div class="flex items-center justify-between mb-1">
          <label class="text-xs text-gray-500">
            Models
            <span class="text-gray-600 ml-1">(optional — assign roles and pricing per model)</span>
          </label>
          <button
            type="button"
            class="text-xs text-accent hover:text-accent-hover"
            onclick={() => form.models = [...form.models, emptyModel()]}
          >+ Add model</button>
        </div>
        {#if form.models.length > 0}
          <div class="border border-surface-600 rounded overflow-hidden">
            <table class="w-full text-xs">
              <thead class="bg-surface-700 text-gray-500">
                <tr>
                  <th class="px-2 py-1 text-left font-medium">Model name</th>
                  <th class="px-2 py-1 text-left font-medium">Roles</th>
                  <th class="px-2 py-1 text-left font-medium">In $/M</th>
                  <th class="px-2 py-1 text-left font-medium">Out $/M</th>
                  <th class="px-2 py-1 text-left font-medium" title="Text tool calls">TTC</th>
                  <th class="px-2 py-1 text-left font-medium" title="Fold system into user">FSU</th>
                  <th class="px-2 py-1 text-left font-medium">Sys prefix</th>
                  <th class="px-2 py-1 text-left font-medium">Tool allowlist</th>
                  <th class="px-2 py-1"></th>
                </tr>
              </thead>
              <tbody>
                {#each form.models as m, i}
                  <tr class="border-t border-surface-700">
                    <td class="px-2 py-1">
                      <input
                        class="w-28 bg-surface-700 rounded px-2 py-1 text-gray-200 font-mono focus:outline-none focus:ring-1 focus:ring-accent"
                        placeholder="gemma3:4b"
                        bind:value={m.name}
                      />
                    </td>
                    <td class="px-2 py-1">
                      <input
                        class="w-28 bg-surface-700 rounded px-2 py-1 text-gray-200 focus:outline-none focus:ring-1 focus:ring-accent"
                        placeholder="worker, reviewer"
                        bind:value={m.roles}
                      />
                    </td>
                    <td class="px-2 py-1">
                      <input
                        class="w-16 bg-surface-700 rounded px-2 py-1 text-gray-200 focus:outline-none focus:ring-1 focus:ring-accent"
                        type="number" step="0.001" min="0"
                        placeholder="0.05"
                        bind:value={m.input_per_million}
                      />
                    </td>
                    <td class="px-2 py-1">
                      <input
                        class="w-16 bg-surface-700 rounded px-2 py-1 text-gray-200 focus:outline-none focus:ring-1 focus:ring-accent"
                        type="number" step="0.001" min="0"
                        placeholder="0.10"
                        bind:value={m.output_per_million}
                      />
                    </td>
                    <td class="px-2 py-1 text-center">
                      <input type="checkbox" bind:checked={m.text_tool_calls} class="accent-accent" title="Text tool calls (model outputs JSON blocks instead of native function calls)" />
                    </td>
                    <td class="px-2 py-1 text-center">
                      <input type="checkbox" bind:checked={m.fold_system_into_user} class="accent-accent" title="Fold system prompt into first user message (for models without a system role)" />
                    </td>
                    <td class="px-2 py-1">
                      <input
                        class="w-20 bg-surface-700 rounded px-2 py-1 text-gray-200 font-mono focus:outline-none focus:ring-1 focus:ring-accent"
                        placeholder="<|think|>"
                        bind:value={m.system_prefix}
                      />
                    </td>
                    <td class="px-2 py-1">
                      <input
                        class="w-36 bg-surface-700 rounded px-2 py-1 text-gray-200 font-mono focus:outline-none focus:ring-1 focus:ring-accent"
                        placeholder="write_file, read_file"
                        bind:value={m.tool_allowlist}
                      />
                    </td>
                    <td class="px-2 py-1">
                      <button
                        type="button"
                        class="text-red-400 hover:text-red-300"
                        onclick={() => form.models = form.models.filter((_, j) => j !== i)}
                      >✕</button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>

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
    <Skeleton rows={3} />
  {:else if providers.length === 0}
    <p class="text-gray-400 text-sm mb-8">
      No providers configured. Add one above.
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
              {#if p.roles?.length > 0}
                <div class="flex flex-wrap gap-1 mt-1">
                  {#each p.roles as role}
                    <span class="text-xs px-1.5 py-0.5 rounded bg-surface-700 text-blue-400">{role}</span>
                  {/each}
                </div>
              {/if}
              {#if p.models?.length > 0}
                <div class="mt-2 flex flex-col gap-0.5">
                  {#each p.models as m}
                    <div class="text-xs text-gray-500 font-mono flex flex-wrap gap-x-2">
                      <span class="text-gray-400">{m.name}</span>
                      {#if m.roles?.length > 0}
                        <span class="text-blue-500">{m.roles.join(', ')}</span>
                      {/if}
                      {#if m.input_per_million > 0 || m.output_per_million > 0}
                        <span class="text-gray-600">${m.input_per_million}/${m.output_per_million}/M</span>
                      {/if}
                      {#if m.text_tool_calls}
                        <span class="text-yellow-600" title="Text tool calls">TTC</span>
                      {/if}
                      {#if m.fold_system_into_user}
                        <span class="text-yellow-600" title="Fold system into user">FSU</span>
                      {/if}
                      {#if m.system_prefix}
                        <span class="text-gray-600">prefix:{m.system_prefix}</span>
                      {/if}
                    </div>
                  {/each}
                </div>
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

  <!-- Metrics (cost is surfaced here too — the separate Cost box was redundant) -->
  {#if metrics && (metricEntries(metrics).length > 0 || metricTableEntries(metrics).length > 0)}
    <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wide mb-3">Metrics</h2>
    {#if metricEntries(metrics).length > 0}
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 mb-4">
        {#each metricEntries(metrics) as [key, val]}
          <div class="p-3 bg-surface-800 rounded border border-surface-600">
            <div class="text-xs text-gray-500 mb-1 capitalize">{key.replace(/_/g, ' ')}</div>
            <div class="text-lg font-semibold text-gray-100">
              {key.includes('cost')
                ? '~$' + (Number(val) || 0).toFixed(4)
                : (typeof val === 'number' ? val.toLocaleString() : String(val))}
            </div>
          </div>
        {/each}
      </div>
    {/if}
    {#each metricTableEntries(metrics) as [key, rows]}
      <div class="mb-4">
        <div class="text-xs text-gray-500 uppercase tracking-wide mb-1 capitalize">{key.replace(/_/g, ' ')}</div>
        <div class="overflow-x-auto">
          <table class="w-full text-xs text-left text-gray-300">
            <thead class="text-gray-500">
              <tr>
                {#each Object.keys(rows[0]) as col}
                  <th class="pr-4 pb-1 capitalize font-medium">{col.replace(/_/g, ' ')}</th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each rows as row}
                <tr class="border-t border-surface-700">
                  {#each Object.values(row) as cell}
                    <td class="pr-4 py-1">
                      {typeof cell === 'number' ? cell.toLocaleString() : String(cell ?? '')}
                    </td>
                  {/each}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/each}
  {/if}

  <!-- Cost breakdown (F6): distinguish cost by agent vs chat, agent type, etc. -->
  <div class="mb-8">
    <div class="flex items-center justify-between mb-3">
      <div class="flex items-center gap-3">
        <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wide">Cost breakdown</h2>
        <button class="text-xs text-accent hover:underline" onclick={() => router.go('costs')}>View details →</button>
      </div>
      <select
        class="bg-surface-700 border border-surface-600 rounded px-2 py-1 text-xs text-gray-300 focus:outline-none"
        bind:value={breakdownGroup}
        onchange={loadBreakdown}
      >
        <option value="source">Agent vs Chat</option>
        <option value="agent_role">By agent type</option>
        <option value="agent_id">By specific agent</option>
        <option value="provider">By provider</option>
        <option value="model">By model</option>
      </select>
    </div>
    {#if breakdown.length === 0}
      <p class="text-xs text-gray-600 italic">No cost data recorded yet.</p>
    {:else}
      <div class="flex flex-col gap-1.5">
        {#each breakdown as b}
          <div class="flex items-center gap-2 text-xs">
            <div class="w-40 shrink-0 truncate font-mono text-gray-300" title={b.key}>{b.key || '(none)'}</div>
            <div class="flex-1 bg-surface-800 rounded h-4 overflow-hidden">
              <div class="bg-accent h-4 rounded" style="width: {breakdownPct(b)}%"></div>
            </div>
            <div class="w-20 shrink-0 text-right text-gray-200">~${(b.cost ?? 0).toFixed(4)}</div>
            <div class="w-14 shrink-0 text-right text-gray-500">{b.count}×</div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
