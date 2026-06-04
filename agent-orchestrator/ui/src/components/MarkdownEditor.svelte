<script>
  import { marked }    from "marked"
  import DOMPurify     from "dompurify"
  import { onMount }   from "svelte"

  /**
   * Props (same API as before):
   *   value      — bound markdown string
   *   placeholder — hint shown when empty
   *   minHeight  — CSS min-height (default: '160px')
   *   readonly   — render-only, no editing
   */
  let {
    value = $bindable(""),
    placeholder = "Write here…",
    minHeight   = "160px",
    readonly    = false,
  } = $props()

  let editorEl  = $state(null)   // bind:this on the contenteditable div
  let isFocused = $state(false)

  // ── HTML ↔ Markdown ────────────────────────────────────────────────────────

  function toHTML(md) {
    if (!md) return ''
    return DOMPurify.sanitize(
      marked.parse(md, { breaks: true, gfm: true }),
      { ADD_ATTR: ['target'] }
    )
  }

  // Minimal HTML → Markdown serialiser that covers the cases `marked` produces.
  function nodeToMd(node) {
    if (node.nodeType === Node.TEXT_NODE) {
      return node.textContent
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return ''

    const tag      = node.tagName.toLowerCase()
    const children = [...node.childNodes].map(nodeToMd).join('')

    switch (tag) {
      case 'p':         return children + '\n\n'
      case 'br':        return '\n'
      case 'strong': case 'b': return `**${children}**`
      case 'em': case 'i':     return `_${children}_`
      case 'h1':        return `# ${children}\n`
      case 'h2':        return `## ${children}\n`
      case 'h3':        return `### ${children}\n`
      case 'h4':        return `#### ${children}\n`
      case 'h5':        return `##### ${children}\n`
      case 'h6':        return `###### ${children}\n`
      case 'a': {
        const href = node.getAttribute('href') || ''
        return `[${children}](${href})`
      }
      case 'code': {
        // Inline code — parent is not <pre>
        if (node.parentElement?.tagName.toLowerCase() !== 'pre') {
          return `\`${children}\``
        }
        return children
      }
      case 'pre': {
        const codeEl = node.querySelector('code')
        const lang   = (codeEl?.className || '').replace('language-', '')
        const text   = codeEl?.textContent ?? children
        return `\`\`\`${lang}\n${text}\n\`\`\`\n`
      }
      case 'ul': {
        return [...node.children].map(li => `- ${nodeToMd(li).trim()}`).join('\n') + '\n'
      }
      case 'ol': {
        return [...node.children].map((li, i) => `${i + 1}. ${nodeToMd(li).trim()}`).join('\n') + '\n'
      }
      case 'li':        return children
      case 'blockquote': return children.split('\n').map(l => `> ${l}`).join('\n') + '\n'
      case 'hr':        return '\n---\n'
      case 'div':       return children + '\n'
      // Skip wrapper elements — just return children.
      default:          return children
    }
  }

  function htmlToMd(html) {
    const wrapper = document.createElement('div')
    wrapper.innerHTML = html
    return [...wrapper.childNodes].map(nodeToMd).join('').replace(/\n{3,}/g, '\n\n').trimEnd()
  }

  // ── Sync: value → editor ───────────────────────────────────────────────────
  // Called on mount and whenever `value` changes from outside while not focused.
  function setEditorContent(md) {
    if (!editorEl) return
    const html = toHTML(md)
    // Only update if content actually changed (avoids cursor jumping).
    if (editorEl.innerHTML !== html) {
      editorEl.innerHTML = html || ''
    }
  }

  onMount(() => {
    setEditorContent(value)
  })

  // Propagate external value changes into the editor (e.g. form reset).
  // While focused we skip syncing to avoid cursor jumps during typing, but an
  // external reset to empty (e.g. the chat clearing the box after Enter-to-send,
  // which keeps focus) must still clear the editor.
  $effect(() => {
    if (!isFocused) {
      setEditorContent(value)
    } else if (value === '') {
      setEditorContent('')
    }
  })

  // ── Sync: editor → value ───────────────────────────────────────────────────
  function onInput() {
    if (!editorEl) return
    value = htmlToMd(editorEl.innerHTML)
  }

  // ── Toolbar ────────────────────────────────────────────────────────────────
  const TOOLBAR = [
    { label: 'B',   title: 'Bold',          cmd: 'bold' },
    { label: 'I',   title: 'Italic',        cmd: 'italic' },
    { label: 'H2',  title: 'Heading 2',     cmd: 'formatBlock', val: 'h2' },
    { label: 'H3',  title: 'Heading 3',     cmd: 'formatBlock', val: 'h3' },
    { sep: true },
    { label: 'UL',  title: 'Bullet list',   cmd: 'insertUnorderedList' },
    { label: 'OL',  title: 'Numbered list', cmd: 'insertOrderedList' },
    { label: '>',   title: 'Blockquote',    cmd: 'formatBlock', val: 'blockquote' },
    { sep: true },
    { label: '`',   title: 'Inline code',   wrap: ['`', '`'] },
    { label: '🔗',  title: 'Link',          link: true },
  ]

  function runCmd(item) {
    if (!editorEl) return
    editorEl.focus()
    if (item.wrap) {
      const sel   = window.getSelection()
      const range = sel?.getRangeAt(0)
      if (range) {
        const text = range.toString()
        document.execCommand('insertText', false, item.wrap[0] + text + item.wrap[1])
      }
    } else if (item.link) {
      const url = prompt('URL:', 'https://')
      if (url) document.execCommand('createLink', false, url)
    } else if (item.val) {
      document.execCommand(item.cmd, false, item.val)
    } else {
      document.execCommand(item.cmd, false, null)
    }
    onInput()
  }

  function onKeyDown(e) {
    // Tab → indent (prevent focus loss)
    if (e.key === 'Tab') {
      e.preventDefault()
      document.execCommand('insertText', false, '  ')
    }
  }
</script>

<div class="markdown-editor flex flex-col border border-surface-500 rounded overflow-hidden">
  {#if !readonly}
    <!-- Toolbar -->
    <div class="flex items-center gap-0.5 px-2 py-1 bg-surface-800 border-b border-surface-600 flex-wrap shrink-0">
      {#each TOOLBAR as item}
        {#if item.sep}
          <span class="w-px h-4 bg-surface-600 mx-1"></span>
        {:else}
          <button
            type="button"
            title={item.title}
            class="px-1.5 py-0.5 text-xs font-mono text-gray-400 hover:text-gray-200
                   hover:bg-surface-700 rounded transition-colors"
            onmousedown={(e) => { e.preventDefault(); runCmd(item) }}
          >{item.label}</button>
        {/if}
      {/each}
    </div>
  {/if}

  <!-- Single editing pane — contenteditable renders markdown inline -->
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_noninteractive_element_interactions -->
  <div
    bind:this={editorEl}
    role={readonly ? 'document' : 'textbox'}
    aria-multiline="true"
    aria-label={placeholder}
    contenteditable={readonly ? false : true}
    class="flex-1 overflow-y-auto p-3 bg-surface-900 text-gray-200 text-sm leading-relaxed
           prose prose-invert prose-sm max-w-none focus:outline-none"
    style="min-height:{minHeight}"
    oninput={onInput}
    onfocus={() => isFocused = true}
    onblur={() => isFocused = false}
    onkeydown={onKeyDown}
    data-placeholder={placeholder}
  ></div>
</div>

<style>
  /* Placeholder via CSS ::before when empty */
  [contenteditable][data-placeholder]:empty::before {
    content: attr(data-placeholder);
    color: #6b7280;
    pointer-events: none;
    font-style: italic;
  }

  /* Prose styles */
  .prose :global(h1), .prose :global(h2), .prose :global(h3) {
    color: #e5e7eb;
    font-weight: 600;
    margin: 0.75em 0 0.5em;
    line-height: 1.3;
  }
  .prose :global(h1) { font-size: 1.25rem; }
  .prose :global(h2) { font-size: 1.1rem; }
  .prose :global(h3) { font-size: 1rem; }
  .prose :global(p)  { margin: 0.5em 0; }
  .prose :global(ul), .prose :global(ol) { margin: 0.5em 0; padding-left: 1.5em; }
  .prose :global(li) { margin: 0.2em 0; }
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
  .prose :global(pre code) { background: none; padding: 0; }
  .prose :global(blockquote) {
    border-left: 3px solid #4b5563;
    padding-left: 0.75em;
    color: #9ca3af;
    margin: 0.5em 0;
  }
  .prose :global(a)      { color: #60a5fa; text-decoration: underline; }
  .prose :global(hr)     { border-color: #374151; margin: 1em 0; }
  .prose :global(strong) { color: #f3f4f6; font-weight: 600; }
  .prose :global(em)     { font-style: italic; color: #d1d5db; }
  .prose :global(table)  { width: 100%; border-collapse: collapse; font-size: 0.85em; }
  .prose :global(th), .prose :global(td) {
    border: 1px solid #374151;
    padding: 0.3em 0.6em;
    text-align: left;
  }
  .prose :global(th) { background: #1f2937; color: #e5e7eb; }
</style>
