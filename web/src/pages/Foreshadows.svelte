<script lang="ts">
  import { t, text, label as stateLabel, message as uiMessage, errorMessage, labelArgument } from '../lib/i18n';
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type {
    APIErrorPayload,
    Foreshadow,
    ForeshadowInput,
    ForeshadowStatus,
    LedgerDashboard,
    LedgerDiagnostic,
    ProjectSummary
  } from '../lib/types';

  let projects: ProjectSummary[] = [];
  let selectedProject = '';
  let currentChapter = 1;
  let items: Foreshadow[] = [];
  let dashboard: LedgerDashboard | null = null;
  let diagnostics: LedgerDiagnostic[] = [];
  let loading = true;
  let pending = '';
  let error = '';
  let structuredError: APIErrorPayload | undefined;
  let success = '';
  let onlyOverdue = false;
  let statusFilter = '';

  let title = '';
  let description = '';
  let importance: ForeshadowInput['importance'] = 'medium';
  let urgency: ForeshadowInput['urgency'] = 'normal';
  let plantedChapter = 0;
  let payoffMin = 1;
  let payoffMax = 3;
  let sourceVersion = 'human-v1';

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
    if (!selectedProject) {
      items = [];
      dashboard = null;
      diagnostics = [];
      loading = false;
      return;
    }
    loading = true;
    error = '';
    structuredError = undefined;
    try {
      const filters: Record<string, string> = {};
      if (onlyOverdue) filters.overdue = 'true';
      if (statusFilter) filters.status = statusFilter;
      const [page, metrics, diagnosticPage] = await Promise.all([
        api.listForeshadows(selectedProject, currentChapter, filters),
        api.ledgerDashboard(selectedProject, currentChapter),
        api.ledgerDiagnostics(selectedProject, currentChapter)
      ]);
      items = page.foreshadows;
      dashboard = metrics;
      diagnostics = diagnosticPage.diagnostics;
      location.hash = `/foreshadows?project=${encodeURIComponent(selectedProject)}`;
    } catch (cause) {
      recordError(cause, uiMessage("ui_a1a42bb4169b"));
    } finally {
      loading = false;
    }
  }

  async function createItem() {
    if (!selectedProject || pending || !title.trim()) return;
    pending = 'create';
    error = '';
    structuredError = undefined;
    try {
      await api.createForeshadow(selectedProject, {
        title: title.trim(),
        description: description.trim(),
        importance,
        planted_chapter: plantedChapter,
        expected_payoff_min: payoffMin,
        expected_payoff_max: payoffMax,
        status: plantedChapter > 0 ? 'planted' : 'planned',
        related_entities: [],
        related_arcs: [],
        last_progress_chapter: plantedChapter,
        urgency,
        source_version: sourceVersion.trim()
      });
      title = '';
      description = '';
      success = uiMessage("ui_615d1b749072");
      await loadAll();
    } catch (cause) {
      recordError(cause, uiMessage("ui_895db59ec5bf"));
    } finally {
      pending = '';
    }
  }

  async function transition(item: Foreshadow, status: ForeshadowStatus) {
    if (!selectedProject || pending) return;
    pending = `${status}:${item.id}`;
    error = '';
    structuredError = undefined;
    try {
      const patch: Parameters<typeof api.updateForeshadow>[2] = {
        status,
        chapter: currentChapter,
        reason: `human ${status}`,
        source_version: sourceVersion.trim() || item.source_version
      };
      if (status === 'progressing') patch.last_progress_chapter = currentChapter;
      if (status === 'resolved') patch.actual_payoff = currentChapter;
      await api.updateForeshadow(selectedProject, item.id, patch);
      success = uiMessage("foreshadow.transition", { p0: item.title, p1: labelArgument(status) });
      await loadAll();
    } catch (cause) {
      recordError(cause, uiMessage("ui_c249afa095be"));
    } finally {
      pending = '';
    }
  }

  onMount(async () => {
    try {
      const page = await api.listProjects();
      projects = page.projects;
      const requested = new URLSearchParams(location.hash.split('?', 2)[1] ?? '').get('project') ?? '';
      selectedProject = projects.some((item) => item.id === requested) ? requested : (projects[0]?.id ?? '');
      const project = projects.find((item) => item.id === selectedProject);
      currentChapter = Math.max(1, project?.current_chapter ?? 1);
      plantedChapter = currentChapter;
      payoffMin = currentChapter + 1;
      payoffMax = currentChapter + 3;
      await loadAll();
    } catch (cause) {
      recordError(cause, uiMessage("ui_ff3bf973a0f9"));
      loading = false;
    }
  });
</script>

<div class="space-y-5">
  <div>
    <p class="text-sm font-medium text-primary">{$t("ui_b32b888505fb")}</p>
    <h1 class="workspace-title mt-1">{$t("ui_f56ec867ab33")}</h1>
    <p class="muted mt-2">{$t("ui_6141fea21ddd")}</p>
  </div>

  <div class="workspace-card grid gap-3 p-4 md:grid-cols-4">
    <label class="form-control md:col-span-2"><span class="label-text">{$t("ui_79f326be4409")}</span><select class="select select-bordered" bind:value={selectedProject} on:change={loadAll} disabled={!!pending}>{#each projects as project}<option value={project.id}>{project.title}</option>{/each}</select></label>
    <label class="form-control"><span class="label-text">{$t("ui_a1bd919abc64")}</span><input class="input input-bordered" type="number" min="0" bind:value={currentChapter} on:change={loadAll} disabled={!!pending} /></label>
    <div class="flex items-end"><button class="btn w-full" on:click={loadAll} disabled={!!pending}>{$t("ui_4320020ec2ba")}</button></div>
  </div>

  {#if error}<div class="alert alert-error" role="alert"><div><div class="font-mono font-medium">{(structuredError?.code ?? 'LEDGER_UI_ERROR')}</div><div>{$text(error)}</div>{#if structuredError?.trace_id}<div class="font-mono text-xs">{$t("ui_a64eea9c1aa5", {p0: structuredError.trace_id})}</div>{/if}</div></div>{/if}
  {#if success}<div class="alert alert-success" role="status"><span>{$text(success)}</span></div>{/if}

  {#if dashboard}
    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <div class="metric"><div class="muted">{$t("ui_92340695899b")}</div><div class="metric-value">{dashboard.active_foreshadows}</div></div>
      <div class="metric"><div class="muted">{$t("ui_ba7fe5030495")}</div><div class="metric-value text-error">{dashboard.overdue_count}</div></div>
      <div class="metric"><div class="muted">{$t("ui_08920036bbd5")}</div><div class="metric-value text-error">{dashboard.critical_overdue}</div></div>
      <div class="metric"><div class="muted">{$t("ui_5f1a2542e4e4")}</div><div class="metric-value">{dashboard.upcoming_payoffs}</div></div>
      <div class="metric"><div class="muted">{$t("ui_268f14bbfe11")}</div><div class="metric-value">{diagnostics.length}</div></div>
    </div>
  {/if}

  <section class="workspace-card p-5">
    <h2 class="text-lg font-semibold">{$t("ui_50cde9ab7a68")}</h2>
    <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      <label class="form-control md:col-span-2"><span class="label-text">{$t("ui_c3405f8c7d9d")}</span><input class="input input-bordered" bind:value={title} maxlength="200" /></label>
      <label class="form-control"><span class="label-text">{$t("ui_3fc78b5e1295")}</span><select class="select select-bordered" bind:value={importance}><option value="low">{$t("ui_6c1ff09db3a7")}</option><option value="medium">{$t("ui_c082456a7766")}</option><option value="high">{$t("ui_6ef7c9b15ecd")}</option><option value="critical">{$t("ui_bfba7033571d")}</option></select></label>
      <label class="form-control"><span class="label-text">{$t("ui_03d37e9a5379")}</span><select class="select select-bordered" bind:value={urgency}><option value="low">{$t("ui_6c1ff09db3a7")}</option><option value="normal">{$t("ui_317b32c14369")}</option><option value="high">{$t("ui_6ef7c9b15ecd")}</option><option value="critical">{$t("ui_bfba7033571d")}</option></select></label>
      <label class="form-control md:col-span-2"><span class="label-text">{$t("ui_526e0087cc3f")}</span><textarea class="textarea textarea-bordered" bind:value={description}></textarea></label>
      <label class="form-control"><span class="label-text">{$t("ui_60d7c3ed20cd")}</span><input class="input input-bordered" type="number" min="0" bind:value={plantedChapter} /></label>
      <label class="form-control"><span class="label-text">{$t("ui_650334cdb673")}</span><input class="input input-bordered" bind:value={sourceVersion} /></label>
      <label class="form-control"><span class="label-text">{$t("ui_46bb32dd3ffe")}</span><input class="input input-bordered" type="number" min={plantedChapter} bind:value={payoffMin} /></label>
      <label class="form-control"><span class="label-text">{$t("ui_3d6a9ec0db6a")}</span><input class="input input-bordered" type="number" min={payoffMin} bind:value={payoffMax} /></label>
      <div class="flex items-end md:col-span-2"><button class="btn btn-primary w-full" on:click={createItem} disabled={!selectedProject || !title.trim() || !!pending}>{(pending === 'create' ? $t("ui_c79ed9492e3c") : $t("ui_4759498ac2a7"))}</button></div>
    </div>
  </section>

  <section class="workspace-card overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 border-b border-base-300 p-5">
      <div class="flex-1"><h2 class="text-lg font-semibold">{$t("ui_856d40c16a02")}</h2><p class="muted">{$t("ui_e5d7f333f88f")}</p></div>
      <label class="label cursor-pointer gap-2"><span class="label-text">{$t("ui_17205ec0f91b")}</span><input class="checkbox" type="checkbox" bind:checked={onlyOverdue} on:change={loadAll} /></label>
      <select class="select select-bordered select-sm" bind:value={statusFilter} on:change={loadAll}><option value="">{$t("ui_0a379c1e7398")}</option><option value="planned">{$t("ui_9b0f1b10aff5")}</option><option value="planted">{$t("ui_372eb3774802")}</option><option value="progressing">{$t("ui_97cf426f558b")}</option><option value="resolved">{$t("ui_dc676b428599")}</option><option value="abandoned">{$t("ui_20e7f550122e")}</option><option value="contradicted">{$t("ui_1af293e87a01")}</option></select>
    </div>
    {#if loading}<div class="flex min-h-40 items-center justify-center" aria-busy="true"><span class="loading loading-spinner loading-lg"></span></div>
    {:else if !selectedProject}<div class="p-8 text-center"><p class="font-medium">{$t("ui_279a00bbb592")}</p></div>
    {:else if !items.length}<div class="p-8 text-center"><p class="font-medium">{$t("ui_7a8ef1a78895")}</p></div>
    {:else}<div class="divide-y divide-base-300">{#each items as item}<article class="p-5"><div class="flex flex-wrap items-start justify-between gap-3"><div><div class="flex flex-wrap items-center gap-2"><h3 class="font-semibold">{item.title}</h3><span class="badge badge-outline">{$stateLabel(item.status)}</span>{#if item.overdue}<span class="badge badge-error">{$t("ui_a04a42c0c1a4", {p0: item.overdue_by_chapters})}</span>{/if}<span class="badge badge-outline">{$stateLabel(item.importance)}</span></div><p class="muted mt-2">{item.description}</p><p class="mt-2 text-xs">{$t("ui_bdbd522c8146", {p0: item.expected_payoff_min, p1: item.expected_payoff_max, p2: item.last_progress_chapter, p3: item.source_version})}</p></div><div class="flex flex-wrap gap-2"><button class="btn btn-xs" on:click={() => transition(item, 'progressing')} disabled={!!pending || item.status === 'resolved' || item.status === 'abandoned'}>{$t("ui_4664827f8e89")}</button><button class="btn btn-success btn-xs" on:click={() => transition(item, 'resolved')} disabled={!!pending || item.status === 'resolved' || item.status === 'abandoned'}>{$t("ui_c8f193b315c8")}</button><button class="btn btn-warning btn-xs" on:click={() => transition(item, 'abandoned')} disabled={!!pending || item.status === 'resolved' || item.status === 'abandoned'}>{$t("ui_d09252c46a7b")}</button></div></div></article>{/each}</div>{/if}
  </section>
</div>
