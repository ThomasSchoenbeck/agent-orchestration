<script>
  import { onDestroy, tick } from 'svelte'
  import { createChatSocket } from '../lib/ws.js'
  import { toasts } from '../lib/stores.js'

  // ── Svelte 5 runes ────────────────────────────────────────────────────────
  let messages  = $state([])   // { role: 'user'|'assistant'|'error', content: string }
  let input     = $state('')
  let connected = $state(false)
  let sending   = $state(false)
  let chatEl    = $state(null)

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
    },
    onClose() {
      connected = false
    },
    onError() {
      sending = false
      toasts.error('WebSocket error')
    },
  })

  // ── Send a message ────────────────────────────────────────────────────────
  async function send() {
    const text = input.trim()
    if (!text || sending) return
    input   = ''
    sending = true
    messages.push({ role: 'user', content: text })
    scrollBottom()
    sock.send({ role: 'user', content: text })
  }

  function handleKey(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  async function scrollBottom() {
    await tick()
    if (chatEl) chatEl.scrollTop = chatEl.scrollHeight
  }

  onDestroy(() => sock.close())
</script>

<div class="flex-1 overflow-hidden flex flex-col">
  <!-- Header -->
  <div class="shrink-0 px-6 py-3 border-b border-surface-600 flex items-center gap-2">
    <h1 class="text-xl font-semibold text-gray-100">Chat</h1>
    <span class="ml-auto flex items-center gap-1.5 text-xs {connected ? 'text-green-400' : 'text-gray-500'}">
      <span class="w-1.5 h-1.5 rounded-full {connected ? 'bg-green-400' : 'bg-gray-500'}"></span>
      {connected ? 'Connected' : 'Reconnecting…'}
    </span>
  </div>

  <!-- Messages -->
  <div
    class="flex-1 overflow-y-auto p-6 flex flex-col gap-4"
    bind:this={chatEl}
    role="log"
    aria-live="polite"
  >
    {#if messages.length === 0}
      <p class="text-gray-500 text-sm text-center mt-12">
        Send a message to start a conversation with the orchestrator.
      </p>
    {:else}
      {#each messages as m, i (i)}
        <div class="flex {m.role === 'user' ? 'justify-end' : 'justify-start'}">
          <div class="max-w-[75%] px-4 py-2.5 rounded-2xl text-sm leading-relaxed whitespace-pre-wrap
            {m.role === 'user'
              ? 'bg-accent text-white rounded-br-sm'
              : m.role === 'error'
                ? 'bg-red-900 text-red-200 rounded-bl-sm'
                : 'bg-surface-700 text-gray-200 rounded-bl-sm'}"
          >
            {m.content}
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
  <div class="shrink-0 px-6 py-4 border-t border-surface-600">
    <form class="flex gap-3" onsubmit={(e) => { e.preventDefault(); send() }}>
      <textarea
        class="flex-1 bg-surface-700 border border-surface-500 rounded-xl px-4 py-2.5 text-sm text-gray-200
               placeholder-gray-500 focus:outline-none focus:border-accent resize-none leading-relaxed"
        placeholder="Message the orchestrator…"
        rows="1"
        bind:value={input}
        onkeydown={handleKey}
        disabled={!connected}
      ></textarea>
      <button
        type="submit"
        class="px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm rounded-xl transition-colors
               disabled:opacity-40 disabled:cursor-not-allowed"
        disabled={!connected || sending || !input.trim()}
      >
        Send
      </button>
    </form>
    <p class="text-xs text-gray-600 mt-1.5">Enter to send · Shift+Enter for newline</p>
  </div>
</div>
