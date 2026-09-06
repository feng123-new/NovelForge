<script lang="ts">
  import { t, text, label as stateLabel, locale, message as uiMessage, errorMessage } from '../lib/i18n';
  import { onMount } from 'svelte';
  import LanguageSwitcher from '../components/LanguageSwitcher.svelte';
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
      error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_05a4babdeaee");
    } finally {
      loading = false;
    }
  });

  function setTheme(value: WorkspaceTheme) {
    applyTheme(value);
  }
</script>

<div class="space-y-5">
  <div><h1 class="workspace-title">{$t("ui_74a883a037bc")}</h1><p class="muted mt-2">{$t("ui_0e1120d367ba")}</p></div>
  <section class="workspace-card p-6"><h2 class="text-lg font-semibold">{$t("ui_3d13868593ae")}</h2><div class="mt-3"><LanguageSwitcher id="settings-language" /></div><p class="muted mt-3">{$t("ui_5f0c035b727b")}</p></section>
  {#if error}<div class="alert alert-error" role="alert"><span>{$text(error)}</span></div>{/if}
  {#if loading}<div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>{:else if settings}
    <section class="workspace-card p-6"><h2 class="text-lg font-semibold">{$t("ui_86a63f23a076")}</h2><div class="mt-4 flex flex-wrap gap-3"><button class:btn-primary={$theme === 'novelforge-light'} class="btn" on:click={() => setTheme('novelforge-light')}>{$t("ui_dbcd5e7bb7a0")}</button><button class:btn-primary={$theme === 'novelforge-dark'} class="btn" on:click={() => setTheme('novelforge-dark')}>{$t("ui_60acc53f13a5")}</button></div><p class="muted mt-3">{$t("ui_c6b737578f7c")}</p></section>
    <section class="workspace-card p-6"><h2 class="text-lg font-semibold">{$t("ui_2fbee191632d")}</h2><dl class="mt-4 grid gap-4 sm:grid-cols-2"><div><dt class="muted">{$t("ui_87bb59ba2f92")}</dt><dd>{settings.workspace}</dd></div><div><dt class="muted">{$t("ui_dd167905de0d")}</dt><dd>{settings.version}</dd></div><div><dt class="muted">{$t("ui_c8e5998f6a39")}</dt><dd>{settings.api_version}</dd></div><div><dt class="muted">{$t("ui_225d29f6201e")}</dt><dd>{settings.listen_host}:{settings.listen_port}</dd></div><div><dt class="muted">{$t("ui_51342e0eb9ba")}</dt><dd>{(settings.loopback_only ? $t("ui_8a798890fe93") : $t("ui_9390298f3fb0"))}</dd></div><div><dt class="muted">{$t("ui_64705b95cb3b")}</dt><dd>{$t("ui_5e0e6b13bab2", {p0: settings.request_limits.body_bytes?.toLocaleString($locale)})}</dd></div></dl></section>
    <section class="workspace-card p-6"><h2 class="text-lg font-semibold">{$t("ui_9460f16ac9b5")}</h2><div class="mt-4 grid gap-3 sm:grid-cols-2">{#each Object.entries(settings.capabilities) as [name, value]}<div class="flex items-center justify-between gap-4 rounded-xl bg-base-200 p-3"><span class="text-sm">{$stateLabel(name)}</span><span class:text-success={value === true} class:text-warning={value === false} class="font-mono text-sm">{$stateLabel(value)}</span></div>{/each}</div></section>
  {/if}
</div>
