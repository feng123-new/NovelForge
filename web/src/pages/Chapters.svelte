<script lang="ts">
  import { onMount } from 'svelte';
  import { api, APIClientError } from '../lib/api';
  import { currentRoute } from '../lib/router';
  import type { ChapterSummary, ProjectSummary } from '../lib/types';

  let projects: ProjectSummary[] = [];
  let selectedProject = currentRoute().query.get('project') ?? '';
  let chapters: ChapterSummary[] = [];
  let loading = true;
  let error = '';

  async function loadProjects() {
    const page = await api.listProjects();
    projects = page.projects;
    if (!selectedProject && projects.length) selectedProject = projects[0].id;
  }

  async function loadChapters() {
    if (!selectedProject) {
      chapters = [];
      loading = false;
      return;
    }
    loading = true;
    error = '';
    try {
      chapters = (await api.listChapters(selectedProject)).chapters;
      location.hash = `/chapters?project=${encodeURIComponent(selectedProject)}`;
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '无法加载章节';
    } finally {
      loading = false;
    }
  }

  onMount(async () => {
    try {
      await loadProjects();
      await loadChapters();
    } catch (cause) {
      error = cause instanceof APIClientError ? cause.message : '无法加载章节工作区';
      loading = false;
    }
  });
</script>

<div class="space-y-5">
  <div>
    <h1 class="workspace-title">章节</h1>
    <p class="muted mt-2">当前阶段提供真实章节文件列表；编辑与版本工作流将在 Phase 8 接入。</p>
  </div>
  <div class="workspace-card p-4">
    <label class="form-control max-w-xl">
      <span class="label-text mb-2">项目</span>
      <select class="select select-bordered" bind:value={selectedProject} on:change={loadChapters}>
        {#each projects as project}<option value={project.id}>{project.title}</option>{/each}
      </select>
    </label>
  </div>
  {#if error}<div class="alert alert-error" role="alert"><span>{error}</span></div>{/if}
  {#if loading}
    <div class="workspace-card flex min-h-48 items-center justify-center"><span class="loading loading-spinner loading-lg"></span></div>
  {:else if !selectedProject}
    <div class="workspace-card p-10 text-center"><p class="font-medium">请先创建项目</p></div>
  {:else if chapters.length === 0}
    <div class="workspace-card p-10 text-center"><p class="font-medium">暂无章节文件</p><p class="muted mt-1">TUI、Headless 或后续 Web 生成流程写入章节后会出现在这里。</p></div>
  {:else}
    <div class="workspace-card overflow-x-auto">
      <table class="table">
        <thead><tr><th>章节</th><th>标题</th><th>字符数</th><th>更新时间</th><th>状态</th></tr></thead>
        <tbody>
          {#each chapters as chapter}
            <tr>
              <td>{chapter.chapter}</td>
              <td class="font-medium">{chapter.title}</td>
              <td>{chapter.character_count.toLocaleString()}{chapter.truncated ? '+' : ''}</td>
              <td>{new Date(chapter.updated_at).toLocaleString()}</td>
              <td><span class="badge badge-success badge-outline">{chapter.status}</span></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
