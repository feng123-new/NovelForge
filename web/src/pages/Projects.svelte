<script lang="ts">
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
      error = cause instanceof APIClientError ? cause.message : '无法加载项目';
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
      notice = project.archived ? '项目已恢复' : '项目已归档';
      await load();
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '项目状态更新失败';
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
      notice = '副本已创建，敏感配置未复制';
      await load();
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '复制失败';
    } finally {
      pending = '';
    }
  }

  async function remove(project: ProjectSummary) {
    const confirmation = globalThis.prompt(`输入项目名称“${project.title}”或项目 ID 以移入回收站`);
    if (confirmation === null) return;
    pending = project.id;
    error = '';
    notice = '';
    try {
      await api.deleteProject(project.id, confirmation);
      notice = '项目已移入 Workspace 私有回收站';
      await load();
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '删除失败';
    } finally {
      pending = '';
    }
  }

  onMount(load);
</script>

<div class="space-y-5">
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div>
      <h1 class="workspace-title">项目</h1>
      <p class="muted mt-2">创建、打开、归档和安全复制均连接真实 Project API。</p>
    </div>
    <a class="btn btn-primary" href="#/new">新建小说</a>
  </div>

  <form class="workspace-card flex gap-3 p-4" on:submit|preventDefault={load}>
    <label class="input input-bordered flex flex-1 items-center gap-2">
      <span aria-hidden="true">⌕</span>
      <input class="grow" bind:value={query} placeholder="搜索标题或简介" aria-label="搜索项目" />
    </label>
    <button class="btn" type="submit">搜索</button>
  </form>

  {#if error}<div class="alert alert-error" role="alert"><span>{error}</span></div>{/if}
  {#if notice}<div class="alert alert-success" role="status"><span>{notice}</span></div>{/if}

  {#if loading}
    <div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>
  {:else if projects.length === 0}
    <div class="workspace-card p-10 text-center"><p class="font-medium">没有匹配项目</p><p class="muted mt-1">修改搜索条件或创建新项目。</p></div>
  {:else}
    <div class="grid gap-4 xl:grid-cols-2">
      {#each projects as project}
        <article class="workspace-card p-5">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-lg font-semibold">{project.title}</h2>
              <p class="muted mt-1">{project.total_words.toLocaleString()} 字 · {project.completed_chapters}/{project.total_chapters || '—'} 章</p>
            </div>
            <span class:badge-warning={project.archived} class:badge-success={!project.archived} class="badge badge-outline">
              {project.archived ? '已归档' : '进行中'}
            </span>
          </div>
          <progress class="progress progress-primary mt-5 w-full" value={project.completed_chapters} max={Math.max(project.total_chapters, 1)}></progress>
          <div class="mt-5 flex flex-wrap gap-2">
            <a class="btn btn-primary btn-sm" href={`#/chapters?project=${project.id}`}>章节</a>
            <button class="btn btn-sm" disabled={pending === project.id} on:click={() => toggleArchive(project)}>
              {pending === project.id ? '处理中…' : project.archived ? '取消归档' : '归档'}
            </button>
            <button class="btn btn-sm" disabled={pending === project.id} on:click={() => duplicate(project)}>安全复制</button>
            <button class="btn btn-error btn-outline btn-sm" disabled={pending === project.id} on:click={() => remove(project)}>移入回收站</button>
          </div>
        </article>
      {/each}
    </div>
  {/if}
</div>
