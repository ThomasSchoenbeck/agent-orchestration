/**
 * Component tests for src/components/MultiSelect.svelte
 */
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import MultiSelect from '../components/MultiSelect.svelte'

describe('MultiSelect', () => {
  it('renders selected values as removable chips', () => {
    render(MultiSelect, { props: { value: ['alpha'], options: ['alpha', 'beta'] } })
    expect(screen.getByRole('button', { name: 'Remove alpha' })).toBeInTheDocument()
  })

  it('adds a free-text value on Enter', async () => {
    const user = userEvent.setup()
    render(MultiSelect, { props: { value: [], options: [], allowFree: true } })
    await user.type(screen.getByRole('textbox'), 'gamma{Enter}')
    expect(screen.getByRole('button', { name: 'Remove gamma' })).toBeInTheDocument()
  })

  it('adds an option by clicking it in the dropdown', async () => {
    const user = userEvent.setup()
    render(MultiSelect, { props: { value: [], options: ['read_file', 'write_file'] } })
    const input = screen.getByRole('textbox')
    await user.click(input)
    await user.type(input, 'write')
    await user.click(screen.getByText('write_file'))
    expect(screen.getByRole('button', { name: 'Remove write_file' })).toBeInTheDocument()
  })

  it('renders chips via labelFor (e.g. id → name)', () => {
    render(MultiSelect, {
      props: { value: ['rid-1'], options: [], labelFor: (v) => (v === 'rid-1' ? 'worker' : v) },
    })
    expect(screen.getByRole('button', { name: 'Remove worker' })).toBeInTheDocument()
  })

  it('removes a chip when its × is clicked', async () => {
    const user = userEvent.setup()
    render(MultiSelect, { props: { value: ['alpha', 'beta'], options: [] } })
    await user.click(screen.getByRole('button', { name: 'Remove alpha' }))
    expect(screen.queryByRole('button', { name: 'Remove alpha' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Remove beta' })).toBeInTheDocument()
  })
})
