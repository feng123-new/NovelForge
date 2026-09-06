<script lang="ts">
  import { t, text, label as stateLabel, serverText, locale, message as uiMessage, errorMessage, labelArgument } from '../lib/i18n';
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
  let rejectReason = '';
  let loading = true;
  let pending = '';
  let error = '';
  let structuredError: APIErrorPayload | undefined;
  let success = '';

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
      recordError(cause, uiMessage("ui_9972b7e2df85"));
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
      recordError(cause, uiMessage("ui_9c8ad0bae317"));
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
        success = uiMessage("ui_1ac5d66e9868", {p0: created.version_number});
      } else if (action === 'check' && selected) {
        const result = await chapterVersions.check(projectID, chapter, selected.id);
        evaluation = result.evaluation;
        success = uiMessage("ui_0c59267b4242");
      } else if (action === 'accept' && selected) {
        const result = await chapterVersions.accept(projectID, chapter, selected.id, 'Accepted in Chapter Version workspace');
        selected = result.version;
        success = uiMessage("ui_66241f17fcc5");
      } else if (action === 'reject' && selected) {
        const result = await chapterVersions.reject(projectID, chapter, selected.id, rejectReason.trim() || 'Rejected by human review');
        selected = result.version;
        success = uiMessage("ui_4c463a4041f4");
      } else if (action === 'restore' && selected) {
        const result = await chapterVersions.restore(projectID, chapter, selected.id);
        selected = result.version;
        editor = result.version.content ?? editor;
        success = uiMessage("ui_4f5c4e25191e", {p0: result.version.version_number});
      } else if (action === 'finalize' && selected) {
        const result = await chapterVersions.finalize(projectID, chapter, selected.id);
        selected = result.active_final;
        success = uiMessage("ui_643a6281ba45", {p0: result.active_final.version_number, p1: result.truth_events});
      } else if (action === 'sync') {
        const observed = syncStatus?.observed_sha ?? '';
        const result = await chapterVersions.sync(projectID, chapter, observed);
        evaluation = result.evaluation ?? null;
        if (result.version) selected = result.version;
        success = uiMessage("ui_8aec8cec84d9");
      }
      await loadWorkspace(true);
    } catch (cause) {
      recordError(cause, uiMessage("ui_c84fc86d2962", {p0: labelArgument(action)}));
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
      recordError(cause, uiMessage("ui_d8cde8c420bf"));
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
      recordError(cause, uiMessage("ui_9972b7e2df85"));
      loading = false;
    }
  });
</script>

<div class="space-y-5" data-testid="chapter-version-workspace">
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div>
      <h1 class="workspace-title">{$t("ui_66e3cd9a895b")}</h1>
      <p class="muted mt-2">{$t("ui_0fc14237e2b0")}</p>
    </div>
    <button class="btn btn-sm" on:click={() => loadWorkspace(true)} disabled={!!pending}>{$t("ui_4320020ec2ba")}</button>
  </div>

  <div class="workspace-card grid gap-4 p-4 md:grid-cols-[minmax(0,1fr)_10rem]">
    <label class="form-control"><span class="label-text mb-2">{$t("ui_79f326be4409")}</span><select class="select select-bordered" bind:value={projectID} on:change={() => loadWorkspace(false)} disabled={!!pending}>{#each projects as project}<option value={project.id}>{project.title}</option>{/each}</select></label>
    <label class="form-control"><span class="label-text mb-2">{$t("ui_548c31f731ec")}</span><input class="input input-bordered" type="number" min="1" bind:value={chapter} on:change={() => loadWorkspace(false)} disabled={!!pending} /></label>
  </div>

  {#if error}
    <div class="alert alert-error" role="alert" data-testid="chapter-version-error"><div><div class="font-semibold">{(structuredError?.code ?? 'CHAPTER_VERSION_UI_ERROR')}</div><div>{$text(error)}</div>{#if structuredError?.trace_id}<div class="mt-1 font-mono text-xs">{$t("ui_a64eea9c1aa5", {p0: structuredError.trace_id})}</div>{/if}</div></div>
  {/if}
  {#if success}<div class="alert alert-success" role="status">{$text(success)}</div>{/if}

  {#if syncStatus?.sync_required}
    <div class="alert alert-warning" role="alert" data-testid="sync-warning">
      <div class="min-w-0 flex-1"><div class="font-semibold">{$t("ui_d3af7250ff2e")}</div><div class="text-sm">{$t("ui_19cee1eca374", {p0: shortSHA(syncStatus.expected_sha), p1: shortSHA(syncStatus.observed_sha)})}</div><div class="text-xs">{$t("ui_77abc451f916")}</div></div>
      <button class="btn btn-warning btn-sm" on:click={() => run('sync')} disabled={!!pending}>{(pending === 'sync' ? $t("ui_8a046cc90ab0") : $t("ui_78683424ec0a"))}</button>
    </div>
  {/if}

  {#if loading}
    <div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>
  {:else if !projectID}
    <div class="workspace-card p-8 text-center">{$t("ui_3185d9dd0e91")}</div>
  {:else}
    <div class="grid gap-5 xl:grid-cols-[15rem_minmax(0,1.25fr)_minmax(20rem,0.85fr)]">
      <aside class="workspace-card p-4">
        <div class="mb-3 flex items-center justify-between"><h2 class="font-semibold">{$t("ui_0e7696009337")}</h2><span class="badge">{(state?.version_count ?? versions.length)}</span></div>
        {#if versions.length === 0}<p class="muted text-sm">{$t("ui_1ff1e7289529")}</p>{/if}
        <div class="space-y-2" data-testid="version-history">
          {#each versions as version}
            <button class:btn-active={selected?.id === version.id} class="btn h-auto min-h-0 w-full justify-start px-3 py-2 text-left" on:click={() => selectVersion(version.id)}>
              <span class="min-w-0 flex-1"><span class="block font-semibold">{$t("ui_cc6d4ea1e502", {p0: version.version_number, p1: $stateLabel(version.type)})}</span><span class="block truncate font-mono text-[11px]">{shortSHA(version.content_sha)}</span></span>
              <span class="flex flex-col gap-1">{#if version.active_final}<span class="badge badge-success badge-xs">{$t("ui_92340695899b")}</span>{/if}{#if version.rejected}<span class="badge badge-error badge-xs">{$t("ui_aea4a04a8042")}</span>{/if}{#if version.author_type === 'human' && version.type === 'final'}<span class="badge badge-primary badge-xs">{$t("ui_92ca4e9dfb5f")}</span>{/if}</span>
            </button>
          {/each}
        </div>
      </aside>

      <main class="workspace-card p-5">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2"><div><h2 class="font-semibold">{$t("ui_be5aea8826c6")}</h2><p class="muted text-xs">{$t("ui_04a6d5b1b3ba")}</p></div>{#if selected}<span class="font-mono text-xs">{$t("ui_cc6d4ea1e502", {p0: selected.version_number, p1: $stateLabel(selected.author_type)})}</span>{/if}</div>
        <textarea class="textarea textarea-bordered min-h-[30rem] w-full font-mono text-sm leading-relaxed" bind:value={editor} aria-label={$t("ui_6f8e1176c350")}></textarea>
        <div class="mt-3 flex flex-wrap gap-2">
          <button class="btn btn-primary" on:click={() => run('save')} disabled={!!pending || !editor.trim()}>{(pending === 'save' ? $t("ui_23e39291d613") : $t("ui_89570e9f6c76"))}</button>
          <button class="btn" on:click={() => run('check')} disabled={!!pending || !selected || selected.rejected}>{(pending === 'check' ? $t("ui_ec963ffc911b") : $t("ui_9d60841e0a78"))}</button>
          <button class="btn btn-success" on:click={() => run('accept')} disabled={!!pending || !selected || selected.rejected || evaluation?.continuity?.status === 'FAIL'}>{$t("ui_89713b9c9c1b")}</button>
          <button class="btn btn-error btn-outline" on:click={() => run('reject')} disabled={!!pending || !selected || selected.active_final}>{$t("ui_ab604a360777")}</button>
          <button class="btn btn-outline" on:click={() => run('restore')} disabled={!!pending || !selected}>{$t("ui_098471150eb1")}</button>
          <button class="btn btn-success" on:click={() => run('finalize')} disabled={!!pending || !canFinalize()}>{(pending === 'finalize' ? $t("ui_fae6ce42a3ec") : $t("ui_bcc55d805d23"))}</button>
        </div>
        <label class="form-control mt-3"><span class="label-text">{$t("ui_b810e78e017b")}</span><input class="input input-bordered input-sm" bind:value={rejectReason} /></label>
      </main>

      <aside class="space-y-4">
        <section class="workspace-card p-4"><h2 class="font-semibold">{$t("ui_da188e3b1cef")}</h2><div class="mt-3 grid grid-cols-2 gap-2 text-sm"><div class="muted">{$t("ui_fd5b373f1445")}</div><div>{(state?.active_final ? $t("ui_aa7c5ba4306f", {p0: state.active_final.version_number}) : '—')}</div><div class="muted">{$t("ui_8971f58f9ccf")}</div><div>{$stateLabel(evaluation?.continuity?.status ?? continuityStatus(selected))}</div><div class="muted">{$t("ui_9f5ea35dc7f9")}</div><div>{(evaluation?.conflicts?.length ?? 0)}</div><div class="muted">{$t("ui_5738a3174499")}</div><div>{$stateLabel(state?.derived_state ?? 'ready')}</div><div class="muted">{$t("ui_95aefb081ece")}</div><div>{(rebuild?.state ?? rebuild?.status ?? $stateLabel('ready'))}</div></div>{#if selected}<div class="mt-3 border-t border-base-300 pt-3 text-xs"><div>{$t("ui_8add5fd1782e")} <span class="font-mono">{(selected.parent_version_id ?? '—')}</span></div><div>{$t("ui_00d2ce6a4d7e")} <span class="font-mono">{shortSHA(selected.content_sha)}</span></div><div>{$t("ui_801bf84ff3a3", {p0: new Date(selected.created_at).toLocaleString($locale)})}</div><div>{$t("ui_2ea909a03065", {p0: $stateLabel(selected.authority ?? '—')})}</div></div>{/if}</section>
        <section class="workspace-card p-4"><h2 class="font-semibold">{$t("ui_41b17670dead")}</h2>{#if impacts.length === 0}<p class="muted mt-2 text-sm">{$t("ui_9de1420b034d")}</p>{:else}<div class="mt-2 space-y-2">{#each impacts as impact}<div class="rounded-box border border-base-300 p-2 text-xs"><div class="font-semibold">{$t("ui_4e2e57cc3cf4", {p0: impact.chapter, p1: $stateLabel(impact.severity)})}</div><div>{impact.affected_fact}</div><div class="muted">{$serverText(impact.action_required)} · {$serverText(impact.reason)}</div></div>{/each}</div>{/if}</section>
      </aside>
    </div>

    <section class="workspace-card p-5" data-testid="chapter-diff">
      <div class="flex flex-wrap items-end gap-3">
        <label class="form-control min-w-48 flex-1"><span class="label-text">{$t("ui_388fe4290a98")}</span><select class="select select-bordered select-sm" bind:value={fromVersion}>{#each versions as version}<option value={version.id}>{$t("ui_cc6d4ea1e502", {p0: version.version_number, p1: $stateLabel(version.type)})}</option>{/each}</select></label>
        <label class="form-control min-w-48 flex-1"><span class="label-text">{$t("ui_316f16fe5efd")}</span><select class="select select-bordered select-sm" bind:value={toVersion}>{#each versions as version}<option value={version.id}>{$t("ui_cc6d4ea1e502", {p0: version.version_number, p1: $stateLabel(version.type)})}</option>{/each}</select></label>
        <label class="form-control"><span class="label-text">{$t("ui_5e23ec6a300d")}</span><select class="select select-bordered select-sm" bind:value={diffMode}><option value="inline">{$t("ui_99ed40acbd94")}</option><option value="side_by_side">{$t("ui_efca80330d7b")}</option></select></label>
        <button class="btn btn-sm" on:click={() => loadDiff('')} disabled={!!pending || !fromVersion || !toVersion}>{(pending === 'diff' ? $t("ui_ba3bbbe10d8b") : $t("ui_7ecf46284588"))}</button>
      </div>
      {#if diff}
        <div class="mt-4 flex gap-3 text-xs"><span class="badge badge-success">+{diff.additions}</span><span class="badge badge-error">-{diff.deletions}</span><span class="badge">={diff.unchanged}</span>{#if diff.truncated}<span class="badge badge-warning">{$t("ui_d9d9fcf3bd8a")}</span>{/if}</div>
        <div class="mt-3 space-y-3 overflow-x-auto font-mono text-xs">
          {#each diff.hunks as hunk}
            <div class="rounded-box border border-base-300 p-3"><div class="mb-2 text-base-content/60">@@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},{hunk.new_lines} @@</div>{#each hunk.lines ?? [] as line}<div class="grid gap-2" class:grid-cols-2={diffMode === 'side_by_side'}><pre class="whitespace-pre-wrap">{(line.old_text ?? line.text ?? '')}</pre>{#if diffMode === 'side_by_side'}<pre class="whitespace-pre-wrap">{(line.new_text ?? line.text ?? '')}</pre>{/if}</div>{/each}</div>
          {/each}
        </div>
        {#if diff.next_cursor}<button class="btn btn-sm mt-3" on:click={() => loadDiff(diff?.next_cursor ?? '')}>{$t("ui_82461f65bd50")}</button>{/if}
      {/if}
    </section>
  {/if}
</div>
