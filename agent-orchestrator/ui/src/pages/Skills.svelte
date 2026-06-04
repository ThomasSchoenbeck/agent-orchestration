<script>
  import { onMount } from 'svelte'
  import { toasts } from '../lib/stores.js'
  import { listSkills, createSkill, updateSkill, deleteSkill, seedSkills, getMetaTools } from '../lib/api.js'
  import MultiSelect from '../components/MultiSelect.svelte'

  let skills = $state([])
  let availTools = $state([])
  let loading = $state(true)
  let editing = $state(null) // skill id being edited, or 'new'
  let buf = $state(emptyBuf())

  function emptyBuf() {
    return {
      name: '', label: '', description: '', prompt_fragment: '',
      context_include: '', context_exclude: '', allowed_tools: [], enabled: true,
    }
  }
  const joinTags  = (a) => (Array.isArray(a) ? a.join(' ') : '')
  const splitTags = (s) => (s || '').split(/\s+/).map(x => x.trim()).filter(Boolean)

  async function load() {
    loading = true
    try {
      const [sk, ts] = await Promise.all([listSkills(), getMetaTools().catch(() => [])])
      skills = sk || []
      availTools = Array.isArray(ts) ? ts : []
    }
    catch (e) { toasts.error('Failed to load skills: ' + e.message) }
    finally { loading = false }
  }
  onMount(load)

  function startNew() { editing = 'new'; buf = emptyBuf() }
  function startEdit(s) {
    editing = s.id
    buf = {
      name: s.name, label: s.label, description: s.description,
      prompt_fragment: s.prompt_fragment,
      context_include: joinTags(s.context_include),
      context_exclude: joinTags(s.context_exclude),
      allowed_tools: Array.isArray(s.allowed_tools) ? [...s.allowed_tools] : [],
      enabled: s.enabled,
    }
  }
  function cancel() { editing = null; buf = emptyBuf() }

  async function save() {
    if (!buf.name.trim()) { toasts.error('Name is required'); return }
    const payload = {
      name: buf.name.trim(), label: buf.label, description: buf.description,
      prompt_fragment: buf.prompt_fragment,
      context_include: splitTags(buf.context_include),
      context_exclude: splitTags(buf.context_exclude),
      allowed_tools: buf.allowed_tools,
      enabled: buf.enabled,
    }
    try {
      if (editing === 'new') await createSkill(payload)
      else await updateSkill(editing, payload)
      toasts.success('Skill saved')
      cancel()
      await load()
    } catch (e) { toasts.error('Save failed: ' + e.message) }
  }

  async function remove(s) {
    if (!confirm(`Delete skill "${s.name}"?`)) return
    try { await deleteSkill(s.id); await load() }
    catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  async function seed() {
    try { const r = await seedSkills(); toasts.success(`Seeded ${r.seeded} skill(s)`); await load() }
    catch (e) { toasts.error('Seed failed: ' + e.message) }
  }
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center justify-between mb-5">
    <div>
      <h1 class="text-xl font-semibold text-gray-100">Skills</h1>
      <p class="text-sm text-gray-500">Specializations agents compose on top of their roles (prompt, context, tools).</p>
    </div>
    <div class="flex gap-2">
      <button class="px-3 py-1.5 text-sm rounded border border-surface-500 text-gray-300 hover:bg-surface-700" onclick={seed}>Seed starter set</button>
      <button class="px-4 py-1.5 text-sm rounded bg-accent hover:bg-accent-hover text-white" onclick={startNew}>New skill</button>
    </div>
  </div>

  {#if editing}
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3">
      <div class="flex gap-3">
        <input class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="name (slug, e.g. backend)" bind:value={buf.name} />
        <input class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="Label" bind:value={buf.label} />
        <label class="flex items-center gap-2 text-sm text-gray-300"><input type="checkbox" bind:checked={buf.enabled} /> Enabled</label>
      </div>
      <input class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="Description" bind:value={buf.description} />
      <div>
        <label class="text-xs text-gray-500 mb-1 block">Prompt fragment (the "soul" injected into the system prompt)</label>
        <textarea rows="4" class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm font-mono" bind:value={buf.prompt_fragment}></textarea>
      </div>
      <div class="grid grid-cols-3 gap-3">
        <input class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="context include (space-separated)" bind:value={buf.context_include} />
        <input class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="context exclude" bind:value={buf.context_exclude} />
        <MultiSelect bind:value={buf.allowed_tools} options={availTools} placeholder="allowed tools" />
      </div>
      <div class="flex justify-end gap-2">
        <button class="px-3 py-1.5 text-sm text-gray-400 hover:text-gray-200" onclick={cancel}>Cancel</button>
        <button class="px-4 py-1.5 text-sm rounded bg-accent hover:bg-accent-hover text-white" onclick={save}>Save</button>
      </div>
    </div>
  {/if}

  {#if loading}
    <p class="text-sm text-gray-400">Loading…</p>
  {:else if skills.length === 0}
    <p class="text-sm text-gray-400">No skills yet. Create one or seed the starter set.</p>
  {:else}
    <div class="flex flex-col gap-2">
      {#each skills as s (s.id)}
        <div class="p-4 bg-surface-800 rounded border border-surface-600 flex items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex items-center gap-2 mb-1">
              <span class="text-sm font-medium text-gray-200">{s.label || s.name}</span>
              <span class="text-xs text-gray-500 font-mono">{s.name}</span>
              {#if !s.enabled}<span class="text-[10px] px-1.5 py-0.5 rounded bg-gray-700 text-gray-400">disabled</span>{/if}
            </div>
            {#if s.description}<p class="text-xs text-gray-400 mb-1">{s.description}</p>{/if}
            {#if s.prompt_fragment}<p class="text-xs text-gray-500 font-mono line-clamp-2">{s.prompt_fragment}</p>{/if}
          </div>
          <div class="flex gap-2 shrink-0">
            <button class="text-xs text-blue-400 hover:text-blue-300" onclick={() => startEdit(s)}>Edit</button>
            <button class="text-xs text-red-400 hover:text-red-300" onclick={() => remove(s)}>Delete</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
