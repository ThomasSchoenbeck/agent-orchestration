/**
 * Component tests for src/pages/CostDetail.svelte
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import CostDetail from '../pages/CostDetail.svelte'

vi.mock('../lib/api.js', () => ({
  getCostBreakdown:     vi.fn(),
  getCostFilterOptions: vi.fn(),
}))

import { getCostBreakdown, getCostFilterOptions } from '../lib/api.js'

const OPTIONS = {
  models:      ['gpt-4', 'claude'],
  agent_roles: ['worker', 'reviewer'],
  sources:     ['agent', 'chat'],
  providers:   ['p1', 'p2'],
  min_date:    '2026-01-01',
  max_date:    '2026-06-01',
}

beforeEach(() => {
  getCostFilterOptions.mockResolvedValue(OPTIONS)
  getCostBreakdown.mockResolvedValue([
    { key: 'a1', input_tokens: 100, output_tokens: 20, cost: 0.12, count: 3 },
  ])
})

describe('CostDetail — rendering', () => {
  it('renders heading and breakdown buckets', async () => {
    render(CostDetail)
    await waitFor(() => expect(screen.getByText('Cost detail')).toBeInTheDocument())
    expect(await screen.findByText('a1')).toBeInTheDocument()
    // Appears in both the totals line and the single bucket row.
    expect(screen.getAllByText('~$0.1200').length).toBeGreaterThan(0)
  })

  it('loads the initial breakdown grouped by agent_id', async () => {
    render(CostDetail)
    await waitFor(() => expect(getCostBreakdown).toHaveBeenCalled())
    expect(getCostBreakdown.mock.calls[0][0]).toBe('agent_id')
  })

  it('populates filter options from getCostFilterOptions', async () => {
    render(CostDetail)
    await waitFor(() => expect(getCostFilterOptions).toHaveBeenCalled())
    const model = await screen.findByLabelText('Model')
    const opts = Array.from(model.options).map(o => o.value)
    expect(opts).toContain('gpt-4')
    expect(opts).toContain('claude')
  })
})

describe('CostDetail — interactions', () => {
  it('reloads with the model filter applied', async () => {
    render(CostDetail)
    const user = userEvent.setup()
    const model = await screen.findByLabelText('Model')
    await user.selectOptions(model, 'gpt-4')
    await waitFor(() =>
      expect(getCostBreakdown).toHaveBeenLastCalledWith(
        'agent_id', expect.objectContaining({ model: 'gpt-4' })
      )
    )
  })

  it('switches the breakdown dimension', async () => {
    render(CostDetail)
    const user = userEvent.setup()
    await waitFor(() => expect(getCostBreakdown).toHaveBeenCalled())
    await user.selectOptions(screen.getByLabelText('Break down'), 'task')
    await waitFor(() =>
      expect(getCostBreakdown).toHaveBeenLastCalledWith('task', expect.anything())
    )
  })

  it('clears filters back to defaults', async () => {
    render(CostDetail)
    const user = userEvent.setup()
    const model = await screen.findByLabelText('Model')
    await user.selectOptions(model, 'gpt-4')
    await user.click(screen.getByText('Clear filters'))
    await waitFor(() =>
      expect(getCostBreakdown).toHaveBeenLastCalledWith(
        'agent_id', expect.objectContaining({ model: '' })
      )
    )
  })
})
