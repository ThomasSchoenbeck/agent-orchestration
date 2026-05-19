/**
 * Formats a timestamp string or Date as a short date + time string.
 * Example output: "May 20, 14:32:05"
 *
 * Using month+day+time keeps the display compact while making entries
 * from different days unambiguous.
 *
 * @param {string|number|Date} ts
 * @returns {string}
 */
export function formatTimestamp(ts) {
  if (!ts) return '—'
  try {
    return new Date(ts).toLocaleString([], {
      month: 'short',
      day:   'numeric',
      hour:   '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return String(ts)
  }
}
