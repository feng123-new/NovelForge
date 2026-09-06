<script lang="ts">
  import { t, text, codeText, label as stateLabel, message as uiMessage, errorMessage } from '../lib/i18n';
 import { onMount, onDestroy } from 'svelte';
 import { api } from '../lib/api';
 import { currentRoute } from '../lib/router';
 import type { ProjectSummary } from '../lib/types';
 import type { ManuscriptImport, ImportDetail, MigrationResult } from '../lib/lifecycle';
 let projects: ProjectSummary[] = []; let projectId = ''; let imports: ManuscriptImport[] = [];
 let selected = ''; let detail: ImportDetail | null = null; let offset = 0; let listOffset = 0;
 let file: File | null = null; let backupFile: File | null = null; let start = 1;
 let busy = false; let running = false; let stop = false; let disposed = false;
 let modelAvailable = false; let consent = false; let error = ''; let notice = '';
 let format: 'txt'|'md'|'epub' = 'md'; let from = 1; let to = 0; let migration: MigrationResult | null = null;
 const message = (e: unknown) => errorMessage(e);
 onDestroy(() => { disposed = true; stop = true; });
 onMount(() => { void loadProjects(); });
 async function loadProjects(prefer = '') {
  try { projects = (await api.listProjects()).projects; projectId = prefer || currentRoute().query.get('project') || projects.find(p => !p.archived)?.id || ''; if (projectId) await selectProject(); }
  catch(e) { error = message(e); }
 }
 async function selectProject() { selected='';detail=null;offset=0;listOffset=0;migration=null;consent=false;await refresh(); }
 async function refresh() {
  const id = projectId; if (!id) return;
  try { const page = await api.manuscriptImports(id,listOffset); if (id!==projectId || disposed) return; imports=page.imports;modelAvailable=page.model_available; }
  catch(e) { error=message(e); }
 }
 async function inspect(id: string, page=0) {
  const project=projectId;try {const value=await api.manuscriptImport(project,id,page);if(project!==projectId || disposed)return;selected=id;detail=value;offset=page;}catch(e){error=message(e);}
 }
 async function upload() {
  if(!file || !projectId)return;busy=true;error='';notice='';
  try { if(file.size>32*1024*1024)throw new Error(uiMessage("ui_7c39672d8ead"));const value=await api.importManuscript(projectId,file,start);await refresh();await inspect(value.import.id);notice=uiMessage("ui_1d02b69d8208"); }
  catch(e){error=message(e);}finally{busy=false;}
 }
 async function process(analyze: boolean) {
  if(!detail || (analyze && (!consent || !modelAvailable)))return;
  busy=true;running=true;stop=false;error='';notice='';const id=projectId;const batch=selected;
  try { while(!stop && !disposed) {
   const page=await api.manuscriptImport(id,batch,offset);if(disposed)break;detail=page;
   const n=analyze?page.import.next_analysis:page.import.next_save;if(!n)break;
   await api.stepManuscript(id,batch,n,analyze);
  }
  if(!disposed){await inspect(batch,offset);await refresh();notice=stop?uiMessage("ui_2b9c77a4dc87"):uiMessage("ui_aa18c70dda28");}
  }catch(e){error=message(e);if(!disposed)await inspect(batch,offset);}finally{busy=false;running=false;}
 }
 async function download(kind: string) {
  busy=true;error='';try {const blob=await api.downloadLifecycle(projectId,kind,from,to);const u=URL.createObjectURL(blob);const a=document.createElement('a');a.href=u;a.download=kind==='backup'||kind.startsWith('lifecycle-migrate-')?'novelforge-backup.zip':`novelforge.${kind}`;a.click();setTimeout(()=>URL.revokeObjectURL(u),1000);}catch(e){error=message(e);}finally{busy=false;}
 }
 async function restore() {
  if(!backupFile)return;busy=true;error='';notice='';
  try {if(backupFile.size>64*1024*1024)throw new Error(uiMessage("ui_83625e375017"));const result=await api.restoreLifecycle(backupFile);notice=uiMessage("ui_e34a7511337c");await loadProjects(result.project.id);}catch(e){error=message(e);}finally{busy=false;}
 }
 async function migrate() {
  const p=projects.find(p=>p.id===projectId);if(!p)return;busy=true;error='';
  try{migration=await api.migrateLifecycle(projectId,p.format_version??2);notice=migration.changed?uiMessage("ui_44bc8355e244"):uiMessage("ui_edb58cac7dab");}catch(e){error=message(e);}finally{busy=false;}
 }
</script>

<div class="space-y-6">
 <header><h1 class="text-2xl font-semibold">{$t("ui_fb1a00b3abfa")}</h1><p class="muted mt-2">{$t("ui_5a16d5c83627")}</p></header>
 {#if error}<div role="alert" class="alert alert-error">{$text(error)}</div>{/if}
 {#if notice}<div role="status" class="alert alert-info">{$text(notice)}</div>{/if}
 <label class="form-control"><span class="label-text">{$t("ui_d2668157d668")}</span><select class="select select-bordered" bind:value={projectId} on:change={selectProject} disabled={busy}><option value="">{$t("ui_48211ee622b3")}</option>{#each projects as p}<option value={p.id}>{p.title}</option>{/each}</select></label>
 {#if projectId}
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">{$t("ui_b2b6af35211f")}</h2><p class="muted">{$t("ui_c449e53e7aee")}</p>
  <label class="form-control"><span class="label-text">{$t("ui_6f84bf5fa6c9")}</span><input class="file-input file-input-bordered" type="file" accept=".txt,.md,.markdown,.epub" disabled={busy} on:change={(e)=>file=e.currentTarget.files?.[0]??null}/></label>
  <label class="form-control"><span class="label-text">{$t("ui_50a18643ef46")}</span><input class="input input-bordered" type="number" min="1" max="1000" bind:value={start} disabled={busy}/></label>
  <button class="btn btn-primary" disabled={busy||!file} on:click={upload}>{$t("ui_3406287280c9")}</button>
  <div class="flex flex-wrap gap-2">{#each imports as item}<button class="btn btn-outline btn-sm" disabled={busy} on:click={()=>inspect(item.id)}>{$t("ui_bb69dfc7994d", {p0: item.filename, p1: item.saved, p2: item.total})}</button>{/each}</div>
  <div class="flex gap-2"><button class="btn btn-sm" disabled={busy||listOffset===0} on:click={()=>{listOffset-=50;void refresh();}}>{$t("ui_b2987138830e")}</button><button class="btn btn-sm" disabled={busy||imports.length<50} on:click={()=>{listOffset+=50;void refresh();}}>{$t("ui_c11d2cfdfce4")}</button></div>
  {#if detail}
   <p>{$t("ui_7e1092a117f5", {p0: detail.import.filename, p1: detail.import.saved, p2: detail.import.total, p3: detail.import.analyzed})}</p>
   <div class="flex flex-wrap gap-2"><button class="btn btn-secondary" disabled={busy||detail.import.next_save===0} on:click={()=>process(false)}>{$t("ui_73a06eee9c09")}</button><button class="btn btn-outline" disabled={busy||!consent||!modelAvailable||detail.import.next_analysis===0} on:click={()=>process(true)}>{$t("ui_38ec96983db3")}</button><button class="btn btn-warning" disabled={!running} on:click={()=>stop=true}>{$t("ui_0c82f37c6b34")}</button></div>
   <label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={consent} disabled={busy}/>{$t("ui_466f59a0f8b9")}</label>
   {#if !modelAvailable}<p class="muted">{$t("ui_2d2d078c4ef8")}</p>{/if}
   <div class="overflow-x-auto"><table class="table"><thead><tr><th>{$t("ui_f12d1c90df4d")}</th><th>{$t("ui_c3405f8c7d9d")}</th><th>{$t("ui_6320b4a8722a")}</th><th>{$t("ui_5f76b2bf82dd")}</th></tr></thead><tbody>{#each detail.chapters as c}<tr><td>{c.chapter}</td><td>{c.title}</td><td>{$stateLabel(c.state)}{(c.error_code ? ` · ${$codeText(c.error_code)}` : '')}</td><td>{#if c.version_id}<a class="link" href={`#/versions?project=${encodeURIComponent(projectId)}&chapter=${c.chapter}`}>{$t("ui_db6a686f13dc")}</a>{/if}</td></tr>{/each}</tbody></table></div>
   <div class="flex gap-2"><button class="btn btn-sm" disabled={busy||offset===0} on:click={()=>inspect(selected,Math.max(0,offset-50))}>{$t("ui_c9b9ae7a6144")}</button><button class="btn btn-sm" disabled={busy||offset+50>=detail.import.total} on:click={()=>inspect(selected,offset+50)}>{$t("ui_8a8542f69648")}</button></div>
  {/if}
 </section>
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">{$t("ui_c77a574683dd")}</h2><p class="muted">{$t("ui_5b3b4b0cd328")}</p>
  <div class="flex flex-wrap gap-3"><label>{$t("ui_0e8b1c78c5f3")} <select class="select select-bordered" bind:value={format}><option value="md">{$t("ui_0e52f6b9d025")}</option><option value="txt">{$t("ui_d3dc6ac94909")}</option><option value="epub">{$t("ui_3433a1e4005d")}</option></select></label><label>{$t("ui_b012db99df35")} <input class="input input-bordered w-24" type="number" min="1" max="1000" bind:value={from}/> {$t("ui_7b5a5d75887b")}</label><label>{$t("ui_0d561741f946")} <input class="input input-bordered w-24" type="number" min="0" max="1000" bind:value={to}/> {$t("ui_6451ba875995")}</label></div>
  <button class="btn btn-primary" disabled={busy} on:click={()=>download(format)}>{$t("ui_1c8a5eb6d9e1")}</button>
 </section>
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">{$t("ui_c88fa3e155b3")}</h2><p class="muted">{$t("ui_5d19ef835421")}</p>
  <button class="btn btn-outline" disabled={busy} on:click={()=>download('backup')}>{$t("ui_c57b4eb10542")}</button>
  <button class="btn btn-outline" disabled={busy} on:click={migrate}>{$t("ui_fc751942d6a6")}</button>
  {#if migration?.backup_id}<button class="btn btn-sm" disabled={busy} on:click={()=>download(migration!.backup_id)}>{$t("ui_337eae9b967c")}</button>{/if}
 </section>
 {/if}
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">{$t("ui_3618afa25fc6")}</h2><p class="muted">{$t("ui_124161f1cb09")}</p>
  <label class="form-control"><span class="label-text">{$t("ui_9fb7abd461a4")}</span><input class="file-input file-input-bordered" type="file" accept=".zip" disabled={busy} on:change={(e)=>backupFile=e.currentTarget.files?.[0]??null}/></label>
  <button class="btn btn-primary" disabled={busy||!backupFile} on:click={restore}>{$t("ui_266623f7bbad")}</button>
 </section>
</div>
