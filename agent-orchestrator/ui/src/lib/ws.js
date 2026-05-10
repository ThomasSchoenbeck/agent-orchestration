/**
 * ws.js — WebSocket helper for the /ws/chat endpoint.
 *
 * Usage:
 *   import { createChatSocket } from './ws.js'
 *   const sock = createChatSocket({
 *     onMessage: (msg) => { ... },
 *     onOpen:    ()    => { ... },
 *     onClose:   ()    => { ... },
 *     onError:   (e)   => { ... },
 *   })
 *   sock.send({ role: 'user', content: 'Hello' })
 *   sock.close()
 */

export function createChatSocket({ onMessage, onOpen, onClose, onError } = {}) {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const url   = `${proto}://${window.location.host}/ws/chat`

  let ws = null
  let closed = false

  function connect() {
    ws = new WebSocket(url)

    ws.onopen = () => {
      if (onOpen) onOpen()
    }

    ws.onmessage = (evt) => {
      try {
        const data = JSON.parse(evt.data)
        if (onMessage) onMessage(data)
      } catch (_) {
        if (onMessage) onMessage({ role: 'assistant', content: evt.data })
      }
    }

    ws.onerror = (e) => {
      if (onError) onError(e)
    }

    ws.onclose = (evt) => {
      if (onClose) onClose(evt)
      // Auto-reconnect after 2 s unless deliberately closed
      if (!closed) setTimeout(connect, 2000)
    }
  }

  connect()

  return {
    /** Send a message object; will be JSON-serialised. */
    send(msgObj) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(msgObj))
      }
    },
    /** Permanently close — disables auto-reconnect. */
    close() {
      closed = true
      if (ws) ws.close()
    },
    /** True when the socket is currently open. */
    get ready() {
      return ws !== null && ws.readyState === WebSocket.OPEN
    },
  }
}
