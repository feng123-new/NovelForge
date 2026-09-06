<script lang="ts">
  import { t, text, label as stateLabel, locale, message as uiMessage, errorMessage } from '../lib/i18n';
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { ModelEntry } from '../lib/types';

  let models: ModelEntry[] = [];
  let query = '';
  let loading = true;
  let error = '';

  async function load() {
    loading = true;
    error = '';
    try {
      models = (await api.listModels(query)).models;
    } catch (cause) {
      error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_91f378ea3006");
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="space-y-5">
  <div><h1 class="workspace-title">{$t("ui_d17d2d78d76e")}</h1><p class="muted mt-2">{$t("ui_0925d09567e3")}</p></div>
  <form class="workspace-card flex gap-3 p-4" on:submit|preventDefault={load}><input class="input input-bordered flex-1" bind:value={query} placeholder={$t("ui_8681ab1ab27e")} aria-label={$t("ui_93a8734cce30")} /><button class="btn" type="submit">{$t("ui_b5f15473fdc0")}</button></form>
  {#if error}<div class="alert alert-error" role="alert"><span>{$text(error)}</span></div>{/if}
  {#if loading}<div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>{:else if models.length === 0}<div class="workspace-card p-10 text-center">{$t("ui_c32f03541b15")}</div>{:else}<div class="workspace-card overflow-x-auto"><table class="table table-zebra"><thead><tr><th>{$t("ui_472590ae974d")}</th><th>{$t("ui_5e2c614c23f0")}</th><th>{$t("ui_6fc38ef3602f")}</th><th>{$t("ui_d66010376a09")}</th><th>{$t("ui_2d5873c69a7d")}</th><th>{$t("ui_0cf8c3b0e012")}</th></tr></thead><tbody>{#each models as model}<tr><td><span class="badge badge-outline">{model.provider}</span></td><td><div class="font-medium">{model.name}</div><div class="muted">{model.id}</div></td><td>{model.context_window.toLocaleString($locale)}</td><td>{model.max_tokens.toLocaleString($locale)}</td><td>{(model.input_cost_per_1m > 0 ? `$${model.input_cost_per_1m.toFixed(2)}` : $stateLabel('unknown'))}</td><td>{(model.output_cost_per_1m > 0 ? `$${model.output_cost_per_1m.toFixed(2)}` : $stateLabel('unknown'))}</td></tr>{/each}</tbody></table></div>{/if}
</div>
