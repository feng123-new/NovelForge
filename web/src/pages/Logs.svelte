<script lang="ts">
  import { t, text, label as stateLabel, locale } from '../lib/i18n';
  import { connectionState, recentEvents } from '../lib/events';
</script>

<div class="space-y-5">
  <div class="flex items-end justify-between gap-4"><div><h1 class="workspace-title">{$t("ui_ea2100dc89ae")}</h1><p class="muted mt-2">{$t("ui_5153e157d760")}</p></div><span class:badge-success={$connectionState === 'connected'} class:badge-warning={$connectionState !== 'connected'} class="badge badge-outline">{$stateLabel($connectionState)}</span></div>
  {#if $recentEvents.length === 0}<div class="workspace-card p-10 text-center"><p class="font-medium">{$t("ui_a3a8bd85ca1a")}</p><p class="muted mt-1">{$t("ui_921430eb323a")}</p></div>{:else}<div class="space-y-3">{#each $recentEvents as event}<article class="workspace-card p-4"><div class="flex flex-wrap items-center justify-between gap-3"><div class="flex items-center gap-2"><span class="badge badge-primary badge-outline">{$stateLabel(event.type)}</span>{#if event.project}<span class="muted">{$t("ui_805c7e080cb1", {p0: event.project})}</span>{/if}</div><time class="muted">{new Date(event.time).toLocaleString($locale)}</time></div>{#if event.data}<pre class="mt-3 max-h-48 overflow-auto rounded-xl bg-base-200 p-3 text-xs">{JSON.stringify(event.data, null, 2)}</pre>{/if}</article>{/each}</div>{/if}
</div>
