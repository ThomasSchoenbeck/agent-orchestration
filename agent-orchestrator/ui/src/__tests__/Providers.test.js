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
  getMetricsCosts: vi.fn(),
  getCostBreakdown: vi.fn(),
  listRoles:      vi.fn(),
  getMetaTools:   vi.fn(),
}))

import {
  listProviders, createProvider, updateProvider, listRoles, getMetrics, getMetricsCosts,
  getCostBreakdown, getMetaTools,
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

const TOOLS = [
  { value: 'read_file',  label: 'read_file',  description: 'Read a file' },
  { value: 'write_file', label: 'write_file', description: 'Write a file' },
]

beforeEach(() => {
  listProviders.mockResolvedValue([PROVIDER])
  listRoles.mockResolvedValue(ROLES)
  createProvider.mockResolvedValue({ id: 'p2', ...PROVIDER, name: 'new-provider' })
  updateProvider.mockResolvedValue({ ...PROVIDER })
  getMetrics.mockResolvedValue(null)
  getMetricsCosts.mockResolvedValue(null)
  getCostBreakdown.mockResolvedValue([])
  getMetaTools.mockResolvedValue(TOOLS)
})

describe('Providers — cost breakdown (F6)', () => {
  it('renders breakdown buckets from the group_by endpoint', async () => {
    getCostBreakdown.mockResolvedValue([
      { key: 'agent', input_tokens: 100, output_tokens: 20, cost: 0.12, count: 3 },
      { key: 'chat', input_tokens: 200, output_tokens: 40, cost: 0.05, count: 2 },
    ])
    render(Providers)
    await waitFor(() =>
      expect(screen.getByText('Cost breakdown')).toBeInTheDocument()
    )
    expect(screen.getByText('agent')).toBeInTheDocument()
    expect(screen.getByText('chat')).toBeInTheDocument()
    expect(screen.getByText('~$0.1200')).toBeInTheDocument()
  })
})

describe('Providers — cost detail link', () => {
  it('navigates to #/costs when "View details" is clicked', async () => {
    render(Providers)
    const user = userEvent.setup()
    await waitFor(() => expect(screen.getByText('View details →')).toBeInTheDocument())
    await user.click(screen.getByText('View details →'))
    expect(window.location.hash).toBe('#/costs')
  })
})

describe('Providers — cost (merged into Metrics section)', () => {
  it('renders total cost from the metrics summary, currency-formatted', async () => {
    getMetrics.mockResolvedValue({ total_cost: 0.1234, total_tokens: 500 })
    render(Providers)
    await waitFor(() =>
      expect(screen.getByText('~$0.1234')).toBeInTheDocument()
    )
    // The separate "Cost" box is gone — cost lives in the Metrics grid now.
    expect(screen.queryByText('Total cost (actual usage)')).not.toBeInTheDocument()
  })
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
  // Helper: get the roles <select> element (labelled "Roles")
  function rolesSelect() {
    return screen.getByLabelText('Roles')
  }

  it('shows no-roles guidance when listRoles returns empty', async () => {
    listRoles.mockResolvedValue([])
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() =>
      expect(screen.getByText(/No roles defined yet/)).toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Roles')).not.toBeInTheDocument()
  })

  it('shows error message when listRoles rejects', async () => {
    listRoles.mockRejectedValue(new Error('server error'))
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() =>
      expect(screen.getByText(/Failed to load roles/)).toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Roles')).not.toBeInTheDocument()
  })

  it('shows roles select in the create form with all role options', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => expect(rolesSelect()).toBeInTheDocument())

    const select = rolesSelect()
    expect(select.multiple).toBe(true)
    const options = Array.from(select.options).map(o => o.text)
    expect(options).toContain('Planner')
    expect(options).toContain('Executor')
    expect(options).toContain('Reviewer')
  })

  it('no roles are selected by default on new form', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => expect(rolesSelect()).toBeInTheDocument())

    const selected = Array.from(rolesSelect().selectedOptions).map(o => o.value)
    expect(selected).toHaveLength(0)
  })

  it('pre-selects existing roles when editing a provider', async () => {
    render(Providers)
    await waitFor(() => screen.getByText('my-openai'))

    const user = userEvent.setup()
    await user.click(screen.getByText('Edit'))
    await waitFor(() => expect(rolesSelect()).toBeInTheDocument())

    const selected = Array.from(rolesSelect().selectedOptions).map(o => o.value)
    expect(selected).toEqual(['planner'])
  })

  it('includes selected roles in create body', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => expect(rolesSelect()).toBeInTheDocument())

    await user.type(screen.getByPlaceholderText('e.g. my-openai'), 'test-provider')
    await user.selectOptions(rolesSelect(), ['executor'])
    await user.click(screen.getByText('Create'))

    await waitFor(() =>
      expect(createProvider).toHaveBeenCalledWith(
        expect.objectContaining({ roles: ['executor'] })
      )
    )
  })

  it('reflects added and removed role selections in update body', async () => {
    render(Providers)
    await waitFor(() => screen.getByText('my-openai'))

    const user = userEvent.setup()
    await user.click(screen.getByText('Edit'))
    await waitFor(() => expect(rolesSelect()).toBeInTheDocument())

    // planner is pre-selected; deselect it and select executor instead
    await user.deselectOptions(rolesSelect(), ['planner'])
    await user.selectOptions(rolesSelect(), ['executor'])
    await user.click(screen.getByText('Update'))

    await waitFor(() =>
      expect(updateProvider).toHaveBeenCalledWith(
        'p1',
        expect.objectContaining({ roles: ['executor'] })
      )
    )
  })
})

// ── Form: provider-level system prompt (T5.8) ───────────────────────────────────
describe('Providers — system prompt', () => {
  it('includes the provider system prompt in the create body config', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => expect(screen.getByPlaceholderText('e.g. my-openai')).toBeInTheDocument())

    await user.type(screen.getByPlaceholderText('e.g. my-openai'), 'test-provider')
    await user.type(screen.getByPlaceholderText('e.g. Always prefer concise answers.'), 'Be terse.')
    await user.click(screen.getByText('Create'))

    await waitFor(() =>
      expect(createProvider).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ system_prompt: 'Be terse.' }),
        })
      )
    )
  })

  it('no longer renders a per-model Roles column', async () => {
    render(Providers)
    const user = userEvent.setup()
    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => expect(screen.getByText('+ Add model')).toBeInTheDocument())
    await user.click(screen.getByText('+ Add model'))

    // The model row exposes a System prompt column, not Roles.
    await waitFor(() => expect(screen.getByPlaceholderText('model prompt layer')).toBeInTheDocument())
    expect(screen.queryByPlaceholderText('worker, reviewer')).not.toBeInTheDocument()
  })
})

// ── Form: tool allowlist multi-select (predefined list) ─────────────────────────
describe('Providers — tool allowlist multi-select', () => {
  const toolInput = () => screen.getByPlaceholderText('write_file, read_file, list_files…')

  it('offers predefined tools from getMetaTools as suggestions', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => expect(toolInput()).toBeInTheDocument())

    await user.click(toolInput())
    await user.type(toolInput(), 'read')
    // suggestion from the predefined list shows in the dropdown listbox
    await waitFor(() =>
      expect(screen.getByRole('option', { name: /read_file/ })).toBeInTheDocument()
    )
  })

  it('includes selected tools as an array in the create body config', async () => {
    render(Providers)
    const user = userEvent.setup()

    await user.click(screen.getByText('+ Add Provider'))
    await waitFor(() => expect(toolInput()).toBeInTheDocument())

    await user.type(screen.getByPlaceholderText('e.g. my-openai'), 'test-provider')
    await user.type(toolInput(), 'read_file{Enter}')
    await user.click(screen.getByText('Create'))

    await waitFor(() =>
      expect(createProvider).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ tool_allowlist: ['read_file'] }),
        })
      )
    )
  })

  it('pre-fills existing tool allowlist as tags when editing', async () => {
    listProviders.mockResolvedValue([
      { ...PROVIDER, config: { tool_allowlist: ['write_file'] } },
    ])
    render(Providers)
    await waitFor(() => screen.getByText('my-openai'))

    const user = userEvent.setup()
    await user.click(screen.getByText('Edit'))
    // The MultiSelect hides its placeholder once it has a value, so wait on the
    // edit form (Update button) rather than the tool input's placeholder.
    await waitFor(() => expect(screen.getByText('Update')).toBeInTheDocument())

    // the existing tool appears as a removable tag with an aria-labelled remove button
    expect(screen.getByLabelText('Remove write_file')).toBeInTheDocument()
  })
})
