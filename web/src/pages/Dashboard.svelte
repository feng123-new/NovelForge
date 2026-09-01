<script lang="ts">
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import type { Health, ProjectList } from '../lib/types';

  let health: Health | undefined;
  let projects: ProjectList | undefined;
  let loading = true;
  let error = '';

  onMount(async () => {
    try {
      [health, projects] = await Promise.all([api.health(), api.listProjects()]);
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '无法加载工作台状态';
    } finally {
      loading = false;
    }
  });
</script>

<div class="space-y-6">
  <div>
    <p class="text-sm font-medium text-primary">LOCAL-FIRST NOVEL OPERATIONS</p>
    <h1 class="workspace-title mt-1">创作总览</h1>
    <p class="muted mt-2">项目、章节、字数和运行连接均来自本地 NovelForge 服务。</p>
  </div>

  {#if loading}
    <div class="workspace-card flex min-h-40 items-center justify-center" aria-busy="true">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>
  {:else if error}
    <div class="alert alert-error" role="alert"><span>{error}</span></div>
  {:else}
    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div class="metric"><div class="muted">项目</div><div class="metric-value">{projects?.total ?? 0}</div></div>
      <div class="metric"><div class="muted">已完成章节</div><div class="metric-value">{projects?.projects.reduce((sum, item) => sum + item.completed_chapters, 0) ?? 0}</div></div>
      <div class="metric"><div class="muted">累计字数</div><div class="metric-value">{(projects?.projects.reduce((sum, item) => sum + item.total_words, 0) ?? 0).toLocaleString()}</div></div>
      <div class="metric"><div class="muted">服务状态</div><div class="metric-value text-success">{health?.status ?? 'unknown'}</div></div>
    </div>

    <section class="workspace-card overflow-hidden">
      <div class="border-b border-base-300 p-5">
        <h2 class="text-lg font-semibold">最近项目</h2>
        <p class="muted">按后端稳定顺序展示。</p>
      </div>
      {#if projects?.projects.length}
        <div class="divide-y divide-base-300">
          {#each projects.projects.slice(0, 6) as project}
            <a class="flex items-center justify-between gap-4 p-5 transition hover:bg-base-200" href={`#/chapters?project=${project.id}`}>
              <div>
                <div class="font-medium">{project.title}</div>
                <div class="muted">第 {project.current_chapter || 0} 章 · {project.total_words.toLocaleString()} 字</div>
              </div>
              <span class:badge-warning={project.archived} class:badge-success={!project.archived} class="badge badge-outline">
                {project.archived ? '已归档' : '进行中'}
              </span>
            </a>
          {/each}
        </div>
      {:else}
        <div class="p-8 text-center">
          <p class="font-medium">还没有项目</p>
          <p class="muted mt-1">从新建小说向导创建第一部作品。</p>
          <a class="btn btn-primary btn-sm mt-4" href="#/new">新建小说</a>
        </div>
      {/if}
    </section>
  {/if}
</div>
