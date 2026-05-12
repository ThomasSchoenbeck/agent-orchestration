<script>
  import { onMount } from 'svelte'
  import { listProjects, createProject, deleteProject } from '../lib/api.js'
  import { router, toasts } from '../lib/stores.js'

  // ── Svelte 5 runes ────────────────────────────────────────────────────────
  let projects = $state([])
  let loading  = $state(false)
  let showForm = $state(false)
  let form     = $state({ name: '', description: '', repo_path: '', git_url: '' })

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
      await createProject({
        name:        form.name.trim(),
        description: form.description.trim(),
        repo_path:   form.repo_path.trim(),
        git_url:     form.git_url.trim(),
      })
      toasts.success('Project created')
      form     = { name: '', description: '', repo_path: '', git_url: '' }
      showForm = false
      await load()
    } catch (e) {
      toasts.error('Create failed: ' + e.message)
    }
  }

  // ── Delete ────────────────────────────────────────────────────────────────
  async function remove(e, id) {
    e.stopPropagation()
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
      <div class="grid grid-cols-2 gap-3">
        <input
          class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-200
                 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="Local path (optional)"
          bind:value={form.repo_path}
        />
        <input
          class="bg-surface-700 border border-surface-500 rounded px-3 py-2 text-sm text-gray-200
                 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="Git remote URL (optional)"
          bind:value={form.git_url}
        />
      </div>
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
        <!-- Card: clickable area + separate delete button, no nested <button> -->
        <div class="flex items-stretch bg-surface-800 rounded border border-surface-600
                    hover:border-accent transition-colors">
          <!-- Clickable main area -->
          <div
            role="button"
            tabindex="0"
            class="flex-1 p-4 cursor-pointer min-w-0"
            onclick={() => router.push('projects', p.id)}
            onkeydown={(e) => e.key === 'Enter' && router.push('projects', p.id)}
          >
            <div class="flex items-center gap-2 flex-wrap mb-1">
              <span class="font-medium text-gray-100">{p.name}</span>
              <span class="text-xs px-1.5 py-0.5 rounded bg-surface-700 text-gray-400">
                {p.status}
              </span>
            </div>
            {#if p.description}
              <div class="text-sm text-gray-400 line-clamp-2">{p.description}</div>
            {/if}
            <div class="flex gap-3 mt-2 flex-wrap text-xs text-gray-600 font-mono">
              {#if p.repo_path}<span>📁 {p.repo_path}</span>{/if}
              {#if p.git_url}<span>🔗 {p.git_url}</span>{/if}
              <span>{p.id}</span>
            </div>
          </div>
          <!-- Delete button: sibling, not child, of the clickable div -->
          <div class="flex items-center px-4 border-l border-surface-600">
            <button
              class="text-xs text-red-400 hover:text-red-300 transition-colors"
              onclick={(e) => remove(e, p.id)}
            >Delete</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
