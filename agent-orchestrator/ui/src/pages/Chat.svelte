<script>
  import { onDestroy, onMount, tick } from 'svelte'
  import { marked } from 'marked'
  import DOMPurify from 'dompurify'
  import { createChatSocket } from '../lib/ws.js'
  import { toasts } from '../lib/stores.js'
  import {
    listConversations, getConversation, createConversation,
    updateConversation, deleteConversation, addMessage,
    listProviders,
  } from '../lib/api.js'
  import MarkdownEditor from '../components/MarkdownEditor.svelte'

  // ── State ─────────────────────────────────────────────────────────────────
  let conversations  = $state([])
  let providers       = $state([])
  let activeConvID   = $state(null)
  let activeConv     = $state(null)
  let messages       = $state([])
  let input          = $state('')
  let connected      = $state(false)
  let sending        = $state(false)
  let loading        = $state(true)
  let selectedProvider = $state('')
  let chatEl         = $state(null)
  let showNewForm    = $state(false)
  let newConvTitle   = $state('')
  let renamingID     = $state(null)
  let renameTitle    = $state('')

  // ── WebSocket ────────────────────────────────────────────────────────────
  const sock = createChatSocket({
    onOpen() {
      connected = true
    },
    onMessage(data) {
      sending = false
      const content = typeof data === 'string' ? data : (data.content ?? JSON.stringify(data))
      messages.push({ role: 'assistant', content })
      scrollBottom()
      // Save assistant message to DB
      if (activeConvID) {
        addMessage(activeConvID, { role: 'assistant', content })
          .catch(e => console.error('Failed to save message:', e))
      }
    },
    onClose() {
      connected = false
    },
    onError() {
      sending = false
      toasts.error('WebSocket error')
    },
  })

  // ── Load initial data ─────────────────────────────────────────────────────
  async function loadAll() {
    loading = true
    try {
      const [convs, provs] = await Promise.all([
        listConversations({ limit: 100 }),
        listProviders(),
      ])
      conversations = Array.isArray(convs) ? convs : []
      providers = Array.isArray(provs) ? provs : []
      if (providers.length > 0) {
        selectedProvider = providers[0].id
      }
      // Load first conversation if any
      if (conversations.length > 0) {
        await selectConversation(conversations[0].id)
      }
    } catch (e) {
      toasts.error('Failed to load: ' + e.message)
    } finally {
      loading = false
    }
  }

  // ── Conversation actions ──────────────────────────────────────────────────
  async function createNew() {
    if (!newConvTitle.trim()) {
      toasts.error('Title is required')
      return
    }
    try {
      const conv = await createConversation({
        title: newConvTitle.trim(),
        provider_id: selectedProvider,
      })
      conversations.unshift(conv)
      newConvTitle = ''
      showNewForm = false
      await selectConversation(conv.id)
      toasts.success('Conversation created')
    } catch (e) {
      toasts.error('Create failed: ' + e.message)
    }
  }

  async function selectConversation(id) {
    try {
      const resp = await getConversation(id, 50)
      activeConvID = id
      activeConv = resp.conversation
      messages = Array.isArray(resp.messages) ? resp.messages : []
      if (activeConv?.provider_id) {
        selectedProvider = activeConv.provider_id
      }
      scrollBottom()
    } catch (e) {
      toasts.error('Failed to load conversation: ' + e.message)
    }
  }

  async function startRename(id, title) {
    renamingID = id
    renameTitle = title
  }

  async function saveRename() {
    if (!renameTitle.trim()) {
      toasts.error('Title is required')
      return
    }
    try {
      const updated = await updateConversation(renamingID, { title: renameTitle.trim() })
      const idx = conversations.findIndex(c => c.id === renamingID)
      if (idx !== -1) {
        conversations[idx] = updated
      }
      if (activeConvID === renamingID) {
        activeConv = updated
      }
      renamingID = null
      renameTitle = ''
      toasts.success('Renamed')
    } catch (e) {
      toasts.error('Rename failed: ' + e.message)
    }
  }

  async function handleDelete(id) {
    if (!confirm('Delete this conversation?')) return
    try {
      await deleteConversation(id)
      conversations = conversations.filter(c => c.id !== id)
      if (activeConvID === id) {
        activeConvID = null
        activeConv = null
        messages = []
      }
      toasts.success('Conversation deleted')
    } catch (e) {
      toasts.error('Delete failed: ' + e.message)
    }
  }

  async function changeProvider() {
    if (!activeConv) return
    try {
      await updateConversation(activeConvID, { provider_id: selectedProvider })
      activeConv.provider_id = selectedProvider
      toasts.success('Provider updated')
    } catch (e) {
      toasts.error('Update failed: ' + e.message)
    }
  }

  // ── Send message ──────────────────────────────────────────────────────────
  async function send() {
    const text = input.trim()
    if (!text || sending || !activeConvID) return
    input = ''
    sending = true
    messages.push({ role: 'user', content: text })
    scrollBottom()

    // Save user message to DB
    try {
      await addMessage(activeConvID, { role: 'user', content: text })
    } catch (e) {
      console.error('Failed to save message:', e)
    }

    sock.send({
      type: 'chat',
      role: 'user',
      content: text,
      conversation_id: activeConvID,
      provider_id: selectedProvider,
    })
  }

  function handleKey(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  // Render markdown content to HTML
  function renderMarkdown(content) {
    return DOMPurify.sanitize(
      marked.parse(content || '', { breaks: true, gfm: true })
    )
  }

  async function scrollBottom() {
    await tick()
    if (chatEl) chatEl.scrollTop = chatEl.scrollHeight
  }

  onMount(loadAll)
  onDestroy(() => sock.close())
</script>

<style>
  /* Prose styles for rendered markdown in messages */
  .prose :global(h1), .prose :global(h2), .prose :global(h3) {
    color: #e5e7eb;
    font-weight: 600;
    margin: 0.75em 0 0.5em;
    line-height: 1.3;
  }
  .prose :global(h1) { font-size: 1.1rem; }
  .prose :global(h2) { font-size: 1rem; }
  .prose :global(h3) { font-size: 0.95rem; }
  .prose :global(p)  { margin: 0.35em 0; }
  .prose :global(ul), .prose :global(ol) { margin: 0.35em 0; padding-left: 1.25em; }
  .prose :global(li) { margin: 0.1em 0; }
  .prose :global(code) {
    background: rgba(0, 0, 0, 0.3);
    padding: 0.1em 0.3em;
    border-radius: 3px;
    font-size: 0.85em;
    font-family: monospace;
  }
  .prose :global(pre) {
    background: rgba(0, 0, 0, 0.4);
    padding: 0.75em 1em;
    border-radius: 6px;
    overflow-x: auto;
    margin: 0.35em 0;
  }
  .prose :global(pre code) { background: none; padding: 0; }
  .prose :global(blockquote) {
    border-left: 3px solid rgba(255, 255, 255, 0.2);
    padding-left: 0.75em;
    color: rgba(255, 255, 255, 0.7);
    margin: 0.35em 0;
  }
  .prose :global(a) { color: #60a5fa; text-decoration: underline; }
  .prose :global(hr) { border-color: rgba(255, 255, 255, 0.15); margin: 0.75em 0; }
  .prose :global(strong) { color: #f3f4f6; font-weight: 600; }
  .prose :global(em) { font-style: italic; color: #d1d5db; }
  .prose :global(table) { width: 100%; border-collapse: collapse; font-size: 0.85em; }
  .prose :global(th), .prose :global(td) {
    border: 1px solid rgba(255, 255, 255, 0.2);
    padding: 0.3em 0.6em;
    text-align: left;
  }
  .prose :global(th) { background: rgba(0, 0, 0, 0.2); }
</style>

<div class="flex h-full gap-0">
  <!-- ── Conversation list ────────────────────────────────────────────────── -->
  <div class="w-56 shrink-0 bg-surface-800 border-r border-surface-600 flex flex-col overflow-hidden">
    <div class="shrink-0 px-4 py-3 border-b border-surface-600">
      <button
        class="w-full px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors"
        onclick={() => { showNewForm = !showNewForm; newConvTitle = '' }}
      >
        {showNewForm ? 'Cancel' : '+ New'}
      </button>
    </div>

    {#if showNewForm}
      <div class="shrink-0 p-3 border-b border-surface-600 flex flex-col gap-2">
        <input
          class="bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs text-gray-200
                 placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="Conversation title"
          bind:value={newConvTitle}
          autofocus
        />
        <button
          class="px-3 py-1 bg-green-800 hover:bg-green-700 text-white text-xs rounded transition-colors"
          onclick={createNew}
        >Create</button>
      </div>
    {/if}

    <div class="flex-1 overflow-y-auto">
      {#if conversations.length === 0}
        <p class="p-3 text-xs text-gray-500 text-center">No conversations yet</p>
      {:else}
        {#each conversations as conv (conv.id)}
          <div
            class="px-3 py-2 border-b border-surface-700 cursor-pointer transition-colors hover:bg-surface-700
              {activeConvID === conv.id ? 'bg-surface-700' : ''}"
            onclick={() => selectConversation(conv.id)}
          >
            {#if renamingID === conv.id}
              <div class="flex gap-1.5">
                <input
                  class="flex-1 bg-surface-600 border border-surface-500 rounded px-2 py-1 text-xs text-gray-200
                         focus:outline-none focus:border-accent"
                  bind:value={renameTitle}
                  onkeydown={(e) => { if (e.key === 'Enter') saveRename(); if (e.key === 'Escape') renamingID = null }}
                  autofocus
                />
                <button
                  class="px-2 py-1 text-xs text-green-400 hover:text-green-300 transition-colors"
                  onclick={saveRename}
                >✓</button>
              </div>
            {:else}
              <div class="flex items-start justify-between gap-2 group">
                <p class="text-xs text-gray-300 flex-1 truncate">{conv.title || 'Untitled'}</p>
                <button
                  class="text-xs text-gray-600 hover:text-gray-400 transition-colors opacity-0 group-hover:opacity-100"
                  onclick={(e) => { e.stopPropagation(); startRename(conv.id, conv.title) }}
                  title="Rename"
                >✎</button>
                <button
                  class="text-xs text-red-600 hover:text-red-400 transition-colors opacity-0 group-hover:opacity-100"
                  onclick={(e) => { e.stopPropagation(); handleDelete(conv.id) }}
                  title="Delete"
                >×</button>
              </div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  </div>

  <!-- ── Chat area ────────────────────────────────────────────────────────── -->
  <div class="flex-1 flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="shrink-0 px-6 py-3 border-b border-surface-600 flex items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <h1 class="text-lg font-semibold text-gray-100">{activeConv?.title || 'Chat'}</h1>
        <span class="ml-auto flex items-center gap-1.5 text-xs {connected ? 'text-green-400' : 'text-gray-500'}">
          <span class="w-1.5 h-1.5 rounded-full {connected ? 'bg-green-400' : 'bg-gray-500'}"></span>
          {connected ? 'Connected' : 'Reconnecting…'}
        </span>
      </div>

      {#if activeConv && providers.length > 0}
        <select
          class="bg-surface-700 border border-surface-500 rounded px-3 py-1 text-xs text-gray-300
                 focus:outline-none focus:border-accent"
          bind:value={selectedProvider}
          onchange={changeProvider}
        >
          {#each providers as p}
            <option value={p.id}>{p.name}</option>
          {/each}
        </select>
      {/if}
    </div>

    {#if !activeConv}
      <div class="flex-1 flex items-center justify-center text-gray-500 text-sm">
        {loading ? 'Loading…' : 'Create a new conversation to get started'}
      </div>
    {:else}
      <!-- Messages -->
      <div
        class="flex-1 overflow-y-auto p-6 flex flex-col gap-4"
        bind:this={chatEl}
        role="log"
        aria-live="polite"
      >
        {#if messages.length === 0}
          <p class="text-gray-500 text-sm text-center mt-12">Start a conversation with the orchestrator.</p>
        {:else}
          {#each messages as m, i (i)}
            <div class="flex {m.role === 'user' ? 'justify-end' : 'justify-start'}">
              <div class="max-w-[75%] px-4 py-2.5 rounded-2xl text-sm leading-relaxed
                {m.role === 'user'
                  ? 'bg-accent text-white rounded-br-sm'
                  : m.role === 'error'
                    ? 'bg-red-900 text-red-200 rounded-bl-sm'
                    : 'bg-surface-700 text-gray-200 rounded-bl-sm'}"
              >
                {#if m.role === 'user'}
                  <!-- User messages: render as markdown -->
                  <div class="prose prose-invert prose-sm max-w-none">
                    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                    {@html renderMarkdown(m.content)}
                  </div>
                {:else}
                  <!-- Assistant/error messages: render as markdown -->
                  <div class="prose prose-invert prose-sm max-w-none">
                    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                    {@html renderMarkdown(m.content)}
                  </div>
                {/if}
              </div>
            </div>
          {/each}
          {#if sending}
            <div class="flex justify-start">
              <div class="px-4 py-2.5 bg-surface-700 rounded-2xl rounded-bl-sm">
                <span class="text-gray-400 text-sm">…</span>
              </div>
            </div>
          {/if}
        {/if}
      </div>

      <!-- Input -->
      <div class="shrink-0 px-6 py-4 border-t border-surface-600" onkeydown={handleKey}>
        <form class="flex gap-3" onsubmit={(e) => { e.preventDefault(); send() }}>
          <div class="flex-1 relative">
            <MarkdownEditor
              bind:value={input}
              placeholder="Message the orchestrator…"
              minHeight="100px"
            />
          </div>
          <button
            type="submit"
            class="self-end px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm rounded-xl transition-colors
                   disabled:opacity-40 disabled:cursor-not-allowed"
            disabled={!connected || sending || !input.trim() || !activeConv}
          >
            Send
          </button>
        </form>
        <p class="text-xs text-gray-600 mt-1.5">Supports markdown · Shift+Enter for newline</p>
      </div>
    {/if}
  </div>
</div>
