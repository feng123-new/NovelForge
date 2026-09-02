<script lang="ts">
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

  let title = '新章节';
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
      recordError(cause, '无法加载章节');
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
      recordError(cause, '无法加载章节质量状态');
    }
  }

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
      success = `${action} 已提交并从服务端恢复最新权威状态`;
      await loadChapters(false);
    } catch (cause) {
      recordError(cause, '质量操作失败');
    } finally {
      pending = '';
    }
  }

  onMount(async () => {
    try {
      await loadProjects();
      await loadChapters(true);
    } catch (cause) {
      recordError(cause, '无法加载章节工作区');
      loading = false;
    }
  });
</script>

<div class="space-y-5">
  <div>
    <h1 class="workspace-title">章节</h1>
    <p class="muted mt-2">章节文件仍由服务端权威保存；质量事务现在提供 Librarian、Continuity、Editor、有限 Rewrite 与 Finalize。版本历史、Diff、Restore 仍留在 Phase 8。</p>
  </div>

  <div class="workspace-card grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_12rem]">
    <label class="form-control">
      <span class="label-text mb-2">项目</span>
      <select class="select select-bordered" bind:value={selectedProject} on:change={() => loadChapters(true)} disabled={!!pending}>
        {#each projects as project}<option value={project.id}>{project.title}</option>{/each}
      </select>
    </label>
    <label class="form-control">
      <span class="label-text mb-2">质量事务章节</span>
      <input class="input input-bordered" type="number" min="1" bind:value={chapterNumber} on:change={loadQuality} disabled={!!pending} />
    </label>
  </div>

  {#if error}
    <div class="alert alert-error" role="alert">
      <div>
        <div class="font-medium">{structuredError?.code ?? 'QUALITY_UI_ERROR'}</div>
        <div>{error}</div>
        {#if structuredError?.trace_id}<div class="mt-1 font-mono text-xs">trace {structuredError.trace_id}</div>{/if}
      </div>
    </div>
  {/if}
  {#if success}<div class="alert alert-success" role="status"><span>{success}</span></div>{/if}

  {#if loading}
    <div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>
  {:else if !selectedProject}
    <div class="workspace-card p-10 text-center"><p class="font-medium">请先创建项目</p></div>
  {:else}
    <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(22rem,0.85fr)]">
      <section class="workspace-card p-5">
        <div class="mb-4 flex items-center justify-between gap-3">
          <div><h2 class="text-lg font-semibold">Chapter {chapterNumber} Quality Gate</h2><p class="muted text-sm">所有状态刷新后均从后端恢复。</p></div>
          <button class="btn btn-sm" on:click={loadQuality} disabled={!!pending}>刷新</button>
        </div>
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">Transaction</div><div class="font-mono text-sm">{quality?.snapshot.transaction.state ?? 'not_started'}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">Draft candidates</div><div class="text-lg font-semibold">{quality?.snapshot.candidates.length ?? 0}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">Librarian</div><div class="font-medium">{quality?.snapshot.proposal ? 'FACTS PROPOSED' : 'PENDING'}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">Continuity</div><div class="font-medium">{quality?.snapshot.continuity?.status ?? 'PENDING'}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">Editor score</div><div class="text-lg font-semibold">{quality?.snapshot.editor?.score ?? '—'}</div></div>
          <div class="rounded-box border border-base-300 p-3"><div class="muted text-xs">Rewrite</div><div class="font-medium">{quality?.snapshot.transaction.attempt ?? 0} / {quality?.snapshot.transaction.max_rewrites ?? 2}</div></div>
        </div>
        {#if quality?.snapshot.transaction.final_candidate_id}
          <div class="mt-3 rounded-box border border-success/40 p-3"><span class="muted text-xs">Final candidate</span><div class="font-mono text-sm">{quality.snapshot.transaction.final_candidate_id}</div><div class="text-sm">{quality.snapshot.transaction.last_reason}</div></div>
        {/if}
        {#if quality?.snapshot.transaction.hold_reason}
          <div class="alert alert-warning mt-3"><div><div class="font-semibold">HOLD</div><div>{quality.snapshot.transaction.hold_reason}</div></div></div>
        {/if}
        {#if quality?.snapshot.continuity?.issues?.length}
          <div class="mt-4 space-y-2">
            <h3 class="font-semibold">Continuity issues</h3>
            {#each quality.snapshot.continuity.issues as issue}
              <div class="rounded-box border border-base-300 p-3 text-sm"><div class="font-mono font-medium">{issue.issue_code} · {issue.severity}</div><div>{issue.entity} / {issue.predicate}</div><div class="muted">{issue.suggested_action}</div></div>
            {/each}
          </div>
        {/if}
        <div class="mt-5 flex flex-wrap gap-2">
          <button class="btn btn-primary" on:click={() => run('generate')} disabled={!!pending || !quality?.actions.generate || !planReady()}>{pending === 'generate' ? 'Generating…' : 'Generate'}</button>
          <button class="btn" on:click={() => run('check')} disabled={!!pending || !quality?.actions.check}>{pending === 'check' ? 'Checking…' : 'Check'}</button>
          <button class="btn" on:click={() => run('rewrite')} disabled={!!pending || !quality?.actions.rewrite || !planReady()}>{pending === 'rewrite' ? 'Rewriting…' : 'Rewrite'}</button>
          <button class="btn btn-success" on:click={() => run('finalize')} disabled={!!pending || !quality?.actions.finalize}>{pending === 'finalize' ? 'Finalizing…' : 'Finalize'}</button>
        </div>
        {#if quality && !quality.actions.generate && !quality.snapshot.transaction.transaction_id}
          <p class="muted mt-3 text-sm">当前后端未配置质量模型服务，因此不会显示可执行的 Generate 操作；读取权威状态仍然可用。</p>
        {/if}
      </section>

      <section class="workspace-card p-5">
        <h2 class="text-lg font-semibold">Structured Chapter Plan</h2>
        <p class="muted mb-4 text-sm">该计划直接作为 Generate / Rewrite 的严格 JSON 请求，不会在浏览器中修改 Truth。</p>
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="form-control"><span class="label-text">标题</span><input class="input input-bordered" bind:value={title} /></label>
          <label class="form-control"><span class="label-text">POV</span><input class="input input-bordered" bind:value={pov} /></label>
          <label class="form-control"><span class="label-text">地点</span><input class="input input-bordered" bind:value={locationName} /></label>
          <label class="form-control"><span class="label-text">Ending hook</span><input class="input input-bordered" bind:value={endingHook} /></label>
        </div>
        <label class="form-control mt-3"><span class="label-text">Objective</span><input class="input input-bordered" bind:value={objective} /></label>
        <label class="form-control mt-3"><span class="label-text">Conflict</span><input class="input input-bordered" bind:value={conflict} /></label>
        <div class="mt-3 grid gap-3 sm:grid-cols-2">
          <label class="form-control"><span class="label-text">Required beats · 每行一项</span><textarea class="textarea textarea-bordered min-h-24" bind:value={requiredBeats}></textarea></label>
          <label class="form-control"><span class="label-text">Forbidden outcomes</span><textarea class="textarea textarea-bordered min-h-24" bind:value={forbiddenOutcomes}></textarea></label>
          <label class="form-control"><span class="label-text">Knowledge boundary</span><textarea class="textarea textarea-bordered min-h-24" bind:value={knowledgeBoundary}></textarea></label>
          <label class="form-control"><span class="label-text">Inventory constraints</span><textarea class="textarea textarea-bordered min-h-24" bind:value={inventoryConstraints}></textarea></label>
          <label class="form-control sm:col-span-2"><span class="label-text">Foreshadow obligations</span><textarea class="textarea textarea-bordered min-h-20" bind:value={foreshadowObligations}></textarea></label>
        </div>
      </section>
    </div>

    <section class="workspace-card overflow-x-auto">
      <table class="table">
        <thead><tr><th>章节</th><th>标题</th><th>字符数</th><th>更新时间</th><th>状态</th></tr></thead>
        <tbody>
          {#if chapters.length === 0}
            <tr><td colspan="5" class="py-8 text-center muted">暂无已 Finalize 的章节文件。质量事务可以先生成 Draft，只有 Finalize 完成后才进入这里。</td></tr>
          {:else}
            {#each chapters as chapter}
              <tr>
                <td><button class="btn btn-ghost btn-xs" on:click={() => { chapterNumber = chapter.chapter; loadQuality(); }}>{chapter.chapter}</button></td>
                <td class="font-medium">{chapter.title}</td>
                <td>{chapter.character_count.toLocaleString()}{chapter.truncated ? '+' : ''}</td>
                <td>{new Date(chapter.updated_at).toLocaleString()}</td>
                <td><span class="badge badge-success badge-outline">{chapter.status}</span></td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </section>
  {/if}
</div>
