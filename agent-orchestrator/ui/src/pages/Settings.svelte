<script>
  import { onMount } from 'svelte'
  import { listSettings, updateSetting } from '../lib/api.js'
  import { toasts } from '../lib/stores.js'

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

  let settings = $state({})
  let overrides = $state({})
  let loading = $state(true)
  let saving = $state(false)

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

  onMount(load)
</script>

<div class="overflow-y-auto p-6 max-w-4xl mx-auto">
  <h1 class="text-xl font-semibold text-gray-100 mb-6">Settings</h1>

  {#if loading}
    <p class="text-gray-500 text-sm">Loading…</p>
  {:else}
    <!-- Agent Log Retention -->
    <section class="mb-8">
      <h2 class="text-base font-semibold text-gray-200 mb-1">Agent Log Retention</h2>
      <p class="text-xs text-gray-500 mb-3">Note: rows for short-retention types may persist up to 24 h extra (partition boundary).</p>

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
      <p class="text-xs text-gray-500 mb-3">Note: rows for short-retention types may persist up to 24 h extra (partition boundary).</p>

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
  {/if}
</div>
