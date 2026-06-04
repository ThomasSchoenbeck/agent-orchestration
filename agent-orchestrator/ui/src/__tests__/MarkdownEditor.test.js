/**
 * Component tests for src/components/MarkdownEditor.svelte
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import { tick } from 'svelte'

// Mock marked and DOMPurify (used inside the component)
vi.mock('marked', () => ({
  marked: {
    parse: (md) => `<p>${md}</p>`,
  },
}))
vi.mock('dompurify', () => ({
  default: {
    sanitize: (html) => html,
  },
}))

import MarkdownEditor from '../components/MarkdownEditor.svelte'

describe('MarkdownEditor — rendering', () => {
  it('renders without error', () => {
    render(MarkdownEditor)
    // Editor div should be present
    expect(document.querySelector('[contenteditable]')).toBeInTheDocument()
  })

  it('toolbar is hidden in readonly mode', () => {
    render(MarkdownEditor, { props: { readonly: true, value: 'hello' } })
    // No toolbar buttons in readonly mode
    expect(screen.queryByTitle('Bold')).not.toBeInTheDocument()
  })

  it('shows toolbar buttons in edit mode', () => {
    render(MarkdownEditor, { props: { readonly: false } })
    expect(screen.getByTitle('Bold')).toBeInTheDocument()
    expect(screen.getByTitle('Italic')).toBeInTheDocument()
  })

  it('contenteditable is false in readonly mode', () => {
    render(MarkdownEditor, { props: { readonly: true } })
    const el = document.querySelector('[contenteditable]')
    expect(el?.getAttribute('contenteditable')).toBe('false')
  })

  it('contenteditable is true in edit mode', () => {
    render(MarkdownEditor, { props: { readonly: false } })
    const el = document.querySelector('[contenteditable]')
    expect(el?.getAttribute('contenteditable')).toBe('true')
  })

  it('applies placeholder attribute', () => {
    render(MarkdownEditor, { props: { placeholder: 'Enter text here' } })
    const el = document.querySelector('[data-placeholder]')
    expect(el?.getAttribute('data-placeholder')).toBe('Enter text here')
  })

  it('applies minHeight style', () => {
    render(MarkdownEditor, { props: { minHeight: '240px' } })
    const el = document.querySelector('[contenteditable]')
    expect(el?.getAttribute('style')).toContain('240px')
  })
})

describe('MarkdownEditor — single pane', () => {
  it('does not have a textarea element', () => {
    render(MarkdownEditor)
    expect(document.querySelector('textarea')).not.toBeInTheDocument()
  })

  it('does not have a Preview/Edit toggle button', () => {
    render(MarkdownEditor)
    expect(screen.queryByTitle('Toggle preview')).not.toBeInTheDocument()
    expect(screen.queryByText('Preview')).not.toBeInTheDocument()
  })

  it('has a single contenteditable area', () => {
    render(MarkdownEditor)
    expect(document.querySelectorAll('[contenteditable]').length).toBe(1)
  })
})

describe('MarkdownEditor — external reset', () => {
  it('clears the editor when value is reset to empty while focused (Enter-to-send)', async () => {
    const { rerender } = render(MarkdownEditor, { props: { value: 'hello', readonly: false } })
    const el = document.querySelector('[contenteditable]')
    await tick()
    expect(el.innerHTML).not.toBe('')

    // Enter-to-send keeps the editor focused, then the parent sets value = ''.
    el.dispatchEvent(new FocusEvent('focus'))
    await tick()
    await rerender({ value: '', readonly: false })
    await tick()

    expect(el.innerHTML).toBe('')
  })
})
