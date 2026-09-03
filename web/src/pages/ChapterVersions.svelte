<script lang="ts">
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import {
    chapterVersions,
    continuityStatus,
    shortSHA,
    type ChapterDiff,
    type ChapterDiffMode,
    type ChapterEvaluation,
    type ChapterPlanImpact,
    type ChapterSyncStatus,
    type ChapterVersion,
    type ChapterVersionView,
    type DerivedStateRebuild
  } from '../lib/chapterVersions';
  import { currentRoute } from '../lib/router';
  import type { APIErrorPayload, ProjectSummary } from '../lib/types';

  let projects: ProjectSummary[] = [];
  let projectID = currentRoute().query.get('project') ?? '';
  let chapter = Math.max(1, Number(currentRoute().query.get('chapter') ?? '1') || 1);
  let state: ChapterVersionView | null = null;
  let versions: ChapterVersion[] = [];
  let selected: ChapterVersion | null = null;
  let editor = '';
  let evaluation: ChapterEvaluation | null = null;
  let syncStatus: ChapterSyncStatus | null = null;
  let rebuild: DerivedStateRebuild | null = null;
  let impacts: ChapterPlanImpact[] = [];
  let fromVersion = '';
  let toVersion = '';
  let diffMode: ChapterDiffMode = 'inline';
  let diff: ChapterDiff | null = null;
  let rejectReason = 'Rejected by human review';
  let loading = true;
  let pending = '';
  let error = '';
  let structuredError: APIErrorPayload | undefined;
  let success = '';

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

  async function loadProjects() {
    const page = await api.listProjects();
    projects = page.projects;
    if (!projectID && projects.length) projectID = projects[0].id;
  }

  async function loadWorkspace(keepSelection = true) {
    if (!projectID || chapter < 1) return;
    loading = true;
    error = '';
    structuredError = undefined;
    try {
      const [nextState, page, nextRebuild, impactPage] = await Promise.all([
        chapterVersions.state(projectID, chapter),
        chapterVersions.list(projectID, chapter),
        chapterVersions.rebuild(projectID, chapter),
        chapterVersions.planImpact(projectID, chapter)
      ]);
      state = nextState;
      versions = page.versions;
      syncStatus = nextState.sync;
      rebuild = nextRebuild;
      impacts = impactPage.impacts;
      const selectedID = keepSelection ? selected?.id : '';
      const wanted = versions.find((item) => item.id === selectedID) ?? versions[0] ?? null;
      if (wanted) await selectVersion(wanted.id, false);
      else {
        selected = null;
        editor = '';
      }
      if (!fromVersion && versions.length) fromVersion = versions[versions.length - 1].id;
      if (!toVersion && versions.length) toVersion = versions[0].id;
      location.hash = `/versions?project=${encodeURIComponent(projectID)}&chapter=${chapter}`;
    } catch (cause) {
      recordError(cause, '无法加载 ChapterVersion 工作区');
    } finally {
      loading = false;
    }
  }

  async function selectVersion(id: string, updateEditor = true) {
    if (!projectID || !id) return;
    try {
      const full = await chapterVersions.get(projectID, chapter, id);
      selected = full;
      if (updateEditor || !editor) editor = full.content ?? '';
      evaluation = null;
    } catch (cause) {
      recordError(cause, '无法读取版本正文');
    }
  }

  async function run(action: 'save' | 'check' | 'accept' | 'reject' | 'restore' | 'finalize' | 'sync') {
    if (!projectID || pending) return;
    if (action !== 'save' && action !== 'sync' && !selected) return;
    pending = action;
    error = '';
    structuredError = undefined;
    success = '';
    try {
      if (action === 'save') {
        const created = await chapterVersions.saveHuman(projectID, chapter, editor);
        selected = created;
        success = `Human revision v${created.version_number} 已创建；Active Final 未被覆盖。`;
      } else if (action === 'check' && selected) {
        const result = await chapterVersions.check(projectID, chapter, selected.id);
        evaluation = result.evaluation;
        success = 'Continuity、Review 与 Truth Conflict 检查已完成。';
      } else if (action === 'accept' && selected) {
        const result = await chapterVersions.accept(projectID, chapter, selected.id, 'Accepted in Chapter Version workspace');
        selected = result.version;
        success = '候选版本已 Accept；尚未提交 Truth。';
      } else if (action === 'reject' && selected) {
        const result = await chapterVersions.reject(projectID, chapter, selected.id, rejectReason.trim() || 'Rejected by human review');
        selected = result.version;
        success = '版本已 Reject 并保留在 History。';
      } else if (action === 'restore' && selected) {
        const result = await chapterVersions.restore(projectID, chapter, selected.id);
        selected = result.version;
        editor = result.version.content ?? editor;
        success = `Restore 已创建新版本 v${result.version.version_number}，没有覆盖历史版本。`;
      } else if (action === 'finalize' && selected) {
        const result = await chapterVersions.finalize(projectID, chapter, selected.id);
        selected = result.active_final;
        success = `Finalize 完成：Active Final v${result.active_final.version_number}，Truth events ${result.truth_events}。`;
      } else if (action === 'sync') {
        const observed = syncStatus?.observed_sha ?? '';
        const result = await chapterVersions.sync(projectID, chapter, observed);
        evaluation = result.evaluation ?? null;
        if (result.version) selected = result.version;
        success = '外部修改已显式同步为 human_revision，并重新执行质量与冲突检查。';
      }
      await loadWorkspace(true);
    } catch (cause) {
      recordError(cause, `${action} 操作失败`);
    } finally {
      pending = '';
    }
  }

  async function loadDiff(cursor = '') {
    if (!projectID || !fromVersion || !toVersion || pending) return;
    pending = 'diff';
    error = '';
    structuredError = undefined;
    try {
      diff = await chapterVersions.diff(projectID, chapter, fromVersion, toVersion, diffMode, cursor);
    } catch (cause) {
      recordError(cause, 'Diff 失败');
      diff = null;
    } finally {
      pending = '';
    }
  }

  function canFinalize() {
    if (!selected || selected.rejected || !selected.accepted) return false;
    const status = evaluation?.continuity?.status ?? continuityStatus(selected);
    return status !== 'FAIL' && !evaluation?.continuity?.blocking && (evaluation?.conflicts?.length ?? 0) === 0;
  }

  onMount(async () => {
    try {
      await loadProjects();
      await loadWorkspace(false);
    } catch (cause) {
      recordError(cause, '无法加载 ChapterVersion 工作区');
      loading = false;
    }
  });
</script>

<div class="space-y-5" data-testid="chapter-version-workspace">
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div>
      <h1 class="workspace-title">Chapter Versions</h1>
      <p class="muted mt-2">不可变 History、Human Revision、Diff、Restore、External Sync 与 Active Final 都来自后端权威状态。</p>
    </div>
    <button class="btn btn-sm" on:click={() => loadWorkspace(true)} disabled={!!pending}>刷新权威状态</button>
  </div>

  <div class="workspace-card grid gap-4 p-4 md:grid-cols-[minmax(0,1fr)_10rem]">
    <label class="form-control"><span class="label-text mb-2">项目</span><select class="select select-bordered" bind:value={projectID} on:change={() => loadWorkspace(false)} disabled={!!pending}>{#each projects as project}<option value={project.id}>{project.title}</option>{/each}</select></label>
    <label class="form-control"><span class="label-text mb-2">Chapter</span><input class="input input-bordered" type="number" min="1" bind:value={chapter} on:change={() => loadWorkspace(false)} disabled={!!pending} /></label>
  </div>

  {#if error}
    <div class="alert alert-error" role="alert" data-testid="chapter-version-error"><div><div class="font-semibold">{structuredError?.code ?? 'CHAPTER_VERSION_UI_ERROR'}</div><div>{error}</div>{#if structuredError?.trace_id}<div class="mt-1 font-mono text-xs">trace {structuredError.trace_id}</div>{/if}</div></div>
  {/if}
  {#if success}<div class="alert alert-success" role="status">{success}</div>{/if}

  {#if syncStatus?.sync_required}
    <div class="alert alert-warning" role="alert" data-testid="sync-warning">
      <div class="min-w-0 flex-1"><div class="font-semibold">External edit detected · Sync required</div><div class="text-sm">expected {shortSHA(syncStatus.expected_sha)} · observed {shortSHA(syncStatus.observed_sha)}</div><div class="text-xs">不会静默覆盖数据库 Final，也不会继续把旧派生状态当作最新权威。</div></div>
      <button class="btn btn-warning btn-sm" on:click={() => run('sync')} disabled={!!pending}>{pending === 'sync' ? 'Syncing…' : 'Sync External Edit'}</button>
    </div>
  {/if}

  {#if loading}
    <div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>
  {:else if !projectID}
    <div class="workspace-card p-8 text-center">请先创建项目。</div>
  {:else}
    <div class="grid gap-5 xl:grid-cols-[15rem_minmax(0,1.25fr)_minmax(20rem,0.85fr)]">
      <aside class="workspace-card p-4">
        <div class="mb-3 flex items-center justify-between"><h2 class="font-semibold">History</h2><span class="badge">{state?.version_count ?? versions.length}</span></div>
        {#if versions.length === 0}<p class="muted text-sm">暂无版本。</p>{/if}
        <div class="space-y-2" data-testid="version-history">
          {#each versions as version}
            <button class:btn-active={selected?.id === version.id} class="btn h-auto min-h-0 w-full justify-start px-3 py-2 text-left" on:click={() => selectVersion(version.id)}>
              <span class="min-w-0 flex-1"><span class="block font-semibold">v{version.version_number} · {version.type}</span><span class="block truncate font-mono text-[11px]">{shortSHA(version.content_sha)}</span></span>
              <span class="flex flex-col gap-1">{#if version.active_final}<span class="badge badge-success badge-xs">Active</span>{/if}{#if version.rejected}<span class="badge badge-error badge-xs">Rejected</span>{/if}{#if version.author_type === 'human' && version.type === 'final'}<span class="badge badge-primary badge-xs">Human Final</span>{/if}</span>
            </button>
          {/each}
        </div>
      </aside>

      <main class="workspace-card p-5">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2"><div><h2 class="font-semibold">Markdown Editor / Reader</h2><p class="muted text-xs">Save 总是创建 human_revision；不会覆盖 Active Final。</p></div>{#if selected}<span class="font-mono text-xs">v{selected.version_number} · {selected.author_type}</span>{/if}</div>
        <textarea class="textarea textarea-bordered min-h-[30rem] w-full font-mono text-sm leading-relaxed" bind:value={editor} aria-label="Chapter markdown editor"></textarea>
        <div class="mt-3 flex flex-wrap gap-2">
          <button class="btn btn-primary" on:click={() => run('save')} disabled={!!pending || !editor.trim()}>{pending === 'save' ? 'Saving…' : 'Save Human Revision'}</button>
          <button class="btn" on:click={() => run('check')} disabled={!!pending || !selected || selected.rejected}>{pending === 'check' ? 'Checking…' : 'Check'}</button>
          <button class="btn btn-success" on:click={() => run('accept')} disabled={!!pending || !selected || selected.rejected || evaluation?.continuity?.status === 'FAIL'}>Accept</button>
          <button class="btn btn-error btn-outline" on:click={() => run('reject')} disabled={!!pending || !selected || selected.active_final}>Reject</button>
          <button class="btn btn-outline" on:click={() => run('restore')} disabled={!!pending || !selected}>Restore as New Version</button>
          <button class="btn btn-success" on:click={() => run('finalize')} disabled={!!pending || !canFinalize()}>{pending === 'finalize' ? 'Finalizing…' : 'Finalize'}</button>
        </div>
        <label class="form-control mt-3"><span class="label-text">Reject reason</span><input class="input input-bordered input-sm" bind:value={rejectReason} /></label>
      </main>

      <aside class="space-y-4">
        <section class="workspace-card p-4"><h2 class="font-semibold">Inspector</h2><div class="mt-3 grid grid-cols-2 gap-2 text-sm"><div class="muted">Active Final</div><div>{state?.active_final ? `v${state.active_final.version_number}` : '—'}</div><div class="muted">Continuity</div><div>{evaluation?.continuity?.status ?? continuityStatus(selected)}</div><div class="muted">Conflicts</div><div>{evaluation?.conflicts?.length ?? 0}</div><div class="muted">Derived state</div><div>{state?.derived_state ?? 'ready'}</div><div class="muted">Rebuild</div><div>{rebuild?.state ?? rebuild?.status ?? 'ready'}</div></div>{#if selected}<div class="mt-3 border-t border-base-300 pt-3 text-xs"><div>Parent: <span class="font-mono">{selected.parent_version_id ?? '—'}</span></div><div>SHA: <span class="font-mono">{shortSHA(selected.content_sha)}</span></div><div>Created: {new Date(selected.created_at).toLocaleString()}</div><div>Authority: {selected.authority ?? '—'}</div></div>{/if}</section>
        <section class="workspace-card p-4"><h2 class="font-semibold">Plan Impact</h2>{#if impacts.length === 0}<p class="muted mt-2 text-sm">无后续计划影响。</p>{:else}<div class="mt-2 space-y-2">{#each impacts as impact}<div class="rounded-box border border-base-300 p-2 text-xs"><div class="font-semibold">Chapter {impact.chapter} · {impact.severity}</div><div>{impact.affected_fact}</div><div class="muted">{impact.action_required} · {impact.reason}</div></div>{/each}</div>{/if}</section>
      </aside>
    </div>

    <section class="workspace-card p-5" data-testid="chapter-diff">
      <div class="flex flex-wrap items-end gap-3">
        <label class="form-control min-w-48 flex-1"><span class="label-text">From version</span><select class="select select-bordered select-sm" bind:value={fromVersion}>{#each versions as version}<option value={version.id}>v{version.version_number} · {version.type}</option>{/each}</select></label>
        <label class="form-control min-w-48 flex-1"><span class="label-text">To version</span><select class="select select-bordered select-sm" bind:value={toVersion}>{#each versions as version}<option value={version.id}>v{version.version_number} · {version.type}</option>{/each}</select></label>
        <label class="form-control"><span class="label-text">Mode</span><select class="select select-bordered select-sm" bind:value={diffMode}><option value="inline">Inline</option><option value="side_by_side">Side-by-side</option></select></label>
        <button class="btn btn-sm" on:click={() => loadDiff('')} disabled={!!pending || !fromVersion || !toVersion}>{pending === 'diff' ? 'Loading…' : 'Diff'}</button>
      </div>
      {#if diff}
        <div class="mt-4 flex gap-3 text-xs"><span class="badge badge-success">+{diff.additions}</span><span class="badge badge-error">-{diff.deletions}</span><span class="badge">={diff.unchanged}</span>{#if diff.truncated}<span class="badge badge-warning">Truncated</span>{/if}</div>
        <div class="mt-3 space-y-3 overflow-x-auto font-mono text-xs">
          {#each diff.hunks as hunk}
            <div class="rounded-box border border-base-300 p-3"><div class="mb-2 text-base-content/60">@@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},{hunk.new_lines} @@</div>{#each hunk.lines ?? [] as line}<div class="grid gap-2" class:grid-cols-2={diffMode === 'side_by_side'}><pre class="whitespace-pre-wrap">{line.old_text ?? line.text ?? ''}</pre>{#if diffMode === 'side_by_side'}<pre class="whitespace-pre-wrap">{line.new_text ?? line.text ?? ''}</pre>{/if}</div>{/each}</div>
          {/each}
        </div>
        {#if diff.next_cursor}<button class="btn btn-sm mt-3" on:click={() => loadDiff(diff?.next_cursor ?? '')}>Next diff chunk</button>{/if}
      {/if}
    </section>
  {/if}
</div>
