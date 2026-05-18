<!--
  Skeleton loading placeholder.

  Props:
    rows  - number of animated rows to show (default 3)
    mode  - 'card'  : padded card-like rows (px-6 pb-4)
            'table' : compact table-row-like rows (p-4)
-->
<script>
  let { rows = 3, mode = 'card' } = $props()
</script>

{#if mode === 'card'}
  <div class="px-6 pb-4 flex flex-col gap-2" aria-busy="true" aria-label="Loading">
    {#each Array(rows) as _, i}
      <div class="animate-pulse flex flex-col gap-1.5">
        <div class="h-3 bg-surface-600 rounded w-3/4"></div>
        <div class="h-2.5 bg-surface-700 rounded w-1/2"></div>
      </div>
    {/each}
  </div>
{:else}
  <div class="p-4 flex flex-col gap-3" aria-busy="true" aria-label="Loading">
    {#each Array(rows) as _, i}
      <div class="animate-pulse h-3 bg-surface-700 rounded {i % 3 === 0 ? 'w-full' : i % 3 === 1 ? 'w-4/5' : 'w-2/3'}"></div>
    {/each}
  </div>
{/if}
