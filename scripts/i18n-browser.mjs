#!/usr/bin/env node
/** Real embedded-server audit. Node 22+ and a local Chrome binary, no npm/browser-driver download. */
import assert from 'node:assert/strict';
import {spawn, execFileSync} from 'node:child_process';
import {mkdtempSync, mkdirSync, writeFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import net from 'node:net';
import {randomUUID, createHash} from 'node:crypto';

const binary=path.resolve(process.argv[2]??'./novelforge');
const output=path.resolve(process.env.I18N_AUDIT_OUTPUT??'verification-results/i18n');mkdirSync(output,{recursive:true});
const temporary=mkdtempSync(path.join(tmpdir(),'novelforge-i18n-'));
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
async function unusedPort(){const server=net.createServer();await new Promise(r=>server.listen(0,'127.0.0.1',r));const port=server.address().port;await new Promise(r=>server.close(r));return port;}
const port=await unusedPort(),chromePort=await unusedPort(),base=`http://127.0.0.1:${port}`;
let server,chrome,socket;const processErrors=[],exceptions=[],httpErrors=[],requests=[],routes=[];
const environment={...process.env,HOME:temporary,USERPROFILE:temporary};delete environment.NOVELFORGE_CONFIG;
async function poll(check,description,timeout=15000){const end=Date.now()+timeout;let last;while(Date.now()<end){try{const value=await check();if(value)return value;}catch(e){last=e;}await sleep(75);}throw Error(description+(last?': '+last.message:''));}
async function api(url,method='GET',body){const response=await fetch(base+'/api'+url,{method,headers:{'Content-Type':'application/json','Idempotency-Key':randomUUID()},...(body===undefined?{}:{body:JSON.stringify(body)})});const data=await response.json();assert(response.ok,`${method} ${url}: ${JSON.stringify(data)}`);return data;}
let sequence=0;const pending=new Map();
function send(method,params={}){const id=++sequence;return new Promise((resolve,reject)=>{const timeout=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout: '+method));},20000);pending.set(id,{resolve,reject,timeout});socket.send(JSON.stringify({id,method,params}));});}
async function evaluate(expression){const value=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(value.exceptionDetails)throw Error(value.exceptionDetails.exception?.description??value.exceptionDetails.text);return value.result?.value;}
async function setLanguage(language){await evaluate(`(() => {const select=document.getElementById('workspace-language');if(!select)throw Error('Missing language selector');select.value=${JSON.stringify(language)};select.dispatchEvent(new Event('change',{bubbles:true}));})()`);await poll(()=>evaluate(`document.documentElement.lang===${JSON.stringify(language)}`),'Language change did not apply');await sleep(100);}
const pages=[['dashboard','创作总览','Dashboard'],['projects','项目管理','Projects'],['new','新建小说','New novel'],['chapters','章节','Chapter'],['versions','章节版本','Chapter Versions'],['foreshadows','伏笔管理','Foreshadows'],['secrets','秘密与知情关系','Secrets'],['models','模型','Models'],['logs','日志','Logs'],['settings','设置','Settings'],['autopilot','自动创作','Autopilot'],['authoring','写作技能与资料库','Writing skills and libraries'],['lifecycle','导入、导出与备份','Import, export and backup'],['observability','诊断与费用','Diagnostics & Cost']];
try{
 server=spawn(binary,['server','--host','127.0.0.1','--port',String(port),'--workspace',path.join(temporary,'workspace'),'--no-autopilot'],{env:environment,stdio:['ignore','pipe','pipe']});server.on('error',e=>processErrors.push(String(e)));let serverLog='';server.stdout.on('data',b=>serverLog+=b);server.stderr.on('data',b=>serverLog+=b);
 await poll(async()=>{const r=await fetch(base+'/api/health');return r.ok;},'Embedded server failed to start');
 const project=await api('/projects','POST',{title:'原稿 Original 保留',genre:'test',language:'zh-TW',target_words:3000,target_chapters:3,words_per_chapter:1000});
 const manuscript='# 原稿 Original\n\n这段正文不得翻译、改写或丢失。 Preserve exactly.';
 const version=await api(`/projects/${project.id}/chapters/1/versions`,'POST',{content:manuscript});
 const before=await api(`/projects/${project.id}`),jobsBefore=await api(`/projects/${project.id}/autopilot`),costBefore=await api(`/projects/${project.id}/observability`);
 const chromePath=process.env.CHROME_PATH??execFileSync('which',['google-chrome'],{encoding:'utf8'}).trim();
 chrome=spawn(chromePath,['--headless=new','--no-sandbox','--disable-gpu','--no-first-run','--disable-dev-shm-usage',`--remote-debugging-port=${chromePort}`,`--user-data-dir=${path.join(temporary,'chrome')}`,'about:blank'],{stdio:['ignore','ignore','pipe']});chrome.on('error',e=>processErrors.push(String(e)));
 const target=await poll(async()=>{const targets=await(await fetch(`http://127.0.0.1:${chromePort}/json/list`)).json();return targets.find(t=>t.type==='page');},'Chrome debugger unavailable');
 socket=new WebSocket(target.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{socket.addEventListener('open',resolve,{once:true});socket.addEventListener('error',reject,{once:true});});
 socket.addEventListener('message',event=>{const message=JSON.parse(event.data);if(message.id){const item=pending.get(message.id);if(item){clearTimeout(item.timeout);pending.delete(message.id);message.error?item.reject(Error(JSON.stringify(message.error))):item.resolve(message.result);}return;}if(message.method==='Runtime.exceptionThrown')exceptions.push(message.params.exceptionDetails);if(message.method==='Network.requestWillBeSent')requests.push({method:message.params.request.method,url:message.params.request.url});if(message.method==='Network.responseReceived'&&message.params.response.status>=500)httpErrors.push({status:message.params.response.status,url:message.params.response.url});});
 await send('Page.enable');await send('Runtime.enable');await send('Network.enable');await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await send('Page.navigate',{url:base+'/#/dashboard'});await poll(()=>evaluate("document.querySelector('main h1')?.textContent==='创作总览'"),'Initial Chinese view');
 for(const [route,zh,en]of pages){
  await setLanguage('zh-CN');await evaluate(`location.hash=${JSON.stringify(`/${route}?project=${project.id}&chapter=1`)}`);
  await poll(()=>evaluate(`document.querySelector('main h1')?.textContent===${JSON.stringify(zh)}`),route+' heading');
  await poll(()=>evaluate("!document.querySelector('main .loading-spinner')"),route+' loading');await sleep(350);
  const errors=await evaluate("[...document.querySelectorAll('main .alert-error')].map(e=>e.textContent)");assert.deepEqual(errors,[],route+' server-facing error alerts');
  // Prepare unsaved state using native browser events, then retain actual DOM identities.
  const count=await evaluate(`(() => {
    const main=document.querySelector('main');
    const field=main.querySelector('textarea:not(:disabled),input:not([type]):not(:disabled),input[type=text]:not(:disabled)');
    if(field){field.value='未保存 Unsaved ${route}';field.dispatchEvent(new Event('input',{bubbles:true}));field.focus();field.setSelectionRange?.(2,5);}
    const file=main.querySelector('input[type=file][accept*=".txt"]');
    if(file){const data=new DataTransfer();data.items.add(new File(['原始文件 Original'],'original.md',{type:'text/markdown'}));file.files=data.files;file.dispatchEvent(new Event('change',{bubbles:true}));}
    const details=main.querySelector('details');if(details)details.open=true;
    window.__i18nProbe={main,field,details,controls:[...main.querySelectorAll('input,textarea,select')].filter(e=>e.id!=='settings-language')};
    return window.__i18nProbe.controls.length;
  })()`);await sleep(50);
  await evaluate(`window.__i18nProbe.values=window.__i18nProbe.controls.map(e=>({value:e.value,checked:e.checked??null,files:e.files?[...e.files].map(f=>f.name):[],start:e.selectionStart??null,end:e.selectionEnd??null}));`);
  const requestStart=requests.length;
  await setLanguage('en');
  const state=await evaluate(`(() => {const p=window.__i18nProbe;return {heading:document.querySelector('main h1')?.textContent,samePage:p.main===document.querySelector('main'),sameNodes:p.controls.every(e=>e.isConnected),values:p.controls.map(e=>({value:e.value,checked:e.checked??null,files:e.files?[...e.files].map(f=>f.name):[],start:e.selectionStart??null,end:e.selectionEnd??null})),expected:p.values,details:p.details?p.details.open:true,rawTokens:/NovelForge:message:|NovelForge:error:|ui_[a-f0-9]{12}/.test(document.body.innerText),untranslated:[...document.querySelectorAll('main h1,main h2,main button,main .label-text,main [aria-label]')].map(e=>e.getAttribute('aria-label')??e.textContent).filter(s=>/[\u3400-\u9fff]/.test(s)&&!s.includes('原稿 Original 保留'))};})()`);
  assert.equal(state.heading,en,route+' English heading');assert(state.samePage&&state.sameNodes,route+' remounted page or controls');assert.deepEqual(state.values,state.expected,route+' changed unsaved controls');assert(state.details,route+' closed expanded panel');assert(!state.rawTokens,route+' leaked translation token');assert.deepEqual(state.untranslated,[],route+' untranslated operation text');
  assert.deepEqual(requests.slice(requestStart).filter(r=>!['GET','HEAD','OPTIONS'].includes(r.method)),[],route+' language switch wrote to API');
  routes.push({route,language:'zh-CN',heading:zh,controls:count});routes.push({route,language:'en',heading:en,controls:count,state_preserved:true});
  await setLanguage('zh-CN');
  assert(await evaluate(`window.__i18nProbe.controls.every(e=>e.isConnected)`),route+' lost controls on switching back');
  console.log('BILINGUAL_ROUTE_OK '+route+' controls='+count);
 }
 // Reload only a non-editing page to verify persisted preference; never reload an editor to switch.
 await evaluate("location.hash='/dashboard'");await sleep(250);await setLanguage('en');await send('Page.reload',{ignoreCache:true});await poll(()=>evaluate("document.documentElement.lang==='en' && document.querySelector('main h1')?.textContent==='Dashboard'"),'Persisted English preference after reload');
 for(const language of ['en','zh-CN']){await setLanguage(language);const screenshot=await send('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});writeFileSync(path.join(output,`dashboard-${language}.png`),Buffer.from(screenshot.data,'base64'));}
 const after=await api(`/projects/${project.id}`),jobsAfter=await api(`/projects/${project.id}/autopilot`),costAfter=await api(`/projects/${project.id}/observability`),saved=await api(`/projects/${project.id}/chapters/1/versions/${version.id}`);
 for(const field of ['id','title','language','target_words','total_chapters','completed_chapters'])assert.deepEqual(after[field],before[field],'Project field changed: '+field);
 assert.equal(saved.content,manuscript,'Original manuscript changed');assert.deepEqual(jobsAfter.jobs,jobsBefore.jobs,'Jobs changed');assert.equal(costAfter.totals.calls,costBefore.totals.calls,'Model attempt count changed');assert.equal(costAfter.totals.calls,0,'Unexpected model attempt');
 assert.deepEqual(exceptions,[],'Browser exception');assert.deepEqual(httpErrors,[],'HTTP 5xx');assert.deepEqual(processErrors,[],'Child process failure');
 const report={commit:process.env.GITHUB_SHA??null,run_id:process.env.GITHUB_RUN_ID??null,route_views:routes.length,routes,language_switch_api_writes:0,model_attempts:costAfter.totals.calls,original_content_sha256:createHash('sha256').update(manuscript).digest('hex'),manuscript_language_preserved:after.language,preference_reload:true,browser_exceptions:exceptions,http_5xx:httpErrors,scope:'Linux native binary + real Chrome, no paid models; not full Phase 13B acceptance'};
 writeFileSync(path.join(output,'result.json'),JSON.stringify(report,null,2)+'\n');console.log('BILINGUAL_BROWSER_SUCCESS '+JSON.stringify(report));
}catch(error){writeFileSync(path.join(output,'failure.json'),JSON.stringify({error:String(error),exceptions,httpErrors,routes,requests:requests.slice(-30)},null,2));throw error;}
finally{socket?.close();for(const item of pending.values()){clearTimeout(item.timeout);item.reject(Error('Audit ended'));}chrome?.kill('SIGTERM');server?.kill('SIGTERM');await sleep(250);rmSync(temporary,{recursive:true,force:true});}
