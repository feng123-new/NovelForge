<script lang="ts">
  import { onMount } from 'svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import Autopilot from './pages/Autopilot.svelte';
  import Projects from './pages/Projects.svelte';
  import NewNovelWizard from './pages/NewNovelWizard.svelte';
  import Chapters from './pages/Chapters.svelte';
  import ChapterVersions from './pages/ChapterVersions.svelte';
  import Foreshadows from './pages/Foreshadows.svelte';
  import Secrets from './pages/Secrets.svelte';
  import Models from './pages/Models.svelte';
  import Logs from './pages/Logs.svelte';
  import Settings from './pages/Settings.svelte';
  import { connectionState, recentEvents, startEventStream } from './lib/events';
  import { currentRoute, subscribeRoute, type Route, type RouteName } from './lib/router';
  import { initializeTheme, theme, applyTheme } from './lib/theme';

  const pages = {
    dashboard: Dashboard,
    autopilot: Autopilot,
    projects: Projects,
    new: NewNovelWizard,
    chapters: Chapters,
    versions: ChapterVersions,
    foreshadows: Foreshadows,
    secrets: Secrets,
    models: Models,
    logs: Logs,
    settings: Settings
  };
  const navigation: { name: RouteName; label: string; icon: string; href: string }[] = [
    { name: 'dashboard', label: 'Dashboard', icon: '◫', href: '#/dashboard' },
    { name: 'projects', label: 'Projects', icon: '◇', href: '#/projects' },
    { name: 'autopilot', label: 'Autopilot', icon: '▷', href: '#/autopilot' },
    { name: 'new', label: 'New Novel', icon: '＋', href: '#/new' },
    { name: 'chapters', label: 'Chapters', icon: '≡', href: '#/chapters' },
    { name: 'versions', label: 'Versions', icon: '↺', href: '#/versions' },
    { name: 'foreshadows', label: 'Foreshadows', icon: '⌁', href: '#/foreshadows' },
    { name: 'secrets', label: 'Secrets', icon: '◈', href: '#/secrets' },
    { name: 'models', label: 'Models', icon: '⌘', href: '#/models' },
    { name: 'logs', label: 'Logs', icon: '↯', href: '#/logs' },
    { name: 'settings', label: 'Settings', icon: '⚙', href: '#/settings' }
  ];

  let route: Route = currentRoute();
  $: activeComponent = pages[route.name];
  $: routeLabel = navigation.find((item) => item.name === route.name)?.label ?? 'Dashboard';

  onMount(() => {
    initializeTheme();
    const unsubscribeRoute = subscribeRoute((next) => (route = next));
    const stopEvents = startEventStream();
    return () => {
      unsubscribeRoute();
      stopEvents();
    };
  });

  function toggleTheme() {
    applyTheme($theme === 'novelforge-dark' ? 'novelforge-light' : 'novelforge-dark');
  }
</script>

<div class="drawer min-h-screen lg:drawer-open">
  <input id="workspace-drawer" type="checkbox" class="drawer-toggle" />
  <div class="drawer-content flex min-w-0 flex-col">
    <header class="sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-base-300 bg-base-100/90 px-4 backdrop-blur lg:px-6">
      <label for="workspace-drawer" class="btn btn-square btn-ghost lg:hidden" aria-label="打开导航">☰</label>
      <div class="min-w-0 flex-1">
        <p class="truncate text-sm font-medium">{routeLabel}</p>
        <p class="truncate text-xs text-base-content/50">NovelForge · authoritative state from local server</p>
      </div>
      <span class:badge-success={$connectionState === 'connected'} class:badge-warning={$connectionState !== 'connected'} class="badge badge-outline hidden sm:inline-flex">
        {$connectionState}
      </span>
      <button class="btn btn-square btn-ghost" on:click={toggleTheme} aria-label="切换主题">{$theme === 'novelforge-dark' ? '☀' : '☾'}</button>
    </header>

    <div class="grid min-h-0 flex-1 xl:grid-cols-[minmax(0,1fr)_18rem]">
      <main class="min-w-0 p-4 lg:p-6 xl:p-8">
        <svelte:component this={activeComponent} />
      </main>
      <aside class="hidden border-l border-base-300 bg-base-100/60 p-5 xl:block">
        <div class="sticky top-24 space-y-5">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">AI Inspector</p>
            <h2 class="mt-2 text-lg font-semibold">{routeLabel}</h2>
            <p class="muted mt-2">此面板只显示已接入的服务能力；未实现的生成动作不会显示假按钮。</p>
          </div>
          <div class="rounded-2xl bg-base-200 p-4">
            <div class="flex items-center justify-between"><span class="muted">SSE</span><span class="font-mono text-xs">{$connectionState}</span></div>
            <div class="mt-3 flex items-center justify-between"><span class="muted">事件缓存</span><span class="font-mono text-xs">{$recentEvents.length}</span></div>
            <div class="mt-3 flex items-center justify-between"><span class="muted">权威状态</span><span class="font-mono text-xs">server</span></div>
          </div>
          <a class="btn btn-outline btn-sm w-full" href="/api/openapi.json" target="_blank" rel="noreferrer">OpenAPI 3.1</a>
        </div>
      </aside>
    </div>

    <footer class="border-t border-base-300 bg-neutral px-4 py-3 text-neutral-content lg:px-6" aria-label="任务和日志">
      <div class="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs">
        <span class="font-semibold uppercase tracking-wider">Task / Log</span>
        <span>connection={$connectionState}</span>
        {#if $recentEvents[0]}
          <span class="truncate">latest={$recentEvents[0].type}{ $recentEvents[0].project ? ` · ${$recentEvents[0].project}` : ''}</span>
        {:else}
          <span>等待持久化事件…</span>
        {/if}
      </div>
    </footer>
  </div>

  <div class="drawer-side z-30">
    <label for="workspace-drawer" aria-label="关闭导航" class="drawer-overlay"></label>
    <aside class="min-h-full w-72 border-r border-base-300 bg-base-100 p-4">
      <a class="flex items-center gap-3 rounded-2xl px-3 py-4" href="#/dashboard">
        <span class="grid h-11 w-11 place-items-center rounded-2xl bg-primary text-xl font-black text-primary-content">N</span>
        <span><strong class="block text-lg">NovelForge</strong><span class="text-xs text-base-content/50">Long-form story system</span></span>
      </a>
      <nav class="mt-5" aria-label="主导航">
        <ul class="menu gap-1 rounded-box p-0">
          {#each navigation as item}
            <li><a class:active={route.name === item.name} href={item.href}><span aria-hidden="true" class="w-5 text-center">{item.icon}</span>{item.label}</a></li>
          {/each}
        </ul>
      </nav>
      <div class="mt-8 rounded-2xl border border-base-300 bg-base-200 p-4">
        <p class="text-sm font-medium">Local-first</p>
        <p class="muted mt-1">API Key 不进入静态 JavaScript、SSE 或浏览器持久化。</p>
      </div>
    </aside>
  </div>
</div>
