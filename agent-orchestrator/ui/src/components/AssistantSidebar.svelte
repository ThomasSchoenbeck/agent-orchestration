<script>
  import { onMount } from 'svelte'
  import { toasts } from '../lib/stores.js'
  import { projectChat, listProviders } from '../lib/api.js'

  let { projectId, onApplyToDescription = null } = $props()

  // ── State ─────────────────────────────────────────────────────────────────
  let messages = $state([])
  let input = $state('')
  let sending = $state(false)
  let providers = $state([])
  let selectedProvider = $state('')
  let conversationId = $state(null)
  let chatEl = $state(null)
  let loading = $state(true)

  // ── Load providers ─────────────────────────────────────────────────────────
  async function loadProviders() {
    try {
      const provs = await listProviders()
      providers = Array.isArray(provs) ? provs.filter(p => p.enabled) : []
      if (providers.length > 0) {
        selectedProvider = providers[0].id
      }
    } catch (e) {
      toasts.error('Failed to load providers: ' + e.message)
    } finally {
      loading = false
    }
  }

  // ── Send message ───────────────────────────────────────────────────────────
  async function send() {
    const text = input.trim()
    if (!text || sending) return

    input = ''
    sending = true
    messages.push({ role: 'user', content: text, isUser: true })
    scrollBottom()

    try {
      const resp = await projectChat(projectId, {
        message: text,
        provider_id: selectedProvider,
        conversation_id: conversationId,
      })

      messages.push({ role: 'assistant', content: resp.reply, isUser: false, canApply: true })
      conversationId = resp.conversation_id
      scrollBottom()
    } catch (e) {
      toasts.error('Chat failed: ' + e.message)
      // Remove the user message if request failed
      messages = messages.filter(m => m.content !== text)
    } finally {
      sending = false
    }
  }

  function handleKey(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  function applyToDescription(content) {
    if (onApplyToDescription) {
      onApplyToDescription(content)
      toasts.success('Applied to description')
    }
  }

  async function scrollBottom() {
    await new Promise(r => setTimeout(r, 0))
    if (chatEl) chatEl.scrollTop = chatEl.scrollHeight
  }

  onMount(loadProviders)
</script>

<div class="w-96 shrink-0 bg-surface-800 border-l border-surface-600 flex flex-col overflow-hidden">
  <!-- Header -->
  <div class="shrink-0 px-4 py-3 border-b border-surface-600">
    <h3 class="text-sm font-semibold text-gray-200 mb-2">Project Assistant</h3>
    {#if providers.length > 0}
      <select
        class="w-full bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs text-gray-300
               focus:outline-none focus:border-accent"
        bind:value={selectedProvider}
      >
        {#each providers as p}
          <option value={p.id}>{p.name}</option>
        {/each}
      </select>
    {:else}
      <p class="text-xs text-gray-500">No providers available</p>
    {/if}
  </div>

  {#if loading}
    <div class="flex-1 flex items-center justify-center text-gray-500 text-xs">Loading…</div>
  {:else if providers.length === 0}
    <div class="flex-1 flex items-center justify-center text-gray-500 text-xs p-4 text-center">
      No LLM providers configured
    </div>
  {:else}
    <!-- Messages -->
    <div
      class="flex-1 overflow-y-auto p-4 flex flex-col gap-3"
      bind:this={chatEl}
      role="log"
      aria-live="polite"
    >
      {#if messages.length === 0}
        <p class="text-gray-500 text-xs text-center mt-6">
          Ask the assistant for help with this project.
        </p>
      {:else}
        {#each messages as m (m.content + Math.random())}
          <div class="flex {m.isUser ? 'justify-end' : 'justify-start'}">
            <div class="max-w-[90%]">
              <div class="px-3 py-2 rounded text-xs leading-relaxed whitespace-pre-wrap
                {m.isUser
                  ? 'bg-accent text-white rounded-br-none'
                  : 'bg-surface-700 text-gray-200 rounded-bl-none'}"
              >
                {m.content}
              </div>
              {#if m.canApply}
                <button
                  class="text-xs text-accent hover:text-accent-hover transition-colors mt-1"
                  onclick={() => applyToDescription(m.content)}
                >
                  ↓ Apply to description
                </button>
              {/if}
            </div>
          </div>
        {/each}
        {#if sending}
          <div class="flex justify-start">
            <div class="px-3 py-2 bg-surface-700 rounded text-xs text-gray-400 rounded-bl-none">…</div>
          </div>
        {/if}
      {/if}
    </div>

    <!-- Input -->
    <div class="shrink-0 px-4 py-3 border-t border-surface-600">
      <form class="flex gap-2" onsubmit={(e) => { e.preventDefault(); send() }}>
        <textarea
          class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs text-gray-200
                 placeholder-gray-500 focus:outline-none focus:border-accent resize-none leading-relaxed"
          placeholder="Ask a question…"
          rows="2"
          bind:value={input}
          onkeydown={handleKey}
          disabled={sending || providers.length === 0}
        ></textarea>
        <button
          type="submit"
          class="px-2 py-1.5 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors
                 disabled:opacity-40 disabled:cursor-not-allowed shrink-0"
          disabled={sending || !input.trim() || providers.length === 0}
        >
          ↑
        </button>
      </form>
    </div>
  {/if}
</div>
