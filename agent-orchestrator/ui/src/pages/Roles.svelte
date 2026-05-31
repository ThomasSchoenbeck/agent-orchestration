<script>
  import { onMount } from 'svelte'
  import {
    listRoles, createRole, updateRole, deleteRole, seedRoles, previewRolePrompt,
  } from '../lib/api.js'
  import { listProviders } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'
  import Skeleton from '../components/Skeleton.svelte'

  // ── State ─────────────────────────────────────────────────────────────────
  let roles     = $state([])
  let providers = $state([])
  let loading   = $state(false)
  let showForm  = $state(false)
  let editingId = $state(null)  // null = create mode

  // Form fields
  const emptyForm = () => ({
    name:            '',
    label:           '',
    description:     '',
    provider_id:     '',
    model_override:  '',
    system_prompt:   '',
    context_include: '',  // space-separated tags
    context_exclude: '',
    capabilities:    '',
    allowed_tools:   '',
    temperature:     0.7,
    max_tokens:      4096,
    enabled:         true,
  })
  let form = $state(emptyForm())

  // Prompt preview state
  let previewing     = $state(false)
  let previewResult  = $state('')
  let previewVarsRaw = $state('{}')

  // ── Derived ───────────────────────────────────────────────────────────────
  // Providers whose roles list includes the current form's role name.
  // Falls back to all providers when the name is blank or no provider matches.
  let compatibleProviders = $derived.by(() => {
    const name = form.name.trim()
    if (!name) return providers
    const filtered = providers.filter(p => Array.isArray(p.roles) && p.roles.includes(name))
    return filtered.length > 0 ? filtered : providers
  })

  let compatibleFiltered = $derived.by(() => {
    const name = form.name.trim()
    if (!name) return false
    return providers.some(p => Array.isArray(p.roles) && p.roles.includes(name))
  })

  // ── Constants ─────────────────────────────────────────────────────────────
  const SYSTEM_PROMPT_PLACEHOLDER = 'You are a {{.role}} agent…  (Go text/template syntax supported)'
  const PREVIEW_VARS_PLACEHOLDER = 'Variables JSON, e.g. {"role":"worker"}'

  // ── Helpers ───────────────────────────────────────────────────────────────
  function splitTags(s) {
    return s.split(/[\s,]+/).map(t => t.trim()).filter(Boolean)
  }

  function joinTags(arr) {
    return Array.isArray(arr) ? arr.join(' ') : ''
  }

  function providerLabel(id) {
    const p = providers.find(p => p.id === id)
    return p ? p.name : id || '—'
  }

  // ── Data loading ──────────────────────────────────────────────────────────
  async function load() {
    loading = true
    try {
      const [rs, ps] = await Promise.all([
        listRoles().catch(() => []),
        listProviders().catch(() => []),
      ])
      roles     = Array.isArray(rs) ? rs : []
      providers = Array.isArray(ps) ? ps : (ps?.providers ?? [])
    } catch (e) {
      toasts.error('Failed to load: ' + e.message)
    } finally {
      loading = false
    }
  }

  // ── Create / Edit ─────────────────────────────────────────────────────────
  function startCreate() {
    form      = emptyForm()
    editingId = null
    previewResult  = ''
    previewVarsRaw = '{}'
    showForm  = true
  }

  function startEdit(r) {
    form = {
      name:            r.name,
      label:           r.label,
      description:     r.description,
      provider_id:     r.provider_id ?? '',
      model_override:  r.model_override ?? '',
      system_prompt:   r.system_prompt ?? '',
      context_include: joinTags(r.context_include),
      context_exclude: joinTags(r.context_exclude),
      capabilities:    joinTags(r.capabilities),
      allowed_tools:   joinTags(r.allowed_tools),
      temperature:     r.temperature ?? 0.7,
      max_tokens:      r.max_tokens ?? 4096,
      enabled:         r.enabled,
    }
    editingId      = r.id
    previewResult  = ''
    previewVarsRaw = '{}'
    showForm       = true
  }

  async function submit() {
    if (!form.name.trim()) return
    const body = {
      name:            form.name.trim(),
      label:           form.label.trim(),
      description:     form.description.trim(),
      provider_id:     form.provider_id || '',
      model_override:  form.model_override.trim(),
      system_prompt:   form.system_prompt,
      context_include: splitTags(form.context_include),
      context_exclude: splitTags(form.context_exclude),
      capabilities:    splitTags(form.capabilities),
      allowed_tools:   splitTags(form.allowed_tools),
      temperature:     parseFloat(form.temperature) || 0.7,
      max_tokens:      parseInt(form.max_tokens, 10) || 4096,
      enabled:         form.enabled,
    }
    try {
      if (editingId) {
        await updateRole(editingId, body)
        toasts.success('Role updated')
      } else {
        await createRole(body)
        toasts.success('Role created')
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
    if (!confirm('Delete this role definition?')) return
    try {
      await deleteRole(id)
      toasts.success('Role deleted')
      await load()
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  // ── Enable / disable toggle ───────────────────────────────────────────────
  async function toggleEnabled(role) {
    try {
      await updateRole(role.id, { ...role, enabled: !role.enabled })
      await load()
    } catch (e) {
      toasts.error('Update failed: ' + e.message)
    }
  }

  // ── Seed from config ──────────────────────────────────────────────────────
  async function runSeed() {
    try {
      const res = await seedRoles()
      toasts.success(`Seeded ${res.seeded ?? 0} new role(s) from config`)
      await load()
    } catch (e) {
      toasts.error('Seed failed: ' + e.message)
    }
  }

  // ── Prompt preview ────────────────────────────────────────────────────────
  async function runPreview() {
    if (!editingId) return
    previewing = true
    previewResult = ''
    try {
      let vars = {}
      try { vars = JSON.parse(previewVarsRaw) } catch (_) {}
      const res = await previewRolePrompt(editingId, vars)
      previewResult = res.rendered ?? ''
    } catch (e) {
      toasts.error('Preview failed: ' + e.message)
    } finally {
      previewing = false
    }
  }

  onMount(load)
</script>

<div class="flex-1 overflow-y-auto p-6">

  <!-- Header -->
  <div class="flex items-center justify-between mb-5">
    <h1 class="text-xl font-semibold text-gray-100">Roles</h1>
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
      >{showForm && !editingId ? 'Cancel' : '+ Add Role'}</button>
    </div>
  </div>

  <!-- Create / Edit form -->
  {#if showForm}
    <form
      class="mb-6 p-4 bg-surface-800 rounded border border-surface-600 flex flex-col gap-4"
      onsubmit={(e) => { e.preventDefault(); submit() }}
    >
      <h2 class="text-sm font-semibold text-gray-300">
        {editingId ? 'Edit Role' : 'New Role'}
      </h2>

      <!-- General -->
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="text-xs text-gray-500 mb-1 block">Name * (slug)</label>
          <input
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="e.g. worker"
            bind:value={form.name}
            required
            readonly={!!editingId}
          />
        </div>
        <div>
          <label class="text-xs text-gray-500 mb-1 block">Label</label>
          <input
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="e.g. Worker Agent"
            bind:value={form.label}
          />
        </div>
        <div class="col-span-2">
          <label class="text-xs text-gray-500 mb-1 block">Description</label>
          <input
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="What does this role do?"
            bind:value={form.description}
          />
        </div>
      </div>

      <!-- Model -->
      <div class="border-t border-surface-600 pt-3">
        <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Model</p>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-xs text-gray-500 mb-1 block">
              Provider
              {#if compatibleFiltered}
                <span class="text-blue-400 ml-1">· filtered for this role</span>
              {:else if form.name.trim() && providers.length > 0}
                <span class="text-gray-600 ml-1">· no role-matched providers, showing all</span>
              {/if}
            </label>
            <select
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-300 focus:outline-none focus:border-accent"
              bind:value={form.provider_id}
            >
              <option value="">— none —</option>
              {#each compatibleProviders as p}
                <option value={p.id}>{p.name}</option>
              {/each}
            </select>
          </div>
          <div>
            <label class="text-xs text-gray-500 mb-1 block">Model override (optional)</label>
            <input
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
              placeholder="leave blank to use provider default"
              bind:value={form.model_override}
            />
          </div>
          <div>
            <label class="text-xs text-gray-500 mb-1 block">Temperature</label>
            <input
              type="number" min="0" max="2" step="0.05"
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-200 focus:outline-none focus:border-accent"
              bind:value={form.temperature}
            />
          </div>
          <div>
            <label class="text-xs text-gray-500 mb-1 block">Max tokens</label>
            <input
              type="number" min="1" step="1"
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-200 focus:outline-none focus:border-accent"
              bind:value={form.max_tokens}
            />
          </div>
        </div>
      </div>

      <!-- Routing -->
      <div class="border-t border-surface-600 pt-3">
        <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Routing</p>
        <div>
          <label class="text-xs text-gray-500 mb-1 block">
            Capabilities (space or comma-separated)
            <span class="text-gray-600 ml-1">known: handles_review, creates_tasks, handles_merge, handles_deploy</span>
          </label>
          <input
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="e.g. handles_review"
            bind:value={form.capabilities}
          />
        </div>
        <div>
          <label class="text-xs text-gray-500 mb-1 block">
            Tool allowlist
            <span class="text-gray-600 ml-1">(space or comma-separated — leave empty to send all tools)</span>
          </label>
          <input
            class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                   text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
            placeholder="e.g. write_file read_file list_files apply_diff run_tests"
            bind:value={form.allowed_tools}
          />
        </div>
      </div>

      <!-- Context rules -->
      <div class="border-t border-surface-600 pt-3">
        <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Context rules</p>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-xs text-gray-500 mb-1 block">Include types</label>
            <input
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
              placeholder="summary embedding"
              bind:value={form.context_include}
            />
          </div>
          <div>
            <label class="text-xs text-gray-500 mb-1 block">Exclude types</label>
            <input
              class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                     text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
              placeholder="raw_output"
              bind:value={form.context_exclude}
            />
          </div>
        </div>
      </div>

      <!-- System prompt -->
      <div class="border-t border-surface-600 pt-3">
        <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">System prompt</p>
        <textarea
          class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm
                 text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent
                 resize-y min-h-24"
          placeholder={SYSTEM_PROMPT_PLACEHOLDER}
          bind:value={form.system_prompt}
        ></textarea>

        <!-- Prompt preview (only when editing an existing role) -->
        {#if editingId}
          <div class="mt-2 flex flex-col gap-2">
            <div class="flex items-center gap-2">
              <input
                class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-1.5 text-xs
                       text-gray-300 font-mono focus:outline-none focus:border-accent"
                placeholder={PREVIEW_VARS_PLACEHOLDER}
                bind:value={previewVarsRaw}
              />
              <button
                type="button"
                class="px-3 py-1.5 text-xs bg-surface-600 hover:bg-surface-500 text-gray-300
                       rounded transition-colors"
                onclick={runPreview}
                disabled={previewing}
              >{previewing ? 'Rendering…' : 'Preview'}</button>
            </div>
            {#if previewResult}
              <pre class="text-xs text-gray-300 bg-surface-900 rounded p-2 overflow-auto max-h-40
                          whitespace-pre-wrap">{previewResult}</pre>
            {/if}
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

  <!-- Role list -->
  {#if loading}
    <Skeleton rows={3} />
  {:else if roles.length === 0}
    <p class="text-gray-400 text-sm mb-8">
      No role definitions yet. Add one above or
      <button class="underline hover:text-gray-300" onclick={runSeed}>import from config</button>.
    </p>
  {:else}
    <div class="flex flex-col gap-3">
      {#each roles as role (role.id)}
        <div class="p-4 bg-surface-800 rounded border border-surface-600 hover:border-surface-500 transition-colors">
          <div class="flex items-start justify-between gap-4">
            <!-- Info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap mb-1">
                <span class="font-medium text-gray-100">{role.label || role.name}</span>
                <span class="text-xs font-mono text-gray-500">{role.name}</span>
                {#if !role.enabled}
                  <span class="text-xs px-1.5 py-0.5 rounded bg-surface-700 text-gray-500">disabled</span>
                {/if}
              </div>

              {#if role.description}
                <p class="text-xs text-gray-400 mb-2">{role.description}</p>
              {/if}

              <!-- Provider / model -->
              <div class="flex items-center gap-3 mb-2 flex-wrap">
                {#if role.provider_id}
                  <span class="text-xs text-gray-400">
                    <span class="text-gray-600">provider:</span> {providerLabel(role.provider_id)}
                  </span>
                {/if}
                {#if role.model_override}
                  <span class="text-xs font-mono text-gray-400">
                    <span class="text-gray-600">model:</span> {role.model_override}
                  </span>
                {/if}
                <span class="text-xs text-gray-600">
                  temp: {role.temperature} · max_tokens: {role.max_tokens}
                </span>
              </div>

              <!-- Capability pills -->
              {#if role.capabilities && role.capabilities.length > 0}
                <div class="flex flex-wrap gap-1">
                  {#each role.capabilities as cap}
                    <span class="text-xs px-1.5 py-0.5 rounded bg-accent/20 text-accent/80 font-mono">
                      {cap}
                    </span>
                  {/each}
                </div>
              {/if}
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-3 shrink-0">
              <label class="flex items-center gap-1.5 cursor-pointer text-xs text-gray-400 hover:text-gray-200">
                <input
                  type="checkbox"
                  checked={role.enabled}
                  onchange={() => toggleEnabled(role)}
                  class="accent-accent"
                />
                {role.enabled ? 'Enabled' : 'Disabled'}
              </label>
              <button
                class="text-xs text-gray-400 hover:text-gray-200 transition-colors"
                onclick={() => startEdit(role)}
              >Edit</button>
              <button
                class="text-xs text-red-400 hover:text-red-300 transition-colors"
                onclick={() => remove(role.id)}
              >Delete</button>
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
