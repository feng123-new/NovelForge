<script lang="ts">
 import { onMount } from 'svelte';
 import { api, APIClientError } from '../lib/api';
 import { currentRoute } from '../lib/router';
 import { emptyEntry, type AuthoringKind, type AuthoringEntry, type AuthoringState, type AuthoringLint } from '../lib/authoring';
 import type { ProjectList } from '../lib/types';
 let projects: ProjectList['projects'] = [];
 let projectID = currentRoute().query.get('project') ?? '';
 let kind: AuthoringKind = 'skill';
 let state: AuthoringState | null = null;
 let draft = emptyEntry(kind);
 let pending = false, error = '', notice = '', disposed = false, epoch = 0;
 let offset = 0, phrases = '', confirmDelete = false;
 let query = '', chapter = 1, pov = '', searchResults: AuthoringEntry[] = [];
 let lintText = '', lintResult: AuthoringLint | null = null;
 function message(cause: unknown) { return cause instanceof APIClientError ? `${cause.payload?.code ?? cause.status}: ${cause.message}` : cause instanceof Error ? cause.message : '操作失败'; }
 async function load(reset = false) {
  if (reset) { offset = 0; draft = emptyEntry(kind); confirmDelete = false; searchResults = []; lintResult = null; }
  const generation=++epoch, id=projectID;
  state=null;
  if (!id) return;
  pending=true; error='';
  try { const data=await api.authoring(id,kind,offset); if(!disposed&&generation===epoch){state=data; phrases=data.rules.phrases.join('\n');} }
  catch(cause){if(!disposed&&generation===epoch)error=message(cause);}
  finally{if(!disposed&&generation===epoch)pending=false;}
 }
 onMount(()=>{void(async()=>{try{const result=await api.listProjects();if(disposed)return;projects=result.projects.filter(p=>!p.archived);if(!projectID)projectID=projects[0]?.id??'';await load(true);}catch(cause){if(!disposed)error=message(cause);}})();return()=>{disposed=true;epoch++;};});
 function edit(entry: AuthoringEntry) { draft={...entry}; confirmDelete=false; notice=''; }
 async function mutate(mode:'entry'|'rules'|'delete') {
  if(pending||!state)return;
  if(mode==='delete'&&(!draft.id||!confirmDelete))return;
  pending=true;error='';notice='';
  const input=mode==='entry'?{expected_revision:state.revision,entry:draft}:mode==='delete'?{expected_revision:state.revision,delete_id:draft.id}:{expected_revision:state.revision,rules:{...state.rules,phrases:phrases.split('\n').map(p=>p.trim()).filter(Boolean)}};
  try{await api.saveAuthoring(projectID,input);notice='已保存到当前项目。新请求使用更新；已有请求保留其选定资料。';await load(true);}
  catch(cause){error=message(cause);}
  finally{pending=false;}
 }
 async function importMarkdown(event: Event) {
  const input=event.currentTarget as HTMLInputElement, file=input.files?.[0];if(!file)return;
  if(file.size>16384||!file.name.toLowerCase().endsWith('.md')){error='仅支持不超过 16 KiB 的 Markdown 文件';input.value='';return;}
  pending=true;error='';try{const text=await file.text();if(!disposed){draft={...draft,markdown:text,title:draft.title||file.name.replace(/\.md$/i,'')};}}catch(cause){error=message(cause);}finally{pending=false;input.value='';}
 }
 async function search() {if(pending||!projectID)return;pending=true;error='';try{searchResults=(await api.searchAuthoring(projectID,kind,query,chapter,pov)).entries;}catch(cause){error=message(cause);}finally{pending=false;}}
 async function lint() {if(pending||!projectID)return;pending=true;error='';try{lintResult=await api.lintAuthoring(projectID,chapter,lintText);}catch(cause){error=message(cause);}finally{pending=false;}}
 async function turnPage(next:number){offset=next;await load();}
</script>

<div class="space-y-6">
 <header><h1 class="workspace-title">Skills · 风格库 · 资料库</h1><p class="muted mt-2">Markdown 写作方法与参考资料，不是小说事实。不会执行脚本、访问资料链接或绕过定稿审核。</p></header>
 <div class="grid gap-4 md:grid-cols-2">
  <label class="form-control"><span class="label-text">小说项目</span><select class="select select-bordered" bind:value={projectID} disabled={pending} on:change={()=>load(true)}><option value="">选择项目</option>{#each projects as p}<option value={p.id}>{p.title}</option>{/each}</select></label>
  <label class="form-control"><span class="label-text">资源分类</span><select class="select select-bordered" bind:value={kind} disabled={pending} on:change={()=>load(true)}><option value="skill">Skills</option><option value="style">风格库</option><option value="knowledge">资料库</option></select></label>
 </div>
 {#if error}<div role="alert" class="alert alert-error">{error}</div>{/if}
 {#if notice}<div role="status" class="alert alert-success">{notice}</div>{/if}
 {#if pending}<p role="status">正在读取或提交…</p>{/if}
 {#if state}
 <p class="muted">项目资源修订 {state.revision} · 当前分类共 {state.total} 项。任务运行中需先暂停才能编辑；规划后变更会触发旧计划保护。</p>
 {#if kind==='skill'}<details class="workspace-card p-4"><summary>内置 Markdown Skills（只读；自定义规则作为补充）</summary>{#each state.builtins as builtin}<details class="mt-3"><summary>{builtin.role}</summary><pre class="whitespace-pre-wrap text-sm">{builtin.markdown}</pre></details>{/each}</details>{/if}
 <div class="grid gap-5 lg:grid-cols-2">
  <section class="workspace-card space-y-3 p-5" aria-label="资源列表">
   <div class="flex justify-between"><h2 class="text-lg font-semibold">项目资源</h2><button class="btn btn-sm" disabled={pending} on:click={()=>{draft=emptyEntry(kind);confirmDelete=false;}}>新建资源</button></div>
   {#each state.entries as entry(entry.id)}<button class="btn btn-outline h-auto min-h-12 w-full justify-start py-2 text-left" disabled={pending} on:click={()=>edit(entry)}>{entry.title} · {entry.role||entry.kind} · {entry.enabled?'启用':'停用'}{entry.pinned?' · 置顶':''}</button>{:else}<p class="muted">此分类尚无自定义资源。</p>{/each}
   <div class="flex gap-2"><button class="btn btn-sm" disabled={pending||offset===0} on:click={()=>turnPage(Math.max(0,offset-50))}>上一页</button><button class="btn btn-sm" disabled={pending||offset+state.entries.length>=state.total} on:click={()=>turnPage(offset+50)}>下一页</button></div>
  </section>
  <form class="workspace-card space-y-4 p-5" on:submit|preventDefault={()=>mutate('entry')} aria-label="资源编辑">
   <h2 class="text-lg font-semibold">{draft.id?'编辑资源':'新建资源'}</h2>
   <label class="form-control"><span class="label-text">标题</span><input class="input input-bordered" bind:value={draft.title} required maxlength="160" disabled={pending}/></label>
   {#if kind==='skill'}<label class="form-control"><span class="label-text">Skill 类型</span><select class="select select-bordered" bind:value={draft.role} disabled={pending}><option value="writing">写作 Writing</option><option value="review">审稿 Review</option><option value="polish">润色 Polish</option><option value="planning">规划 Planning</option></select></label>{/if}
   <label class="form-control"><span class="label-text">Markdown 内容（最多 16 KiB）</span><textarea class="textarea textarea-bordered min-h-48" bind:value={draft.markdown} required disabled={pending}></textarea></label>
   <label class="form-control"><span class="label-text">导入单份 Markdown</span><input class="file-input file-input-bordered" type="file" accept=".md,text/markdown" disabled={pending} on:change={importMarkdown}/></label>
   <label class="form-control"><span class="label-text">来源说明（只作文本记录，不自动访问）</span><input class="input input-bordered" bind:value={draft.source} disabled={pending}/></label>
   <div class="grid grid-cols-3 gap-3"><label class="form-control"><span class="label-text">优先级</span><input class="input input-bordered" type="number" min="0" max="100" bind:value={draft.priority} disabled={pending}/></label><label class="form-control"><span class="label-text">可用起始章</span><input class="input input-bordered" type="number" min="0" max="1000" bind:value={draft.from_chapter} disabled={pending}/></label><label class="form-control"><span class="label-text">限定 POV（可空）</span><input class="input input-bordered" bind:value={draft.pov} disabled={pending}/></label></div>
   <div class="flex gap-5"><label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={draft.enabled} disabled={pending}/>启用</label>{#if kind!=='skill'}<label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={draft.pinned} disabled={pending}/>置顶候选</label>{/if}</div>
   <button class="btn btn-primary" type="submit" disabled={pending}>保存资源</button>
   {#if draft.id}<div class="flex flex-wrap items-center gap-3"><label><input type="checkbox" bind:checked={confirmDelete} disabled={pending}/>确认删除当前资源</label><button class="btn btn-error btn-outline btn-sm" type="button" disabled={pending||!confirmDelete} on:click={()=>mutate('delete')}>删除资源</button></div>{/if}
  </form>
 </div>
 <section class="workspace-card space-y-4 p-5">
  <h2 class="text-lg font-semibold">检索预览</h2><p class="muted">只返回已启用且符合章节／POV 范围的当前分类资料。实际模型上下文仍受预算裁剪；每类最多检索 6 项，加最多 6 项置顶候选。</p>
  <div class="grid gap-3 md:grid-cols-3"><label class="form-control"><span class="label-text">查询词</span><input class="input input-bordered" bind:value={query} disabled={pending}/></label><label class="form-control"><span class="label-text">预览章节</span><input class="input input-bordered" type="number" min="1" max="1000" bind:value={chapter} disabled={pending}/></label><label class="form-control"><span class="label-text">预览 POV</span><input class="input input-bordered" bind:value={pov} disabled={pending}/></label></div>
  <button class="btn btn-outline" disabled={pending||!query.trim()} on:click={search}>检索当前分类</button>
  {#each searchResults as result}<details><summary>{result.title}</summary><pre class="whitespace-pre-wrap break-words">{result.markdown}</pre></details>{/each}
 </section>
 <section class="workspace-card space-y-4 p-5" aria-label="写作规则">
  <h2 class="text-lg font-semibold">表达与重复规则</h2><p class="muted">只生成可解释的编辑建议，不判定作品是否由 AI 写成，不自动扣分或绕过一致性门禁。跨章比较最多使用前 3 章有效定稿的前 48,000 字符。</p>
  <label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={state.rules.enabled} disabled={pending}/>启用规则检查</label>
  <label class="form-control"><span class="label-text">关注词组（每行一个，最多 32 个）</span><textarea class="textarea textarea-bordered" bind:value={phrases} disabled={pending}></textarea></label>
  <div class="grid gap-3 md:grid-cols-4"><label class="form-control"><span class="label-text">单章词组次数上限</span><input class="input input-bordered" type="number" min="0" max="100" bind:value={state.rules.max_phrase_occurrences} disabled={pending}/></label><label class="form-control"><span class="label-text">同句次数上限</span><input class="input input-bordered" type="number" min="1" max="20" bind:value={state.rules.max_sentence_repeats} disabled={pending}/></label><label class="form-control"><span class="label-text">最短比较句长</span><input class="input input-bordered" type="number" min="4" max="200" bind:value={state.rules.min_sentence_runes} disabled={pending}/></label><label class="form-control"><span class="label-text">比较前文章数</span><input class="input input-bordered" type="number" min="0" max="3" bind:value={state.rules.previous_chapters} disabled={pending}/></label></div>
  <button class="btn btn-primary" disabled={pending} on:click={()=>mutate('rules')}>保存规则</button>
  <label class="form-control"><span class="label-text">试检文本（使用已保存规则与上面的预览章节，不调用模型）</span><textarea class="textarea textarea-bordered min-h-32" bind:value={lintText} disabled={pending}></textarea></label><button class="btn btn-outline" disabled={pending||!lintText.trim()} on:click={lint}>检查表达与重复</button>
  {#if lintResult}<p role="status">规则修订 {lintResult.revision} · {lintResult.report.findings.length} 项建议{lintResult.report.truncated?'（结果已截断）':''}</p>{#each lintResult.report.findings as finding}<p class="text-sm break-words">{finding.message}</p>{/each}{/if}
 </section>
 {/if}
</div>
