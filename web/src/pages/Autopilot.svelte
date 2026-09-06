<script lang="ts">
  import { t, text, codeText, label as stateLabel, message as uiMessage, errorMessage } from '../lib/i18n';
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
  catch (cause) { if (!disposed && generation === epoch) error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_b43c21f8578b"); }
  finally { refreshing = false; }
 }
 async function selectProject() {
  epoch++; page = null; detail = null; error = '';
  const id = projectID, generation = epoch;
  try { const p = await api.getProject(id); if (!disposed && generation === epoch) { startChapter = p.completed_chapters + 1; targetChapter = Math.max(startChapter, Math.min(p.total_chapters || startChapter + 2, 1000)); } }
  catch { if (!disposed && generation === epoch) error = uiMessage("ui_35936b049f44"); }
  await refresh();
  const recommended = (page as AutopilotPage | null)?.next_chapter;
  if (recommended) startChapter = recommended;
 }
 onMount(() => {
  disposed = false;
  void (async () => {
   try { const result = await api.listProjects(); if (disposed) return; projects = result.projects.filter((p) => !p.archived); if (!projectID) projectID = projects[0]?.id ?? ''; if (projectID) await selectProject(); }
   catch { if (!disposed) error = uiMessage("ui_682b95566c0d"); }
  })();
  const unsubscribe = recentEvents.subscribe((events) => { if (events[0]?.type === 'autopilot.changed' && events[0]?.project === projectID) void refresh(); });
  const timer = setInterval(() => void refresh(), 2000);
  return () => { disposed = true; epoch++; clearInterval(timer); unsubscribe(); };
 });
 async function start() {
  if (pending || !projectID || !Number.isInteger(startChapter) || !Number.isInteger(targetChapter) || startChapter < 1 || targetChapter < startChapter || targetChapter > 1000 || !Number.isInteger(reviewEvery) || reviewEvery < 0 || reviewEvery > 100) { error = uiMessage("ui_2f185d748fe2"); return; }
  pending = true; error = '';
  try { await api.startAutopilot(projectID, { start_chapter: startChapter, target_chapter: targetChapter, review_every: reviewEvery }); await refresh(); }
  catch (cause) { error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_1da77ae009e6"); }
  finally { pending = false; }
 }
 async function control(id: string, action: 'pause' | 'stop' | 'resume') {
  pending = true; error = '';
  try {
   const job = page?.jobs.find((item) => item.id === id);
   const approval = action === 'resume' && job?.error_code === 'REVIEW_REQUIRED' ? { expected_revision: detail?.job.revision, review_candidate_id: detail?.candidate_id } : {};
   if (action === 'resume' && job?.error_code === 'REVIEW_REQUIRED' && (!detail || detail.job.id !== id || !detail.candidate_id)) throw new Error(uiMessage("ui_e276cac30e31"));
   await api.controlAutopilot(projectID, id, action, approval); detail = null; await refresh();
  }
  catch (cause) { error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_c4b7640743ef"); }
  finally { pending = false; }
 }
 async function inspect(id: string) {
  pending = true; error = '';
  const project = projectID, generation = epoch;
  try { const result = await api.autopilotDetail(project, id); if (!disposed && generation === epoch && project === projectID) detail = result; }
  catch (cause) { error = cause instanceof APIClientError ? errorMessage(cause) : uiMessage("ui_77b97ff1b9b1"); }
  finally { pending = false; }
 }
 $: liveJob = page?.jobs.some((j) => j.state !== 'completed' && j.state !== 'cancelled') ?? false;
</script>

<div class="space-y-5">
 <div><h1 class="workspace-title">{$t("ui_0c62cfcf5c72")}</h1><p class="muted mt-2">{$t("ui_2ce01a71f470")}</p></div>
 <label class="form-control"><span class="label-text mb-2">{$t("ui_d2668157d668")}</span><select class="select select-bordered" bind:value={projectID} on:change={selectProject} disabled={pending}><option value="">{$t("ui_682279bfae23")}</option>{#each projects as p}<option value={p.id}>{p.title}</option>{/each}</select></label>
 {#if error}<div role="alert" class="alert alert-error">{$text(error)}</div>{/if}
 {#if page}
  {#if !page.worker_available}<div class="alert alert-warning">{$t("ui_24b3b9974157")}</div>{/if}
  {#if !page.model_available}<div class="alert alert-warning">{$t("ui_825570d32a7b")}</div>{/if}
  <form class="workspace-card grid gap-4 p-5 md:grid-cols-3" on:submit|preventDefault={start}>
   <label class="form-control"><span class="label-text mb-2">{$t("ui_50a18643ef46")}</span><input class="input input-bordered" type="number" min="1" max="1000" bind:value={startChapter} /></label>
   <label class="form-control"><span class="label-text mb-2">{$t("ui_73cc2cc6e709")}</span><input class="input input-bordered" type="number" min={startChapter} max="1000" bind:value={targetChapter} /></label>
   <label class="form-control"><span class="label-text mb-2">{$t("ui_3c92d97fe06b")}</span><input class="input input-bordered" type="number" min="0" max="100" bind:value={reviewEvery} /></label>
   <div class="space-y-2 md:col-span-3"><p class="muted">{$t("ui_ea1dd0fc5805")}</p><button class="btn btn-primary" disabled={pending || liveJob || !page.worker_available || !page.model_available} type="submit">{$t("ui_d28efc37caaa")}</button></div>
  </form>
  <div class="space-y-3" aria-label={$t("ui_c5ced4b36c07")}>
   {#each page.jobs as job (job.id)}
    <article class="workspace-card space-y-3 p-5">
     <div class="flex flex-wrap justify-between gap-2"><strong>{$stateLabel(job.state)} · {$stateLabel(job.stage)}</strong><span>{$t("ui_0ee0c123c8ac", {p0: job.completed_through, p1: job.target_chapter})}</span></div>
     <p class="break-all font-mono text-xs">{job.id}</p>
     {#if job.control}<p role="status">{$t("ui_b6acc6180229", {p0: $stateLabel(job.control)})}</p>{/if}
     {#if job.error_code}<p role="status" class="text-warning">{(job.error_code === 'REVIEW_REQUIRED' ? $t("ui_5f7adcfd7ef0") : $codeText(job.error_code))}</p>{/if}
     {#if job.state === 'retrying'}<p class="muted">{$t("ui_0e894cd48a31", {p0: job.retries, p1: job.max_retries, p2: job.next_run})}</p>{/if}
     <div class="flex flex-wrap gap-2">
      <button class="btn btn-sm" disabled={pending} on:click={() => inspect(job.id)}>{$t("ui_e55587482e9f")}</button>
      <button class="btn btn-sm" disabled={pending || !job.actions.pause} on:click={() => control(job.id, 'pause')}>{$t("ui_8d12fc0d4eb2")}</button>
      <button class="btn btn-sm btn-primary" disabled={pending || !job.actions.resume || (job.error_code === 'REVIEW_REQUIRED' && (!detail || detail.job.id !== job.id || detail.job.revision !== job.revision || detail.candidate_id !== job.review_candidate_id))} on:click={() => control(job.id, 'resume')}>{(job.error_code === 'REVIEW_REQUIRED' ? $t("ui_d680c8e07d29") : $t("ui_1d5f407ac7dc"))}</button>
      <button class="btn btn-sm btn-outline" disabled={pending || !job.actions.stop} on:click={() => control(job.id, 'stop')}>{$t("ui_ca4d973c0b00")}</button>
      <a class="btn btn-sm btn-ghost" href={`#/versions?project=${projectID}`}>{$t("ui_0f7f4cdfa1f5")}</a>
     </div>
    </article>
   {:else}<div class="workspace-card p-5 muted">{$t("ui_e26ca049d430")}</div>{/each}
  </div>
 {/if}
 {#if detail}
  <section class="workspace-card space-y-4 p-5" aria-label={$t("ui_f69293a64b1b")}>
   <h2 class="text-xl font-semibold">{$t("ui_e6578fe22cd9", {p0: detail.job.chapter})}</h2>
   <pre class="max-h-[36rem] overflow-auto whitespace-pre-wrap break-words font-sans">{(detail.candidate_text || $t("ui_b4d89cbfd684"))}</pre>
   <details><summary class="cursor-pointer">{$t("ui_eda7897bb136")}</summary><pre class="overflow-auto whitespace-pre-wrap break-words text-xs">{JSON.stringify(detail.chapter_plan, null, 2)}</pre></details>
   <details><summary class="cursor-pointer">{$t("ui_02a51cadcbf3")}</summary><pre class="overflow-auto whitespace-pre-wrap break-words text-xs">{JSON.stringify(detail.foundation, null, 2)}</pre></details>
  </section>
 {/if}
</div>
