import { writable, derived } from 'svelte/store'

// ── Current page (hash router) ───────────────────────────────────────────────
// Route format: #/<page>[/<param1>[/<param2>...]]
// $router = { page: string, params: string[] }
function createRouter() {
  function parse() {
    const raw = window.location.hash.replace(/^#\/?/, '') || 'projects'
    const [page, ...params] = raw.split('/')
    return { page: page || 'projects', params }
  }

  const { subscribe, set } = writable(parse())
  window.addEventListener('hashchange', () => set(parse()))

  return {
    subscribe,
    /** Navigate to a top-level page */
    go: (page) => { window.location.hash = '#/' + page },
    /** Navigate to a page with additional path segments */
    push: (...segments) => { window.location.hash = '#/' + segments.join('/') },
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
