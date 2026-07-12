<script>
  // Ordered provider>model priority list editor (Phase 5, T5.8). Each entry is
  // { provider, model } — both referenced by name. The router walks the list in
  // order and fails over to the next entry on error/unavailability. Rows render
  // only for existing entries, so an empty list adds no form controls.
  let { value = $bindable([]), providers = [] } = $props()

  function providerModels(name) {
    const p = providers.find((p) => p.name === name)
    return Array.isArray(p?.models) ? p.models : []
  }
  function addRow() {
    value = [...value, { provider: '', model: '' }]
  }
  function removeRow(i) {
    value = value.filter((_, j) => j !== i)
  }
  function move(i, delta) {
    const j = i + delta
    if (j < 0 || j >= value.length) return
    const next = [...value]
    ;[next[i], next[j]] = [next[j], next[i]]
    value = next
  }
</script>

<div class="flex flex-col gap-2">
  {#if value.length > 0}
    <div class="flex flex-col gap-1.5">
      {#each value as row, i}
        <div class="flex items-center gap-2">
          <span class="text-xs text-gray-600 w-5 text-right">{i + 1}.</span>
          <select
            aria-label="priority provider"
            class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs
                   text-gray-200 focus:outline-none focus:border-accent"
            bind:value={row.provider}
          >
            <option value="">— provider —</option>
            {#each providers as p}
              <option value={p.name}>{p.name}</option>
            {/each}
          </select>
          {#if providerModels(row.provider).length > 0}
            <select
              aria-label="priority model"
              class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs
                     text-gray-200 font-mono focus:outline-none focus:border-accent"
              bind:value={row.model}
            >
              <option value="">— model —</option>
              {#each providerModels(row.provider) as m}
                <option value={m.name}>{m.name}</option>
              {/each}
            </select>
          {:else}
            <input
              aria-label="priority model"
              class="flex-1 bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-xs
                     text-gray-200 font-mono placeholder-gray-500 focus:outline-none focus:border-accent"
              placeholder="model name"
              bind:value={row.model}
            />
          {/if}
          <button
            type="button"
            class="text-xs text-gray-500 hover:text-gray-300 disabled:opacity-30"
            title="Move up"
            disabled={i === 0}
            onclick={() => move(i, -1)}
          >↑</button>
          <button
            type="button"
            class="text-xs text-gray-500 hover:text-gray-300 disabled:opacity-30"
            title="Move down"
            disabled={i === value.length - 1}
            onclick={() => move(i, 1)}
          >↓</button>
          <button
            type="button"
            class="text-red-400 hover:text-red-300 text-xs"
            title="Remove"
            onclick={() => removeRow(i)}
          >✕</button>
        </div>
      {/each}
    </div>
  {/if}
  <button
    type="button"
    class="self-start text-xs text-accent hover:text-accent-hover"
    onclick={addRow}
  >+ Add route</button>
</div>
