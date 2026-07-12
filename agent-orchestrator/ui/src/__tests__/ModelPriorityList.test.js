/**
 * Component tests for src/components/ModelPriorityList.svelte
 */
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ModelPriorityList from '../components/ModelPriorityList.svelte'

const PROVIDERS = [
  { name: 'openai', models: [{ name: 'gpt-4o' }, { name: 'gpt-4o-mini' }] },
  { name: 'local', models: [] },
]

describe('ModelPriorityList', () => {
  it('renders no row controls until a route is added', () => {
    render(ModelPriorityList, { props: { value: [], providers: PROVIDERS } })
    expect(screen.queryByLabelText('priority provider')).not.toBeInTheDocument()
    expect(screen.getByText('+ Add route')).toBeInTheDocument()
  })

  it('shows a model dropdown once a provider with models is chosen', async () => {
    const user = userEvent.setup()
    render(ModelPriorityList, { props: { value: [], providers: PROVIDERS } })
    await user.click(screen.getByText('+ Add route'))

    // Before a provider is chosen the model field is a free-text input.
    expect(screen.getByLabelText('priority model').tagName).toBe('INPUT')

    await user.selectOptions(screen.getByLabelText('priority provider'), 'openai')
    const modelSel = screen.getByLabelText('priority model')
    expect(modelSel.tagName).toBe('SELECT')
    const opts = Array.from(modelSel.options).map(o => o.value)
    expect(opts).toContain('gpt-4o')
    expect(opts).toContain('gpt-4o-mini')
  })

  it('keeps a text input for a provider with no configured models', async () => {
    const user = userEvent.setup()
    render(ModelPriorityList, { props: { value: [], providers: PROVIDERS } })
    await user.click(screen.getByText('+ Add route'))
    await user.selectOptions(screen.getByLabelText('priority provider'), 'local')
    expect(screen.getByLabelText('priority model').tagName).toBe('INPUT')
  })

  it('removes a route', async () => {
    const user = userEvent.setup()
    render(ModelPriorityList, {
      props: { value: [{ provider: 'openai', model: 'gpt-4o' }], providers: PROVIDERS },
    })
    expect(screen.getByLabelText('priority provider')).toBeInTheDocument()
    await user.click(screen.getByTitle('Remove'))
    expect(screen.queryByLabelText('priority provider')).not.toBeInTheDocument()
  })

  it('reorders routes with the move-up control', async () => {
    const user = userEvent.setup()
    render(ModelPriorityList, {
      props: {
        value: [
          { provider: 'openai', model: 'gpt-4o' },
          { provider: 'local', model: 'llama' },
        ],
        providers: PROVIDERS,
      },
    })
    const before = screen.getAllByLabelText('priority provider').map(s => s.value)
    expect(before).toEqual(['openai', 'local'])

    // Move the 2nd row up (the first row's up control is disabled).
    await user.click(screen.getAllByTitle('Move up')[1])

    const after = screen.getAllByLabelText('priority provider').map(s => s.value)
    expect(after).toEqual(['local', 'openai'])
  })
})
