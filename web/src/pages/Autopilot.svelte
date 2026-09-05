<script lang="ts">
 import { onMount } from 'svelte';
 import { api, APIClientError } from '../lib/api';
 import { currentRoute } from '../lib/router';
 import { recentEvents } from '../lib/events';
 import type { AutopilotPage, AutopilotDetail } from '../lib/autopilot';
 import type { ProjectList } from '../lib/types';

 let projects: ProjectList['projects'] = [];
 let projectID = currentRoute().query.get('project') ?? '';
 let page: AutopilotPage | null = null;
 let detail: AutopilotDetail | null = null;
 let startChapter = 1;
 let targetChapter = 3;
 let reviewEvery = 1;
 let pending = false;
 let error = '';
 let disposed = false;
 let epoch = 0;
 let refreshing = false;

 async function refresh() {
  if (!projectID || refreshing || disposed) return;
  const id = projectID, generation = epoch;
  refreshing = true;
  try { const result = await api.listAutopilot(id); if (!disposed && id === projectID && generation === epoch) page = result; }
  catch (cause) { if (!disposed && generation === epoch) error = cause instanceof APIClientError ? cause.message : '任务读取失败'; }
  finally { refreshing = false; }
 }
 async function selectProject() {
  epoch++; page = null; detail = null; error = '';
  const id = projectID, generation = epoch;
  try { const p = await api.getProject(id); if (!disposed && generation === epoch) { startChapter = p.completed_chapters + 1; targetChapter = Math.max(startChapter, Math.min(p.total_chapters || startChapter + 2, 1000)); } }
  catch { if (!disposed && generation === epoch) error = '项目读取失败'; }
  await refresh();
  const recommended = (page as AutopilotPage | null)?.next_chapter;
  if (recommended) startChapter = recommended;
 }
 onMount(() => {
  disposed = false;
  void (async () => {
   try { const result = await api.listProjects(); if (disposed) return; projects = result.projects.filter((p) => !p.archived); if (!projectID) projectID = projects[0]?.id ?? ''; if (projectID) await selectProject(); }
   catch { if (!disposed) error = '项目列表读取失败'; }
  })();
  const unsubscribe = recentEvents.subscribe((events) => { if (events[0]?.type === 'autopilot.changed' && events[0]?.project === projectID) void refresh(); });
  const timer = setInterval(() => void refresh(), 2000);
  return () => { disposed = true; epoch++; clearInterval(timer); unsubscribe(); };
 });
 async function start() {
  if (pending || !projectID || !Number.isInteger(startChapter) || !Number.isInteger(targetChapter) || startChapter < 1 || targetChapter < startChapter || targetChapter > 1000 || !Number.isInteger(reviewEvery) || reviewEvery < 0 || reviewEvery > 100) { error = '请检查章节范围和审阅间隔'; return; }
  pending = true; error = '';
  try { await api.startAutopilot(projectID, { start_chapter: startChapter, target_chapter: targetChapter, review_every: reviewEvery }); await refresh(); }
  catch (cause) { error = cause instanceof APIClientError ? cause.message : '任务启动失败'; }
  finally { pending = false; }
 }
 async function control(id: string, action: 'pause' | 'stop' | 'resume') {
  pending = true; error = '';
  try {
   const job = page?.jobs.find((item) => item.id === id);
   const approval = action === 'resume' && job?.error_code === 'REVIEW_REQUIRED' ? { expected_revision: detail?.job.revision, review_candidate_id: detail?.candidate_id } : {};
   if (action === 'resume' && job?.error_code === 'REVIEW_REQUIRED' && (!detail || detail.job.id !== id || !detail.candidate_id)) throw new Error('请先查看当前候选稿');
   await api.controlAutopilot(projectID, id, action, approval); detail = null; await refresh();
  }
  catch (cause) { error = cause instanceof APIClientError ? cause.message : '控制请求失败'; }
  finally { pending = false; }
 }
 async function inspect(id: string) {
  pending = true; error = '';
  const project = projectID, generation = epoch;
  try { const result = await api.autopilotDetail(project, id); if (!disposed && generation === epoch && project === projectID) detail = result; }
  catch (cause) { error = cause instanceof APIClientError ? cause.message : '候选正文读取失败'; }
  finally { pending = false; }
 }
 $: liveJob = page?.jobs.some((j) => j.state !== 'completed' && j.state !== 'cancelled') ?? false;
</script>

<div class="space-y-5">
 <div><h1 class="workspace-title">Autopilot</h1><p class="muted mt-2">持久化任务 → Foundation → 章节计划 → 质量门禁 → 版本定稿。模型调用可能产生费用。</p></div>
 <label class="form-control"><span class="label-text mb-2">小说项目</span><select class="select select-bordered" bind:value={projectID} on:change={selectProject} disabled={pending}><option value="">选择项目</option>{#each projects as p}<option value={p.id}>{p.title}</option>{/each}</select></label>
 {#if error}<div role="alert" class="alert alert-error">{error}</div>{/if}
 {#if page}
  {#if !page.worker_available}<div class="alert alert-warning">Worker 未运行；不会启动或恢复模型任务。</div>{/if}
  {#if !page.model_available}<div class="alert alert-warning">项目模型未配置。请先配置服务端 Provider；模型目录不代表连接可用。</div>{/if}
  <form class="workspace-card grid gap-4 p-5 md:grid-cols-3" on:submit|preventDefault={start}>
   <label class="form-control"><span class="label-text mb-2">起始章节</span><input class="input input-bordered" type="number" min="1" max="1000" bind:value={startChapter} /></label>
   <label class="form-control"><span class="label-text mb-2">结束章节</span><input class="input input-bordered" type="number" min={startChapter} max="1000" bind:value={targetChapter} /></label>
   <label class="form-control"><span class="label-text mb-2">审阅间隔（0 为全自动）</span><input class="input input-bordered" type="number" min="0" max="100" bind:value={reviewEvery} /></label>
   <div class="space-y-2 md:col-span-3"><p class="muted">需要已有新建小说请求。同一项目一次只保留一个未结束任务；暂停后可人工修改，停止不删除正文。暂停／停止在当前有界步骤结束后生效。</p><button class="btn btn-primary" disabled={pending || liveJob || !page.worker_available || !page.model_available} type="submit">启动创作任务</button></div>
  </form>
  <div class="space-y-3" aria-label="任务列表">
   {#each page.jobs as job (job.id)}
    <article class="workspace-card space-y-3 p-5">
     <div class="flex flex-wrap justify-between gap-2"><strong>{job.state} · {job.stage}</strong><span>已完成至第 {job.completed_through} 章 / 目标 {job.target_chapter} 章</span></div>
     <p class="break-all font-mono text-xs">{job.id}</p>
     {#if job.control}<p role="status">已请求 {job.control}，当前步骤尚未退出。</p>{/if}
     {#if job.error_code}<p role="status" class="text-warning">{job.error_code === 'REVIEW_REQUIRED' ? '待人工确认：请查看本章候选，批准后再定稿。' : job.error_code}</p>{/if}
     {#if job.state === 'retrying'}<p class="muted">重试 {job.retries}/{job.max_retries} · {job.next_run}</p>{/if}
     <div class="flex flex-wrap gap-2">
      <button class="btn btn-sm" disabled={pending} on:click={() => inspect(job.id)}>查看候选与计划</button>
      <button class="btn btn-sm" disabled={pending || !job.actions.pause} on:click={() => control(job.id, 'pause')}>暂停</button>
      <button class="btn btn-sm btn-primary" disabled={pending || !job.actions.resume || (job.error_code === 'REVIEW_REQUIRED' && (!detail || detail.job.id !== job.id || detail.job.revision !== job.revision || detail.candidate_id !== job.review_candidate_id))} on:click={() => control(job.id, 'resume')}>{job.error_code === 'REVIEW_REQUIRED' ? '批准本章并继续' : '继续 / 重试'}</button>
      <button class="btn btn-sm btn-outline" disabled={pending || !job.actions.stop} on:click={() => control(job.id, 'stop')}>停止</button>
      <a class="btn btn-sm btn-ghost" href={`#/versions?project=${projectID}`}>章节版本</a>
     </div>
    </article>
   {:else}<div class="workspace-card p-5 muted">尚无任务。启动前确认模型配置和新建小说请求。</div>{/each}
  </div>
 {/if}
 {#if detail}
  <section class="workspace-card space-y-4 p-5" aria-label="候选正文">
   <h2 class="text-xl font-semibold">第 {detail.job.chapter} 章候选</h2>
   <pre class="max-h-[36rem] overflow-auto whitespace-pre-wrap break-words font-sans">{detail.candidate_text || '当前尚未生成候选正文。'}</pre>
   <details><summary class="cursor-pointer">章节计划</summary><pre class="overflow-auto whitespace-pre-wrap break-words text-xs">{JSON.stringify(detail.chapter_plan, null, 2)}</pre></details>
   <details><summary class="cursor-pointer">Foundation（规划，不是已发生事实）</summary><pre class="overflow-auto whitespace-pre-wrap break-words text-xs">{JSON.stringify(detail.foundation, null, 2)}</pre></details>
  </section>
 {/if}
</div>
