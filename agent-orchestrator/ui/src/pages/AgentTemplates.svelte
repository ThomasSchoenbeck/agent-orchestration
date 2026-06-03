<script>
  import { onMount, onDestroy } from 'svelte'
  import { toasts } from '../lib/stores.js'
  import {
    listAgentTemplates, createAgentTemplate, updateAgentTemplate, deleteAgentTemplate,
    scaleTemplate, startTemplate, stopTemplate, listAgents,
  } from '../lib/api.js'

  let templates = $state([])
  let agents    = $state([])
  let loading   = $state(true)
  let creating  = $state(false)
  let buf       = $state(emptyBuf())

  function emptyBuf() {
    return { name: '', roles: 'worker', skills: '', replicas: 1, autostart: false }
  }
  const splitCsv = (s) => (s || '').split(/[\s,]+/).map(x => x.trim()).filter(Boolean)

  async function load() {
    loading = true
    try {
      const [tpls, ags] = await Promise.all([listAgentTemplates(), listAgents()])
      templates = tpls || []
      agents = Array.isArray(ags) ? ags : (ags?.agents ?? [])
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
    return agents.filter(a => a.template_id === id)
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
  async function toggleAutostart(t) {
    try { await updateAgentTemplate(t.id, { ...t, autostart: !t.autostart }); await load() }
    catch (e) { toasts.error('Update failed: ' + e.message) }
  }
  async function remove(t) {
    if (!confirm(`Delete template "${t.name}"? All its instances will be stopped.`)) return
    try { await deleteAgentTemplate(t.id); await load() }
    catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  const dot = { online: 'bg-green-400', offline: 'bg-gray-500', starting: 'bg-yellow-400', stopping: 'bg-orange-400', failed: 'bg-red-500' }
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center justify-between mb-5">
    <div>
      <h1 class="text-xl font-semibold text-gray-100">Managed Agents</h1>
      <p class="text-sm text-gray-500">Define an agent template and run N co-located instances managed by the server.</p>
    </div>
    <button class="px-4 py-1.5 text-sm rounded bg-accent hover:bg-accent-hover text-white" onclick={() => { creating = true; buf = emptyBuf() }}>New template</button>
  </div>

  {#if creating}
    <div class="mb-6 p-5 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3">
      <div class="flex gap-3 flex-wrap">
        <input class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="Template name" bind:value={buf.name} />
        <input class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="roles (comma-separated)" bind:value={buf.roles} />
        <input class="flex-1 bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm" placeholder="skills (comma-separated)" bind:value={buf.skills} />
      </div>
      <div class="flex items-center gap-4">
        <label class="flex items-center gap-2 text-xs text-gray-400">Replicas
          <input type="number" min="0" class="w-20 bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm" bind:value={buf.replicas} />
        </label>
        <label class="flex items-center gap-2 text-sm text-gray-300"><input type="checkbox" bind:checked={buf.autostart} /> Autostart on server boot</label>
        <div class="ml-auto flex gap-2">
          <button class="px-3 py-1.5 text-sm text-gray-400 hover:text-gray-200" onclick={() => creating = false}>Cancel</button>
          <button class="px-4 py-1.5 text-sm rounded bg-accent hover:bg-accent-hover text-white" onclick={create}>Create</button>
        </div>
      </div>
    </div>
  {/if}

  {#if loading}
    <p class="text-sm text-gray-400">Loading…</p>
  {:else if templates.length === 0}
    <p class="text-sm text-gray-400">No templates yet.</p>
  {:else}
    <div class="flex flex-col gap-3">
      {#each templates as t (t.id)}
        {@const insts = instancesOf(t.id)}
        <div class="p-4 bg-surface-800 rounded border border-surface-600">
          <div class="flex items-center gap-3 flex-wrap">
            <span class="text-sm font-medium text-gray-100">{t.name}</span>
            {#each (t.roles ?? []) as r}<span class="text-xs px-1.5 py-0.5 rounded-full bg-green-900 text-green-300">{r}</span>{/each}
            {#each (t.skills ?? []) as sk}<span class="text-xs px-1.5 py-0.5 rounded-full bg-teal-900 text-teal-300">{sk}</span>{/each}
            {#if t.autostart}<span class="text-[10px] px-1.5 py-0.5 rounded-full bg-blue-900 text-blue-300">autostart</span>{/if}

            <div class="ml-auto flex items-center gap-2">
              <span class="text-xs text-gray-500">Replicas</span>
              <button class="w-6 h-6 rounded bg-surface-700 text-gray-300 hover:bg-surface-600" onclick={() => scale(t, -1)}>−</button>
              <span class="text-sm text-gray-200 w-6 text-center">{t.replicas}</span>
              <button class="w-6 h-6 rounded bg-surface-700 text-gray-300 hover:bg-surface-600" onclick={() => scale(t, 1)}>+</button>
            </div>
          </div>

          <div class="flex items-center gap-2 mt-3">
            <button class="text-xs px-2 py-0.5 rounded bg-green-900 text-green-200 hover:bg-green-800" onclick={() => start(t)}>Start</button>
            <button class="text-xs px-2 py-0.5 rounded bg-rose-900 text-rose-200 hover:bg-rose-800" onclick={() => stop(t)}>Stop all</button>
            <button class="text-xs text-blue-400 hover:text-blue-300" onclick={() => toggleAutostart(t)}>{t.autostart ? 'Disable autostart' : 'Enable autostart'}</button>
            <button class="text-xs text-red-400 hover:text-red-300 ml-auto" onclick={() => remove(t)}>Delete</button>
          </div>

          {#if insts.length > 0}
            <div class="mt-3 border-t border-surface-700 pt-2 flex flex-col gap-1">
              {#each insts as a (a.id)}
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-2 h-2 rounded-full {dot[a.status] || 'bg-gray-500'}"></span>
                  <span class="font-mono text-gray-300">{a.name}</span>
                  <span class="text-gray-500 capitalize">{a.status}</span>
                  {#if a.desired_state === 'stop'}<span class="text-rose-400">· stopping</span>{/if}
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
