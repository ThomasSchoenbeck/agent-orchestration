/**
 * Component tests for src/pages/Projects.svelte
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Projects from '../pages/Projects.svelte'

// ── fetch mock helpers ────────────────────────────────────────────────────────
const PROJECTS = [
  { id: 'p1', name: 'Alpha',  description: 'First project',  created_at: new Date().toISOString() },
  { id: 'p2', name: 'Beta',   description: '',                created_at: new Date().toISOString() },
]

function mockFetch(response, status = 200) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok:     status < 400,
    status,
    json:   () => Promise.resolve(response),
  }))
}

afterEach(() => vi.unstubAllGlobals())

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Projects — rendering', () => {
  it('shows the page heading', () => {
    mockFetch([])
    render(Projects)
    expect(screen.getByText('Projects')).toBeInTheDocument()
  })

  it('shows "New Project" button', () => {
    mockFetch([])
    render(Projects)
    expect(screen.getByText('+ New Project')).toBeInTheDocument()
  })

  it('shows empty-state message when project list is empty', async () => {
    mockFetch([])
    render(Projects)
    await waitFor(() =>
      expect(screen.getByText(/No projects yet/i)).toBeInTheDocument()
    )
  })

  it('renders a card for each project returned by the API', async () => {
    mockFetch(PROJECTS)
    render(Projects)
    await waitFor(() => {
      expect(screen.getByText('Alpha')).toBeInTheDocument()
      expect(screen.getByText('Beta')).toBeInTheDocument()
    })
  })

  it('shows description when present', async () => {
    mockFetch(PROJECTS)
    render(Projects)
    await waitFor(() =>
      expect(screen.getByText('First project')).toBeInTheDocument()
    )
  })

  it('calls GET /api/projects on mount', async () => {
    mockFetch([])
    render(Projects)
    await waitFor(() =>
      expect(fetch).toHaveBeenCalledWith('/api/projects', expect.anything())
    )
  })
})

// ── Create form ───────────────────────────────────────────────────────────────
describe('Projects — create form', () => {
  it('toggles form visibility', async () => {
    mockFetch([])
    const user = userEvent.setup()
    render(Projects)
    expect(screen.queryByPlaceholderText('Project name')).not.toBeInTheDocument()

    await user.click(screen.getByText('+ New Project'))
    expect(screen.getByPlaceholderText('Project name')).toBeInTheDocument()

    await user.click(screen.getByText('Cancel'))
    expect(screen.queryByPlaceholderText('Project name')).not.toBeInTheDocument()
  })

  it('submits POST /api/projects with name and description', async () => {
    // First load returns empty, second (after create) returns the new project
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve([]) })
      .mockResolvedValueOnce({ ok: true, status: 201, json: () => Promise.resolve({ id: 'p3', name: 'Gamma' }) })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve([{ id: 'p3', name: 'Gamma' }]) })
    )
    const user = userEvent.setup()
    render(Projects)
    await waitFor(() => screen.getByText('+ New Project'))

    await user.click(screen.getByText('+ New Project'))
    await user.type(screen.getByPlaceholderText('Project name'), 'Gamma')
    await user.click(screen.getByText('Create'))

    await waitFor(() => {
      const calls = fetch.mock.calls
      const postCall = calls.find(([url, opts]) => url === '/api/projects' && opts?.method === 'POST')
      expect(postCall).toBeTruthy()
      expect(JSON.parse(postCall[1].body).name).toBe('Gamma')
    })
  })

  it('does not submit when name is empty', async () => {
    mockFetch([])
    const user = userEvent.setup()
    render(Projects)
    await user.click(screen.getByText('+ New Project'))
    // The Create button has type="submit" and the input has required
    // so HTML validation prevents submission; fetch should only have been
    // called once (the initial load).
    await user.click(screen.getByText('Create'))
    expect(fetch).toHaveBeenCalledTimes(1)
  })
})

// ── Delete ────────────────────────────────────────────────────────────────────
describe('Projects — delete', () => {
  it('calls DELETE /api/projects/:id after confirm', async () => {
    mockFetch(PROJECTS)
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve(PROJECTS) })
      .mockResolvedValueOnce({ ok: true, status: 204 })
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve([PROJECTS[1]]) })
    )
    const user = userEvent.setup()
    render(Projects)
    const deleteButtons = await waitFor(() => screen.getAllByText('Delete'))
    await user.click(deleteButtons[0])

    const calls = fetch.mock.calls
    const delCall = calls.find(([url, opts]) => opts?.method === 'DELETE')
    expect(delCall).toBeTruthy()
    expect(delCall[0]).toBe('/api/projects/p1')
  })

  it('does NOT delete if confirm is cancelled', async () => {
    mockFetch(PROJECTS)
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const user = userEvent.setup()
    render(Projects)
    const deleteButtons = await waitFor(() => screen.getAllByText('Delete'))
    await user.click(deleteButtons[0])
    const calls = fetch.mock.calls
    expect(calls.every(([, opts]) => opts?.method !== 'DELETE')).toBe(true)
  })
})

// ── Error handling ────────────────────────────────────────────────────────────
describe('Projects — error handling', () => {
  it('does not crash on API load error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Network error')))
    expect(() => render(Projects)).not.toThrow()
  })
})
