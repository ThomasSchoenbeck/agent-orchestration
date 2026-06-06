<script>
  /**
   * Searchable multi-select tag input.
   *   value     — bound array of selected string values
   *   options   — available suggestions: array of strings or {value,label,description}
   *   placeholder
   *   allowFree — allow adding values not present in options (default true)
   */
  let {
    value = $bindable([]),
    options = [],
    placeholder = 'Search…',
    allowFree = true,
    labelFor = (v) => v, // map a selected value to its display label (e.g. id → name)
  } = $props()

  let query  = $state('')
  let open   = $state(false)
  let active = $state(0)

  const norm = (o) => {
    if (typeof o === 'string') return { value: o, label: o }
    const v = o?.value ?? o?.name ?? ''
    return { value: v, label: String(o?.label ?? o?.name ?? v), description: o?.description }
  }
  let normOptions = $derived(options.map(norm).filter(o => o.value))
  let filtered = $derived(
    normOptions
      .filter(o => !value.includes(o.value))
      .filter(o => o.label.toLowerCase().includes(query.trim().toLowerCase()))
      .slice(0, 50)
  )

  function add(val) {
    const v = (val ?? '').trim()
    if (!v) return
    if (!value.includes(v)) value = [...value, v]
    query = ''
    active = 0
  }
  function remove(v) { value = value.filter(x => x !== v) }

  // Commit the current entry: prefer an exact option match, then the highlighted
  // filtered option, then free text (when allowed).
  function commit(e) {
    const q = query.trim()
    const exact = normOptions.find(o => o.value.toLowerCase() === q.toLowerCase() && !value.includes(o.value))
    const pick = exact || filtered[active]
    if (pick) { e.preventDefault(); add(pick.value) }
    else if (allowFree && q) { e.preventDefault(); add(q) }
  }

  function onKey(e) {
    if (e.key === 'Enter' || e.key === 'Tab' || (e.key === ' ' && query.trim())) {
      commit(e)
    } else if (e.key === 'Backspace' && !query && value.length) {
      remove(value[value.length - 1])
    } else if (e.key === 'ArrowDown') {
      e.preventDefault(); active = Math.min(active + 1, Math.max(0, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault(); active = Math.max(active - 1, 0)
    } else if (e.key === 'Escape') {
      open = false
    }
  }
</script>

<div class="relative">
  <div
    class="flex flex-wrap gap-1 items-center bg-surface-700 border border-surface-500 rounded px-2 py-1.5 text-sm focus-within:border-accent"
  >
    {#each value as v (v)}
      <span class="flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface-600 text-gray-200 text-xs">
        {labelFor(v)}
        <button type="button" class="text-gray-400 hover:text-rose-300 leading-none" aria-label={`Remove ${labelFor(v)}`} onclick={() => remove(v)}>×</button>
      </span>
    {/each}
    <input
      class="flex-1 min-w-[6rem] bg-transparent outline-none text-gray-200 placeholder-gray-500"
      placeholder={value.length ? '' : placeholder}
      bind:value={query}
      onkeydown={onKey}
      onfocus={() => open = true}
      onblur={() => setTimeout(() => open = false, 120)}
    />
  </div>

  {#if open && filtered.length > 0}
    <ul role="listbox" class="absolute z-20 mt-1 w-full max-h-48 overflow-y-auto bg-surface-800 border border-surface-600 rounded shadow-lg text-sm">
      {#each filtered as o, i (o.value)}
        <li role="option" aria-selected={i === active}>
          <button
            type="button"
            class="w-full text-left px-3 py-1.5 hover:bg-surface-700 {i === active ? 'bg-surface-700' : ''}"
            onmousedown={(e) => { e.preventDefault(); add(o.value) }}
            onmouseenter={() => active = i}
          >
            <span class="text-gray-200">{o.label}</span>
            {#if o.description}<span class="text-gray-500 text-xs ml-2">{o.description}</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
