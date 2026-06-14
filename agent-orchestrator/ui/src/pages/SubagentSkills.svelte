<script>
  import { onMount } from 'svelte'
  import { toasts } from '../lib/stores.js'
  import {
    listSubagentSkills, createSubagentSkill, updateSubagentSkill,
    deleteSubagentSkill, seedSubagentSkills, getMetaTools,
  } from '../lib/api.js'
  import MultiSelect from '../components/MultiSelect.svelte'

  let skills = $state([])
  let availTools = $state([])
  let loading = $state(true)
  let editing = $state(null) // skill id being edited, or 'new'
  let buf = $state(emptyBuf())

  function emptyBuf() {
    return {
      name: '', label: '', description: '', prompt_template: '',
      tool_allowlist: [], context_include: '', context_exclude: '',
      max_rounds: 8, enabled: true,
    }
  }
  const joinTags  = (a) => (Array.isArray(a) ? a.join(' ') : '')
  const splitTags = (s) => (s || '').split(/\s+/).map(x => x.trim()).filter(Boolean)

  async function load() {
    loading = true
    try {
      const [sk, ts] = await Promise.all([listSubagentSkills(), getMetaTools().catch(() => [])])
      skills = sk || []
      // run_subagent can never be granted to a subagent (no nesting). Handle both
      // string and {value,label} catalog shapes.
      availTools = (Array.isArray(ts) ? ts : []).filter(t => (t && t.value !== undefined ? t.value : t) !== 'run_subagent')
    }
    catch (e) { toasts.error('Failed to load subagent skills: ' + e.message) }
    finally { loading = false }
  }
  onMount(load)

  function startNew() { editing = 'new'; buf = emptyBuf() }
  function startEdit(s) {
    editing = s.id
    buf = {
      name: s.name, label: s.label, description: s.description,
      prompt_template: s.prompt_template,
      tool_allowlist: Array.isArray(s.tool_allowlist) ? [...s.tool_allowlist] : [],
      context_include: joinTags(s.context_include),
      context_exclude: joinTags(s.context_exclude),
      max_rounds: s.max_rounds || 8,
      enabled: s.enabled,
    }
  }
  function cancel() { editing = null; buf = emptyBuf() }

  async function save() {
    if (!buf.name.trim()) { toasts.error('Name is required'); return }
    const payload = {
      name: buf.name.trim(), label: buf.label, description: buf.description,
      prompt_template: buf.prompt_template,
      tool_allowlist: buf.tool_allowlist,
      context_include: splitTags(buf.context_include),
      context_exclude: splitTags(buf.context_exclude),
      max_rounds: Number(buf.max_rounds) || 8,
      enabled: buf.enabled,
    }
    try {
      if (editing === 'new') await createSubagentSkill(payload)
      else await updateSubagentSkill(editing, payload)
      toasts.success('Subagent skill saved')
      cancel()
      await load()
    } catch (e) { toasts.error('Save failed: ' + e.message) }
  }

  async function remove(s) {
    if (!confirm(`Delete subagent skill "${s.name}"?`)) return
    try { await deleteSubagentSkill(s.id); await load() }
    catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  async function seed() {
    try { const r = await seedSubagentSkills(); toasts.success(`Seeded ${r.seeded} subagent skill(s)`); await load() }
    catch (e) { toasts.error('Seed failed: ' + e.message) }
  }
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center justify-between mb-5">
    <div>
      <h1 class="text-xl font-semibold text-gray-100">Subagent Skills</h1>
      <p class="text-sm text-gray-500">Spawnable units of work an agent can delegate to via <span class="font-mono">run_subagent</span>. The subagent's context stays isolated; only its summary returns.</p>
    </div>
    <div class="flex gap-2">
      <button class="px-3 py-1.5 text-sm rounded border border-surface-500 text-gray-300 hover:bg-surface-700" onclick={seed}>Seed starter set</button>
      <button class="px-4 py-1.5 text-sm rounded bg-accent hover:bg-accent-hover text-white" onclick={startNew}>New subagent skill</button>
    </div>
  </div>

  {#if editing}
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3">
      <div class="flex gap-3">
        <input class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="name (slug, e.g. investigate_codebase)" bind:value={buf.name} />
        <input class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="Label" bind:value={buf.label} />
        <label class="flex items-center gap-2 text-sm text-gray-300"><input type="checkbox" bind:checked={buf.enabled} /> Enabled</label>
      </div>
      <input class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="Description" bind:value={buf.description} />
      <div>
        <label class="text-xs text-gray-500 mb-1 block">Prompt template — use <span class="font-mono">{'{{instructions}}'}</span> where the main agent's ask is injected; end by asking for a concise summary</label>
        <textarea rows="5" class="w-full bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm font-mono" bind:value={buf.prompt_template}></textarea>
      </div>
      <div class="grid grid-cols-4 gap-3">
        <div class="col-span-2"><MultiSelect bind:value={buf.tool_allowlist} options={availTools} placeholder="tool allowlist" /></div>
        <input class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="context include (space-separated)" bind:value={buf.context_include} />
        <input class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="context exclude" bind:value={buf.context_exclude} />
      </div>
      <div class="flex items-center gap-2">
        <label class="text-xs text-gray-500">Max rounds</label>
        <input type="number" min="1" class="w-24 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" bind:value={buf.max_rounds} />
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
    <p class="text-sm text-gray-400">No subagent skills yet. Create one or seed the starter set.</p>
  {:else}
    <div class="flex flex-col gap-2">
      {#each skills as s (s.id)}
        <div class="p-4 bg-surface-800 rounded border border-surface-600 flex items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex items-center gap-2 mb-1">
              <span class="text-sm font-medium text-gray-200">{s.label || s.name}</span>
              <span class="text-xs text-gray-500 font-mono">{s.name}</span>
              {#if !s.enabled}<span class="text-[10px] px-1.5 py-0.5 rounded bg-gray-700 text-gray-400">disabled</span>{/if}
              <span class="text-[10px] px-1.5 py-0.5 rounded bg-surface-700 text-gray-400">{s.max_rounds} rounds</span>
            </div>
            {#if s.description}<p class="text-xs text-gray-400 mb-1">{s.description}</p>{/if}
            {#if Array.isArray(s.tool_allowlist) && s.tool_allowlist.length}
              <p class="text-xs text-gray-500 font-mono">tools: {s.tool_allowlist.join(', ')}</p>
            {/if}
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
