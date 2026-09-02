<script lang="ts">
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
      error = cause.message;
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
      recordError(cause, '无法加载秘密台账');
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
      description = ''; truth = ''; success = 'Secret 已写入权威管理视图';
      await loadAll();
    } catch (cause) { recordError(cause, '创建 Secret 失败'); }
    finally { pending = ''; }
  }

  async function reveal(item: SecretRecord) {
    if (!selectedProject || pending) return;
    pending = `reveal:${item.id}`;
    try {
      await api.updateSecret(selectedProject, item.id, { public_status: 'public', revealed_chapter: currentChapter, source_version: sourceVersion, chapter: currentChapter, reason: 'human public reveal' });
      success = `${item.description} 已在 Chapter ${currentChapter} 公开`;
      await loadAll();
    } catch (cause) { recordError(cause, '公开 Secret 失败'); }
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
      holderEntity = ''; success = 'Holder 时态范围已添加'; await loadAll();
    } catch (cause) { recordError(cause, '添加 Holder 失败'); }
    finally { pending = ''; }
  }

  onMount(async () => {
    try {
      projects = (await api.listProjects()).projects;
      const requested = new URLSearchParams(location.hash.split('?', 2)[1] ?? '').get('project') ?? '';
      selectedProject = projects.some((item) => item.id === requested) ? requested : (projects[0]?.id ?? '');
      currentChapter = Math.max(1, projects.find((item) => item.id === selectedProject)?.current_chapter ?? 1);
      await loadAll();
    } catch (cause) { recordError(cause, '无法加载 Secrets 页面'); loading = false; }
  });
</script>

<div class="space-y-5">
  <div><p class="text-sm font-medium text-primary">NARRATIVE LEDGER</p><h1 class="workspace-title mt-1">Secrets</h1><p class="muted mt-2">管理视图明确区分“权威真相”和“角色知识”。Planner/Writer 边界接口不会泄露未授权 Truth。</p></div>
  <div class="workspace-card grid gap-3 p-4 md:grid-cols-4"><label class="form-control md:col-span-2"><span class="label-text">项目</span><select class="select select-bordered" bind:value={selectedProject} on:change={loadAll} disabled={!!pending}>{#each projects as project}<option value={project.id}>{project.title}</option>{/each}</select></label><label class="form-control"><span class="label-text">Chapter-N</span><input class="input input-bordered" type="number" min="0" bind:value={currentChapter} on:change={loadAll} /></label><div class="flex items-end"><button class="btn w-full" on:click={loadAll} disabled={!!pending}>刷新</button></div></div>
  {#if error}<div class="alert alert-error" role="alert"><div><div class="font-mono font-medium">{structuredError?.code ?? 'SECRET_UI_ERROR'}</div><div>{error}</div>{#if structuredError?.trace_id}<div class="font-mono text-xs">trace {structuredError.trace_id}</div>{/if}</div></div>{/if}
  {#if success}<div class="alert alert-success" role="status"><span>{success}</span></div>{/if}

  <section class="workspace-card p-5"><h2 class="text-lg font-semibold">Create Secret</h2><div class="mt-4 grid gap-3 md:grid-cols-2"><label class="form-control"><span class="label-text">Description</span><input class="input input-bordered" bind:value={description} /></label><label class="form-control"><span class="label-text">Source version</span><input class="input input-bordered" bind:value={sourceVersion} /></label><label class="form-control md:col-span-2"><span class="label-text">Authority truth</span><textarea class="textarea textarea-bordered min-h-28" bind:value={truth}></textarea></label><div class="md:col-span-2"><button class="btn btn-primary w-full" on:click={createItem} disabled={!!pending || !description.trim() || !truth.trim()}>{pending === 'create' ? 'Creating…' : 'Create private Secret'}</button></div></div></section>

  <section class="workspace-card p-5"><h2 class="text-lg font-semibold">Add Chapter-N Holder</h2><div class="mt-4 grid gap-3 md:grid-cols-3"><select class="select select-bordered" bind:value={holderSecret}><option value="">选择 Secret</option>{#each items as item}<option value={item.id}>{item.description}</option>{/each}</select><input class="input input-bordered" placeholder="角色 / entity ID" bind:value={holderEntity} /><button class="btn" on:click={addHolder} disabled={!!pending || !holderSecret || !holderEntity.trim()}>{pending === 'holder' ? 'Saving…' : `Add from Chapter ${currentChapter}`}</button></div></section>

  <section class="workspace-card overflow-hidden"><div class="border-b border-base-300 p-5"><h2 class="text-lg font-semibold">Chapter-N authority and holder ranges</h2><p class="muted">Secret 是独立模型，不等同于 Foreshadow。</p></div>{#if loading}<div class="flex min-h-40 items-center justify-center" aria-busy="true"><span class="loading loading-spinner loading-lg"></span></div>{:else if !selectedProject}<div class="p-8 text-center">请先创建项目</div>{:else if !items.length}<div class="p-8 text-center">尚无 Secret</div>{:else}<div class="divide-y divide-base-300">{#each items as item}<article class="p-5"><div class="flex flex-wrap justify-between gap-3"><div class="min-w-0 flex-1"><div class="flex flex-wrap items-center gap-2"><h3 class="font-semibold">{item.description}</h3><span class:badge-success={item.public_at_chapter} class="badge badge-outline">{item.public_at_chapter ? 'PUBLIC' : 'PRIVATE'}</span></div><div class="mt-3 rounded-box border border-warning/40 bg-warning/5 p-3"><div class="text-xs font-semibold uppercase">Authority truth · management only</div><p class="mt-1 break-words">{item.truth ?? 'Truth hidden by this response policy'}</p></div><div class="mt-3"><div class="text-xs font-semibold uppercase">Knowledge holders at Chapter {currentChapter}</div>{#if item.holders.length}<div class="mt-2 flex flex-wrap gap-2">{#each item.holders as holder}<span class="badge badge-outline">{holder.entity_id}: {holder.valid_from_chapter}–{holder.valid_to_chapter ?? '∞'}</span>{/each}</div>{:else}<p class="muted mt-1">No role-bound holder at this chapter.</p>{/if}</div></div><button class="btn btn-success btn-sm" on:click={() => reveal(item)} disabled={!!pending || item.public_at_chapter}>{pending === `reveal:${item.id}` ? 'Revealing…' : 'Public Reveal'}</button></div></article>{/each}</div>{/if}</section>
</div>
