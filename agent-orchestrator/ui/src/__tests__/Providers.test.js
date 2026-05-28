/**
 * Component tests for src/pages/Providers.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Providers from '../pages/Providers.svelte'

// ── Mock the api module ───────────────────────────────────────────────────────
vi.mock('../lib/api.js', () => ({
  listProviders:  vi.fn(),
  createProvider: vi.fn(),
  updateProvider: vi.fn(),
  deleteProvider: vi.fn(),
  testProvider:   vi.fn(),
  seedProviders:  vi.fn(),
  getMetrics:     vi.fn(),
  listRoles:      vi.fn(),
}))

import {
  listProviders, createProvider, updateProvider, listRoles, getMetrics,
} from '../lib/api.js'

const ROLES = [
  { id: 'r1', name: 'planner',  label: 'Planner'  },
  { id: 'r2', name: 'executor', label: 'Executor' },
  { id: 'r3', name: 'reviewer', label: 'Reviewer' },
]

const PROVIDER = {
  id: 'p1', name: 'my-openai', type: 'openai_compatible',
  model_name: 'gpt-4o', base_url: 'https://api.openai.com/v1',
  enabled: true, roles: ['planner'], config: {},
}

beforeEach(() => {
  listProviders.mockResolvedValue([PROVIDER])
  listRoles.mockResolvedValue(ROLES)
  createProvider.mockResolvedValue({ id: 'p2', ...PROVIDER, name: 'new-provider' })
  updateProvider.mockResolvedValue({ ...PROVIDER })
  getMetrics.mockResolvedValue(null)
})

// ── Rendering ─────────────────────────────────────────────────────────────────
describe('Providers — rendering', () => {
  it('shows page heading', () => {
    render(Providers)
    expect(screen.getByText('Providers')).toBeInTheDocument()
  })

  it('renders provider name after load', async () => {
    render(Providers)
    await waitFor(() =>
      expect(screen.getByText('my-openai')).toBeInTheDocument()
    )
  })

  it('renders role tag on provider card', async () => {
    render(Providers)
    await waitFor(() =>
      expect(screen.getByText('planner')).toBeInTheDocument()
    )
  })

  it('shows no role tags when provider has no roles', async () => {
    listProviders.mockResolvedValue([{ ...PROVIDER, roles: [] }])
    render(Providers)
    await waitFor(() => screen.getByText('my-openai'))
    // role tags container should not appear
    expect(screen.queryByText('planner')).not.toBeInTheDocument()
  })
})

// ── Form: roles multi-select ───────────────────────────────────────────────────
describe('Providers — roles multi-select', () => {
  it('shows role checkboxes in the create form', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() =>
      expect(screen.getByLabelText('Planner')).toBeInTheDocument()
    )
    expect(screen.getByLabelText('Executor')).toBeInTheDocument()
    expect(screen.getByLabelText('Reviewer')).toBeInTheDocument()
  })

  it('roles default to unchecked on new form', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => screen.getByLabelText('Planner'))

    expect(screen.getByLabelText('Planner')).not.toBeChecked()
    expect(screen.getByLabelText('Executor')).not.toBeChecked()
  })

  it('pre-checks roles when editing a provider', async () => {
    render(Providers)
    await waitFor(() => screen.getByText('my-openai'))

    const user = userEvent.setup()
    await user.click(screen.getByText('Edit'))

    await waitFor(() => screen.getByLabelText('Planner'))
    expect(screen.getByLabelText('Planner')).toBeChecked()
    expect(screen.getByLabelText('Executor')).not.toBeChecked()
  })

  it('includes selected roles in create body', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => screen.getByLabelText('Planner'))

    // fill required name
    await user.type(screen.getByPlaceholderText('e.g. my-openai'), 'test-provider')
    // select executor role
    await user.click(screen.getByLabelText('Executor'))
    await user.click(screen.getByText('Create'))

    await waitFor(() =>
      expect(createProvider).toHaveBeenCalledWith(
        expect.objectContaining({ roles: ['executor'] })
      )
    )
  })

  it('includes updated roles in update body', async () => {
    render(Providers)
    await waitFor(() => screen.getByText('my-openai'))

    const user = userEvent.setup()
    await user.click(screen.getByText('Edit'))
    await waitFor(() => screen.getByLabelText('Planner'))

    // uncheck planner, check executor
    await user.click(screen.getByLabelText('Planner'))
    await user.click(screen.getByLabelText('Executor'))
    await user.click(screen.getByText('Update'))

    await waitFor(() =>
      expect(updateProvider).toHaveBeenCalledWith(
        'p1',
        expect.objectContaining({ roles: ['executor'] })
      )
    )
  })
})
