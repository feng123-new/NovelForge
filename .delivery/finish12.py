from pathlib import Path
import json,re,subprocess,sys
root=Path(sys.argv[1]);changed=[]
def rep(name,old,new):
 p=root/name;s=p.read_text();assert s.count(old)==1,(name,old,s.count(old));p.write_text(s.replace(old,new));changed.append(name)
rep('internal/observability/model.go','type Page struct {','type Page struct { Health []Health `json:"health"`;')
rep('internal/observability/store.go','return out,err\n}\n\ntype Reconciliation','if err==nil {out.Health,err=s.Health(ctx,out.State.Policy)}\n return out,err\n}\n\ntype Reconciliation')
rep('internal/observability/store.go',"WHERE state='hold' OR state='failed'","WHERE lower(state)='hold' OR lower(state)='failed'")
rep('internal/observability/store.go','for _,c:=range checks{','checks=append(checks,struct{code,sql,message,action string}{"TOKEN_ESTIMATE_DRIFT",`SELECT count(*) FROM observation_attempts WHERE json_extract(payload_json,\'$.input_tokens\')>json_extract(payload_json,\'$.input_estimate\')*2 AND json_extract(payload_json,\'$.input_tokens\')>1000`,"部分返回用量明显超过本地输入估算。","检查服务商计费字段和上下文；当前费率只按输入/输出总量估算。"})\n for _,c:=range checks{')
rep('internal/qualitygate/services.go','ended := now().UTC()','ended := now().UTC()\n if observability.ControlCode(invokeErr)!="" { return response,ModelCall{},false,invokeErr }')
rep('internal/server/autopilot_engine.go','return j, autopilot.Retry("QUALITY_STEP_ERROR")','var modelFailure *qualitygate.ModelCallError\n if errors.As(err,&modelFailure) && !modelFailure.Retryable {return j,autopilot.Stop(modelFailure.Code)}\n return j, autopilot.Retry("QUALITY_STEP_ERROR")')
rep('internal/server/observations.go','chapter := 1','chapter, nextErr := s.projects.AutopilotNextChapter(r.Context(),id)\n if nextErr!=nil {return nil,nextErr};if chapter>1000 {chapter=1000}') if 'chapter := 1' in (root/'internal/server/observations.go').read_text() else rep('internal/server/observations.go','chapter:=1','chapter,nextErr:=s.projects.AutopilotNextChapter(r.Context(),id);if nextErr!=nil{return nil,nextErr};if chapter>1000{chapter=1000}')
p=root/'internal/server/observations.go';s=p.read_text();s=s.replace('if j.Chapter > chapter {','if !j.Terminal() && j.Chapter > chapter {').replace('if j.Chapter>chapter{','if !j.Terminal()&&j.Chapter>chapter{');p.write_text(s)
rep('web/src/App.svelte',"import Dashboard from './pages/Dashboard.svelte';","import Dashboard from './pages/Dashboard.svelte';\n import Observability from './pages/Observability.svelte';")
rep('web/src/App.svelte','dashboard: Dashboard,','dashboard: Dashboard,\n observability: Observability,')
rep('web/src/App.svelte',"{ name: 'dashboard',", "{ name: 'observability', label: 'Diagnostics & Cost', icon: '◎', href: '#/observability' },\n    { name: 'dashboard',")
rep('web/src/lib/router.ts','export type RouteName =','export type RouteName = \'observability\' |')
rep('web/src/lib/router.ts',"'/dashboard': 'dashboard',","'/dashboard': 'dashboard',\n '/observability': 'observability',")
rep('web/src/lib/api.ts','export class APIClient {', '''import type { ObservationPage, ObservationMutation, ObservationChange, ObservationDiagnostics } from './observability';
export class APIClient {
 observations(id:string,task='',chapter=0,offset=0):Promise<ObservationPage>{const q=new URLSearchParams({task,offset:String(offset),limit:'50'});if(chapter>0)q.set('chapter',String(chapter));return this.request(`/projects/${encodeURIComponent(id)}/observability?${q}`);}
 saveObservations(id:string,input:ObservationMutation):Promise<ObservationChange>{return this.write(`/projects/${encodeURIComponent(id)}/observability`,'POST',input);}
 observationDiagnostics(id:string):Promise<ObservationDiagnostics>{return this.request(`/projects/${encodeURIComponent(id)}/observability/diagnostics`);}
 observationReport(id:string):Promise<Record<string,unknown>>{return this.request(`/projects/${encodeURIComponent(id)}/observability/report`);}
''')
rep('web/src/pages/Observability.svelte',"let reconciliation = '', amount = 0, confirmed = false;","let reconciliation = '', amount = 0, confirmed = false; let sequence=0;")
rep('web/src/pages/Observability.svelte',"if (!id || loading) return; const target=id; loading=true; error='';","if (!id) return; const target=id, generation=++sequence; loading=true; error='';")
rep('web/src/pages/Observability.svelte','if (id!==target) return;','if (id!==target || generation!==sequence) return;')
rep('web/src/pages/Observability.svelte','finally{loading=false;}','finally{if(generation===sequence)loading=false;}')
# Generate typed contracts from the actual new Go DTOs, no duplicate source schemas.
structs={}
for file in (root/'internal/observability').glob('*.go'):
 text=file.read_text()
 for name,body in re.findall(r'type\s+(\w+)\s+struct\s*\{(.*?)\}',text,re.S):
  fields=re.findall(r'(\w+)\s+([\w.\[\]*]+)\s+`json:"([^"`]+)"`',body)
  if fields:structs[name]=fields

def schema(typ):
 if typ.startswith('*'):return {'anyOf':[schema(typ[1:]),{'type':'null'}]}
 if typ.startswith('[]'):return {'type':'array','items':schema(typ[2:])}
 if typ=='time.Time':return {'type':'string','format':'date-time'}
 if typ in ['int','int64']:return {'type':'integer'}
 if typ=='bool':return {'type':'boolean'}
 if typ=='string':return {'type':'string'}
 assert typ in structs,typ
 return {'$ref':'#/components/schemas/Observation'+typ}
components={}
for name,fields in structs.items():
 props={};required=[]
 for field,typ,tag in fields:
  parts=tag.split(',');key=parts[0];props[key]=schema(typ)
  if 'omitempty' not in parts:required.append(key)
 components['Observation'+name]={'type':'object','properties':props,'required':required,'additionalProperties':False}
error={'type':'object','required':['error'],'properties':{'error':{'type':'object','required':['code','message','details','retryable','trace_id'],'properties':{'code':{'type':'string'},'message':{'type':'string'},'details':{'type':'object'},'retryable':{'type':'boolean'},'trace_id':{'type':'string'}}}}}
def response(s):return {'description':'Response','content':{'application/json':{'schema':s}}}
def operation(name,output,post=False):
 op={'operationId':name,'parameters':[{'name':'id','in':'path','required':True,'schema':{'type':'string','minLength':1}}],'responses':{'200':response(output)}}
 for status in [400,404,409,500]:op['responses'][str(status)]=response(error)
 if post:
  op['parameters'].append({'name':'Idempotency-Key','in':'header','required':True,'schema':{'type':'string','minLength':1,'maxLength':128}});op['requestBody']={'required':True,'content':{'application/json':{'schema':schema('Mutation')}}}
 return op
paths={}
base='/api/projects/{id}/observability'
paths[base]={'get':operation('getObservations',schema('Page')),'post':operation('changeObservations',schema('Change'),True)}
for name,typ in [('task','string'),('chapter','integer'),('limit','integer'),('offset','integer')]:paths[base]['get']['parameters'].append({'name':name,'in':'query','schema':{'type':typ}})
paths[base+'/diagnostics']={'get':operation('getObservationDiagnostics',{'type':'object','required':['findings','generated_at','task_history_limit'],'properties':{'findings':{'type':'array','items':schema('Finding')},'generated_at':{'type':'string','format':'date-time'},'task_history_limit':{'type':'integer'}}})}
paths[base+'/report']={'get':operation('exportObservationReport',{'type':'object','description':'Bounded redacted metadata report. No prompts, manuscript, credentials, paths or free-form configuration notes.'})}
p=root/'internal/server/openapi_observability.json';p.write_text(json.dumps({'paths':paths,'components':{'schemas':components}},ensure_ascii=False,indent=2)+'\n');changed.append(str(p.relative_to(root)))
# Keep current behavior separate from historical evidence.
for name in ['README.md','docs/README.md','docs/ROADMAP.md','docs/DEVELOPMENT.md']:
 p=root/name;s=p.read_text();s+='\n\n## Phase 12 — diagnostics and costs\n\nProject-scoped SDK-attempt accounting, conservative pre-call quotas, immutable price snapshots, explicit unknown-cost reconciliation, provider observations and the Diagnostics & Cost page are implemented. See '+('[diagnostics](docs/DIAGNOSTICS.md)' if name=='README.md' else '[diagnostics](DIAGNOSTICS.md)')+'. Phase 13A delivery preparation and Phase 13B local full acceptance remain separate; no full-suite or long-book result is implied.\n';s=s.replace('## Phase 12 — Diagnostics, cost and observability — Deferred; frozen','## Phase 12 — Diagnostics, cost and observability — Implemented; targeted verification only');p.write_text(s);changed.append(name)
subprocess.run(['gofmt','-w',*[str(p) for p in (root/'internal/observability').glob('*.go')],str(root/'internal/server/observations.go'),str(root/'internal/server/autopilot_engine.go'),str(root/'internal/qualitygate/services.go')],check=True)
subprocess.run(['git','add','--',*sorted(set(changed))],cwd=root,check=True)
print('PHASE12_CONTRACTS',len(components),'schemas',len(paths),'paths')
