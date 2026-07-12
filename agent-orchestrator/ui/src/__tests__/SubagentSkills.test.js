/**
 * Component tests for src/pages/SubagentSkills.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SubagentSkills from '../pages/SubagentSkills.svelte'
import { get } from 'svelte/store'
import { toasts } from '../lib/stores.js'

describe('SubagentSkills — error handling', () => {
  it('shows an error toast when skills fail to load', async () => {
    listSubagentSkills.mockRejectedValue(new Error('boom'))
    render(SubagentSkills)
    await waitFor(() => {
      const errs = get(toasts).filter(
        (t) => t.type === 'error' && /Failed to load subagent skills/.test(t.message)
      )
      expect(errs.length).toBeGreaterThan(0)
    })
  })
})

vi.mock('../lib/api.js', () => ({
  listSubagentSkills:   vi.fn(),
  createSubagentSkill:  vi.fn(),
  updateSubagentSkill:  vi.fn(),
  deleteSubagentSkill:  vi.fn(),
  seedSubagentSkills:   vi.fn(),
  getMetaTools:         vi.fn(),
  listProviders:        vi.fn(),
}))

import {
  listSubagentSkills, createSubagentSkill, deleteSubagentSkill,
  seedSubagentSkills, getMetaTools, listProviders,
} from '../lib/api.js'

const SKILL = {
  id: 's1', name: 'investigate_codebase', label: 'Investigate Codebase',
  description: 'Read-only exploration', prompt_template: 'Investigate {{instructions}}',
  tool_allowlist: ['read_file', 'list_files'],
  context_include: [], context_exclude: [], max_rounds: 8, enabled: true,
}

beforeEach(() => {
  listSubagentSkills.mockResolvedValue([SKILL])
  createSubagentSkill.mockResolvedValue({ id: 's2', ...SKILL, name: 'new_one' })
  deleteSubagentSkill.mockResolvedValue({})
  seedSubagentSkills.mockResolvedValue({ seeded: 2 })
  getMetaTools.mockResolvedValue([
    { value: 'read_file', label: 'read_file' },
    { value: 'write_file', label: 'write_file' },
    { value: 'run_subagent', label: 'run_subagent' },
  ])
  listProviders.mockResolvedValue([
    { id: 'p1', name: 'openai', models: [{ name: 'gpt-4o' }] },
  ])
})

describe('SubagentSkills — rendering', () => {
  it('shows page heading', () => {
    render(SubagentSkills)
    expect(screen.getByText('Subagent Skills')).toBeInTheDocument()
  })

  it('renders the seeded skill after load', async () => {
    render(SubagentSkills)
    await waitFor(() => expect(screen.getByText('Investigate Codebase')).toBeInTheDocument())
    expect(screen.getByText('investigate_codebase')).toBeInTheDocument()
  })
})

describe('SubagentSkills — CRUD', () => {
  it('creates a new subagent skill via the API helper', async () => {
    render(SubagentSkills)
    const user = userEvent.setup()

    await user.click(screen.getByText('New subagent skill'))
    await waitFor(() => screen.getByPlaceholderText(/name \(slug/))

    const nameInput = screen.getByPlaceholderText(/name \(slug/)
    await user.clear(nameInput)
    await user.type(nameInput, 'my_skill')
    await user.click(screen.getByText('Save'))

    await waitFor(() => expect(createSubagentSkill).toHaveBeenCalled())
    const payload = createSubagentSkill.mock.calls[0][0]
    expect(payload.name).toBe('my_skill')
    expect(payload.max_rounds).toBe(8)
  })

  it('includes an added provider>model route in the create payload', async () => {
    render(SubagentSkills)
    const user = userEvent.setup()

    await user.click(screen.getByText('New subagent skill'))
    await waitFor(() => screen.getByPlaceholderText(/name \(slug/))
    await user.type(screen.getByPlaceholderText(/name \(slug/), 'routed_skill')

    // Add one priority route and pick provider + model.
    await user.click(screen.getByText('+ Add route'))
    await user.selectOptions(await screen.findByLabelText('priority provider'), 'openai')
    await user.selectOptions(await screen.findByLabelText('priority model'), 'gpt-4o')

    await user.click(screen.getByText('Save'))
    await waitFor(() => expect(createSubagentSkill).toHaveBeenCalled())
    const payload = createSubagentSkill.mock.lastCall[0]
    expect(payload.models).toEqual([{ provider: 'openai', model: 'gpt-4o' }])
  })

  it('calls the seed helper', async () => {
    render(SubagentSkills)
    const user = userEvent.setup()
    await user.click(screen.getByText('Seed starter set'))
    await waitFor(() => expect(seedSubagentSkills).toHaveBeenCalled())
  })
})

describe('SubagentSkills — no nesting', () => {
  it('excludes run_subagent from the tool allowlist options', async () => {
    render(SubagentSkills)
    const user = userEvent.setup()

    await user.click(screen.getByText('New subagent skill'))
    const input = await screen.findByPlaceholderText('tool allowlist')

    // Open the dropdown so options render.
    await user.click(input)

    // Scope to the dropdown options (the heading text mentions run_subagent too).
    const listbox = await screen.findByRole('listbox')
    const opts = within(listbox)
    expect(opts.getByText('read_file')).toBeInTheDocument()
    expect(opts.queryByText('run_subagent')).not.toBeInTheDocument()
  })
})
