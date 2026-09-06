<script lang="ts">
  import { t, text, label as stateLabel, locale, message as uiMessage, errorMessage } from '../lib/i18n';
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { Health, ProjectList } from '../lib/types';

  let health: Health | undefined;
  let projects: ProjectList | undefined;
  let loading = true;
  let error = '';

  onMount(async () => {
    try {
      [health, projects] = await Promise.all([api.health(), api.listProjects()]);
    } catch (cause) {
      error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_26ec44ff0b5f");
    } finally {
      loading = false;
    }
  });
</script>

<div class="space-y-6">
  <div>
    <p class="text-sm font-medium text-primary">{$t("ui_abacd65b75b9")}</p>
    <h1 class="workspace-title mt-1">{$t("ui_3d7170a00471")}</h1>
    <p class="muted mt-2">{$t("ui_36a8ca307b4b")}</p>
  </div>

  {#if loading}
    <div class="workspace-card flex min-h-40 items-center justify-center" aria-busy="true">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>
  {:else if error}
    <div class="alert alert-error" role="alert"><span>{$text(error)}</span></div>
  {:else}
    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div class="metric"><div class="muted">{$t("ui_79f326be4409")}</div><div class="metric-value">{(projects?.total ?? 0)}</div></div>
      <div class="metric"><div class="muted">{$t("ui_293e4e09f7e4")}</div><div class="metric-value">{(projects?.projects.reduce((sum, item) => sum + item.completed_chapters, 0) ?? 0)}</div></div>
      <div class="metric"><div class="muted">{$t("ui_7ee5561de211")}</div><div class="metric-value">{(projects?.projects.reduce((sum, item) => sum + item.total_words, 0) ?? 0).toLocaleString($locale)}</div></div>
      <div class="metric"><div class="muted">{$t("ui_e8a4f7c09d16")}</div><div class="metric-value text-success">{$stateLabel(health?.status ?? 'unknown')}</div></div>
    </div>

    <section class="workspace-card overflow-hidden">
      <div class="border-b border-base-300 p-5">
        <h2 class="text-lg font-semibold">{$t("ui_342b52493b4f")}</h2>
        <p class="muted">{$t("ui_971485936b93")}</p>
      </div>
      {#if projects?.projects.length}
        <div class="divide-y divide-base-300">
          {#each projects.projects.slice(0, 6) as project}
            <a class="flex items-center justify-between gap-4 p-5 transition hover:bg-base-200" href={`#/chapters?project=${project.id}`}>
              <div>
                <div class="font-medium">{project.title}</div>
                <div class="muted">{$t("ui_4d0983c560d3", {p0: (project.current_chapter || 0), p1: project.total_words.toLocaleString($locale)})}</div>
              </div>
              <span class:badge-warning={project.archived} class:badge-success={!project.archived} class="badge badge-outline">
                {(project.archived ? $t("ui_82395d423099") : $t("ui_dc9591e56d50"))}
              </span>
            </a>
          {/each}
        </div>
      {:else}
        <div class="p-8 text-center">
          <p class="font-medium">{$t("ui_c1fe7fb67eca")}</p>
          <p class="muted mt-1">{$t("ui_c874b78a2176")}</p>
          <a class="btn btn-primary btn-sm mt-4" href="#/new">{$t("ui_18f5aa4a51c6")}</a>
        </div>
      {/if}
    </section>
  {/if}
</div>
