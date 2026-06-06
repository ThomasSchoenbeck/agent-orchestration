/**
 * Unit tests for src/lib/roles.js (role id↔name display mapping).
 */
import { describe, it, expect } from 'vitest'
import { roleLabel } from '../lib/roles.js'

describe('roleLabel', () => {
  const defs = [{ id: 'rid-1', name: 'worker', label: 'Worker' }]

  it('maps a stored id to the role name', () => {
    expect(roleLabel('rid-1', defs)).toBe('worker')
  })

  it('passes a name through unchanged', () => {
    expect(roleLabel('worker', defs)).toBe('worker')
  })

  it('falls back to the ref for an unknown role', () => {
    expect(roleLabel('ghost', defs)).toBe('ghost')
  })

  it('works with meta items shaped {id,value,label}', () => {
    const meta = [{ id: 'rid-2', value: 'reviewer', label: 'Reviewer' }]
    expect(roleLabel('rid-2', meta)).toBe('reviewer')
  })

  it('handles empty/undefined gracefully', () => {
    expect(roleLabel('', defs)).toBe('')
    expect(roleLabel('worker')).toBe('worker')
  })
})
