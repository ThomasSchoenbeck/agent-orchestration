<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorState }        from '@codemirror/state'
  import { EditorView, keymap, lineNumbers, highlightActiveLine } from '@codemirror/view'
  import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
  import { oneDark }            from '@codemirror/theme-one-dark'
  import { javascript }         from '@codemirror/lang-javascript'
  import { go }                 from '@codemirror/lang-go'
  import { python }             from '@codemirror/lang-python'
  import { css }                from '@codemirror/lang-css'
  import { html }               from '@codemirror/lang-html'
  import { json }               from '@codemirror/lang-json'
  import { markdown }           from '@codemirror/lang-markdown'
  import { readFile, commitFile } from '../lib/api.js'

  let { projectId, ref = 'main', path = null, readonly = false, onStage = null } = $props()

  let container    = $state(null)
  let view         = null
  let originalText = ''
  let isDirty      = $state(false)
  let isBinary     = $state(false)
  let isNew        = $state(false)   // true when file doesn't exist yet (404)
  let loading      = $state(false)
  let error        = $state(null)
  let showCommit   = $state(false)
  let commitMsg    = $state('')
  let saving       = $state(false)

  // Derive language extension from file extension.
  function langExt(filePath) {
    if (!filePath) return []
    const ext = filePath.split('.').pop().toLowerCase()
    switch (ext) {
      case 'js': case 'ts': case 'jsx': case 'tsx': case 'mjs': case 'cjs':
        return [javascript({ typescript: ext === 'ts' || ext === 'tsx' })]
      case 'go':   return [go()]
      case 'py':   return [python()]
      case 'css':  return [css()]
      case 'html': return [html()]
      case 'json': return [json()]
      case 'md':   return [markdown()]
      default:     return []
    }
  }

  function buildState(content) {
    const extensions = [
      lineNumbers(),
      highlightActiveLine(),
      history(),
      keymap.of([...defaultKeymap, ...historyKeymap]),
      oneDark,
      ...langExt(path),
      EditorView.updateListener.of(update => {
        if (update.docChanged) {
          const current = view.state.doc.toString()
          isDirty = isNew ? current.length > 0 : current !== originalText
        }
      }),
    ]
    if (readonly) {
      extensions.push(EditorState.readOnly.of(true))
    }
    return EditorState.create({ doc: content, extensions })
  }

  async function loadFile() {
    if (!path || !container) return
    loading    = true
    error      = null
    isBinary   = false
    isDirty    = false
    isNew      = false
    showCommit = false
    try {
      const resp = await readFile(projectId, ref, path)
      if (resp.binary) {
        isBinary = true
        return
      }
      originalText = resp.content ?? ''
      const state = buildState(originalText)
      if (view) {
        view.setState(state)
      } else {
        view = new EditorView({ state, parent: container })
      }
    } catch (e) {
      // Treat 404 as a new empty file rather than an error.
      if (e.message?.includes('404') || e.status === 404) {
        isNew        = true
        originalText = ''
        const state  = buildState('')
        if (view) {
          view.setState(state)
        } else {
          view = new EditorView({ state, parent: container })
        }
      } else {
        error = e.message
      }
    } finally {
      loading = false
    }
  }

  async function save() {
    if (!commitMsg.trim()) return
    saving = true
    try {
      await commitFile(projectId, {
        path,
        content: view.state.doc.toString(),
        branch:  ref,
        message: commitMsg.trim(),
      })
      originalText = view.state.doc.toString()
      isDirty    = false
      showCommit = false
      commitMsg  = ''
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }

  // Reload whenever path or ref changes.
  $effect(() => {
    if (path && container) loadFile()
  })

  onMount(() => {
    // container ref is now set; kick off load if path already set.
    if (path) loadFile()
  })

  onDestroy(() => { view?.destroy() })
</script>

<div class="flex flex-col h-full">
  <!-- Toolbar -->
  {#if path}
    <div class="flex items-center justify-between px-3 py-1.5 border-b border-surface-600 shrink-0 gap-2">
      <div class="flex items-center gap-2 min-w-0">
        <span class="text-xs font-mono text-gray-400 truncate">{path}</span>
        {#if isNew}
          <span class="text-xs px-1.5 py-0.5 rounded bg-green-900/40 text-green-400 shrink-0">new</span>
        {/if}
      </div>
      {#if !readonly}
        {#if showCommit}
          <div class="flex items-center gap-2">
            <input
              class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs
                     text-gray-200 focus:outline-none focus:border-accent w-56"
              placeholder="Commit message…"
              bind:value={commitMsg}
              onkeydown={(e) => e.key === 'Enter' && save()}
            />
            <button
              class="px-2 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded
                     disabled:opacity-40 transition-colors"
              disabled={!commitMsg.trim() || saving}
              onclick={save}
            >{saving ? 'Saving…' : 'Commit'}</button>
            <button
              class="px-2 py-1 bg-surface-600 hover:bg-surface-500 text-gray-300 text-xs rounded transition-colors"
              onclick={() => { showCommit = false; commitMsg = '' }}
            >Cancel</button>
          </div>
        {:else}
          <div class="flex items-center gap-1">
            {#if onStage}
              <button
                class="px-2 py-1 text-xs rounded border transition-colors
                       {isDirty
                         ? 'border-blue-500 text-blue-400 hover:bg-blue-500 hover:text-white'
                         : 'border-surface-500 text-gray-500 cursor-default'}"
                disabled={!isDirty}
                onclick={() => {
                  onStage(path, view.state.doc.toString())
                  // Mark as not dirty — staged content is now tracked by the parent.
                  originalText = view.state.doc.toString()
                  isDirty = false
                  isNew   = false
                }}
              >Stage</button>
            {/if}
            <button
              class="px-2 py-1 text-xs rounded border transition-colors
                     {isDirty
                       ? 'border-accent text-accent hover:bg-accent hover:text-white'
                       : 'border-surface-500 text-gray-500 cursor-default'}"
              disabled={!isDirty}
              onclick={() => showCommit = true}
            >Commit changes</button>
          </div>
        {/if}
      {/if}
    </div>
  {/if}

  <!-- Content area -->
  <div class="flex-1 overflow-hidden relative">
    {#if loading}
      <div class="absolute inset-0 flex items-center justify-center text-xs text-gray-500">Loading…</div>
    {:else if error}
      <div class="absolute inset-0 flex items-center justify-center text-xs text-red-400 px-4">{error}</div>
    {:else if isBinary}
      <div class="absolute inset-0 flex items-center justify-center text-xs text-gray-500">Binary file — cannot display</div>
    {:else if !path}
      <div class="absolute inset-0 flex items-center justify-center text-xs text-gray-500">Select a file to view</div>
    {/if}
    <!-- CodeMirror mounts here -->
    <div
      bind:this={container}
      class="h-full overflow-auto text-sm {(loading || error || isBinary || !path) ? 'hidden' : ''}"
    ></div>
  </div>
</div>

<style>
  /* Let CodeMirror fill the container height */
  :global(.cm-editor) {
    height: 100%;
  }
  :global(.cm-scroller) {
    overflow: auto;
  }
</style>
