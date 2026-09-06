<script lang="ts">
  import { t, text, locale, message as uiMessage, errorMessage } from '../lib/i18n';
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { ProjectSummary } from '../lib/types';

  let projects: ProjectSummary[] = [];
  let query = '';
  let loading = true;
  let pending = '';
  let error = '';
  let notice = '';

  async function load() {
    loading = true;
    error = '';
    try {
      projects = (await api.listProjects(query)).projects;
    } catch (cause) {
      error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_ca5d789e5447");
    } finally {
      loading = false;
    }
  }

  async function toggleArchive(project: ProjectSummary) {
    pending = project.id;
    error = '';
    notice = '';
    try {
      await api.archiveProject(project.id, !project.archived);
      notice = project.archived ? uiMessage("ui_43bcc2c2a2a2") : uiMessage("ui_704ae78dcb1d");
      await load();
    } catch (cause) {
      error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_2247840c7200");
    } finally {
      pending = '';
    }
  }

  async function duplicate(project: ProjectSummary) {
    pending = project.id;
    error = '';
    notice = '';
    try {
      await api.duplicateProject(project.id, `${project.title} 副本`);
      notice = uiMessage("ui_391eaf0c62a6");
      await load();
    } catch (cause) {
      error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_753d8bb0da99");
    } finally {
      pending = '';
    }
  }

  async function remove(project: ProjectSummary) {
    const confirmation = globalThis.prompt($text(uiMessage("ui_2859d421ed81", {p0: project.title})));
    if (confirmation === null) return;
    pending = project.id;
    error = '';
    notice = '';
    try {
      await api.deleteProject(project.id, confirmation);
      notice = uiMessage("ui_2e38c353e081");
      await load();
    } catch (cause) {
      error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_c228558cf257");
    } finally {
      pending = '';
    }
  }

  onMount(load);
</script>

<div class="space-y-5">
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="workspace-title">{$t("ui_04e2a9728af7")}</h1>
      <p class="muted mt-2">{$t("ui_89f272ca2432")}</p>
    </div>
    <a class="btn btn-primary" href="#/new">{$t("ui_18f5aa4a51c6")}</a>
  </div>

  <form class="workspace-card flex gap-3 p-4" on:submit|preventDefault={load}>
    <label class="input input-bordered flex flex-1 items-center gap-2">
      <span aria-hidden="true">⌕</span>
      <input class="grow" bind:value={query} placeholder={$t("ui_85fadfc391cc")} aria-label={$t("ui_dfb59ea2b7a2")} />
    </label>
    <button class="btn" type="submit">{$t("ui_44ce7ae909bb")}</button>
  </form>

  {#if error}<div class="alert alert-error" role="alert"><span>{$text(error)}</span></div>{/if}
  {#if notice}<div class="alert alert-success" role="status"><span>{$text(notice)}</span></div>{/if}

  {#if loading}
    <div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>
  {:else if projects.length === 0}
    <div class="workspace-card p-10 text-center"><p class="font-medium">{$t("ui_37b2f0bcebfc")}</p><p class="muted mt-1">{$t("ui_1b212a6c1a08")}</p></div>
  {:else}
    <div class="grid gap-4 xl:grid-cols-2">
      {#each projects as project}
        <article class="workspace-card p-5">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-lg font-semibold">{project.title}</h2>
              <p class="muted mt-1">{$t("ui_61027af16dd8", {p0: project.total_words.toLocaleString($locale), p1: project.completed_chapters, p2: (project.total_chapters || '—')})}</p>
            </div>
            <span class:badge-warning={project.archived} class:badge-success={!project.archived} class="badge badge-outline">
              {(project.archived ? $t("ui_82395d423099") : $t("ui_dc9591e56d50"))}
            </span>
          </div>
          <progress class="progress progress-primary mt-5 w-full" value={project.completed_chapters} max={Math.max(project.total_chapters, 1)}></progress>
          <div class="mt-5 flex flex-wrap gap-2">
            <a class="btn btn-primary btn-sm" href={`#/chapters?project=${project.id}`}>{$t("ui_f12d1c90df4d")}</a>
            <button class="btn btn-sm" disabled={pending === project.id} on:click={() => toggleArchive(project)}>
              {(pending === project.id ? $t("ui_d3d21191f32e") : (project.archived ? $t("ui_be235f67fed2") : $t("ui_5292ab1a36c5")))}
            </button>
            <button class="btn btn-sm" disabled={pending === project.id} on:click={() => duplicate(project)}>{$t("ui_dd14c8e9a69e")}</button>
            <button class="btn btn-error btn-outline btn-sm" disabled={pending === project.id} on:click={() => remove(project)}>{$t("ui_361bdf3f36e3")}</button>
          </div>
        </article>
      {/each}
    </div>
  {/if}
</div>
