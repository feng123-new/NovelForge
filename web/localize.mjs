// One-time, fail-closed AST migration. Removed from the delivery tree after review.
import {createRequire} from 'node:module';
import {createHash} from 'node:crypto';
import fs from 'node:fs';
const require=createRequire(import.meta.url),{parse}=require('svelte/compiler'),ts=require('typescript');
const catalogue=new Map(),missing=new Set();
const normalize=s=>s.replace(/\s+/g,' ').trim();
for(const line of fs.readFileSync('web/i18n-translations.tsv','utf8').trim().split('\n')){
 const [raw,translation,corrected]=line.split('\t'); if(!raw||!translation)throw Error('Invalid catalogue line: '+line);
 const source=normalize(raw),chinese=/[\u3400-\u9fff]/.test(source);
 const pair=chinese?[corrected||source,translation]:[translation,source];
 if(catalogue.has(source)&&JSON.stringify(catalogue.get(source))!==JSON.stringify(pair))throw Error('Conflicting translation: '+source);
 catalogue.set(source,pair);
}
const extras={
 '操作状态已更新。':['操作状态已更新。','Operation status updated.'],
 '项目已创建，但基础规划请求失败。':['项目已创建，但基础规划请求失败。','Project created, but the foundation request failed.'],
 'v{p0}':['版本 {p0}','v{p0}'],
 'New chapter':['新章节','New chapter'],
 'Build Diff':['生成差异比较','Build diff'],
 'Diffing…':['比较中…','Comparing…'],
};
for(const [k,v]of Object.entries(extras))catalogue.set(k,v);
const key=s=>'ui_'+createHash('sha256').update(normalize(s)).digest('hex').slice(0,12);
function lookup(raw){const s=normalize(raw);if(!catalogue.has(s)){missing.add(s);catalogue.set(s,[s,s]);}return JSON.stringify(key(s));}
function edits(source,changes){changes.sort((a,b)=>b[0]-a[0]);let end=source.length+1;for(const [a,b,v]of changes){if(b>end)throw Error('Overlapping edits');source=source.slice(0,a)+v+source.slice(b);end=a;}return source;}
function walk(node,visit,parent=null){if(!node||typeof node!=='object')return;if(visit(node,parent)===false)return;for(const [k,v]of Object.entries(node)){if(['loc','start','end'].includes(k))continue;if(Array.isArray(v))v.forEach(n=>walk(n,visit,node));else walk(v,visit,node);}}
const paths=['web/src/App.svelte',...fs.readdirSync('web/src/pages').filter(x=>x.endsWith('.svelte')).map(x=>'web/src/pages/'+x)];
const results=new Map();
for(const path of paths){
 let source=fs.readFileSync(path,'utf8');if(source.includes('message as uiMessage'))continue;
 // Freeze implicit machine values before translating option labels.
 source=source.replace(/<option>([^<{}]+)<\/option>/g,(_m,value)=>`<option value=${JSON.stringify(value)}>${value}</option>`);
 if(path.endsWith('/Chapters.svelte'))source=source.replace("let title = '新章节';","let title = '';");
 if(path.endsWith('/ChapterVersions.svelte'))source=source.replace("let rejectReason = 'Rejected by human review';","let rejectReason = '';");
 if(path.endsWith('/Authoring.svelte'))source=source.replace(/function message\(cause: unknown\) \{[^\n]+\}/, 'function message(cause: unknown) { return errorMessage(cause); }');
 if(path.endsWith('/Lifecycle.svelte'))source=source.replace(/const message = \(e: unknown\) =>[^\n]+/, 'const message = (e: unknown) => errorMessage(e);');
 source=source.replace(/\bcause\.message\b/g,'errorMessage(cause)').replace(/error\s*=\s*String\(e\)/g,'error=errorMessage(e)');
 if(path.endsWith('/NewNovelWizard.svelte'))source=source.replace('errors = [createdID ? `项目已创建，但 Foundation 请求失败：${message}` : message];',`errors = createdID ? [uiMessage(${lookup('项目已创建，但基础规划请求失败。')}), message] : [message];`);
 if(path.endsWith('/Foreshadows.svelte'))source=source.replace('success = `${item.title} → ${status}`;',`success = uiMessage(${lookup('操作状态已更新。')});`);
 const ast=parse(source),changes=[];
 walk(ast.instance?.content,(n,parent)=>{
  const navLabel=n.type==='Literal'&&parent?.type==='Property'&&parent.key?.name==='label';
  if(n.type==='Literal'&&typeof n.value==='string'&&(/[\u3400-\u9fff]/.test(n.value)||navLabel||n.value==='Dashboard')){
    changes.push([n.start,n.end,`uiMessage(${lookup(n.value)})`]);return false;
  }
  if(n.type==='TemplateLiteral'&&n.quasis.some(q=>/[\u3400-\u9fff]/.test(q.value.cooked??q.value.raw))){
   // A duplicate's title is business data; retain its original, locale-independent suffix.
   const raw=source.slice(n.start,n.end);if(raw==='`${project.title} 副本`')return false;
   const template=n.quasis.map((q,i)=>(q.value.cooked??q.value.raw)+(i<n.expressions.length?`{p${i}}`:'')).join('');
   const args=n.expressions.map((e,i)=>{let v=source.slice(e.start,e.end);if(['action','status'].includes(v))v=`labelArgument(${v})`;return `p${i}: ${v}`;}).join(', ');
   const call=`uiMessage(${lookup(template)}, {${args}})`;
   changes.push([n.start,n.end,parent?.type==='CallExpression'&&source.slice(parent.callee.start,parent.callee.end)==='globalThis.prompt'?`$text(${call})`:call]);return false;
  }
 });
 source=edits(source,changes);
 const html=parse(source).html,patches=[];
 const machineProps=new Set(['state','status','stage','control','importance','urgency','type','author_type','authority','cost_source','operation','agent','role','kind','automationMode','reviewPolicy','severity','derived_state','error_code']);
 const serverProps=new Set(['hold_reason','last_reason','suggested_action','action_required','reason','message','action']);
 const slice=n=>source.slice(n.start,n.end);
 function expression(n){
  if(!n)return '';const raw=slice(n);
  if(n.type==='Literal'&&typeof n.value==='string'){
   if(!/[a-zA-Z\u3400-\u9fff]/.test(n.value))return raw;
   // Raw technical codes remain available, not mistaken for prose.
   if(/^[A-Z]+(?:_[A-Z]+)+$/.test(n.value))return raw;
   if(catalogue.has(normalize(n.value))||/[\u3400-\u9fff]/.test(n.value))return `$t(${lookup(n.value)})`;
   if(/^[a-z_]+$/.test(n.value))return `$label(${raw})`;
   return `$t(${lookup(n.value)})`;
  }
  if(n.type==='ConditionalExpression')return `(${slice(n.test)} ? ${expression(n.consequent)} : ${expression(n.alternate)})`;
  if(n.type==='LogicalExpression')return `(${expression(n.left)} ${n.operator} ${expression(n.right)})`;
  if(n.type==='ChainExpression')return expression(n.expression);
  if(n.type==='TemplateLiteral'){
   const template=n.quasis.map((q,i)=>(q.value.cooked??q.value.raw)+(i<n.expressions.length?`{p${i}}`:'')).join('');
   if(!/[a-zA-Z\u3400-\u9fff]/.test(template.replace(/\{p\d+\}/g,'')))return raw;
   return `$t(${lookup(template)}, {${n.expressions.map((e,i)=>`p${i}: ${expression(e)}`).join(', ')}})`;
  }
  if(['error','notice','success','result','routeLabel','label'].includes(raw)||raw==='item.label')return `$text(${raw})`;
  if(raw==='$connectionState'||raw==='name.replaceAll(\'_\', \' \')'||raw==='String(value)')return `$label(${raw.startsWith('name.')?'name':raw==='String(value)'?'value':raw})`;
  if(n.type==='MemberExpression'&&!n.computed){
   const prop=n.property.name;
   if(machineProps.has(prop))return `$label(${raw})`;
   if(serverProps.has(prop))return `$serverText(${raw})`;
  }
  if(raw.startsWith('continuityStatus('))return `$label(${raw})`;
  if(raw.startsWith('money('))return `$text(${raw})`;
  return raw.replace(/\.toLocaleString\(\)/g,'.toLocaleString($locale)');
 }
 function phrase(nodes){
  if(!nodes.length)return;
  let template='',i=0;const args=[];
  for(const n of nodes){if(n.type==='Text')template+=n.data;else{template+=`{p${i}}`;args.push(`p${i++}: ${expression(n.expression)}`);}}
  const clean=normalize(template),human=/[a-zA-Z\u3400-\u9fff]/.test(clean.replace(/\{p\d+\}/g,''));
  if(human){const start=nodes[0].start,end=nodes.at(-1).end;const prefix=template.match(/^\s*/)[0],suffix=template.match(/\s*$/)[0];patches.push([start,end,`${prefix}{$t(${lookup(clean)}${args.length?', {'+args.join(', ')+'}':''})}${suffix}`]);}
  else for(const n of nodes)if(n.type==='MustacheTag'){const next=expression(n.expression);if(next!==slice(n.expression))patches.push([n.expression.start,n.expression.end,next]);}
 }
 function node(n){if(!n||typeof n!=='object')return;
  if(n.type==='Attribute'){
   if(['aria-label','title','placeholder','alt'].includes(n.name)&&Array.isArray(n.value)){
    if(n.value.length===1&&n.value[0].type==='MustacheTag'){const e=n.value[0].expression;patches.push([n.start,n.end,`${n.name}={${expression(e)}}`]);}
    else if(n.value.every(v=>v.type==='Text')){const s=n.value.map(v=>v.data).join('');if(s.trim())patches.push([n.start,n.end,`${n.name}={$t(${lookup(s)})}`]);}
   }return;
  }
  if(n.attributes)for(const a of n.attributes)node(a);
  if(n.children){let run=[];for(const c of n.children){if(['Text','MustacheTag'].includes(c.type))run.push(c);else{phrase(run);run=[];node(c);}}phrase(run);}
  if(n.else)node(n.else);if(n.pending)node(n.pending);if(n.then)node(n.then);if(n.catch)node(n.catch);
 }
 node(html);source=edits(source,patches);
 const root=path==='web/src/App.svelte'?'./lib/i18n':'../lib/i18n';
 source=source.replace('<script lang="ts">',`<script lang="ts">\n  import { t, text, label, serverText, locale, message as uiMessage, errorMessage, labelArgument } from '${root}';`);
 if(path==='web/src/App.svelte'){
  source=source.replace("import { onMount } from 'svelte';","import { onMount } from 'svelte';\n  import LanguageSwitcher from './components/LanguageSwitcher.svelte';");
  source=source.replace('      <button class="btn btn-square btn-ghost"','      <LanguageSwitcher />\n      <button class="btn btn-square btn-ghost"');
  source=source.replace('</script>',`</script>\n\n<svelte:head><title>{$t(${lookup('NovelForge Workspace')})}</title><meta name="description" content={$t(${lookup('NovelForge local long-form fiction workspace')})} /></svelte:head>`);
 }
 if(path.endsWith('/Settings.svelte')){
  source=source.replace("import { onMount } from 'svelte';","import { onMount } from 'svelte';\n  import LanguageSwitcher from '../components/LanguageSwitcher.svelte';");
  source=source.replace('<div class="space-y-5">',`<div class="space-y-5">\n  <section class="workspace-card p-6"><h2 class="text-lg font-semibold">{$t(${lookup('界面语言')})}</h2><div class="mt-3"><LanguageSwitcher id="settings-language" /></div><p class="muted mt-3">{$t(${lookup('语言仅改变界面，不改变作品、未保存编辑或任务，也不调用模型。')})}</p></section>`);
 }
 results.set(path,source);
}
// Validation messages remain locale-neutral between creation and rendering.
const wizardPath='web/src/lib/wizard.ts';let wizard=fs.readFileSync(wizardPath,'utf8');if(!wizard.includes('message as uiMessage')){
 const ast=ts.createSourceFile(wizardPath,wizard,ts.ScriptTarget.Latest,true),patches=[];
 function visit(n){if(ts.isStringLiteral(n)&&/[\u3400-\u9fff]/.test(n.text))patches.push([n.getStart(ast),n.end,`uiMessage(${lookup(n.text)})`]);else ts.forEachChild(n,visit);}visit(ast);
 wizard="import { message as uiMessage } from './i18n';\n"+edits(wizard,patches);results.set(wizardPath,wizard);
}
if(missing.size){console.log('MISSING_TRANSLATIONS '+JSON.stringify([...missing]));throw Error('Missing translations: '+missing.size);}
const zh={},en={};for(const [s,pair]of catalogue){zh[key(s)]=pair[0];en[key(s)]=pair[1];}
fs.mkdirSync('web/src/lib/i18n',{recursive:true});
fs.writeFileSync('web/src/lib/i18n/zh-CN.ts','// Stable keys shared with en.ts. Keep placeholder sets equal.\nexport const zhCN = '+JSON.stringify(zh,null,2)+' as const;\nexport type MessageKey = keyof typeof zhCN;\n');
fs.writeFileSync('web/src/lib/i18n/en.ts',"import type { MessageKey } from './zh-CN';\nexport const en = "+JSON.stringify(en,null,2)+' satisfies Record<MessageKey, string>;\n');
for(const [path,source]of results)fs.writeFileSync(path,source);
let main=fs.readFileSync('web/src/main.ts','utf8');if(!main.includes('initializeLocale'))main="import { initializeLocale } from './lib/i18n';\n"+main.replace(/const app\s*=/,'initializeLocale();\n\nconst app =');fs.writeFileSync('web/src/main.ts',main);
for(const path of ['web/package.json','web/package-lock.json']){const p=JSON.parse(fs.readFileSync(path,'utf8'));p.version='0.1.0-rc.2';if(p.packages?.[''])p.packages[''].version=p.version;if(path.endsWith('package.json'))p.scripts['test:i18n']='vitest run src/tests/i18n.test.ts src/tests/i18n-pages.test.ts';fs.writeFileSync(path,JSON.stringify(p,null,2)+'\n');}
console.log('Localized '+results.size+' source files; '+catalogue.size+' paired messages.');
