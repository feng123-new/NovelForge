<script lang="ts">
  import { t, text, label as stateLabel, serverText, locale, message as uiMessage, errorMessage, labelArgument } from '../lib/i18n';
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import { currentRoute } from '../lib/router';
  import type { APIErrorPayload, ChapterPlan, ChapterSummary, ProjectSummary, QualityView } from '../lib/types';

  let projects: ProjectSummary[] = [];
  let selectedProject = currentRoute().query.get('project') ?? '';
  let chapters: ChapterSummary[] = [];
  let loading = true;
  let error = '';
  let structuredError: APIErrorPayload | undefined;
  let success = '';
  let pending: '' | 'generate' | 'check' | 'rewrite' | 'finalize' = '';
  let quality: QualityView | null = null;
  let chapterNumber = 1;

  let title = '';
  let pov = '';
  let locationName = '';
  let objective = '';
  let conflict = '';
  let requiredBeats = '';
  let forbiddenOutcomes = '';
  let knowledgeBoundary = '';
  let inventoryConstraints = '';
  let foreshadowObligations = '';
  let endingHook = '';

  function lines(value: string): string[] {
    return value.split('\n').map((item) => item.trim()).filter(Boolean);
  }

  function plan(): ChapterPlan {
    return {
      chapter: chapterNumber,
      title: title.trim(),
      pov: pov.trim(),
      location: locationName.trim(),
      objective: objective.trim(),
      conflict: conflict.trim(),
      required_beats: lines(requiredBeats),
      forbidden_outcomes: lines(forbiddenOutcomes),
      knowledge_boundary: lines(knowledgeBoundary),
      inventory_constraints: lines(inventoryConstraints),
      foreshadow_obligations: lines(foreshadowObligations),
      ending_hook: endingHook.trim()
    };
  }

  function planReady() {
    const value = plan();
    return value.chapter > 0 && value.title && value.pov && value.location && value.objective && value.conflict && value.ending_hook;
  }

  async function loadProjects() {
    const page = await api.listProjects();
    projects = page.projects;
    if (!selectedProject && projects.length) selectedProject = projects[0].id;
  }

  async function loadChapters(resetChapter = false) {
    if (!selectedProject) {
      chapters = [];
      quality = null;
      loading = false;
      return;
    }
    loading = true;
    error = '';
    structuredError = undefined;
    try {
      chapters = (await api.listChapters(selectedProject)).chapters;
      if (resetChapter || chapterNumber <= 0) {
        chapterNumber = chapters.length ? Math.max(...chapters.map((item) => item.chapter)) + 1 : 1;
      }
      location.hash = `/chapters?project=${encodeURIComponent(selectedProject)}`;
      await loadQuality();
    } catch (cause) {
      recordError(cause, uiMessage("ui_aed776d8dcd6"));
    } finally {
      loading = false;
    }
  }

  async function loadQuality() {
    if (!selectedProject || chapterNumber <= 0) {
      quality = null;
      return;
    }
    try {
      quality = await api.quality(selectedProject, chapterNumber);
    } catch (cause) {
      recordError(cause, uiMessage("ui_25dc1111316c"));
    }
  }

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

  async function run(action: 'generate' | 'check' | 'rewrite' | 'finalize') {
    if (!selectedProject || pending) return;
    pending = action;
    error = '';
    structuredError = undefined;
    success = '';
    try {
      if (action === 'generate') quality = await api.generateChapter(selectedProject, chapterNumber, plan());
      if (action === 'check') quality = await api.checkChapter(selectedProject, chapterNumber);
      if (action === 'rewrite') quality = await api.rewriteChapter(selectedProject, chapterNumber, plan());
      if (action === 'finalize') quality = await api.finalizeChapter(selectedProject, chapterNumber);
      success = uiMessage("ui_0c5dd185c7b5", {p0: labelArgument(action)});
      await loadChapters(false);
    } catch (cause) {
      recordError(cause, uiMessage("ui_9973b9e6dff1"));
    } finally {
      pending = '';
    }
  }

  onMount(async () => {
    try {
      await loadProjects();
      await loadChapters(true);
    } catch (cause) {
      recordError(cause, uiMessage("ui_fd1dd87bfce1"));
      loading = false;
    }
  });
</script>

<div class="space-y-5">
  <div>
    <h1 class="workspace-title">{$t("ui_f12d1c90df4d")}</h1>
    <p class="muted mt-2">{$t("ui_1e3e7635c00e")}</p>
  </div>

  <div class="workspace-card grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_12rem]">
    <label class="form-control">
      <span class="label-text mb-2">{$t("ui_79f326be4409")}</span>
      <select class="select select-bordered" bind:value={selectedProject} on:change={() => loadChapters(true)} disabled={!!pending}>
        {#each projects as project}<option value={project.id}>{project.title}</option>{/each}
      </select>
    </label>
    <label class="form-control">
      <span class="label-text mb-2">{$t("ui_59aef472f076")}</span>
      <input class="input input-bordered" type="number" min="1" bind:value={chapterNumber} on:change={loadQuality} disabled={!!pending} />
    </label>
  </div>

  {#if error}
    <div class="alert alert-error" role="alert">
      <div>
        <div class="font-medium">{(structuredError?.code ?? 'QUALITY_UI_ERROR')}</div>
        <div>{$text(error)}</div>
        {#if structuredError?.trace_id}<div class="mt-1 font-mono text-xs">{$t("ui_a64eea9c1aa5", {p0: structuredError.trace_id})}</div>{/if}
      </div>
    </div>
  {/if}
  {#if success}<div class="alert alert-success" role="status"><span>{$text(success)}</span></div>{/if}

  {#if loading}
    <div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>
  {:else if !selectedProject}
    <div class="workspace-card p-10 text-center"><p class="font-medium">{$t("ui_279a00bbb592")}</p></div>
  {:else}
    <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(22rem,0.85fr)]">
      <section class="workspace-card p-5">
        <div class="mb-4 flex items-center justify-between gap-3">
          <div><h2 class="text-lg font-semibold">{$t("ui_9f04047f657f", {p0: chapterNumber})}</h2><p class="muted text-sm">{$t("ui_11fd98e7d681")}</p></div>
          <button class="btn btn-sm" on:click={loadQuality} disabled={!!pending}>{$t("ui_aee887434131")}</button>
        </div>
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">{$t("ui_eec26ddd9a40")}</div><div class="font-mono text-sm">{$stateLabel(quality?.snapshot.transaction.state || 'not_started')}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">{$t("ui_37df6942fb22")}</div><div class="text-lg font-semibold">{(quality?.snapshot.candidates?.length ?? 0)}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">{$t("ui_24f57581408a")}</div><div class="font-medium">{(quality?.snapshot.proposal ? $t("ui_4da8bd85795a") : $t("ui_332011b91ccd"))}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">{$t("ui_8971f58f9ccf")}</div><div class="font-medium">{$stateLabel(quality?.snapshot.continuity?.status ?? 'PENDING')}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">{$t("ui_05c3ea31aadb")}</div><div class="text-lg font-semibold">{(quality?.snapshot.editor?.score ?? '—')}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">{$t("ui_272f25fe45d2")}</div><div class="font-medium">{(quality?.snapshot.transaction.attempt ?? 0)} / {(quality?.snapshot.transaction.max_rewrites ?? 2)}</div></div>
        </div>
        {#if quality?.snapshot.transaction.final_candidate_id}
          <div class="mt-3 rounded-box border border-success/40 p-3"><span class="muted text-xs">{$t("ui_74f0f94d2000")}</span><div class="font-mono text-sm">{quality.snapshot.transaction.final_candidate_id}</div><div class="text-sm">{$serverText(quality.snapshot.transaction.last_reason)}</div></div>
        {/if}
        {#if quality?.snapshot.transaction.hold_reason}
          <div class="alert alert-warning mt-3"><div><div class="font-semibold">{$t("ui_aacf94b7be62")}</div><div>{$serverText(quality.snapshot.transaction.hold_reason)}</div></div></div>
        {/if}
        {#if quality?.snapshot.continuity?.issues?.length}
          <div class="mt-4 space-y-2">
            <h3 class="font-semibold">{$t("ui_cf4449b05df0")}</h3>
            {#each quality.snapshot.continuity.issues as issue}
              <div class="rounded-box border border-base-300 p-3 text-sm"><div class="font-mono font-medium">{issue.issue_code} · {$stateLabel(issue.severity)}</div><div>{issue.entity} / {issue.predicate}</div><div class="muted">{$serverText(issue.suggested_action)}</div></div>
            {/each}
          </div>
        {/if}
        <div class="mt-5 flex flex-wrap gap-2">
          <button class="btn btn-primary" on:click={() => run('generate')} disabled={!!pending || !quality?.actions.generate || !planReady()}>{(pending === 'generate' ? $t("ui_d20a4476a0a8") : $t("ui_49e49bb4401e"))}</button>
          <button class="btn" on:click={() => run('check')} disabled={!!pending || !quality?.actions.check}>{(pending === 'check' ? $t("ui_ec963ffc911b") : $t("ui_9d60841e0a78"))}</button>
          <button class="btn" on:click={() => run('rewrite')} disabled={!!pending || !quality?.actions.rewrite || !planReady()}>{(pending === 'rewrite' ? $t("ui_d0de257ca2ae") : $t("ui_272f25fe45d2"))}</button>
          <button class="btn btn-success" on:click={() => run('finalize')} disabled={!!pending || !quality?.actions.finalize}>{(pending === 'finalize' ? $t("ui_fae6ce42a3ec") : $t("ui_bcc55d805d23"))}</button>
        </div>
        {#if quality && !quality.actions.generate && !quality.snapshot.transaction.transaction_id}
          <p class="muted mt-3 text-sm">{$t("ui_c4f737fc9ecf")}</p>
        {/if}
      </section>

      <section class="workspace-card p-5">
        <h2 class="text-lg font-semibold">{$t("ui_fceae06ac456")}</h2>
        <p class="muted mb-4 text-sm">{$t("ui_881ec1c88a74")}</p>
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="form-control"><span class="label-text">{$t("ui_c3405f8c7d9d")}</span><input class="input input-bordered" bind:value={title} /></label>
          <label class="form-control"><span class="label-text">{$t("ui_ee7a425e7320")}</span><input class="input input-bordered" bind:value={pov} /></label>
          <label class="form-control"><span class="label-text">{$t("ui_253ac66fb8e1")}</span><input class="input input-bordered" bind:value={locationName} /></label>
          <label class="form-control"><span class="label-text">{$t("ui_071cff3a7216")}</span><input class="input input-bordered" bind:value={endingHook} /></label>
        </div>
        <label class="form-control mt-3"><span class="label-text">{$t("ui_3b20adc3b2ce")}</span><input class="input input-bordered" bind:value={objective} /></label>
        <label class="form-control mt-3"><span class="label-text">{$t("ui_014659ab9d98")}</span><input class="input input-bordered" bind:value={conflict} /></label>
        <div class="mt-3 grid gap-3 sm:grid-cols-2">
          <label class="form-control"><span class="label-text">{$t("ui_8b3f14d227df")}</span><textarea class="textarea textarea-bordered min-h-24" bind:value={requiredBeats}></textarea></label>
          <label class="form-control"><span class="label-text">{$t("ui_1ac2e055a6bf")}</span><textarea class="textarea textarea-bordered min-h-24" bind:value={forbiddenOutcomes}></textarea></label>
          <label class="form-control"><span class="label-text">{$t("ui_49a2d21f3ced")}</span><textarea class="textarea textarea-bordered min-h-24" bind:value={knowledgeBoundary}></textarea></label>
          <label class="form-control"><span class="label-text">{$t("ui_7d6cf93b72f0")}</span><textarea class="textarea textarea-bordered min-h-24" bind:value={inventoryConstraints}></textarea></label>
          <label class="form-control sm:col-span-2"><span class="label-text">{$t("ui_e6ff6a93c35d")}</span><textarea class="textarea textarea-bordered min-h-20" bind:value={foreshadowObligations}></textarea></label>
        </div>
      </section>
    </div>

    <section class="workspace-card overflow-x-auto">
      <table class="table">
        <thead><tr><th>{$t("ui_f12d1c90df4d")}</th><th>{$t("ui_c3405f8c7d9d")}</th><th>{$t("ui_0754f9f7337b")}</th><th>{$t("ui_0a5f9a892960")}</th><th>{$t("ui_6320b4a8722a")}</th></tr></thead>
        <tbody>
          {#if chapters.length === 0}
            <tr><td colspan="5" class="py-8 text-center muted">{$t("ui_68682fb2e05a")}</td></tr>
          {:else}
            {#each chapters as chapter}
              <tr>
                <td><button class="btn btn-ghost btn-xs" on:click={() => { chapterNumber = chapter.chapter; loadQuality(); }}>{chapter.chapter}</button></td>
                <td class="font-medium">{chapter.title}</td>
                <td>{chapter.character_count.toLocaleString($locale)}{(chapter.truncated ? '+' : '')}</td>
                <td>{new Date(chapter.updated_at).toLocaleString($locale)}</td>
                <td><span class="badge badge-success badge-outline">{$stateLabel(chapter.status)}</span></td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </section>
  {/if}
</div>
