<script lang="ts">
  import { t, text, label as stateLabel, message as uiMessage } from './lib/i18n';
  import { onMount } from 'svelte';
  import LanguageSwitcher from './components/LanguageSwitcher.svelte';
  import Dashboard from './pages/Dashboard.svelte';
 import Observability from './pages/Observability.svelte';
  import Lifecycle from './pages/Lifecycle.svelte';
  import Autopilot from './pages/Autopilot.svelte';
  import Authoring from './pages/Authoring.svelte';
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
 observability: Observability,
    lifecycle: Lifecycle,
    autopilot: Autopilot,
    authoring: Authoring,
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
    { name: 'observability', label: uiMessage("ui_db87958c5298"), icon: '◎', href: '#/observability' },
    { name: 'dashboard', label: uiMessage("ui_67b696468610"), icon: '◫', href: '#/dashboard' },
    { name: 'projects', label: uiMessage("ui_04e2a9728af7"), icon: '◇', href: '#/projects' },
    { name: 'lifecycle', label: uiMessage("ui_2c15840a98c7"), icon: '⇄', href: '#/lifecycle' },
    { name: 'autopilot', label: uiMessage("ui_0c62cfcf5c72"), icon: '▷', href: '#/autopilot' },
    { name: 'authoring', label: uiMessage("ui_8f75ab75b421"), icon: '✎', href: '#/authoring' },
    { name: 'new', label: uiMessage("ui_9cba1384adde"), icon: '＋', href: '#/new' },
    { name: 'chapters', label: uiMessage("ui_9831375bc651"), icon: '≡', href: '#/chapters' },
    { name: 'versions', label: uiMessage("ui_f89ea27036ac"), icon: '↺', href: '#/versions' },
    { name: 'foreshadows', label: uiMessage("ui_f56ec867ab33"), icon: '⌁', href: '#/foreshadows' },
    { name: 'secrets', label: uiMessage("ui_d8707d411d99"), icon: '◈', href: '#/secrets' },
    { name: 'models', label: uiMessage("ui_d17d2d78d76e"), icon: '⌘', href: '#/models' },
    { name: 'logs', label: uiMessage("ui_ea2100dc89ae"), icon: '↯', href: '#/logs' },
    { name: 'settings', label: uiMessage("ui_74a883a037bc"), icon: '⚙', href: '#/settings' }
  ];

  let route: Route = currentRoute();
  $: activeComponent = pages[route.name];
  $: routeLabel = navigation.find((item) => item.name === route.name)?.label ?? uiMessage("ui_67b696468610");

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

<svelte:head><title>{$t("ui_6eebeb286ed7")}</title><meta name="description" content={$t("ui_866692c13ae1")} /></svelte:head>

<div class="drawer min-h-screen lg:drawer-open">
  <input id="workspace-drawer" type="checkbox" class="drawer-toggle" />
  <div class="drawer-content flex min-w-0 flex-col">
    <header class="sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-base-300 bg-base-100/90 px-4 backdrop-blur lg:px-6">
      <label for="workspace-drawer" class="btn btn-square btn-ghost lg:hidden" aria-label={$t("ui_38e4f2ee1b9a")}>☰</label>
      <div class="min-w-0 flex-1">
        <p class="truncate text-sm font-medium">{$text(routeLabel)}</p>
        <p class="truncate text-xs text-base-content/50">{$t("ui_1f46f79eb8c9")}</p>
      </div>
      <span class:badge-success={$connectionState === 'connected'} class:badge-warning={$connectionState !== 'connected'} class="badge badge-outline hidden sm:inline-flex">
        {$stateLabel($connectionState)}
      </span>
      <LanguageSwitcher />
      <button class="btn btn-square btn-ghost" on:click={toggleTheme} aria-label={$t("ui_7a1604323e8b")}>{($theme === 'novelforge-dark' ? '☀' : '☾')}</button>
    </header>

    <div class="grid min-h-0 flex-1 xl:grid-cols-[minmax(0,1fr)_18rem]">
      <main class="min-w-0 p-4 lg:p-6 xl:p-8">
        <svelte:component this={activeComponent} />
      </main>
      <aside class="hidden border-l border-base-300 bg-base-100/60 p-5 xl:block">
        <div class="sticky top-24 space-y-5">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{$t("ui_9f2730ac595b")}</p>
            <h2 class="mt-2 text-lg font-semibold">{$text(routeLabel)}</h2>
            <p class="muted mt-2">{$t("ui_6c70ae883929")}</p>
          </div>
          <div class="rounded-2xl bg-base-200 p-4">
            <div class="flex items-center justify-between"><span class="muted">{$t("ui_5c89f37c9b97")}</span><span class="font-mono text-xs">{$stateLabel($connectionState)}</span></div>
            <div class="mt-3 flex items-center justify-between"><span class="muted">{$t("ui_dbded978bd45")}</span><span class="font-mono text-xs">{$recentEvents.length}</span></div>
            <div class="mt-3 flex items-center justify-between"><span class="muted">{$t("ui_a49947ce68af")}</span><span class="font-mono text-xs">{$t("ui_b3eacd33433b")}</span></div>
          </div>
          <a class="btn btn-outline btn-sm w-full" href="/api/openapi.json" target="_blank" rel="noreferrer">{$t("ui_261d63504d41")}</a>
        </div>
      </aside>
    </div>

    <footer class="border-t border-base-300 bg-neutral px-4 py-3 text-neutral-content lg:px-6" aria-label={$t("ui_32e4a5fbc9ea")}>
      <div class="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs">
        <span class="font-semibold uppercase tracking-wider">{$t("ui_9dfe4eeee083")}</span>
        <span>{$t("ui_b16877491e51", {p0: $stateLabel($connectionState)})}</span>
        {#if $recentEvents[0]}
          <span class="truncate">{$t("ui_550ae1ef2251", {p0: $stateLabel($recentEvents[0].type), p1: ($recentEvents[0].project ? ` · ${$recentEvents[0].project}` : '')})}</span>
        {:else}
          <span>{$t("ui_58f3eb34d74c")}</span>
        {/if}
      </div>
    </footer>
  </div>

  <div class="drawer-side z-30">
    <label for="workspace-drawer" aria-label={$t("ui_baf9f5c82ab8")} class="drawer-overlay"></label>
    <aside class="min-h-full w-72 border-r border-base-300 bg-base-100 p-4">
      <a class="flex items-center gap-3 rounded-2xl px-3 py-4" href="#/dashboard">
        <span class="grid h-11 w-11 place-items-center rounded-2xl bg-primary text-xl font-black text-primary-content">{$t("ui_8ce86a6ae65d")}</span>
        <span><strong class="block text-lg">{$t("ui_56fb6b1282f1")}</strong><span class="text-xs text-base-content/50">{$t("ui_fc4532ee3e2c")}</span></span>
      </a>
      <nav class="mt-5" aria-label={$t("ui_9bae68040509")}>
        <ul class="menu gap-1 rounded-box p-0">
          {#each navigation as item}
            <li><a class:active={route.name === item.name} href={item.href}><span aria-hidden="true" class="w-5 text-center">{item.icon}</span>{$text(item.label)}</a></li>
          {/each}
        </ul>
      </nav>
      <div class="mt-8 rounded-2xl border border-base-300 bg-base-200 p-4">
        <p class="text-sm font-medium">{$t("ui_610b105f07da")}</p>
        <p class="muted mt-1">{$t("ui_1ee90bd7c01e")}</p>
      </div>
    </aside>
  </div>
</div>
