# Materialize reviewed display mappings. This migration helper is not shipped.
from pathlib import Path
import json,re
root=Path('.')
def read(p): return (root/p).read_text()
def put(p,s): (root/p).write_text(s)
def edit(p,a,b):
 s=read(p)
 if a not in s: raise Exception(f'Missing reviewed source: {p}: {a[:80]}')
 put(p,s.replace(a,b))
new={
 'foreshadow.transition':['{p0} → {p1}','{p0} → {p1}'],
 'technical.details':['原始技术详情','Original technical details'],
 'technical.note':['原始记录保持原文，不随界面语言修改。','Original records are preserved without translation.'],
 'workspace.loading':['正在加载…','Loading…'],
}
for fn,index in [('zh-CN.ts',0),('en.ts',1)]:
 p='web/src/lib/i18n/'+fn;s=read(p);pos=s.rfind('\n}')
 s=s[:pos].rstrip()+',\n'+',\n'.join('  '+json.dumps(k)+': '+json.dumps(v[index],ensure_ascii=False) for k,v in new.items())+s[pos:];put(p,s)
data=json.loads(read('web/review-system.json'))
p='web/src/lib/i18n/system.ts';s=read(p)
for name,values in [('labels',data['labels']),('errorCodes',data['errors']),('serverMessages',{k:[v,k] for k,v in data['backend'].items()})]:
 s+='\nObject.assign('+name+', '+json.dumps(values,ensure_ascii=False,indent=2)+');\n'
for category,codes in data['aliases'].items():s+=f'for (const code of {json.dumps(codes.split())}) errorCodes[code] = errorCodes.{category};\n'
put(p,s)
for name in ['Authoring','Observability','Autopilot','Lifecycle']:
 p=f'web/src/pages/{name}.svelte';s=read(p).replace('t, text, label as stateLabel,','t, text, findingText, codeText, label as stateLabel,')
 s=s.replace('$serverText(finding.message)','$findingText(finding)').replace('$serverText(f.message)','$findingText(f)').replace('$serverText(f.action)',"$findingText(f, 'action')")
 s=s.replace('<strong>{f.code} × {f.count}</strong>','<strong>{$findingText(f, \'title\')} × {f.count}</strong><span class="ml-2 font-mono text-xs">{f.code}</span>')
 s=s.replace('<p class="text-sm opacity-70">{$findingText(f, \'action\')}</p>','<p class="text-sm opacity-70">{$findingText(f, \'action\')}</p><details class="mt-2 text-xs"><summary>{$t("technical.details")}</summary><p>{f.message}</p><p>{f.action}</p></details>')
 s=s.replace('p2: h.last_error','p2: $codeText(h.last_error)').replace("$stateLabel(a.error_code??'')","$codeText(a.error_code??'')").replace('$stateLabel(job.error_code)','$codeText(job.error_code)')
 s=s.replace("` · ${c.error_code}`","` · ${$codeText(c.error_code)}`")
 put(p,s)
edit('web/src/pages/Foreshadows.svelte','success = uiMessage("ui_315f90a7f073");','success = uiMessage("foreshadow.transition", { p0: item.title, p1: labelArgument(status) });')
edit('web/src/pages/Dashboard.test.ts',"screen.findByText('workspace unavailable')","screen.findByText('工作区暂不可用，请检查本地服务和工作区目录。 (WORKSPACE_UNAVAILABLE)')")
edit('web/src/pages/Foreshadows.test.ts',"screen.findByText('ledger unavailable')","screen.findByText('叙事账本暂不可用，请检查项目数据库后重试。 (LEDGER_UNAVAILABLE)')")
edit('web/src/pages/Foreshadows.test.ts',"'trace trace-ledger-page'","'追踪标识：trace-ledger-page'")
edit('web/src/pages/Foreshadows.test.ts',"'OVERDUE +5'","'逾期 5 章'")
edit('web/src/pages/Foreshadows.test.ts',"'The sealed gate → resolved'","'The sealed gate → 已回收'")
edit('web/src/pages/Secrets.test.ts',"screen.findByText('secret store unavailable')","screen.findByText('秘密存储暂不可用，请检查项目数据库后重试。 (SECRET_STORE_UNAVAILABLE)')")
edit('web/src/pages/Secrets.test.ts',"'trace trace-secret-page'","'追踪标识：trace-secret-page'")
edit('web/src/pages/Secrets.test.ts',"'Add from Chapter 50'","'从第 50 章起添加知情关系'")
edit('web/src/pages/Secrets.test.ts',"The heir's origin 已在 Chapter 50 公开","The heir's origin 已在第 50 章公开")
edit('web/src/pages/Observability.test.ts','expect(screen.getByText("未知")).toBeTruthy();',"const attemptRow = screen.getByText('未知 → 未知').closest('tr')!; const costCell = attemptRow.querySelectorAll('td')[5]; expect(costCell).toHaveTextContent('未知'); expect(costCell).not.toHaveTextContent('USD 0');")
for p in [root/'web/src/App.svelte',*(root/'web/src/pages').glob('*.svelte')]:
 s=p.read_text();m=re.search(r"  import \{ ([^\n]+) \} from '[.]+/lib/i18n';",s)
 if not m: continue
 rest=s[:m.start()]+s[m.end():];parts=[]
 for part in m.group(1).split(', '):
  local=part.split(' as ')[-1]
  if re.search(r'(?<![\w])\$?'+re.escape(local)+r'(?!\w)',rest):parts.append(part)
 s=s[:m.start()]+m.group(0).replace(m.group(1),', '.join(parts))+s[m.end():];p.write_text(s)
p=root/'web/src/pages/Settings.svelte';lines=p.read_text().splitlines();pref=next(i for i,x in enumerate(lines) if '<section class="workspace-card p-6"><h2' in x and 'settings-language' in x);lines[pref],lines[pref+1]=lines[pref+1],lines[pref];p.write_text('\n'.join(lines)+'\n')
p=root/'web/package.json';d=json.loads(p.read_text());d['scripts']['check:i18n']='node check-i18n.mjs';d['scripts']['test:i18n']='vitest run src/tests/i18n';p.write_text(json.dumps(d,indent=2)+'\n')
print('Applied reviewed enum, diagnostic and operation display refinements.')
