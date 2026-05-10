<script>
  import { onMount } from 'svelte'
  import { listProjects, createProject, deleteProject } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'

  // ── Svelte 5 runes ────────────────────────────────────────────────────────
  let projects = $state([])
  let loading  = $state(false)
  let showForm = $state(false)
  let form     = $state({ name: '', description: '' })

  // ── Data loading ──────────────────────────────────────────────────────────
  async function load() {
    loading = true
    try {
      const res = await listProjects()
      projects  = Array.isArray(res) ? res : (res.projects ?? [])
    } catch (e) {
      toasts.error('Failed to load projects: ' + e.message)
    } finally {
      loading = false
    }
  }

  // ── Create ────────────────────────────────────────────────────────────────
  async function submit() {
    if (!form.name.trim()) return
    try {
      await createProject({ name: form.name.trim(), description: form.description.trim() })
      toasts.success('Project created')
      form     = { name: '', description: '' }
      showForm = false
      await load()
    } catch (e) {
      toasts.error('Create failed: ' + e.message)
    }
  }

  // ── Delete ────────────────────────────────────────────────────────────────
  async function remove(id) {
    if (!confirm('Delete this project?')) return
    try {
      await deleteProject(id)
      toasts.success('Project deleted')
      await load()
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  onMount(load)
</script>

<div class="flex-1 overflow-y-auto p-6">
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-xl font-semibold text-gray-100">Projects</h1>
    <button
      class="px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
      onclick={() => showForm = !showForm}
    >
      {showForm ? 'Cancel' : '+ New Project'}
    </button>
  </div>

  {#if showForm}
    <form
      class="mb-6 p-4 bg-surface-800 rounded border border-surface-600 flex flex-col gap-3"
      onsubmit={(e) => { e.preventDefault(); submit() }}
    >
      <input
        class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-200
               placeholder-gray-500 focus:outline-none focus:border-accent"
        placeholder="Project name"
        bind:value={form.name}
        required
      />
      <textarea
        class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-200
               placeholder-gray-500 focus:outline-none focus:border-accent resize-none"
        placeholder="Description (optional)"
        rows="2"
        bind:value={form.description}
      ></textarea>
      <button
        type="submit"
        class="self-end px-4 py-1.5 bg-accent hover:bg-accent-hover text-white text-sm rounded transition-colors"
      >
        Create
      </button>
    </form>
  {/if}

  {#if loading}
    <p class="text-gray-500 text-sm">Loading…</p>
  {:else if projects.length === 0}
    <p class="text-gray-500 text-sm">No projects yet. Create one to get started.</p>
  {:else}
    <div class="flex flex-col gap-3">
      {#each projects as p (p.id)}
        <div class="p-4 bg-surface-800 rounded border border-surface-600 hover:border-surface-500 transition-colors">
          <div class="flex items-start justify-between gap-4">
            <div class="flex-1 min-w-0">
              <div class="font-medium text-gray-100 truncate">{p.name}</div>
              {#if p.description}
                <div class="text-sm text-gray-400 mt-1 line-clamp-2">{p.description}</div>
              {/if}
              <div class="text-xs text-gray-600 mt-2 font-mono">{p.id}</div>
            </div>
            <button
              class="text-xs text-red-400 hover:text-red-300 transition-colors shrink-0"
              onclick={() => remove(p.id)}
            >
              Delete
            </button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
