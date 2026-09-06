<script lang="ts">
  import { t, text, findingText, label as stateLabel, message as uiMessage, errorMessage } from '../lib/i18n';
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
 function message(cause: unknown) { return errorMessage(cause); }
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
  try{await api.saveAuthoring(projectID,input);notice=uiMessage("ui_4b3fbbdf65aa");await load(true);}
  catch(cause){error=message(cause);}
  finally{pending=false;}
 }
 async function importMarkdown(event: Event) {
  const input=event.currentTarget as HTMLInputElement, file=input.files?.[0];if(!file)return;
  if(file.size>16384||!file.name.toLowerCase().endsWith('.md')){error=uiMessage("ui_3eb8e884d169");input.value='';return;}
  pending=true;error='';try{const text=await file.text();if(!disposed){draft={...draft,markdown:text,title:draft.title||file.name.replace(/\.md$/i,'')};}}catch(cause){error=message(cause);}finally{pending=false;input.value='';}
 }
 async function search() {if(pending||!projectID)return;pending=true;error='';try{searchResults=(await api.searchAuthoring(projectID,kind,query,chapter,pov)).entries;}catch(cause){error=message(cause);}finally{pending=false;}}
 async function lint() {if(pending||!projectID)return;pending=true;error='';try{lintResult=await api.lintAuthoring(projectID,chapter,lintText);}catch(cause){error=message(cause);}finally{pending=false;}}
 async function turnPage(next:number){offset=next;await load();}
</script>

<div class="space-y-6">
 <header><h1 class="workspace-title">{$t("ui_3493ed2f487e")}</h1><p class="muted mt-2">{$t("ui_79d264583ef7")}</p></header>
 <div class="grid gap-4 md:grid-cols-2">
  <label class="form-control"><span class="label-text">{$t("ui_d2668157d668")}</span><select class="select select-bordered" bind:value={projectID} disabled={pending} on:change={()=>load(true)}><option value="">{$t("ui_682279bfae23")}</option>{#each projects as p}<option value={p.id}>{p.title}</option>{/each}</select></label>
  <label class="form-control"><span class="label-text">{$t("ui_d577bc940207")}</span><select class="select select-bordered" bind:value={kind} disabled={pending} on:change={()=>load(true)}><option value="skill">{$t("ui_66d0f523a379")}</option><option value="style">{$t("ui_7062b2c214ad")}</option><option value="knowledge">{$t("ui_433bdcb25776")}</option></select></label>
 </div>
 {#if error}<div role="alert" class="alert alert-error">{$text(error)}</div>{/if}
 {#if notice}<div role="status" class="alert alert-success">{$text(notice)}</div>{/if}
 {#if pending}<p role="status">{$t("ui_bca2ddb8e850")}</p>{/if}
 {#if state}
 <p class="muted">{$t("ui_627036ce0c76", {p0: state.revision, p1: state.total})}</p>
 {#if kind==='skill'}<details class="workspace-card p-4"><summary>{$t("ui_8254f269ad75")}</summary>{#each state.builtins as builtin}<details class="mt-3"><summary>{$stateLabel(builtin.role)}</summary><pre class="whitespace-pre-wrap text-sm">{builtin.markdown}</pre></details>{/each}</details>{/if}
 <div class="grid gap-5 lg:grid-cols-2">
  <section class="workspace-card space-y-3 p-5" aria-label={$t("ui_9883b7374bba")}>
   <div class="flex justify-between"><h2 class="text-lg font-semibold">{$t("ui_79e58b7a846b")}</h2><button class="btn btn-sm" disabled={pending} on:click={()=>{draft=emptyEntry(kind);confirmDelete=false;}}>{$t("ui_414c535430d2")}</button></div>
   {#each state.entries as entry(entry.id)}<button class="btn btn-outline h-auto min-h-12 w-full justify-start py-2 text-left" disabled={pending} on:click={()=>edit(entry)}>{entry.title} · {$stateLabel(entry.role||entry.kind)} · {(entry.enabled ? $t("ui_f4f0ead1116b") : $t("ui_4e6fd0e28c55"))}{(entry.pinned ? $t("ui_745ced272ce2") : '')}</button>{:else}<p class="muted">{$t("ui_865ff06081f7")}</p>{/each}
   <div class="flex gap-2"><button class="btn btn-sm" disabled={pending||offset===0} on:click={()=>turnPage(Math.max(0,offset-50))}>{$t("ui_c9b9ae7a6144")}</button><button class="btn btn-sm" disabled={pending||offset+state.entries.length>=state.total} on:click={()=>turnPage(offset+50)}>{$t("ui_8a8542f69648")}</button></div>
  </section>
  <form class="workspace-card space-y-4 p-5" on:submit|preventDefault={()=>mutate('entry')} aria-label={$t("ui_55d27f1df900")}>
   <h2 class="text-lg font-semibold">{(draft.id ? $t("ui_30be4a83c453") : $t("ui_414c535430d2"))}</h2>
   <label class="form-control"><span class="label-text">{$t("ui_c3405f8c7d9d")}</span><input class="input input-bordered" bind:value={draft.title} required maxlength="160" disabled={pending}/></label>
   {#if kind==='skill'}<label class="form-control"><span class="label-text">{$t("ui_d7ed78212896")}</span><select class="select select-bordered" bind:value={draft.role} disabled={pending}><option value="writing">{$t("ui_c5fb1100cc23")}</option><option value="review">{$t("ui_f59f6b11a6cf")}</option><option value="polish">{$t("ui_d3b0be6ad155")}</option><option value="planning">{$t("ui_bc9fe8bac483")}</option></select></label>{/if}
   <label class="form-control"><span class="label-text">{$t("ui_a5499a9eb044")}</span><textarea class="textarea textarea-bordered min-h-48" bind:value={draft.markdown} required disabled={pending}></textarea></label>
   <label class="form-control"><span class="label-text">{$t("ui_7074efe201e8")}</span><input class="file-input file-input-bordered" type="file" accept=".md,text/markdown" disabled={pending} on:change={importMarkdown}/></label>
   <label class="form-control"><span class="label-text">{$t("ui_e3adecb360fa")}</span><input class="input input-bordered" bind:value={draft.source} disabled={pending}/></label>
   <div class="grid grid-cols-3 gap-3"><label class="form-control"><span class="label-text">{$t("ui_565d64601d4d")}</span><input class="input input-bordered" type="number" min="0" max="100" bind:value={draft.priority} disabled={pending}/></label><label class="form-control"><span class="label-text">{$t("ui_8ea3d20ba234")}</span><input class="input input-bordered" type="number" min="0" max="1000" bind:value={draft.from_chapter} disabled={pending}/></label><label class="form-control"><span class="label-text">{$t("ui_4eeb390068a9")}</span><input class="input input-bordered" bind:value={draft.pov} disabled={pending}/></label></div>
   <div class="flex gap-5"><label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={draft.enabled} disabled={pending}/>{$t("ui_f4f0ead1116b")}</label>{#if kind!=='skill'}<label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={draft.pinned} disabled={pending}/>{$t("ui_e00d27fc31ea")}</label>{/if}</div>
   <button class="btn btn-primary" type="submit" disabled={pending}>{$t("ui_a9101e16194e")}</button>
   {#if draft.id}<div class="flex flex-wrap items-center gap-3"><label><input type="checkbox" bind:checked={confirmDelete} disabled={pending}/>{$t("ui_531082c4c849")}</label><button class="btn btn-error btn-outline btn-sm" type="button" disabled={pending||!confirmDelete} on:click={()=>mutate('delete')}>{$t("ui_e21576a75f6f")}</button></div>{/if}
  </form>
 </div>
 <section class="workspace-card space-y-4 p-5">
  <h2 class="text-lg font-semibold">{$t("ui_0580caa93196")}</h2><p class="muted">{$t("ui_fc9f5734b991")}</p>
  <div class="grid gap-3 md:grid-cols-3"><label class="form-control"><span class="label-text">{$t("ui_97e406603ba9")}</span><input class="input input-bordered" bind:value={query} disabled={pending}/></label><label class="form-control"><span class="label-text">{$t("ui_f693862d5f24")}</span><input class="input input-bordered" type="number" min="1" max="1000" bind:value={chapter} disabled={pending}/></label><label class="form-control"><span class="label-text">{$t("ui_3a18d21bf0c7")}</span><input class="input input-bordered" bind:value={pov} disabled={pending}/></label></div>
  <button class="btn btn-outline" disabled={pending||!query.trim()} on:click={search}>{$t("ui_1fe7868593bd")}</button>
  {#each searchResults as result}<details><summary>{result.title}</summary><pre class="whitespace-pre-wrap break-words">{result.markdown}</pre></details>{/each}
 </section>
 <section class="workspace-card space-y-4 p-5" aria-label={$t("ui_a33dc91c4d62")}>
  <h2 class="text-lg font-semibold">{$t("ui_173a0d98d482")}</h2><p class="muted">{$t("ui_08f3f0230548")}</p>
  <label class="flex items-center gap-2"><input class="checkbox" type="checkbox" bind:checked={state.rules.enabled} disabled={pending}/>{$t("ui_4e3f9712d19c")}</label>
  <label class="form-control"><span class="label-text">{$t("ui_b70a186b99ba")}</span><textarea class="textarea textarea-bordered" bind:value={phrases} disabled={pending}></textarea></label>
  <div class="grid gap-3 md:grid-cols-4"><label class="form-control"><span class="label-text">{$t("ui_4336111226d5")}</span><input class="input input-bordered" type="number" min="0" max="100" bind:value={state.rules.max_phrase_occurrences} disabled={pending}/></label><label class="form-control"><span class="label-text">{$t("ui_b3193132ae13")}</span><input class="input input-bordered" type="number" min="1" max="20" bind:value={state.rules.max_sentence_repeats} disabled={pending}/></label><label class="form-control"><span class="label-text">{$t("ui_e8663eec398d")}</span><input class="input input-bordered" type="number" min="4" max="200" bind:value={state.rules.min_sentence_runes} disabled={pending}/></label><label class="form-control"><span class="label-text">{$t("ui_10b0b0c37a55")}</span><input class="input input-bordered" type="number" min="0" max="3" bind:value={state.rules.previous_chapters} disabled={pending}/></label></div>
  <button class="btn btn-primary" disabled={pending} on:click={()=>mutate('rules')}>{$t("ui_b5aa919e9a97")}</button>
  <label class="form-control"><span class="label-text">{$t("ui_10137b907e57")}</span><textarea class="textarea textarea-bordered min-h-32" bind:value={lintText} disabled={pending}></textarea></label><button class="btn btn-outline" disabled={pending||!lintText.trim()} on:click={lint}>{$t("ui_3048bd65be99")}</button>
  {#if lintResult}<p role="status">{$t("ui_8911c0a44b91", {p0: lintResult.revision, p1: lintResult.report.findings.length, p2: (lintResult.report.truncated ? $t("ui_8e78e1143e15") : '')})}</p>{#each lintResult.report.findings as finding}<p class="text-sm break-words">{$findingText(finding)}</p>{/each}{/if}
 </section>
 {/if}
</div>
