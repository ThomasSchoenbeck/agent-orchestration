<script>
  import { router, toasts } from './lib/stores.js'
  import Projects      from './pages/Projects.svelte'
  import ProjectDetail from './pages/ProjectDetail.svelte'
  import Tasks         from './pages/Tasks.svelte'
  import TaskDetail    from './pages/TaskDetail.svelte'
  import Agents        from './pages/Agents.svelte'
  import AgentDetail   from './pages/AgentDetail.svelte'
  import Roles         from './pages/Roles.svelte'
  import Skills        from './pages/Skills.svelte'
  import Providers     from './pages/Providers.svelte'
  import CostDetail    from './pages/CostDetail.svelte'
  import Logs          from './pages/Logs.svelte'
  import Chat          from './pages/Chat.svelte'
  import Settings      from './pages/Settings.svelte'

  const pages = [
    { id: 'projects',  label: 'Projects',  icon: '📁' },
    { id: 'tasks',     label: 'Tasks',     icon: '✅' },
    { id: 'agents',    label: 'Agents',    icon: '🤖' },
    { id: 'roles',     label: 'Roles',     icon: '🎭' },
    { id: 'skills',    label: 'Skills',    icon: '🧩' },
    { id: 'providers', label: 'Providers', icon: '🔌' },
    { id: 'logs',      label: 'Logs',      icon: '📋' },
    { id: 'chat',      label: 'Chat',      icon: '💬' },
    { id: 'settings',  label: 'Settings',  icon: '⚙️' },
  ]
</script>

<div class="flex h-screen bg-surface-900 text-gray-200 overflow-hidden">

  <!-- Sidebar -->
  <nav class="w-48 shrink-0 bg-surface-800 border-r border-surface-600 flex flex-col">
    <div class="px-4 py-5 border-b border-surface-600">
      <span class="text-accent font-semibold text-sm tracking-wide uppercase">Agent Orch.</span>
    </div>
    <ul class="flex-1 py-2 list-none m-0 p-0">
      {#each pages as p}
        <li>
          <a
            href="#/{p.id}"
            class="w-full text-left px-4 py-2 flex items-center gap-3 text-sm no-underline transition-colors
              {$router.page === p.id
                ? 'bg-surface-700 text-accent font-medium'
                : 'text-gray-400 hover:bg-surface-700 hover:text-gray-200'}"
          >
            <span class="text-base leading-none">{p.icon}</span>
            {p.label}
          </a>
        </li>
      {/each}
    </ul>
    <div class="px-4 py-3 border-t border-surface-600 text-xs text-gray-500">
      v0.1.0
    </div>
  </nav>

  <!-- Main content -->
  <main class="flex-1 overflow-hidden flex flex-col">
    {#if $router.page === 'projects' && $router.params.length > 0}
      <ProjectDetail projectId={$router.params[0]} />
    {:else if $router.page === 'projects'}
      <Projects />
    {:else if $router.page === 'tasks' && $router.params.length > 0}
      <TaskDetail taskId={$router.params[0]} />
    {:else if $router.page === 'tasks'}
      <Tasks />
    {:else if $router.page === 'agents' && $router.params.length > 0}
      <AgentDetail agentId={$router.params[0]} />
    {:else if $router.page === 'agents'}
      <Agents />
    {:else if $router.page === 'roles'}
      <Roles />
    {:else if $router.page === 'skills'}
      <Skills />
    {:else if $router.page === 'providers'}
      <Providers />
    {:else if $router.page === 'costs'}
      <CostDetail />
    {:else if $router.page === 'logs'}
      <Logs />
    {:else if $router.page === 'chat'}
      <Chat />
    {:else if $router.page === 'settings'}
      <Settings />
    {:else}
      <div class="flex-1 flex items-center justify-center text-gray-500">Page not found</div>
    {/if}
  </main>
</div>

<!-- Toast notifications -->
{#if $toasts.length > 0}
  <div class="fixed bottom-4 right-4 flex flex-col gap-2 z-50" role="status" aria-live="polite">
    {#each $toasts as t (t.id)}
      <div class="flex items-start gap-3 px-4 py-3 rounded text-sm shadow-lg max-w-xs
        {t.type === 'success' ? 'bg-green-800 text-green-100' :
         t.type === 'error'   ? 'bg-red-900   text-red-100'   :
         t.type === 'warning' ? 'bg-yellow-800 text-yellow-100' :
                                'bg-surface-700 text-gray-200'}">
        <span class="mt-0.5 shrink-0" aria-hidden="true">
          {#if t.type === 'success'}✓{:else if t.type === 'error'}✕{:else if t.type === 'warning'}⚠{:else}ℹ{/if}
        </span>
        <span class="flex-1">{t.message}</span>
        <button
          onclick={() => toasts.remove(t.id)}
          class="shrink-0 opacity-70 hover:opacity-100 leading-none"
          aria-label="Dismiss notification"
        >✕</button>
      </div>
    {/each}
  </div>
{/if}
