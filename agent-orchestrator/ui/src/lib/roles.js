// Role reference display mapping (Task 9, Phase 3).
// Stored role references may be ids or names (during the id migration). This
// resolves either form to the human-readable role *name* using a list of role
// definitions (id/name/label) or meta items (id/value/label). Unknown refs
// (roles with no definition) pass through unchanged.
export function roleLabel(ref, defs = []) {
  if (!ref) return ref
  const d = (defs || []).find(
    (x) => x.id === ref || x.name === ref || x.value === ref
  )
  return d ? (d.name ?? d.value ?? ref) : ref
}
