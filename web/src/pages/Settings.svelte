<script lang="ts">
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import { applyTheme, theme, type WorkspaceTheme } from '../lib/theme';
  import type { WorkspaceSettings } from '../lib/types';

  let settings: WorkspaceSettings | undefined;
  let loading = true;
  let error = '';

  onMount(async () => {
    try {
      settings = await api.settings();
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '设置加载失败';
    } finally {
      loading = false;
    }
  });

  function setTheme(value: WorkspaceTheme) {
    applyTheme(value);
  }
</script>

<div class="space-y-5">
  <div><h1 class="workspace-title">Settings</h1><p class="muted mt-2">仅展示安全的运行设置；Provider Secret 不进入 Web API 或浏览器存储。</p></div>
  {#if error}<div class="alert alert-error" role="alert"><span>{error}</span></div>{/if}
  {#if loading}<div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>{:else if settings}
    <section class="workspace-card p-6"><h2 class="text-lg font-semibold">外观</h2><div class="mt-4 flex flex-wrap gap-3"><button class:btn-primary={$theme === 'novelforge-light'} class="btn" on:click={() => setTheme('novelforge-light')}>Light</button><button class:btn-primary={$theme === 'novelforge-dark'} class="btn" on:click={() => setTheme('novelforge-dark')}>Dark</button></div><p class="muted mt-3">主题只保存浏览器偏好，不保存凭据或权威项目状态。</p></section>
    <section class="workspace-card p-6"><h2 class="text-lg font-semibold">本地服务</h2><dl class="mt-4 grid gap-4 sm:grid-cols-2"><div><dt class="muted">Workspace</dt><dd>{settings.workspace}</dd></div><div><dt class="muted">Version</dt><dd>{settings.version}</dd></div><div><dt class="muted">API</dt><dd>{settings.api_version}</dd></div><div><dt class="muted">Listen</dt><dd>{settings.listen_host}:{settings.listen_port}</dd></div><div><dt class="muted">Loopback only</dt><dd>{settings.loopback_only ? 'yes' : 'no'}</dd></div><div><dt class="muted">Body limit</dt><dd>{settings.request_limits.body_bytes?.toLocaleString()} bytes</dd></div></dl></section>
    <section class="workspace-card p-6"><h2 class="text-lg font-semibold">Capabilities</h2><div class="mt-4 grid gap-3 sm:grid-cols-2">{#each Object.entries(settings.capabilities) as [name, value]}<div class="flex items-center justify-between gap-4 rounded-xl bg-base-200 p-3"><span class="text-sm">{name.replaceAll('_', ' ')}</span><span class:text-success={value === true} class:text-warning={value === false} class="font-mono text-sm">{String(value)}</span></div>{/each}</div></section>
  {/if}
</div>
