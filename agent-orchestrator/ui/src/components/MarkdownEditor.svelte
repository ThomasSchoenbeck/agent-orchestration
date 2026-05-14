<script>
  import { marked } from "marked"
  import DOMPurify from "dompurify"

  /**
   * Props:
   *   value      — bound string (markdown source)
   *   placeholder — textarea placeholder text
   *   minHeight  — CSS min-height for the editor panes (default: '160px')
   *   readonly   — if true, renders preview only with no editor pane
   */
  let {
    value = $bindable(""),
    placeholder = "Write markdown…",
    minHeight = "160px",
    readonly = false,
  } = $props()

  let preview = $state(false) // toggle: false = split, true = preview-only

  // Rendered HTML, sanitised
  let html = $derived(DOMPurify.sanitize(marked.parse(value || "", { breaks: true, gfm: true })))

  const toolbarActions = [
    { label: "B", title: "Bold", wrap: ["**", "**"] },
    { label: "I", title: "Italic", wrap: ["_", "_"] },
    { label: "H", title: "Heading", prefix: "## " },
    { label: "`", title: "Inline code", wrap: ["`", "`"] },
    { label: "```", title: "Code block", wrap: ["```\n", "\n```"] },
    { label: "—", title: "Separator", separator: true },
    { label: "🔗", title: "Link", wrap: ["[", "](url)"] },
    { label: "UL", title: "Bullet list", prefix: "- " },
    { label: "OL", title: "Numbered list", prefix: "1. " },
    { label: ">", title: "Blockquote", prefix: "> " },
  ]

  let textarea = $state()

  function applyAction(action) {
    if (!textarea) return
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const sel = value.slice(start, end)

    let replacement
    if (action.prefix) {
      replacement = action.prefix + sel
    } else if (action.wrap) {
      replacement = action.wrap[0] + sel + action.wrap[1]
    } else {
      return
    }

    value = value.slice(0, start) + replacement + value.slice(end)

    // Restore cursor after Svelte re-renders the textarea
    requestAnimationFrame(() => {
      textarea.focus()
      const cursor = action.prefix
        ? start + replacement.length
        : start + action.wrap[0].length + sel.length
      textarea.setSelectionRange(cursor, cursor)
    })
  }
</script>

<div class="markdown-editor flex flex-col border border-surface-500 rounded overflow-hidden">
  <!-- Toolbar -->
  {#if !readonly}
    <div
      class="flex items-center gap-1 px-2 py-1 bg-surface-800 border-b border-surface-600 flex-wrap"
    >
      {#each toolbarActions as action}
        {#if action.separator}
          <span class="w-px h-4 bg-surface-600 mx-1"></span>
        {:else}
          <button
            type="button"
            title={action.title}
            class="px-1.5 py-0.5 text-xs font-mono text-gray-400 hover:text-gray-200
                   hover:bg-surface-700 rounded transition-colors"
            onclick={() => applyAction(action)}>{action.label}</button
          >
        {/if}
      {/each}
      <span class="flex-1"></span>
      <button
        type="button"
        class="text-xs px-2 py-0.5 rounded transition-colors
               {preview ? 'bg-surface-600 text-gray-200' : 'text-gray-500 hover:text-gray-300'}"
        onclick={() => (preview = !preview)}
        title="Toggle preview">{preview ? "Edit" : "Preview"}</button
      >
    </div>
  {/if}

  <!-- Panes -->
  <div class="flex flex-1" style="min-height:{minHeight}">
    <!-- Editor pane -->
    {#if !readonly && !preview}
      <textarea
        bind:this={textarea}
        bind:value
        {placeholder}
        class="flex-1 bg-surface-900 text-gray-200 text-sm p-3 resize-none
               focus:outline-none font-mono leading-relaxed"
        style="min-height:{minHeight}"
      ></textarea>
      <!-- Divider -->
      <div class="w-px bg-surface-600 shrink-0"></div>
    {/if}

    <!-- Preview pane -->
    <div
      class="flex-1 overflow-y-auto p-3 bg-surface-900 prose prose-invert prose-sm max-w-none
             text-gray-300 text-sm leading-relaxed"
      style="min-height:{minHeight}"
    >
      {#if html}
        <!-- eslint-disable-next-line svelte/no-at-html-tags -->
        {@html html}
      {:else}
        <span class="text-gray-600 italic">{readonly ? "No content." : "Preview appears here"}</span
        >
      {/if}
    </div>
  </div>
</div>

<style>
  /* Basic markdown prose styles since we don't have @tailwindcss/typography */
  .prose :global(h1),
  .prose :global(h2),
  .prose :global(h3) {
    color: #e5e7eb;
    font-weight: 600;
    margin: 0.75em 0 0.5em;
    line-height: 1.3;
  }
  .prose :global(h1) {
    font-size: 1.25rem;
  }
  .prose :global(h2) {
    font-size: 1.1rem;
  }
  .prose :global(h3) {
    font-size: 1rem;
  }
  .prose :global(p) {
    margin: 0.5em 0;
  }
  .prose :global(ul),
  .prose :global(ol) {
    margin: 0.5em 0;
    padding-left: 1.5em;
  }
  .prose :global(li) {
    margin: 0.2em 0;
  }
  .prose :global(code) {
    background: #374151;
    padding: 0.1em 0.3em;
    border-radius: 3px;
    font-size: 0.85em;
    font-family: monospace;
  }
  .prose :global(pre) {
    background: #1f2937;
    padding: 0.75em 1em;
    border-radius: 6px;
    overflow-x: auto;
    margin: 0.5em 0;
  }
  .prose :global(pre code) {
    background: none;
    padding: 0;
  }
  .prose :global(blockquote) {
    border-left: 3px solid #4b5563;
    padding-left: 0.75em;
    color: #9ca3af;
    margin: 0.5em 0;
  }
  .prose :global(a) {
    color: #60a5fa;
    text-decoration: underline;
  }
  .prose :global(hr) {
    border-color: #374151;
    margin: 1em 0;
  }
  .prose :global(strong) {
    color: #f3f4f6;
    font-weight: 600;
  }
  .prose :global(em) {
    font-style: italic;
    color: #d1d5db;
  }
  .prose :global(table) {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.85em;
  }
  .prose :global(th),
  .prose :global(td) {
    border: 1px solid #374151;
    padding: 0.3em 0.6em;
    text-align: left;
  }
  .prose :global(th) {
    background: #1f2937;
    color: #e5e7eb;
  }
</style>
