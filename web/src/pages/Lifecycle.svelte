<script lang="ts">
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
 const message = (e: unknown) => e instanceof Error ? e.message : '操作失败，请查看项目状态。';
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
  try { if(file.size>32*1024*1024)throw new Error('正文文件上限为 32 MiB。');const value=await api.importManuscript(projectId,file,start);await refresh();await inspect(value.import.id);notice='原文已暂存。继续导入只创建候选版本，不调用模型、不自动接受。'; }
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
  if(!disposed){await inspect(batch,offset);await refresh();notice=stop?'已在当前步骤结束后暂停；可以继续。':'本轮处理完成。分析完成不等于已接受或已定稿。';}
  }catch(e){error=message(e);if(!disposed)await inspect(batch,offset);}finally{busy=false;running=false;}
 }
 async function download(kind: string) {
  busy=true;error='';try {const blob=await api.downloadLifecycle(projectId,kind,from,to);const u=URL.createObjectURL(blob);const a=document.createElement('a');a.href=u;a.download=kind==='backup'||kind.startsWith('lifecycle-migrate-')?'novelforge-backup.zip':`novelforge.${kind}`;a.click();setTimeout(()=>URL.revokeObjectURL(u),1000);}catch(e){error=message(e);}finally{busy=false;}
 }
 async function restore() {
  if(!backupFile)return;busy=true;error='';notice='';
  try {if(backupFile.size>64*1024*1024)throw new Error('备份文件上限为 64 MiB。');const result=await api.restoreLifecycle(backupFile);notice='已恢复项目。模型凭据需要重新配置；没有自动恢复写作任务。';await loadProjects(result.project.id);}catch(e){error=message(e);}finally{busy=false;}
 }
 async function migrate() {
  const p=projects.find(p=>p.id===projectId);if(!p)return;busy=true;error='';
  try{migration=await api.migrateLifecycle(projectId,p.format_version??2);notice=migration.changed?'迁移完成，原状态备份已保留。':'项目格式和数据库已是当前版本。';}catch(e){error=message(e);}finally{busy=false;}
 }
</script>

<div class="space-y-6">
 <header><h1 class="text-2xl font-semibold">导入、导出与备份</h1><p class="muted mt-2">原文 → 候选版本 → 显式分析与审核 → 定稿。导入和恢复不会自动启动付费模型。</p></header>
 {#if error}<div role="alert" class="alert alert-error">{error}</div>{/if}
 {#if notice}<div role="status" class="alert alert-info">{notice}</div>{/if}
 <label class="form-control"><span class="label-text">小说项目</span><select class="select select-bordered" bind:value={projectId} on:change={selectProject} disabled={busy}><option value="">请选择项目</option>{#each projects as p}<option value={p.id}>{p.title}</option>{/each}</select></label>
 {#if projectId}
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">导入作品</h2><p class="muted">UTF-8 TXT／Markdown 或未加密、以 XHTML 正文为主的 EPUB。最多 1000 章，每章 1 MiB；请检查识别出的标题和章节顺序。</p>
  <label class="form-control"><span class="label-text">正文文件</span><input class="file-input file-input-bordered" type="file" accept=".txt,.md,.markdown,.epub" disabled={busy} on:change={(e)=>file=e.currentTarget.files?.[0]??null}/></label>
  <label class="form-control"><span class="label-text">起始章节</span><input class="input input-bordered" type="number" min="1" max="1000" bind:value={start} disabled={busy}/></label>
  <button class="btn btn-primary" disabled={busy||!file} on:click={upload}>暂存并检查章节</button>
  <div class="flex flex-wrap gap-2">{#each imports as item}<button class="btn btn-outline btn-sm" disabled={busy} on:click={()=>inspect(item.id)}>{item.filename} · {item.saved}/{item.total} 已导入</button>{/each}</div>
  <div class="flex gap-2"><button class="btn btn-sm" disabled={busy||listOffset===0} on:click={()=>{listOffset-=50;void refresh();}}>上一批记录</button><button class="btn btn-sm" disabled={busy||imports.length<50} on:click={()=>{listOffset+=50;void refresh();}}>下一批记录</button></div>
  {#if detail}
   <p>{detail.import.filename}：{detail.import.saved}/{detail.import.total} 候选已保存；{detail.import.analyzed} 章已分析。</p>
   <div class="flex flex-wrap gap-2"><button class="btn btn-secondary" disabled={busy||detail.import.next_save===0} on:click={()=>process(false)}>继续导入候选（不调用模型）</button><button class="btn btn-outline" disabled={busy||!consent||!modelAvailable||detail.import.next_analysis===0} on:click={()=>process(true)}>继续分析（调用模型）</button><button class="btn btn-warning" disabled={!running} on:click={()=>stop=true}>当前步骤后暂停</button></div>
   <label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={consent} disabled={busy}/>我确认使用已配置的模型分析原文，可能产生费用。</label>
   {#if !modelAvailable}<p class="muted">尚未配置分析模型；仍可保存候选版本。</p>{/if}
   <div class="overflow-x-auto"><table class="table"><thead><tr><th>章节</th><th>标题</th><th>状态</th><th>版本</th></tr></thead><tbody>{#each detail.chapters as c}<tr><td>{c.chapter}</td><td>{c.title}</td><td>{c.state}{c.error_code?` · ${c.error_code}`:''}</td><td>{#if c.version_id}<a class="link" href={`#/versions?project=${encodeURIComponent(projectId)}&chapter=${c.chapter}`}>查看、审核与定稿</a>{/if}</td></tr>{/each}</tbody></table></div>
   <div class="flex gap-2"><button class="btn btn-sm" disabled={busy||offset===0} on:click={()=>inspect(selected,Math.max(0,offset-50))}>上一页</button><button class="btn btn-sm" disabled={busy||offset+50>=detail.import.total} on:click={()=>inspect(selected,offset+50)}>下一页</button></div>
  {/if}
 </section>
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">导出有效定稿</h2><p class="muted">只导出连续、提交完整且与章节文件同步的 Final。不会把候选稿混入成品。</p>
  <div class="flex flex-wrap gap-3"><label>格式 <select class="select select-bordered" bind:value={format}><option value="md">Markdown</option><option value="txt">TXT</option><option value="epub">EPUB</option></select></label><label>从第 <input class="input input-bordered w-24" type="number" min="1" max="1000" bind:value={from}/> 章</label><label>至第 <input class="input input-bordered w-24" type="number" min="0" max="1000" bind:value={to}/> 章（0 为全部）</label></div>
  <button class="btn btn-primary" disabled={busy} on:click={()=>download(format)}>导出定稿</button>
 </section>
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">项目备份与格式迁移</h2><p class="muted">备份包含正文、版本、事实、资料和导入进度，不包含模型配置、凭据、工作区任务、日志和旧备份。正文中的敏感内容不会自动脱敏。</p>
  <button class="btn btn-outline" disabled={busy} on:click={()=>download('backup')}>下载项目备份</button>
  <button class="btn btn-outline" disabled={busy} on:click={migrate}>备份并迁移项目格式</button>
  {#if migration?.backup_id}<button class="btn btn-sm" disabled={busy} on:click={()=>download(migration!.backup_id)}>下载迁移前备份</button>{/if}
 </section>
 {/if}
 <section class="rounded-2xl border border-base-300 p-5 space-y-3">
  <h2 class="text-lg font-semibold">从备份恢复</h2><p class="muted">保留原项目 ID，恢复到新目录。同 ID 项目已存在时拒绝覆盖；请在另一个工作区恢复。未完成任务不会自动复活。</p>
  <label class="form-control"><span class="label-text">项目 ZIP 备份</span><input class="file-input file-input-bordered" type="file" accept=".zip" disabled={busy} on:change={(e)=>backupFile=e.currentTarget.files?.[0]??null}/></label>
  <button class="btn btn-primary" disabled={busy||!backupFile} on:click={restore}>验证并恢复到新目录</button>
 </section>
</div>
