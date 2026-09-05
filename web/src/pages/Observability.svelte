<script lang="ts">
 import { onMount } from 'svelte';
 import { api } from '../lib/api';
 import { recentEvents } from '../lib/events';
 import type { ProjectSummary } from '../lib/types';
 import type { ObservationPage, ObservationPolicy, ObservationFinding } from '../lib/observability';
 let projects: ProjectSummary[] = [], id = '', page: ObservationPage | null = null, findings: ObservationFinding[] = [];
 let error = '', notice = '', busy = false, loading = false, offset = 0, chapter = 0, task = '';
 let draft: ObservationPolicy | null = null, draftRevision = 0, projectBudget = 0, taskBudget = 0, paused = '';
 let reconciliation = '', amount = 0, confirmed = false; let sequence=0;
 const money = (n: number | null, currency = 'USD') => n === null ? '未知' : `${currency} ${(n / 1e6).toFixed(6)}`;
 function resetPolicy() { if (!page) return; draft = structuredClone(page.settings.policy); draftRevision = page.settings.revision; projectBudget=draft.project_budget_micros/1e6;taskBudget=draft.task_budget_micros/1e6;paused=draft.paused_providers.join(', '); }
 async function load(reset = false) {
  if (!id) return; const target=id, generation=++sequence; loading=true; error='';
  try { const [p,d]=await Promise.all([api.observations(target,task,chapter,offset),api.observationDiagnostics(target)]); if (id!==target || generation!==sequence) return; page=p;findings=d.findings;if (reset || !draft) resetPolicy(); }
  catch(e){if(id===target) error=String(e);} finally{if(generation===sequence)loading=false;}
 }
 async function choose() { page=null;draft=null;offset=0;task='';chapter=0;reconciliation='';confirmed=false;await load(true); }
 async function save() { if(!id||!draft)return;busy=true;error='';notice='';try{const p=structuredClone(draft);p.project_budget_micros=Math.round(projectBudget*1e6);p.task_budget_micros=Math.round(taskBudget*1e6);p.paused_providers=paused.split(',').map(v=>v.trim()).filter(Boolean);await api.saveObservations(id,{expected_revision:draftRevision,policy:p});notice='配置已保存；历史价格快照不会被改写。';await load(true);}catch(e){error=String(e);}finally{busy=false;} }
 async function reconcile() {if(!page||!reconciliation||!confirmed)return;busy=true;error='';try{await api.saveObservations(id,{expected_revision:page.settings.revision,reconcile:{attempt_id:reconciliation,cost_micros:Math.round(amount*1e6),confirm:true}});reconciliation='';confirmed=false;await load(true);notice='已记录人工对账，原始未知记录保留在修改历史中。';}catch(e){error=String(e);}finally{busy=false;} }
 async function downloadReport(){try{const data=await api.observationReport(id);const url=URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=url;a.download='novelforge-diagnostics.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}catch(e){error=String(e);}}
 onMount(()=>{let alive=true;let last=0;api.listProjects().then(p=>{if(!alive)return;projects=p.projects;id=new URLSearchParams(location.hash.split('?')[1]??'').get('project')??projects[0]?.id??'';void load(true);}).catch(e=>error=String(e));const stop=recentEvents.subscribe(events=>{const e=events[0];if(e&&typeof e.id==='number'&&e.id!==last&&e.project===id&&e.type==='observability.changed'){last=e.id;void load();}});const timer=setInterval(()=>void load(),10000);return()=>{alive=false;stop();clearInterval(timer);};});
</script>

<section class="space-y-6">
 <div><p class="text-xs uppercase tracking-widest text-primary">Phase 12</p><h1 class="text-2xl font-bold">Diagnostics & Cost</h1><p class="mt-2 text-sm opacity-70">实际请求尝试、未知费用与本地重放分开记录。价格表估算不是服务商最终账单。</p></div>
 <div class="flex flex-wrap gap-3"><label class="form-control flex-1"><span class="label-text">小说项目</span><select class="select select-bordered" bind:value={id} on:change={choose} disabled={busy}>{#each projects as p}<option value={p.id}>{p.title}</option>{/each}</select></label><button class="btn" disabled={!id||busy||loading} on:click={()=>load()}>刷新</button><button class="btn btn-outline" disabled={!id} on:click={downloadReport}>导出脱敏诊断</button></div>
 {#if error}<div role="alert" class="alert alert-error"><span>{error}</span></div>{/if}
 {#if notice}<div role="status" class="alert alert-success">{notice}</div>{/if}
 {#if page}
 <div class="grid gap-3 md:grid-cols-4">
  <div class="rounded-xl border border-base-300 p-4"><p class="text-sm opacity-60">已知费用小计</p><strong>{money(page.totals.known_cost_micros,page.settings.policy.currency)}</strong><p class="text-xs">未知费用 {page.totals.unknown_cost} 次；执行中 {page.totals.pending} 次</p></div>
  <div class="rounded-xl border border-base-300 p-4"><p class="text-sm opacity-60">实际尝试</p><strong>{page.totals.calls}</strong><p class="text-xs">成功 {page.totals.completed} / 失败或未知结束 {page.totals.failed}</p></div>
  <div class="rounded-xl border border-base-300 p-4"><p class="text-sm opacity-60">返回用量（不含未知）</p><strong>{page.totals.input_tokens} → {page.totals.output_tokens}</strong><p class="text-xs">{page.totals.unknown_usage} 次没有返回用量</p></div>
  <div class="rounded-xl border border-base-300 p-4"><p class="text-sm opacity-60">项目级缓存重放</p><strong>{page.replays}</strong><p class="text-xs">未单独追踪的旧逻辑调用 {page.legacy_untracked_calls}；不会补算为零费用</p></div>
 </div>
 <div class="rounded-xl border border-base-300 p-4"><h2 class="text-lg font-semibold">待处理问题</h2>{#if findings.length===0}<p class="mt-2 text-sm">当前限定检查未发现待处理项，不等于全项目验收通过。</p>{/if}{#each findings as f}<div class="mt-3 border-t border-base-300 pt-3"><strong>{f.code} × {f.count}</strong><p>{f.message}</p><p class="text-sm opacity-70">{f.action}</p>{#if f.task_id}<a class="link" href={`#/autopilot?project=${encodeURIComponent(id)}`}>打开任务</a>{/if}{#if f.chapter}<a class="link ml-3" href={`#/versions?project=${encodeURIComponent(id)}&chapter=${f.chapter}`}>章节 {f.chapter} 版本</a>{/if}</div>{/each}</div>
 <div class="rounded-xl border border-base-300 p-4"><h2 class="text-lg font-semibold">服务商观察状态</h2><p class="text-xs opacity-60">只依据已记录的请求，不自动发起连通性或付费探测。</p>{#each page.health ?? [] as h}<p class="mt-2 text-sm"><strong>{h.provider}</strong> · {h.manually_paused?'手动暂停':h.state} · 连续失败 {h.consecutive_failures} {h.last_error}{#if h.cooldown_until} · 冷却至 {h.cooldown_until}{/if}</p>{/each}</div>
 <details class="rounded-xl border border-base-300 p-4"><summary class="cursor-pointer font-semibold">预算、模型价格和保护配置</summary>
 {#if draft}<form class="mt-4 space-y-4" on:submit|preventDefault={save}>
 <p class="text-sm">编辑基于修订 {draftRevision}。任务运行中须先暂停；保存不自动恢复任务。预算与次数为 0 表示不启用该上限，手动章节操作归入 manual 范围。</p>
 <div class="grid gap-3 md:grid-cols-3">
 <label class="form-control"><span class="label-text">币种（已有调用后不可更改）</span><input class="input input-bordered" bind:value={draft.currency} maxlength="3" required /></label>
 <label class="form-control"><span class="label-text">项目预算（币种单位）</span><input class="input input-bordered" type="number" min="0" step="0.000001" bind:value={projectBudget} required /></label>
 <label class="form-control"><span class="label-text">每任务预算</span><input class="input input-bordered" type="number" min="0" step="0.000001" bind:value={taskBudget} required /></label>
 <label class="form-control"><span class="label-text">项目尝试次数上限</span><input class="input input-bordered" type="number" min="0" max="1000000" bind:value={draft.project_max_calls} required /></label>
 <label class="form-control"><span class="label-text">每任务尝试次数上限</span><input class="input input-bordered" type="number" min="0" max="1000000" bind:value={draft.task_max_calls} required /></label>
 <label class="form-control"><span class="label-text">单次最大输出 Token</span><input class="input input-bordered" type="number" min="128" max="65536" bind:value={draft.max_output_tokens} required /></label>
 <label class="form-control"><span class="label-text">输入估算上限（非计费用量）</span><input class="input input-bordered" type="number" min="256" max="1000000" bind:value={draft.max_input_estimate} required /></label>
 <label class="form-control"><span class="label-text">连续失败触发阈值</span><input class="input input-bordered" type="number" min="1" max="20" bind:value={draft.failure_threshold} required /></label>
 <label class="form-control"><span class="label-text">冷却秒数</span><input class="input input-bordered" type="number" min="1" max="86400" bind:value={draft.cooldown_seconds} required /></label>
 </div>
 <label class="form-control"><span class="label-text">暂停的服务商别名（逗号分隔）</span><input class="input input-bordered" bind:value={paused}/></label>
 <label class="flex items-center gap-2"><input class="checkbox checkbox-sm" type="checkbox" bind:checked={draft.require_known_price}/>缺少单价时阻止请求</label>
 <label class="flex items-center gap-2"><input class="checkbox checkbox-sm" type="checkbox" bind:checked={draft.block_unknown_cost}/>存在未知费用时暂停（启用金额预算时始终生效）</label>
 <h3 class="font-semibold">精确模型价格表</h3><p class="text-xs opacity-60">与模型配置中的服务商别名、模型 ID 完全匹配。输入单位为微币种／百万 Token：例如 1 USD／百万 Token 填 1000000；显式填 0 才表示免费。当前不单独折算缓存及其他服务商特殊计费。</p>
 {#each draft.prices as price,i}<div class="grid gap-2 md:grid-cols-5"><input aria-label={`服务商 ${i+1}`} class="input input-bordered input-sm" bind:value={price.provider} placeholder="provider alias" required/><input aria-label={`模型 ${i+1}`} class="input input-bordered input-sm" bind:value={price.model} placeholder="model id" required/><input aria-label={`输入单价 ${i+1}`} class="input input-bordered input-sm" type="number" min="0" max="1000000000" bind:value={price.input_micros_per_million} required/><input aria-label={`输出单价 ${i+1}`} class="input input-bordered input-sm" type="number" min="0" max="1000000000" bind:value={price.output_micros_per_million} required/><button class="btn btn-sm" type="button" on:click={()=>{if(draft)draft.prices=draft.prices.filter((_,n)=>n!==i);}}>移除</button></div>{/each}
 <button class="btn btn-sm" type="button" disabled={draft.prices.length>=100} on:click={()=>{if(draft)draft.prices=[...draft.prices,{provider:'',model:'',input_micros_per_million:0,output_micros_per_million:0}];}}>添加价格</button>
 <div class="flex gap-3"><button class="btn btn-primary" disabled={busy}>保存预算和价格</button><button type="button" class="btn btn-outline" on:click={resetPolicy}>放弃编辑并载入最新修订</button></div>
 </form>{/if}</details>
 <div><h2 class="text-lg font-semibold">角色与模型汇总</h2><div class="overflow-x-auto"><table class="table table-sm"><thead><tr><th>角色</th><th>服务商 / 模型</th><th>次数</th><th>已知费用</th><th>未知费用</th></tr></thead><tbody>{#each page.groups as g}<tr><td>{g.agent}</td><td>{g.provider} / {g.model}</td><td>{g.totals.calls}</td><td>{money(g.totals.known_cost_micros,page.settings.policy.currency)}</td><td>{g.totals.unknown_cost}</td></tr>{/each}</tbody></table></div></div>
 <form class="flex flex-wrap gap-2" on:submit|preventDefault={()=>{offset=0;void load();}}><input aria-label="任务 ID 筛选" class="input input-bordered input-sm" bind:value={task} placeholder="任务 ID 或 manual"/><input aria-label="章节筛选" class="input input-bordered input-sm" type="number" min="0" max="1000" bind:value={chapter}/><button class="btn btn-sm">筛选（章节 0 为全部）</button></form>
 <div class="overflow-x-auto"><table class="table table-sm"><thead><tr><th>时间 / 章节</th><th>业务请求</th><th>角色 / 模型</th><th>状态</th><th>返回用量</th><th>费用</th><th>处理</th></tr></thead><tbody>{#each page.attempts as a}<tr><td>{a.started_at}<br/>第 {a.chapter} 章</td><td class="font-mono text-xs">{a.logical_id.slice(0,12)}<br/>{a.task_id}</td><td>{a.agent}:{a.operation}<br/>{a.provider}/{a.model}</td><td>{a.state}<br/>{a.error_code??''}</td><td>{a.input_tokens??'未知'} → {a.output_tokens??'未知'}</td><td><span>{money(a.cost_micros,a.currency)}</span><br/><span>{a.cost_source}</span></td><td>{#if a.cost_micros===null}<button class="btn btn-xs" on:click={()=>{reconciliation=a.id;amount=0;confirmed=false;}}>核对费用</button>{/if}</td></tr>{/each}</tbody></table></div>
 <div class="flex gap-3"><button class="btn btn-sm" disabled={offset===0||loading} on:click={()=>{offset=Math.max(0,offset-50);void load();}}>上一页</button><span>{offset+1}–{Math.min(offset+50,page.total)} / {page.total}</span><button class="btn btn-sm" disabled={offset+50>=page.total||loading} on:click={()=>{offset+=50;void load();}}>下一页</button></div>
 {#if reconciliation}<form class="rounded-xl border border-warning p-4 space-y-3" on:submit|preventDefault={reconcile}><h3 class="font-semibold">明确对账未知请求</h3><p class="text-sm">先暂停任务并确认请求不再执行。对账会保留原记录，不会把未知用量改成零。填写本次核对的费用，不确定时不要确认。</p><label class="form-control"><span class="label-text">本次费用（{page.settings.policy.currency}）</span><input class="input input-bordered" type="number" min="0" step="0.000001" bind:value={amount} required/></label><label class="flex gap-2"><input type="checkbox" class="checkbox" bind:checked={confirmed}/>已核对服务商记录，确认该金额</label><button class="btn btn-warning" disabled={!confirmed||busy}>记录人工对账</button><button class="btn ml-2" type="button" on:click={()=>reconciliation=''}>取消</button></form>{/if}
 {/if}
</section>
