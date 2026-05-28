/**
 * Component tests for src/pages/Roles.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Roles from '../pages/Roles.svelte'

vi.mock('../lib/api.js', () => ({
  listRoles:         vi.fn(),
  createRole:        vi.fn(),
  updateRole:        vi.fn(),
  deleteRole:        vi.fn(),
  seedRoles:         vi.fn(),
  previewRolePrompt: vi.fn(),
  listProviders:     vi.fn(),
}))

import { listRoles, listProviders, createRole } from '../lib/api.js'

const PROVIDERS = [
  { id: 'p1', name: 'gpt-4o',   roles: ['planner', 'reviewer'], enabled: true },
  { id: 'p2', name: 'claude-3', roles: ['executor'],             enabled: true },
  { id: 'p3', name: 'local',    roles: [],                       enabled: true },
]

const ROLE = {
  id: 'r1', name: 'planner', label: 'Planner', description: 'Plans tasks',
  provider_id: 'p1', model_override: '', system_prompt: '',
  context_include: [], context_exclude: [], task_types: ['implement'],
  temperature: 0.7, max_tokens: 4096, enabled: true,
}

beforeEach(() => {
  listRoles.mockResolvedValue([ROLE])
  listProviders.mockResolvedValue(PROVIDERS)
  createRole.mockResolvedValue({ id: 'r2', ...ROLE, name: 'new-role' })
})

describe('Roles — rendering', () => {
  it('shows page heading', () => {
    render(Roles)
    expect(screen.getByText('Roles')).toBeInTheDocument()
  })

  it('renders role name after load', async () => {
    render(Roles)
    await waitFor(() => expect(screen.getByText('Planner')).toBeInTheDocument())
  })
})

describe('Roles — provider dropdown filtering', () => {
  it('shows all providers when role name is empty', async () => {
    render(Roles)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Role'))
    await waitFor(() => screen.getByText('Provider'))

    // Name is blank → all providers should appear
    const options = screen.getAllByRole('option')
    const names = options.map(o => o.textContent)
    expect(names).toContain('gpt-4o')
    expect(names).toContain('claude-3')
    expect(names).toContain('local')
  })

  it('filters providers to role-compatible ones when name matches', async () => {
    render(Roles)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Role'))
    await waitFor(() => screen.getByPlaceholderText('e.g. worker'))

    const nameInput = screen.getByPlaceholderText('e.g. worker')
    fireEvent.input(nameInput, { target: { value: 'planner' } })

    // gpt-4o has 'planner' in its roles → should appear
    await waitFor(() => {
      const options = screen.getAllByRole('option')
      const names = options.map(o => o.textContent)
      expect(names).toContain('gpt-4o')
      // claude-3 only has 'executor' → should be filtered out
      expect(names).not.toContain('claude-3')
    })
  })

  it('falls back to all providers when no provider matches role name', async () => {
    render(Roles)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Role'))
    await waitFor(() => screen.getByPlaceholderText('e.g. worker'))

    const nameInput = screen.getByPlaceholderText('e.g. worker')
    fireEvent.input(nameInput, { target: { value: 'unmatched-role' } })

    // No provider has 'unmatched-role' → falls back to all
    await waitFor(() => {
      const options = screen.getAllByRole('option')
      const names = options.map(o => o.textContent)
      expect(names).toContain('gpt-4o')
      expect(names).toContain('claude-3')
      expect(names).toContain('local')
    })

    // Fallback hint should be shown
    await waitFor(() =>
      expect(screen.getByText(/no role-matched providers, showing all/i)).toBeInTheDocument()
    )
  })

  it('shows filtered hint when compatible providers exist', async () => {
    render(Roles)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Role'))
    await waitFor(() => screen.getByPlaceholderText('e.g. worker'))

    const nameInput = screen.getByPlaceholderText('e.g. worker')
    fireEvent.input(nameInput, { target: { value: 'executor' } })

    await waitFor(() =>
      expect(screen.getByText(/filtered for this role/i)).toBeInTheDocument()
    )
  })

  it('pre-selects provider when editing an existing role', async () => {
    render(Roles)
    await waitFor(() => screen.getByText('Planner'))

    const user = userEvent.setup()
    await user.click(screen.getByText('Edit'))

    await waitFor(() => {
      const select = screen.getByRole('combobox')
      expect(select.value).toBe('p1')
    })
  })
})
