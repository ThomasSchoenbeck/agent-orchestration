/**
 * Component tests for src/components/FileTree.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import FileTree from '../components/FileTree.svelte'

vi.mock('../lib/api.js', () => ({
  listBranches: vi.fn(),
  readTree:     vi.fn(),
}))

import { listBranches, readTree } from '../lib/api.js'

const TREE = [
  { name: 'main.go',   path: 'main.go',   type: 'blob' },
  { name: 'src',       path: 'src',        type: 'tree' },
]

beforeEach(() => {
  listBranches.mockResolvedValue(['main'])
  readTree.mockResolvedValue(TREE)
})

describe('FileTree — new file', () => {
  it('has a "+ File" button', async () => {
    render(FileTree, { props: { projectId: 'p1', onFileSelect: vi.fn() } })
    await waitFor(() => expect(screen.getByText('+ File')).toBeInTheDocument())
  })

  it('shows path input when "+ File" is clicked', async () => {
    render(FileTree, { props: { projectId: 'p1', onFileSelect: vi.fn() } })
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('+ File'))
    await user.click(screen.getByText('+ File'))

    expect(screen.getByPlaceholderText(/path\/to\/new-file/i)).toBeInTheDocument()
  })

  it('calls onFileSelect with entered path on Add click', async () => {
    const onFileSelect = vi.fn()
    render(FileTree, { props: { projectId: 'p1', onFileSelect } })
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('+ File'))
    await user.click(screen.getByText('+ File'))

    const input = screen.getByPlaceholderText(/path\/to\/new-file/i)
    await user.type(input, 'lib/utils.go')
    await user.click(screen.getByRole('button', { name: 'Add' }))

    expect(onFileSelect).toHaveBeenCalledWith('lib/utils.go', 'main')
  })

  it('calls onFileSelect when Enter is pressed in the input', async () => {
    const onFileSelect = vi.fn()
    render(FileTree, { props: { projectId: 'p1', onFileSelect } })
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('+ File'))
    await user.click(screen.getByText('+ File'))

    const input = screen.getByPlaceholderText(/path\/to\/new-file/i)
    await user.type(input, 'README.md')
    await user.keyboard('{Enter}')

    expect(onFileSelect).toHaveBeenCalledWith('README.md', 'main')
  })

  it('hides the input on Escape without calling onFileSelect', async () => {
    const onFileSelect = vi.fn()
    render(FileTree, { props: { projectId: 'p1', onFileSelect } })
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('+ File'))
    await user.click(screen.getByText('+ File'))

    const input = screen.getByPlaceholderText(/path\/to\/new-file/i)
    await user.type(input, 'something.go')
    await user.keyboard('{Escape}')

    expect(screen.queryByPlaceholderText(/path\/to\/new-file/i)).not.toBeInTheDocument()
    expect(onFileSelect).not.toHaveBeenCalled()
  })

  it('does not call onFileSelect when path is empty', async () => {
    const onFileSelect = vi.fn()
    render(FileTree, { props: { projectId: 'p1', onFileSelect } })
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('+ File'))
    await user.click(screen.getByText('+ File'))
    await user.click(screen.getByRole('button', { name: 'Add' }))

    expect(onFileSelect).not.toHaveBeenCalled()
  })
})

describe('FileTree — existing files', () => {
  it('renders file names after load', async () => {
    render(FileTree, { props: { projectId: 'p1', onFileSelect: vi.fn() } })
    await waitFor(() => expect(screen.getByText('main.go')).toBeInTheDocument())
    expect(screen.getByText('src')).toBeInTheDocument()
  })

  it('calls onFileSelect when a file is clicked', async () => {
    const onFileSelect = vi.fn()
    render(FileTree, { props: { projectId: 'p1', onFileSelect } })
    const user = userEvent.setup()

    await waitFor(() => screen.getByText('main.go'))
    await user.click(screen.getByText('main.go'))

    expect(onFileSelect).toHaveBeenCalledWith('main.go', 'main')
  })
})
