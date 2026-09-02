<script lang="ts">
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
      error = cause.message;
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
      recordError(cause, '无法加载伏笔台账');
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
      success = '伏笔已写入权威 Narrative Ledger';
      await loadAll();
    } catch (cause) {
      recordError(cause, '创建伏笔失败');
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
      success = `${item.title} → ${status}`;
      await loadAll();
    } catch (cause) {
      recordError(cause, '更新伏笔失败');
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
      recordError(cause, '无法加载伏笔页面');
      loading = false;
    }
  });
</script>

<div class="space-y-5">
  <div>
    <p class="text-sm font-medium text-primary">NARRATIVE LEDGER</p>
    <h1 class="workspace-title mt-1">Foreshadows</h1>
    <p class="muted mt-2">OVERDUE 是 Chapter-N 计算结果，而不是可写状态。所有状态和指标均来自项目 SQLite。</p>
  </div>

  <div class="workspace-card grid gap-3 p-4 md:grid-cols-4">
    <label class="form-control md:col-span-2"><span class="label-text">项目</span><select class="select select-bordered" bind:value={selectedProject} on:change={loadAll} disabled={!!pending}>{#each projects as project}<option value={project.id}>{project.title}</option>{/each}</select></label>
    <label class="form-control"><span class="label-text">Chapter-N</span><input class="input input-bordered" type="number" min="0" bind:value={currentChapter} on:change={loadAll} disabled={!!pending} /></label>
    <div class="flex items-end"><button class="btn w-full" on:click={loadAll} disabled={!!pending}>刷新权威状态</button></div>
  </div>

  {#if error}<div class="alert alert-error" role="alert"><div><div class="font-mono font-medium">{structuredError?.code ?? 'LEDGER_UI_ERROR'}</div><div>{error}</div>{#if structuredError?.trace_id}<div class="font-mono text-xs">trace {structuredError.trace_id}</div>{/if}</div></div>{/if}
  {#if success}<div class="alert alert-success" role="status"><span>{success}</span></div>{/if}

  {#if dashboard}
    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <div class="metric"><div class="muted">Active</div><div class="metric-value">{dashboard.active_foreshadows}</div></div>
      <div class="metric"><div class="muted">OVERDUE</div><div class="metric-value text-error">{dashboard.overdue_count}</div></div>
      <div class="metric"><div class="muted">Critical overdue</div><div class="metric-value text-error">{dashboard.critical_overdue}</div></div>
      <div class="metric"><div class="muted">Upcoming</div><div class="metric-value">{dashboard.upcoming_payoffs}</div></div>
      <div class="metric"><div class="muted">Diagnostics</div><div class="metric-value">{diagnostics.length}</div></div>
    </div>
  {/if}

  <section class="workspace-card p-5">
    <h2 class="text-lg font-semibold">Create planned / planted foreshadow</h2>
    <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      <label class="form-control md:col-span-2"><span class="label-text">标题</span><input class="input input-bordered" bind:value={title} maxlength="200" /></label>
      <label class="form-control"><span class="label-text">Importance</span><select class="select select-bordered" bind:value={importance}><option>low</option><option>medium</option><option>high</option><option>critical</option></select></label>
      <label class="form-control"><span class="label-text">Urgency</span><select class="select select-bordered" bind:value={urgency}><option>low</option><option>normal</option><option>high</option><option>critical</option></select></label>
      <label class="form-control md:col-span-2"><span class="label-text">Description</span><textarea class="textarea textarea-bordered" bind:value={description}></textarea></label>
      <label class="form-control"><span class="label-text">Planted chapter</span><input class="input input-bordered" type="number" min="0" bind:value={plantedChapter} /></label>
      <label class="form-control"><span class="label-text">Source version</span><input class="input input-bordered" bind:value={sourceVersion} /></label>
      <label class="form-control"><span class="label-text">Expected min</span><input class="input input-bordered" type="number" min={plantedChapter} bind:value={payoffMin} /></label>
      <label class="form-control"><span class="label-text">Expected max</span><input class="input input-bordered" type="number" min={payoffMin} bind:value={payoffMax} /></label>
      <div class="flex items-end md:col-span-2"><button class="btn btn-primary w-full" on:click={createItem} disabled={!selectedProject || !title.trim() || !!pending}>{pending === 'create' ? 'Creating…' : 'Create'}</button></div>
    </div>
  </section>

  <section class="workspace-card overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 border-b border-base-300 p-5">
      <div class="flex-1"><h2 class="text-lg font-semibold">Stable Chapter-N list</h2><p class="muted">排序由后端显式 ORDER BY 控制。</p></div>
      <label class="label cursor-pointer gap-2"><span class="label-text">仅 OVERDUE</span><input class="checkbox" type="checkbox" bind:checked={onlyOverdue} on:change={loadAll} /></label>
      <select class="select select-bordered select-sm" bind:value={statusFilter} on:change={loadAll}><option value="">全部状态</option><option>planned</option><option>planted</option><option>progressing</option><option>resolved</option><option>abandoned</option><option>contradicted</option></select>
    </div>
    {#if loading}<div class="flex min-h-40 items-center justify-center" aria-busy="true"><span class="loading loading-spinner loading-lg"></span></div>
    {:else if !selectedProject}<div class="p-8 text-center"><p class="font-medium">请先创建项目</p></div>
    {:else if !items.length}<div class="p-8 text-center"><p class="font-medium">当前筛选没有伏笔</p></div>
    {:else}<div class="divide-y divide-base-300">{#each items as item}<article class="p-5"><div class="flex flex-wrap items-start justify-between gap-3"><div><div class="flex flex-wrap items-center gap-2"><h3 class="font-semibold">{item.title}</h3><span class="badge badge-outline">{item.status}</span>{#if item.overdue}<span class="badge badge-error">OVERDUE +{item.overdue_by_chapters}</span>{/if}<span class="badge badge-outline">{item.importance}</span></div><p class="muted mt-2">{item.description}</p><p class="mt-2 text-xs">payoff {item.expected_payoff_min}–{item.expected_payoff_max} · last progress {item.last_progress_chapter} · {item.source_version}</p></div><div class="flex flex-wrap gap-2"><button class="btn btn-xs" on:click={() => transition(item, 'progressing')} disabled={!!pending || item.status === 'resolved' || item.status === 'abandoned'}>Progress</button><button class="btn btn-success btn-xs" on:click={() => transition(item, 'resolved')} disabled={!!pending || item.status === 'resolved' || item.status === 'abandoned'}>Resolve</button><button class="btn btn-warning btn-xs" on:click={() => transition(item, 'abandoned')} disabled={!!pending || item.status === 'resolved' || item.status === 'abandoned'}>Abandon</button></div></div></article>{/each}</div>{/if}
  </section>
</div>
