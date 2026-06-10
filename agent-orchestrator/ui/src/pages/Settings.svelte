<script>
  import { onMount } from 'svelte'
  import { listSettings, updateSetting, listChecklistTemplates, createChecklistTemplate, updateChecklistTemplate, deleteChecklistTemplate, getTaskTypes, createTaskType, updateTaskType, deleteTaskType } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'
  import Skeleton from '../components/Skeleton.svelte'

  const AGENT_EVENTS = [
    { key: 'agent_registered', label: 'Agent Registered', desc: 'Agent joined or re-registered' },
    { key: 'agent_poll_task_found', label: 'Poll Task Found', desc: 'Poll found a matching task' },
    { key: 'agent_claim_attempt', label: 'Claim Attempt', desc: 'Agent tried to claim a task' },
    { key: 'agent_claim_success', label: 'Claim Success', desc: 'Claim succeeded' },
    { key: 'agent_claim_failed', label: 'Claim Failed', desc: 'Claim failed (already taken)' },
    { key: 'agent_execute_start', label: 'Execute Start', desc: 'Task execution began' },
    { key: 'agent_llm_call', label: 'LLM Call', desc: 'An LLM round was made' },
    { key: 'agent_tool_call', label: 'Tool Call', desc: 'A tool was invoked' },
    { key: 'agent_tool_error', label: 'Tool Error', desc: 'Tool returned an error' },
    { key: 'agent_context_overflow', label: 'Context Overflow', desc: 'Hit LLM token limit' },
    { key: 'agent_reasoning_step', label: 'Reasoning Step', desc: 'Chain-of-thought captured' },
    { key: 'agent_retry_backoff', label: 'Retry Backoff', desc: 'Task failed; retry scheduled' },
    { key: 'agent_human_approval_req', label: 'Human Approval', desc: 'Awaiting human intervention' },
    { key: 'agent_execute_complete', label: 'Execute Complete', desc: 'Task finished successfully' },
    { key: 'agent_execute_failed', label: 'Execute Failed', desc: 'Task execution failed' },
    { key: 'agent_offline', label: 'Agent Offline', desc: 'Agent stopped heartbeating' },
  ]

  const TASK_EVENTS = [
    { key: 'task_created', label: 'Task Created', desc: 'Task was inserted' },
    { key: 'task_updated', label: 'Task Updated', desc: 'Metadata or priority changed' },
    { key: 'task_queued', label: 'Task Queued', desc: 'Status moved to queued/planned' },
    { key: 'task_claimed', label: 'Task Claimed', desc: 'Agent claimed the task' },
    { key: 'task_started', label: 'Task Started', desc: 'Execution began' },
    { key: 'task_llm_round', label: 'LLM Round', desc: 'Each LLM iteration' },
    { key: 'task_tool_call', label: 'Tool Call', desc: 'A tool call within execution' },
    { key: 'task_result_submitted', label: 'Result Submitted', desc: 'Result payload submitted' },
    { key: 'task_completed', label: 'Task Completed', desc: 'Task completed successfully' },
    { key: 'task_failed', label: 'Task Failed', desc: 'Task failed' },
    { key: 'task_timed_out', label: 'Task Timed Out', desc: 'Task timed out and was reset' },
    { key: 'task_requeued', label: 'Task Requeued', desc: 'Task requeued' },
  ]

  let settings   = $state({})
  let overrides  = $state({})
  let loading    = $state(true)
  let saving     = $state(false)
  let templates  = $state([])
  let newTplName = $state('')
  let newTplItems = $state('')  // newline-separated item labels
  let editingTplId = $state('')
  let tplBuf = $state({ name: '', items: '' })

  // ── Task types ──────────────────────────────────────────────────────────────
  let taskTypes = $state([])
  let newType = $state({ key: '', label: '', branch_template: '' })
  let editingTypeId = $state('')
  let typeBuf = $state({ key: '', label: '', branch_template: '', is_default: false, sort_order: 0 })

  async function loadTaskTypes() {
    try {
      taskTypes = (await getTaskTypes()) ?? []
    } catch (_) { taskTypes = [] }
  }

  async function createType() {
    if (!newType.key.trim() || !newType.label.trim() || !newType.branch_template.trim()) {
      toasts.error('Key, label and branch template are required')
      return
    }
    try {
      await createTaskType({
        key: newType.key.trim(),
        label: newType.label.trim(),
        branch_template: newType.branch_template.trim(),
        is_default: taskTypes.length === 0,
      })
      newType = { key: '', label: '', branch_template: '' }
      await loadTaskTypes()
      toasts.success('Task type created')
    } catch (e) { toasts.error('Create failed: ' + e.message) }
  }

  function startEditType(tt) {
    editingTypeId = tt.id
    typeBuf = { key: tt.key, label: tt.label, branch_template: tt.branch_template, is_default: tt.is_default, sort_order: tt.sort_order }
  }

  async function saveType(id) {
    try {
      await updateTaskType(id, { ...typeBuf })
      editingTypeId = ''
      await loadTaskTypes()
      toasts.success('Task type saved')
    } catch (e) { toasts.error('Save failed: ' + e.message) }
  }

  async function setDefaultType(tt) {
    try {
      await updateTaskType(tt.id, { ...tt, is_default: true })
      await loadTaskTypes()
    } catch (e) { toasts.error('Set default failed: ' + e.message) }
  }

  async function removeType(id) {
    if (!confirm('Delete this task type?')) return
    try {
      await deleteTaskType(id)
      await loadTaskTypes()
    } catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  function settingVal(key) {
    return settings[key]?.value ?? ''
  }

  function effectiveAgentDays(eventKey) {
    const override = overrides['agent_' + eventKey] ?? overrides[eventKey] ?? ''
    if (override !== '') return override + ' (override)'
    return (settingVal('log.retention.agent.default_days') || '14') + ' (default)'
  }

  function effectiveTaskDays(eventKey) {
    const override = overrides['task_' + eventKey] ?? overrides[eventKey] ?? ''
    if (override !== '') return override + ' (override)'
    return (settingVal('log.retention.task.default_days') || '30') + ' (default)'
  }

  async function loadTemplates() {
    try {
      templates = (await listChecklistTemplates()) ?? []
    } catch (_) { templates = [] }
  }

  async function createTemplate() {
    if (!newTplName.trim()) return
    const items = newTplItems.split('\n').map(s => s.trim()).filter(Boolean)
    try {
      await createChecklistTemplate({ name: newTplName.trim(), items_json: JSON.stringify(items) })
      newTplName = ''; newTplItems = ''
      await loadTemplates()
      toasts.success('Template created')
    } catch (e) { toasts.error('Create failed: ' + e.message) }
  }

  async function saveTemplate(id) {
    const items = tplBuf.items.split('\n').map(s => s.trim()).filter(Boolean)
    try {
      await updateChecklistTemplate(id, { name: tplBuf.name, items_json: JSON.stringify(items) })
      editingTplId = ''
      await loadTemplates()
      toasts.success('Template saved')
    } catch (e) { toasts.error('Save failed: ' + e.message) }
  }

  async function removeTemplate(id) {
    if (!confirm('Delete this template?')) return
    try {
      await deleteChecklistTemplate(id)
      await loadTemplates()
    } catch (e) { toasts.error('Delete failed: ' + e.message) }
  }

  async function load() {
    loading = true
    try {
      const all = await listSettings()
      const map = {}
      for (const s of all) map[s.key] = s
      settings = map
      // Populate override inputs from existing per-type keys.
      const ov = {}
      for (const s of all) {
        const m = s.key.match(/^log\.retention\.(agent|task)\.(.+?)_days$/)
        if (m && m[2] !== 'default') {
          ov[m[1] + '_' + m[2]] = s.value
        }
      }
      overrides = ov
    } catch (e) {
      toasts.error('Failed to load settings: ' + e.message)
    } finally {
      loading = false
    }
  }

  async function savePlatform(key, value) {
    saving = true
    try {
      await updateSetting(key, String(value))
      toasts.success('Saved')
      await load()
    } catch (e) {
      toasts.error('Save failed: ' + e.message)
    } finally {
      saving = false
    }
  }

  async function saveDefault(key) {
    const val = settingVal(key)
    if (!val) return
    saving = true
    try {
      await updateSetting(key, val)
      toasts.success('Saved')
      await load()
    } catch (e) {
      toasts.error('Save failed: ' + e.message)
    } finally {
      saving = false
    }
  }

  async function saveAgentOverride(eventKey) {
    const overrideKey = 'agent_' + eventKey
    const days = overrides[overrideKey] ?? ''
    const settingKey = 'log.retention.agent.' + eventKey + '_days'
    saving = true
    try {
      if (days === '') {
        // Clear override: set to match current default.
        const def = settingVal('log.retention.agent.default_days') || '14'
        await updateSetting(settingKey, def)
      } else {
        await updateSetting(settingKey, days)
      }
      toasts.success('Saved')
      await load()
    } catch (e) {
      toasts.error('Save failed: ' + e.message)
    } finally {
      saving = false
    }
  }

  async function saveTaskOverride(eventKey) {
    const overrideKey = 'task_' + eventKey
    const days = overrides[overrideKey] ?? ''
    const settingKey = 'log.retention.task.' + eventKey + '_days'
    saving = true
    try {
      if (days === '') {
        const def = settingVal('log.retention.task.default_days') || '30'
        await updateSetting(settingKey, def)
      } else {
        await updateSetting(settingKey, days)
      }
      toasts.success('Saved')
      await load()
    } catch (e) {
      toasts.error('Save failed: ' + e.message)
    } finally {
      saving = false
    }
  }

  onMount(() => { load(); loadTemplates(); loadTaskTypes() })
</script>

<div class="overflow-y-auto p-6 max-w-4xl mx-auto">
  <h1 class="text-xl font-semibold text-gray-100 mb-6">Settings</h1>

  {#if loading}
    <Skeleton rows={4} />
  {:else}
    <!-- Platform -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">Platform</h2>
      <p class="text-xs text-gray-400 mb-4">Global behaviour settings that take effect immediately.</p>

      <div class="flex flex-col gap-4">
        <!-- Debug mode -->
        <div class="flex items-center justify-between p-3 bg-surface-800 rounded border border-surface-600">
          <div>
            <div class="text-sm text-gray-200 font-medium">Debug mode</div>
            <div class="text-xs text-gray-500 mt-0.5">Emit verbose agent events: heartbeat, poll_query, poll_no_task. Generates high log volume.</div>
          </div>
          <label class="relative inline-flex items-center cursor-pointer ml-4 shrink-0">
            <input
              type="checkbox"
              class="sr-only peer"
              checked={settings['platform.debug_mode']?.value === 'true'}
              onchange={(e) => savePlatform('platform.debug_mode', e.currentTarget.checked)}
              disabled={saving}
            />
            <div class="w-10 h-6 bg-surface-600 peer-focus:outline-none rounded-full peer peer-checked:bg-accent peer-disabled:opacity-40 transition-colors after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:after:translate-x-4"></div>
          </label>
        </div>

        <!-- Chart autorefresh -->
        <div class="flex items-center justify-between p-3 bg-surface-800 rounded border border-surface-600">
          <div>
            <div class="text-sm text-gray-200 font-medium">Chart / log auto-refresh</div>
            <div class="text-xs text-gray-500 mt-0.5">How often the Tasks and Logs pages re-fetch data (milliseconds). Requires page reload.</div>
          </div>
          <div class="flex items-center gap-2 ml-4 shrink-0">
            <input
              type="number" min="1000" step="500"
              class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 w-28 focus:outline-none focus:border-accent"
              value={settings['platform.charts.autorefresh_ms']?.value ?? '5000'}
              oninput={(e) => { const s = settings['platform.charts.autorefresh_ms']; if (s) s.value = e.currentTarget.value }}
            />
            <span class="text-xs text-gray-500">ms</span>
            <button
              class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
              disabled={saving}
              onclick={() => savePlatform('platform.charts.autorefresh_ms', settings['platform.charts.autorefresh_ms']?.value ?? '5000')}
            >Save</button>
          </div>
        </div>
      </div>
    </section>

    <!-- Orchestrator -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">Orchestrator</h2>
      <p class="text-xs text-gray-400 mb-4">Behaviour for the orchestrator role.</p>

      <div class="p-3 bg-surface-800 rounded border border-surface-600">
        <div class="text-sm text-gray-200 font-medium">Re-sync scope prompt</div>
        <div class="text-xs text-gray-500 mt-0.5 mb-2">Task description handed to the orchestrator when a project's "Re-sync scope" action runs. It should tell the agent to read the description, reconcile requirements and features, then create work packages.</div>
        <textarea
          rows="6"
          class="w-full bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 font-mono focus:outline-none focus:border-accent"
          value={settingVal('orchestrator.resync_prompt')}
          oninput={(e) => { const s = settings['orchestrator.resync_prompt']; if (s) s.value = e.currentTarget.value }}
        ></textarea>
        <div class="flex justify-end mt-2">
          <button
            class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
            disabled={saving}
            onclick={() => saveDefault('orchestrator.resync_prompt')}
          >Save</button>
        </div>
      </div>
    </section>

    <!-- Task Types -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">Task Types</h2>
      <p class="text-xs text-gray-400 mb-4">Each type sets the branch name an agent creates for its tasks via a template. Placeholders: <code>{'{slug}'}</code> (from the title), <code>{'{shortid}'}</code>, <code>{'{id}'}</code>, <code>{'{type}'}</code>. One type is the default.</p>

      <div class="flex flex-col gap-2">
        {#each taskTypes as tt (tt.id)}
          <div class="p-3 bg-surface-800 rounded border border-surface-600">
            {#if editingTypeId === tt.id}
              <div class="grid grid-cols-3 gap-2">
                <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 focus:outline-none focus:border-accent" placeholder="key" bind:value={typeBuf.key} />
                <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 focus:outline-none focus:border-accent" placeholder="Label" bind:value={typeBuf.label} />
                <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 font-mono focus:outline-none focus:border-accent" placeholder="feature/{'{slug}'}" bind:value={typeBuf.branch_template} />
              </div>
              <div class="flex justify-end gap-2 mt-2">
                <button class="px-3 py-1 text-xs text-gray-400 hover:text-gray-200" onclick={() => editingTypeId = ''}>Cancel</button>
                <button class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors" onclick={() => saveType(tt.id)}>Save</button>
              </div>
            {:else}
              <div class="flex items-center justify-between">
                <div class="min-w-0">
                  <div class="text-sm text-gray-200 font-medium">
                    {tt.label}
                    <span class="text-xs text-gray-500">({tt.key})</span>
                    {#if tt.is_default}<span class="ml-2 text-[10px] px-1.5 py-0.5 rounded-full bg-accent/20 text-accent">default</span>{/if}
                  </div>
                  <div class="text-xs text-gray-500 font-mono mt-0.5 truncate">{tt.branch_template}</div>
                </div>
                <div class="flex items-center gap-2 ml-4 shrink-0">
                  {#if !tt.is_default}
                    <button class="text-xs text-gray-400 hover:text-accent" onclick={() => setDefaultType(tt)}>Set default</button>
                  {/if}
                  <button class="text-xs text-gray-400 hover:text-gray-200" onclick={() => startEditType(tt)}>Edit</button>
                  {#if !tt.is_default}
                    <button class="text-xs text-gray-500 hover:text-red-400" onclick={() => removeType(tt.id)}>Delete</button>
                  {/if}
                </div>
              </div>
            {/if}
          </div>
        {/each}

        <!-- Add new -->
        <div class="p-3 bg-surface-800 rounded border border-dashed border-surface-600">
          <div class="grid grid-cols-3 gap-2">
            <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 focus:outline-none focus:border-accent" placeholder="key (e.g. bug)" bind:value={newType.key} />
            <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 focus:outline-none focus:border-accent" placeholder="Label" bind:value={newType.label} />
            <input class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 font-mono focus:outline-none focus:border-accent" placeholder="bug/{'{slug}'}" bind:value={newType.branch_template} />
          </div>
          <div class="flex justify-end mt-2">
            <button class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors" onclick={createType}>Add task type</button>
          </div>
        </div>
      </div>
    </section>

    <!-- LLM Resilience -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">LLM Resilience</h2>
      <p class="text-xs text-gray-400 mb-4">How agents handle failing LLM calls. Applied on the agent's next provider sync.</p>

      <div class="flex flex-col gap-3">
        {#each [
          { key: 'llm.max_retries', label: 'Max retries', desc: 'Extra attempts for a failing LLM call before the round fails.', def: '3' },
          { key: 'llm.retry_backoff_ms', label: 'Retry backoff (ms)', desc: 'Base wait between retries; grows linearly per attempt.', def: '1000' },
          { key: 'llm.circuit_breaker_threshold', label: 'Breaker threshold', desc: 'Consecutive provider failures before the circuit opens.', def: '5' },
          { key: 'llm.circuit_breaker_reset_sec', label: 'Breaker reset (s)', desc: 'Seconds the circuit stays open before a probe is allowed.', def: '30' },
        ] as f}
          <div class="flex items-center justify-between p-3 bg-surface-800 rounded border border-surface-600">
            <div>
              <div class="text-sm text-gray-200 font-medium">{f.label}</div>
              <div class="text-xs text-gray-500 mt-0.5">{f.desc}</div>
            </div>
            <div class="flex items-center gap-2 ml-4 shrink-0">
              <input
                type="number" min="0"
                class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 w-24 focus:outline-none focus:border-accent"
                value={settingVal(f.key) || f.def}
                oninput={(e) => { const s = settings[f.key]; if (s) s.value = e.currentTarget.value }}
              />
              <button
                class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
                disabled={saving}
                onclick={() => savePlatform(f.key, settingVal(f.key) || f.def)}
              >Save</button>
            </div>
          </div>
        {/each}

        <div class="flex items-center justify-between p-3 bg-surface-800 rounded border border-surface-600">
          <div>
            <div class="text-sm text-gray-200 font-medium">Fallback provider</div>
            <div class="text-xs text-gray-500 mt-0.5">Provider name to fail over to when the primary keeps failing (empty = none).</div>
          </div>
          <div class="flex items-center gap-2 ml-4 shrink-0">
            <input
              type="text"
              class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 w-40 focus:outline-none focus:border-accent"
              value={settingVal('llm.fallback_provider')}
              oninput={(e) => { const s = settings['llm.fallback_provider']; if (s) s.value = e.currentTarget.value }}
            />
            <button
              class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
              disabled={saving}
              onclick={() => savePlatform('llm.fallback_provider', settingVal('llm.fallback_provider'))}
            >Save</button>
          </div>
        </div>
      </div>
    </section>

    <!-- Agent Log Retention -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">Agent Log Retention</h2>
      <p class="text-xs text-gray-400 mb-3">Note: rows for short-retention types may persist up to 24 h extra (partition boundary).</p>

      <div class="flex items-center gap-3 mb-4">
        <label class="text-sm text-gray-400 w-40 shrink-0">Default (days)</label>
        <input type="number" min="1"
          class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 w-24 focus:outline-none focus:border-accent"
          value={settings['log.retention.agent.default_days']?.value ?? ''}
          oninput={(e) => { const s = settings['log.retention.agent.default_days']; if (s) s.value = e.currentTarget.value }}
        />
        <button class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
          disabled={saving}
          onclick={() => saveDefault('log.retention.agent.default_days')}>Save</button>
      </div>

      <div class="rounded border border-surface-600 overflow-hidden">
        <table class="w-full text-xs">
          <thead class="bg-surface-700">
            <tr>
              <th class="text-left px-3 py-2 text-gray-400 font-medium">Event Type</th>
              <th class="text-left px-3 py-2 text-gray-400 font-medium">Description</th>
              <th class="text-left px-3 py-2 text-gray-400 font-medium w-28">Override (days)</th>
              <th class="text-left px-3 py-2 text-gray-400 font-medium w-36">Effective</th>
              <th class="px-3 py-2 w-16"></th>
            </tr>
          </thead>
          <tbody>
            {#each AGENT_EVENTS as evt}
              <tr class="border-t border-surface-700 hover:bg-surface-700/30">
                <td class="px-3 py-2 font-mono text-gray-300">{evt.key}</td>
                <td class="px-3 py-2 text-gray-500">{evt.desc}</td>
                <td class="px-3 py-2">
                  <input type="number" min="1" placeholder="—"
                    class="bg-surface-700 border border-surface-600 rounded px-2 py-0.5 text-gray-200 w-20 focus:outline-none focus:border-accent"
                    bind:value={overrides['agent_' + evt.key]}
                  />
                </td>
                <td class="px-3 py-2 text-gray-500">{effectiveAgentDays(evt.key)}</td>
                <td class="px-3 py-2">
                  <button class="px-2 py-0.5 bg-surface-600 hover:bg-surface-500 text-gray-300 rounded text-xs disabled:opacity-40"
                    disabled={saving}
                    onclick={() => saveAgentOverride(evt.key)}>Save</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>

    <!-- Task Log Retention -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">Task Log Retention</h2>
      <p class="text-xs text-gray-400 mb-3">Note: rows for short-retention types may persist up to 24 h extra (partition boundary).</p>

      <div class="flex items-center gap-3 mb-4">
        <label class="text-sm text-gray-400 w-40 shrink-0">Default (days)</label>
        <input type="number" min="1"
          class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 w-24 focus:outline-none focus:border-accent"
          value={settings['log.retention.task.default_days']?.value ?? ''}
          oninput={(e) => { const s = settings['log.retention.task.default_days']; if (s) s.value = e.currentTarget.value }}
        />
        <button class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
          disabled={saving}
          onclick={() => saveDefault('log.retention.task.default_days')}>Save</button>
      </div>

      <div class="rounded border border-surface-600 overflow-hidden">
        <table class="w-full text-xs">
          <thead class="bg-surface-700">
            <tr>
              <th class="text-left px-3 py-2 text-gray-400 font-medium">Event Type</th>
              <th class="text-left px-3 py-2 text-gray-400 font-medium">Description</th>
              <th class="text-left px-3 py-2 text-gray-400 font-medium w-28">Override (days)</th>
              <th class="text-left px-3 py-2 text-gray-400 font-medium w-36">Effective</th>
              <th class="px-3 py-2 w-16"></th>
            </tr>
          </thead>
          <tbody>
            {#each TASK_EVENTS as evt}
              <tr class="border-t border-surface-700 hover:bg-surface-700/30">
                <td class="px-3 py-2 font-mono text-gray-300">{evt.key}</td>
                <td class="px-3 py-2 text-gray-500">{evt.desc}</td>
                <td class="px-3 py-2">
                  <input type="number" min="1" placeholder="—"
                    class="bg-surface-700 border border-surface-600 rounded px-2 py-0.5 text-gray-200 w-20 focus:outline-none focus:border-accent"
                    bind:value={overrides['task_' + evt.key]}
                  />
                </td>
                <td class="px-3 py-2 text-gray-500">{effectiveTaskDays(evt.key)}</td>
                <td class="px-3 py-2">
                  <button class="px-2 py-0.5 bg-surface-600 hover:bg-surface-500 text-gray-300 rounded text-xs disabled:opacity-40"
                    disabled={saving}
                    onclick={() => saveTaskOverride(evt.key)}>Save</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>

    <!-- System Log Retention -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-2">System Log Retention</h2>
      <div class="flex items-center gap-3">
        <label class="text-sm text-gray-400 w-40 shrink-0">Default (days)</label>
        <input type="number" min="1"
          class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 w-24 focus:outline-none focus:border-accent"
          value={settings['log.retention.system.default_days']?.value ?? ''}
          oninput={(e) => { const s = settings['log.retention.system.default_days']; if (s) s.value = e.currentTarget.value }}
        />
        <button class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded transition-colors disabled:opacity-40"
          disabled={saving}
          onclick={() => saveDefault('log.retention.system.default_days')}>Save</button>
        {#if settings['log.retention.system.default_days']?.updated_at}
          <span class="text-xs text-gray-600">Updated {new Date(settings['log.retention.system.default_days'].updated_at).toLocaleDateString()}</span>
        {/if}
      </div>
    </section>

    <!-- ── Checklist Templates ──────────────────────────────────────────────── -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">Checklist Templates</h2>
      <p class="text-xs text-gray-400 mb-4">Reusable item lists you can apply to any task's checklist.</p>

      {#if templates.length > 0}
        <div class="flex flex-col gap-2 mb-4">
          {#each templates as tpl (tpl.id)}
            <div class="p-3 bg-surface-800 rounded border border-surface-600">
              {#if editingTplId === tpl.id}
                <div class="flex flex-col gap-2">
                  <input
                    class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 focus:outline-none focus:border-accent"
                    bind:value={tplBuf.name}
                    placeholder="Template name"
                  />
                  <textarea
                    class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs text-gray-200 focus:outline-none focus:border-accent resize-none font-mono"
                    rows="4"
                    placeholder="One item per line…"
                    bind:value={tplBuf.items}
                  ></textarea>
                  <div class="flex gap-2 justify-end">
                    <button class="text-xs text-gray-400 hover:text-gray-200" onclick={() => editingTplId = ''}>Cancel</button>
                    <button class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded" onclick={() => saveTemplate(tpl.id)}>Save</button>
                  </div>
                </div>
              {:else}
                {@const items = (() => { try { return JSON.parse(tpl.items_json || '[]') } catch { return [] } })()}
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <p class="text-sm font-medium text-gray-200">{tpl.name}</p>
                    <p class="text-xs text-gray-400 mt-0.5">{items.length} item{items.length !== 1 ? 's' : ''}{items.length > 0 ? ': ' + items.slice(0,3).join(', ') + (items.length > 3 ? '…' : '') : ''}</p>
                  </div>
                  <div class="flex gap-2 shrink-0">
                    <button
                      class="text-xs text-blue-400 hover:text-blue-300"
                      onclick={() => {
                        editingTplId = tpl.id
                        const items = JSON.parse(tpl.items_json || '[]')
                        tplBuf = { name: tpl.name, items: items.join('\n') }
                      }}
                    >Edit</button>
                    <button class="text-xs text-red-400 hover:text-red-300" onclick={() => removeTemplate(tpl.id)}>Delete</button>
                  </div>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {:else}
        <p class="text-xs text-gray-400 mb-4">No templates yet.</p>
      {/if}

      <!-- Create new template -->
      <div class="p-3 bg-surface-800 rounded border border-surface-600 flex flex-col gap-2">
        <p class="text-xs font-medium text-gray-400">New Template</p>
        <input
          class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-sm text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
          placeholder="Template name"
          bind:value={newTplName}
        />
        <textarea
          class="bg-surface-700 border border-surface-500 rounded px-2 py-1 text-xs text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent resize-none font-mono"
          rows="4"
          placeholder="One item per line…"
          bind:value={newTplItems}
        ></textarea>
        <div class="flex justify-end">
          <button
            class="px-3 py-1 bg-accent hover:bg-accent-hover text-white text-xs rounded disabled:opacity-40"
            disabled={!newTplName.trim()}
            onclick={createTemplate}
          >Create</button>
        </div>
      </div>
    </section>
  {/if}
</div>
