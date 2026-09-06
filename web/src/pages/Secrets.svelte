<script lang="ts">
  import { t, text, message as uiMessage, errorMessage } from '../lib/i18n';
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { APIErrorPayload, ProjectSummary, SecretInput, SecretRecord } from '../lib/types';

  let projects: ProjectSummary[] = [];
  let selectedProject = '';
  let currentChapter = 1;
  let items: SecretRecord[] = [];
  let loading = true;
  let pending = '';
  let error = '';
  let success = '';
  let structuredError: APIErrorPayload | undefined;
  let description = '';
  let truth = '';
  let sourceVersion = 'human-v1';
  let holderEntity = '';
  let holderSecret = '';

  function recordError(cause: unknown, fallback: string) {
    success = '';
    if (cause instanceof APIClientError) {
      error = errorMessage(cause);
      structuredError = cause.payload;
    } else {
      error = fallback;
      structuredError = undefined;
    }
  }

  async function loadAll() {
    if (!selectedProject) { items = []; loading = false; return; }
    loading = true;
    error = '';
    try {
      items = (await api.listSecrets(selectedProject, currentChapter, true)).secrets;
      location.hash = `/secrets?project=${encodeURIComponent(selectedProject)}`;
    } catch (cause) {
      recordError(cause, uiMessage("ui_b039a45ab6ab"));
    } finally {
      loading = false;
    }
  }

  async function createItem() {
    if (!selectedProject || pending || !description.trim() || !truth.trim()) return;
    pending = 'create';
    try {
      const input: SecretInput = {
        description: description.trim(), truth: truth.trim(), created_chapter: currentChapter,
        public_status: 'private', source_version: sourceVersion.trim(), holders: []
      };
      await api.createSecret(selectedProject, input);
      description = ''; truth = ''; success = uiMessage("ui_b9324ec20642");
      await loadAll();
    } catch (cause) { recordError(cause, uiMessage("ui_2a9bcb111bd4")); }
    finally { pending = ''; }
  }

  async function reveal(item: SecretRecord) {
    if (!selectedProject || pending) return;
    pending = `reveal:${item.id}`;
    try {
      await api.updateSecret(selectedProject, item.id, { public_status: 'public', revealed_chapter: currentChapter, source_version: sourceVersion, chapter: currentChapter, reason: 'human public reveal' });
      success = uiMessage("ui_84c15f244ad7", {p0: item.description, p1: currentChapter});
      await loadAll();
    } catch (cause) { recordError(cause, uiMessage("ui_996365cbb968")); }
    finally { pending = ''; }
  }

  async function addHolder() {
    if (!selectedProject || !holderSecret || !holderEntity.trim() || pending) return;
    pending = 'holder';
    try {
      await api.addSecretHolder(selectedProject, holderSecret, {
        entity_id: holderEntity.trim(), valid_from_chapter: currentChapter,
        source_version: sourceVersion.trim(), authority: 'human_final',
        provenance: { type: 'chapter', id: `chapter-${currentChapter}`, chapter: currentChapter, version: sourceVersion.trim() }
      });
      holderEntity = ''; success = uiMessage("ui_2d9ee9a38a67"); await loadAll();
    } catch (cause) { recordError(cause, uiMessage("ui_0f5879b3af53")); }
    finally { pending = ''; }
  }

  onMount(async () => {
    try {
      projects = (await api.listProjects()).projects;
      const requested = new URLSearchParams(location.hash.split('?', 2)[1] ?? '').get('project') ?? '';
      selectedProject = projects.some((item) => item.id === requested) ? requested : (projects[0]?.id ?? '');
      currentChapter = Math.max(1, projects.find((item) => item.id === selectedProject)?.current_chapter ?? 1);
      await loadAll();
    } catch (cause) { recordError(cause, uiMessage("ui_d656f801cba3")); loading = false; }
  });
</script>

<div class="space-y-5">
  <div><p class="text-sm font-medium text-primary">{$t("ui_b32b888505fb")}</p><h1 class="workspace-title mt-1">{$t("ui_d8707d411d99")}</h1><p class="muted mt-2">{$t("ui_803e6cb01c60")}</p></div>
  <div class="workspace-card grid gap-3 p-4 md:grid-cols-4"><label class="form-control md:col-span-2"><span class="label-text">{$t("ui_79f326be4409")}</span><select class="select select-bordered" bind:value={selectedProject} on:change={loadAll} disabled={!!pending}>{#each projects as project}<option value={project.id}>{project.title}</option>{/each}</select></label><label class="form-control"><span class="label-text">{$t("ui_a1bd919abc64")}</span><input class="input input-bordered" type="number" min="0" bind:value={currentChapter} on:change={loadAll} /></label><div class="flex items-end"><button class="btn w-full" on:click={loadAll} disabled={!!pending}>{$t("ui_aee887434131")}</button></div></div>
  {#if error}<div class="alert alert-error" role="alert"><div><div class="font-mono font-medium">{(structuredError?.code ?? 'SECRET_UI_ERROR')}</div><div>{$text(error)}</div>{#if structuredError?.trace_id}<div class="font-mono text-xs">{$t("ui_a64eea9c1aa5", {p0: structuredError.trace_id})}</div>{/if}</div></div>{/if}
  {#if success}<div class="alert alert-success" role="status"><span>{$text(success)}</span></div>{/if}

  <section class="workspace-card p-5"><h2 class="text-lg font-semibold">{$t("ui_7e21ab8af7d7")}</h2><div class="mt-4 grid gap-3 md:grid-cols-2"><label class="form-control"><span class="label-text">{$t("ui_526e0087cc3f")}</span><input class="input input-bordered" bind:value={description} /></label><label class="form-control"><span class="label-text">{$t("ui_650334cdb673")}</span><input class="input input-bordered" bind:value={sourceVersion} /></label><label class="form-control md:col-span-2"><span class="label-text">{$t("ui_e73aa8c0d8c6")}</span><textarea class="textarea textarea-bordered min-h-28" bind:value={truth}></textarea></label><div class="md:col-span-2"><button class="btn btn-primary w-full" on:click={createItem} disabled={!!pending || !description.trim() || !truth.trim()}>{(pending === 'create' ? $t("ui_c79ed9492e3c") : $t("ui_7a9f4e0bd1b1"))}</button></div></div></section>

  <section class="workspace-card p-5"><h2 class="text-lg font-semibold">{$t("ui_49f224ef8743")}</h2><div class="mt-4 grid gap-3 md:grid-cols-3"><select class="select select-bordered" bind:value={holderSecret}><option value="">{$t("ui_acda202b6e6b")}</option>{#each items as item}<option value={item.id}>{item.description}</option>{/each}</select><input class="input input-bordered" placeholder={$t("ui_c174c2b49261")} bind:value={holderEntity} /><button class="btn" on:click={addHolder} disabled={!!pending || !holderSecret || !holderEntity.trim()}>{(pending === 'holder' ? $t("ui_23e39291d613") : $t("ui_f9013744bca4", {p0: currentChapter}))}</button></div></section>

  <section class="workspace-card overflow-hidden"><div class="border-b border-base-300 p-5"><h2 class="text-lg font-semibold">{$t("ui_a5e79f77b987")}</h2><p class="muted">{$t("ui_372cceb3fe25")}</p></div>{#if loading}<div class="flex min-h-40 items-center justify-center" aria-busy="true"><span class="loading loading-spinner loading-lg"></span></div>{:else if !selectedProject}<div class="p-8 text-center">{$t("ui_279a00bbb592")}</div>{:else if !items.length}<div class="p-8 text-center">{$t("ui_6fa064470cf9")}</div>{:else}<div class="divide-y divide-base-300">{#each items as item}<article class="p-5"><div class="flex flex-wrap justify-between gap-3"><div class="min-w-0 flex-1"><div class="flex flex-wrap items-center gap-2"><h3 class="font-semibold">{item.description}</h3><span class:badge-success={item.public_at_chapter} class="badge badge-outline">{(item.public_at_chapter ? $t("ui_d9262e7fb868") : $t("ui_7109ec0807d8"))}</span></div><div class="mt-3 rounded-box border border-warning/40 bg-warning/5 p-3"><div class="text-xs font-semibold uppercase">{$t("ui_51e0638d2315")}</div><p class="mt-1 break-words">{(item.truth ?? $t("ui_ff0a6e73ba50"))}</p></div><div class="mt-3"><div class="text-xs font-semibold uppercase">{$t("ui_a4e08eedb392", {p0: currentChapter})}</div>{#if item.holders.length}<div class="mt-2 flex flex-wrap gap-2">{#each item.holders as holder}<span class="badge badge-outline">{holder.entity_id}: {holder.valid_from_chapter}–{(holder.valid_to_chapter ?? '∞')}</span>{/each}</div>{:else}<p class="muted mt-1">{$t("ui_c0e3bc9a00cc")}</p>{/if}</div></div><button class="btn btn-success btn-sm" on:click={() => reveal(item)} disabled={!!pending || item.public_at_chapter}>{(pending === `reveal:${item.id}` ? $t("ui_52b28a95c1ca") : $t("ui_c2a97e4d75e1"))}</button></div></article>{/each}</div>{/if}</section>
</div>
