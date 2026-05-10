import { writable, derived } from 'svelte/store'

// ── Current page (hash router) ───────────────────────────────────────────────
function createRouter() {
  const getPage = () => window.location.hash.replace(/^#\/?/, '') || 'projects'
  const { subscribe, set } = writable(getPage())

  window.addEventListener('hashchange', () => set(getPage()))

  return {
    subscribe,
    go: (page) => { window.location.hash = '#/' + page },
  }
}
export const router = createRouter()

// ── Notification toasts ──────────────────────────────────────────────────────
const { subscribe: toastSub, update: toastUpdate } = writable([])

let toastSeq = 0
export const toasts = {
  subscribe: toastSub,
  add(message, type = 'info', duration = 4000) {
    const id = ++toastSeq
    toastUpdate(ts => [...ts, { id, message, type }])
    setTimeout(() => toasts.remove(id), duration)
  },
  remove(id) {
    toastUpdate(ts => ts.filter(t => t.id !== id))
  },
  info:    (msg) => toasts.add(msg, 'info'),
  success: (msg) => toasts.add(msg, 'success'),
  error:   (msg) => toasts.add(msg, 'error'),
}

// ── Loading overlay counter ───────────────────────────────────────────────────
const loadingCount = writable(0)
export const loading = derived(loadingCount, $n => $n > 0)
export function startLoading() { loadingCount.update(n => n + 1) }
export function stopLoading()  { loadingCount.update(n => Math.max(0, n - 1)) }
