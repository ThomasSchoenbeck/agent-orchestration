<script>
  import { onMount, onDestroy } from 'svelte'
  import { toasts } from '../lib/stores.js'
  import {
    listAgentTemplates, createAgentTemplate, updateAgentTemplate, deleteAgentTemplate,
    scaleTemplate, startTemplate, stopTemplate,
  } from '../lib/api.js'

  // Agents are owned by the parent (Agents page) which already polls them.
  let { agents = [] } = $props()

  let templates  = $state([])
  let loading    = $state(true)
  let creating   = $state(false)
  let buf        = $state(emptyBuf())
  let editingId  = $state('')
  let editBuf    = $state(emptyBuf())

  function emptyBuf() {
    return { name: '', roles: 'worker', skills: '', replicas: 1, autostart: false }
  }
  const splitCsv = (s) => (s || '').split(/[\s,]+/).map(x => x.trim()).filter(Boolean)

  async function load() {
    try {
      templates = (await listAgentTemplates()) || []
    } catch (e) {
      toasts.error('Failed to load templates: ' + e.message)
    } finally {
      loading = false
    }
  }
  onMount(load)
  let timer = setInterval(load, 5_000)
  onDestroy(() => clearInterval(timer))

  function instancesOf(id) {
    return (agents || []).filter(a => a.template_id === id)
  }

  async function create() {
    if (!buf.name.trim()) { toasts.error('Name is required'); return }
    try {
      await createAgentTemplate({
        name: buf.name.trim(),
        roles: splitCsv(buf.roles),
        skills: splitCsv(buf.skills),
        replicas: Number(buf.replicas) || 1,
        autostart: buf.autostart,
      })
      toasts.success('Template created')
      creating = false
      buf = emptyBuf()
      await load()
    } catch (e) { toasts.error('Create failed: ' + e.message) }
  }

  function startEdit(t) {
    editingId = t.id
    editBuf = {
      name: t.name,
      roles: (t.roles ?? []).join(', '),
      skills: (t.skills ?? []).join(', '),
      replicas: t.replicas ?? 1,
      autostart: !!t.autostart,
    }
  }
  function cancelEdit() { editingId = '' }

  async function saveEdit(t) {
    if (!editBuf.name.trim()) { toasts.error('Name is required'); return }
    try {
      await updateAgentTemplate(t.id, {
        ...t,
        name: editBuf.name.trim(),
        roles: splitCsv(editBuf.roles),
        skills: splitCsv(editBuf.skills),
        replicas: Number(editBuf.replicas) || 0,
        autostart: editBuf.autostart,
      })
      toasts.success('Template updated')
      editingId = ''
      await load()
    } catch (e) { toasts.error('Update failed: ' + e.message) }
  }

  async function scale(t, delta) {
    const next = Math.max(0, (t.replicas || 0) + delta)
    try { await scaleTemplate(t.id, next); await load() }
    catch (e) { toasts.error('Scale failed: ' + e.message) }
  }
  async function start(t) {
    try { await startTemplate(t.id); toasts.success('Start requested'); await load() }
    catch (e) { toasts.error('Start failed: ' + e.message) }
  }
  async function stop(t) {
    try { await stopTemplate(t.id); toasts.success('Stop requested'); await load() }
    catch (e) { toasts.error('Stop failed: ' + e.message) }
  }
  async function remove(t) {
    if (!confirm(`Delete template "${t.name}"? All its instances will be stopped.`)) return
    try { await deleteAgentTemplate(t.id); await load() }
    catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  const dot = { online: 'bg-green-400', offline: 'bg-gray-500', starting: 'bg-yellow-400', stopping: 'bg-orange-400', failed: 'bg-red-500' }
</script>

<div class="p-4">
  <div class="flex items-center justify-between mb-3 sticky top-0 bg-surface-900 z-10 py-1">
    <h2 class="text-sm font-semibold text-gray-100">Templates</h2>
    <button class="text-xs px-2 py-1 rounded bg-accent hover:bg-accent-hover text-white"
      onclick={() => { creating = true; buf = emptyBuf() }}>New template</button>
  </div>

  {#if creating}
    <div class="mb-4 p-3 bg-surface-800 rounded border border-surface-600 flex flex-col gap-2">
      <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" placeholder="Template name" bind:value={buf.name} />
      <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" placeholder="roles (comma-separated)" bind:value={buf.roles} />
      <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" placeholder="skills (comma-separated)" bind:value={buf.skills} />
      <div class="flex items-center gap-3 flex-wrap">
        <label class="flex items-center gap-2 text-xs text-gray-400">Replicas
          <input type="number" min="0" class="w-16 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" bind:value={buf.replicas} />
        </label>
        <label class="flex items-center gap-2 text-xs text-gray-300"><input type="checkbox" bind:checked={buf.autostart} /> Autostart</label>
        <div class="ml-auto flex gap-2">
          <button class="px-2 py-1 text-xs text-gray-400 hover:text-gray-200" onclick={() => creating = false}>Cancel</button>
          <button class="px-3 py-1 text-xs rounded bg-accent hover:bg-accent-hover text-white" onclick={create}>Create</button>
        </div>
      </div>
    </div>
  {/if}

  {#if loading && templates.length === 0}
    <p class="text-sm text-gray-400">Loading…</p>
  {:else if templates.length === 0}
    <p class="text-sm text-gray-400">No templates yet.</p>
  {:else}
    <div class="flex flex-col gap-3">
      {#each templates as t (t.id)}
        {@const insts = instancesOf(t.id)}
        <div class="p-3 bg-surface-800 rounded border border-surface-600">
          {#if editingId === t.id}
            <!-- Edit form (Bug 7: templates were not editable) -->
            <div class="flex flex-col gap-2">
              <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" placeholder="Template name" bind:value={editBuf.name} />
              <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" placeholder="roles (comma-separated)" bind:value={editBuf.roles} />
              <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" placeholder="skills (comma-separated)" bind:value={editBuf.skills} />
              <div class="flex items-center gap-3 flex-wrap">
                <label class="flex items-center gap-2 text-xs text-gray-400">Replicas
                  <input type="number" min="0" class="w-16 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" bind:value={editBuf.replicas} />
                </label>
                <label class="flex items-center gap-2 text-xs text-gray-300"><input type="checkbox" bind:checked={editBuf.autostart} /> Autostart</label>
                <div class="ml-auto flex gap-2">
                  <button class="px-2 py-1 text-xs text-gray-400 hover:text-gray-200" onclick={cancelEdit}>Cancel</button>
                  <button class="px-3 py-1 text-xs rounded bg-accent hover:bg-accent-hover text-white" onclick={() => saveEdit(t)}>Save</button>
                </div>
              </div>
            </div>
          {:else}
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-sm font-medium text-gray-100">{t.name}</span>
              {#each (t.roles ?? []) as r}<span class="text-xs px-1.5 py-0.5 rounded-full bg-green-900 text-green-300">{r}</span>{/each}
              {#each (t.skills ?? []) as sk}<span class="text-xs px-1.5 py-0.5 rounded-full bg-teal-900 text-teal-300">{sk}</span>{/each}
              {#if t.autostart}<span class="text-[10px] px-1.5 py-0.5 rounded-full bg-blue-900 text-blue-300">autostart</span>{/if}

              <div class="ml-auto flex items-center gap-1">
                <button class="w-6 h-6 rounded bg-surface-700 text-gray-300 hover:bg-surface-600" title="Fewer replicas" onclick={() => scale(t, -1)}>−</button>
                <span class="text-sm text-gray-200 w-6 text-center">{t.replicas}</span>
                <button class="w-6 h-6 rounded bg-surface-700 text-gray-300 hover:bg-surface-600" title="More replicas" onclick={() => scale(t, 1)}>+</button>
              </div>
            </div>

            <div class="flex items-center gap-2 mt-2 flex-wrap">
              <button class="text-xs px-2 py-0.5 rounded bg-green-900 text-green-200 hover:bg-green-800" onclick={() => start(t)}>Start</button>
              <button class="text-xs px-2 py-0.5 rounded bg-rose-900 text-rose-200 hover:bg-rose-800" onclick={() => stop(t)}>Stop all</button>
              <button class="text-xs text-blue-400 hover:text-blue-300" onclick={() => startEdit(t)}>Edit</button>
              <button class="text-xs text-red-400 hover:text-red-300 ml-auto" onclick={() => remove(t)}>Delete</button>
            </div>
          {/if}

          {#if insts.length > 0}
            <div class="mt-2 border-t border-surface-700 pt-2 flex flex-col gap-1">
              {#each insts as a (a.id)}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-2 h-2 rounded-full {dot[a.status] || 'bg-gray-500'}"></span>
                  <span class="font-mono text-gray-300">{a.name}</span>
                  <span class="text-gray-500 capitalize">{a.status}</span>
                  {#if a.desired_state === 'stop' && a.status !== 'offline'}<span class="text-rose-400">· stopping</span>{/if}
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
