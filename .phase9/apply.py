#!/usr/bin/env python3
"""Apply the reviewed Phase 9 changes to an isolated fixed-base worktree."""
import json, pathlib, shutil, subprocess, sys
BASE='f16f753530c71ad7ca2221c82a93efd1a0455c2e'
src=pathlib.Path(__file__).resolve().parents[1]
root=pathlib.Path(sys.argv[1]).resolve()
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()==BASE
assert not subprocess.check_output(['git','status','--porcelain'],cwd=root,text=True).strip()
new_paths='''internal/autopilot/model.go
internal/autopilot/store.go
internal/autopilot/runner.go
internal/autopilot/store_test.go
internal/project/autopilot.go
internal/project/autopilot_foundation.go
internal/server/autopilot.go
internal/server/autopilot_engine.go
internal/server/autopilot_test.go
internal/server/repository/autopilot.go
web/src/lib/autopilot.ts
web/src/pages/Autopilot.svelte
web/src/pages/Autopilot.test.ts
docs/AUTOPILOT.md'''.splitlines()
touched=set(new_paths)
for p in new_paths:
 target=root/p;assert not target.exists(),p
 target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(src/p,target)
def edit(p,old,new,count=1):
 f=root/p;text=f.read_text();assert text.count(old)==count,(p,old,text.count(old),count)
 f.write_text(text.replace(old,new));touched.add(p)
edit('internal/autopilot/model.go',' FoundationID string `json:"foundation_id"`',' FoundationID string `json:"foundation_id"`\n ModelProfile map[string]string `json:"model_profile,omitempty"`')
edit('internal/autopilot/model.go','func (in Input) Validate() error {','func (in Input) Validate() error {\n if len(in.ModelProfile)>16{return errors.New("too many model roles")}\n for k,v:=range in.ModelProfile{if len(k)>64||len(v)>256{return errors.New("invalid model profile")}}')
edit('internal/autopilot/store.go',' "fmt"\n','')
edit('internal/autopilot/store.go','func (s Store) String()string{return fmt.Sprintf("autopilot store")}\n','')
edit('internal/server/autopilot.go',' "context"\n',' "bytes"\n "encoding/json"\n "io"\n')
edit('internal/server/autopilot.go','\nvar _ context.Context\n','\n')
edit('internal/server/autopilot.go','if len(parts)>=2&&parts[0]=="api"&&parts[1]=="truth"{id=r.URL.Query().Get("project")}','''if r.URL.Path=="/api/truth/events"||r.URL.Path=="/api/truth/rebuild"{
 body,failure:=readRequestBody(r);if failure!=nil{writeFailure(w,r,*failure);return};r.Body=io.NopCloser(bytes.NewReader(body))
 var identity struct{ProjectID string `json:"project_id"`};if json.Unmarshal(body,&identity)==nil{id=identity.ProjectID}
}''')
edit('internal/server/autopilot.go','if id==""{next.ServeHTTP(w,r);return}\n active,err:=','if id==""{next.ServeHTTP(w,r);return}\n if _,err:=s.projects.Get(r.Context(),id);err!=nil{writeFailure(w,r,*projectFailure(err));return}\n active,err:=')
edit('internal/server/autopilot_engine.go',' "net/http"',' "net/http"\n "os"')
edit('internal/server/autopilot_engine.go',' case "foundation":\n  data,err:=',''' case "foundation":
  saved,loadErr:=e.s.projects.LoadAutopilotFoundation(ctx,j.ProjectID,j.Input.FoundationID)
  if loadErr==nil {j.Foundation=saved;j.Stage="plan_context";j.ErrorCode="";return j,nil}
  if !errors.Is(loadErr,os.ErrNotExist){return j,autopilot.Stop("FOUNDATION_STORAGE_ERROR")}
  data,err:=''')
edit('internal/server/autopilot_engine.go','  j.Foundation=&f;j.Stage="plan_context";','  if err=e.s.projects.SaveAutopilotFoundation(ctx,j.ProjectID,j.Input.FoundationID,f);err!=nil{return j,autopilot.Stop("FOUNDATION_STORAGE_ERROR")}\n  j.Foundation=&f;j.Stage="plan_context";')
edit('internal/server/server.go','"github.com/voocel/ainovel-cli/internal/project"','"github.com/voocel/ainovel-cli/internal/project"\n "github.com/voocel/ainovel-cli/internal/autopilot"')
edit('internal/server/server.go','type Config struct {','type Config struct {\n AutopilotEnabled bool')
edit('internal/server/server.go','type Server struct {','type Server struct {\n jobs *autopilot.Store\n runner *autopilot.Runner')
edit('internal/server/server.go','\tmux := http.NewServeMux()',''' s.jobs=&autopilot.Store{Path:paths.Database,Emit:s.publishJobEvent}
 if cfg.AutopilotEnabled {
  s.runner,err=autopilot.Start(s.jobs,chapterJobEngine{s:s},s.projects.AcquireExecution)
  if err!=nil{return nil,fmt.Errorf("start autopilot worker: %w",err)}
 }
 mux := http.NewServeMux()''')
edit('internal/server/server.go','recoveryMiddleware(mux)','recoveryMiddleware(s.autopilotWriteGuard(mux))')
edit('internal/server/server.go','func (s *Server) Close() error { return nil }','func (s *Server) Close() error { if s.runner!=nil{return s.runner.Close()};return nil }')
edit('internal/server/server.go','func (s *Server) ListenAndServe(ctx context.Context) error {','func (s *Server) ListenAndServe(ctx context.Context) error {\n defer s.Close()')
edit('internal/server/workspace.go','\ts.registerChapterVersionRoutes(mux)','\ts.registerAutopilotRoutes(mux)\n\ts.registerChapterVersionRoutes(mux)')
edit('internal/server/workspace.go','"foundation_worker_available":   false','"foundation_worker_available":   s.autopilotReady()')
edit('internal/server/workspace.go','"autopilot_worker_available":    false','"autopilot_worker_available":    s.autopilotReady()')
edit('internal/server/workspace.go','\t\twriteJSON(w, http.StatusOK, request)','\t\trequest.Automation.WorkerAvailable = s.autopilotReady()\n\t\twriteJSON(w, http.StatusOK, request)')
edit('internal/server/workspace.go','\t\t\t\treturn http.StatusAccepted, request, nil','\t\t\t\trequest.Automation.WorkerAvailable = s.autopilotReady()\n\t\t\t\treturn http.StatusAccepted, request, nil')
edit('cmd/novelforge/server.go','QualityConfigEnabled: true,','QualityConfigEnabled: true,\n AutopilotEnabled: true,')
edit('internal/project/writer_context.go','func (r *Repository) CompileWriterContext(ctx context.Context, req qualitygate.WriterRequest, totalTokens int) (json.RawMessage, error) {','''func (r *Repository) CompileWriterContext(ctx context.Context, req qualitygate.WriterRequest, totalTokens int) (json.RawMessage, error) {
 return r.compileWriterContext(ctx,req,totalTokens,nil)
}
func (r *Repository) compileWriterContext(ctx context.Context, req qualitygate.WriterRequest, totalTokens int, extra []contextcompiler.Item) (json.RawMessage, error) {''')
edit('internal/project/writer_context.go','\tboundaryJSON, err :=','\tfor _,item:=range extra {if item.Layer==contextcompiler.LayerNarrative {narrative=append(narrative,item)}}\n\tboundaryJSON, err :=')
edit('internal/project/writer_context.go','\titems := append(truthItems, pinned...)','\titems := append(truthItems, pinned...)\n for _,item:=range extra {if item.Layer!=contextcompiler.LayerNarrative {items=append(items,item)}}')
edit('internal/project/writer_context.go','\t\tFTS5:       contextcompiler.NewFTSStore(database),','\t\tFTS5:       contextcompiler.NewFTSStore(database),\n Style: all(contextcompiler.LayerStyle,contextcompiler.StageNone),')
edit('internal/server/qualityruntime/runtime.go','"github.com/voocel/ainovel-cli/internal/bootstrap"','"github.com/voocel/ainovel-cli/internal/bootstrap"\n "github.com/voocel/ainovel-cli/internal/autopilot"')
edit('internal/server/qualityruntime/runtime.go','\tcase "writer:draft":',''' case "architect:foundation":
  schema,_:=json.Marshal(schemaFor(reflect.TypeOf(autopilot.Foundation{})))
  return boundary+"Create the story compass, world constraints, stable character identifiers and initial states, volume/arc direction, POV and ending. This is planning, not accepted events. Use the input language and style. Arcs must be within target_chapter; choose POV from characters. Return one strict JSON object without fences matching: "+string(schema),"architect",nil
 case "planner:chapter":
  schema,_:=json.Marshal(schemaFor(reflect.TypeOf(qualitygate.ChapterPlan{})))
  return boundary+"Plan exactly the requested chapter using the selected Chapter-N context. Accepted Final facts outrank foundation plans. Use a stable POV id from foundation.characters; do not grant knowledge of unknown secrets. Include required beats, forbidden outcomes, inventory constraints, overdue foreshadow obligations and ending hook. Do not plan beyond target_chapter. Return one strict JSON object without fences matching: "+string(schema),"architect",nil
 case "writer:draft":''')
# Typed Web client uses the existing idempotent write implementation.
edit('web/src/lib/api.ts',"export class APIClient {","import type { AutopilotPage, AutopilotStart, AutopilotDetail, AutopilotJob } from './autopilot';\n\nexport class APIClient {")
edit('web/src/lib/api.ts','  health(): Promise<Health> {','''  listAutopilot(id: string): Promise<AutopilotPage> { return this.request(`/projects/${encodeURIComponent(id)}/autopilot?limit=100`); }
  startAutopilot(id: string, input: AutopilotStart): Promise<{ job: AutopilotJob }> { return this.write(`/projects/${encodeURIComponent(id)}/autopilot`, 'POST', input); }
  autopilotDetail(id: string, job: string): Promise<AutopilotDetail> { return this.request(`/projects/${encodeURIComponent(id)}/autopilot/${encodeURIComponent(job)}`); }
  controlAutopilot(id: string, job: string, action: 'pause' | 'stop' | 'resume'): Promise<{ job: AutopilotJob }> { return this.write(`/projects/${encodeURIComponent(id)}/autopilot/${encodeURIComponent(job)}/${action}`, 'POST', {}); }

  health(): Promise<Health> {''')
edit('web/src/App.svelte',"  import Dashboard from './pages/Dashboard.svelte';","  import Dashboard from './pages/Dashboard.svelte';\n  import Autopilot from './pages/Autopilot.svelte';")
edit('web/src/App.svelte','    dashboard: Dashboard,','    dashboard: Dashboard,\n    autopilot: Autopilot,')
edit('web/src/App.svelte',"    { name: 'projects', label: 'Projects', icon: '◇', href: '#/projects' },","    { name: 'projects', label: 'Projects', icon: '◇', href: '#/projects' },\n    { name: 'autopilot', label: 'Autopilot', icon: '▷', href: '#/autopilot' },")
edit('web/src/lib/router.ts',"'dashboard' | 'projects'","'autopilot' | 'dashboard' | 'projects'")
edit('web/src/lib/router.ts',"  '/projects': 'projects',","  '/projects': 'projects',\n  '/autopilot': 'autopilot',")
edit('web/src/lib/events.ts',"  'foundation.requested',","  'foundation.requested',\n  'autopilot.changed',")
edit('web/src/pages/NewNovelWizard.svelte','''      const first = models[0];
      if (first) {
        state.architectModel = `${first.provider}/${first.id}`;
        state.writerModel = `${first.provider}/${first.id}`;
      }''','')
edit('web/src/pages/NewNovelWizard.svelte','当前未启动 Autopilot worker。','尚未开始模型调用；请到 Autopilot 页面明确启动任务。')
edit('web/src/pages/NewNovelWizard.svelte','打开章节页</a>','打开章节页</a><a class="link ml-4" href={`#/autopilot?project=${createdID}`}>打开 Autopilot</a>')
edit('web/src/pages/NewNovelWizard.svelte','{#each models as model}<option','<option value="">使用项目配置</option>{#each models as model}<option',2)
edit('web/src/pages/NewNovelWizard.svelte','Autopilot 请求（worker 尚未启用）','Autopilot（创建后单独启动）')
edit('web/src/pages/NewNovelWizard.svelte','生成 worker 与 START/PAUSE/RESUME 控制属于 Phase 9。','随后在 Autopilot 页面启动、暂停、停止或继续；不会在提交向导时自动产生模型费用。')
edit('web/src/lib/wizard.ts',"  if (step === 4 && (!state.architectModel.trim() || !state.writerModel.trim())) {\n    errors.push('请选择 Architect 和 Writer 模型');\n  }", "  // Empty role selections inherit the configured project providers.")
edit('web/src/lib/wizard.ts',"      model_profile: {\n        architect: state.architectModel.trim(),\n        writer: state.writerModel.trim()\n      },", "      model_profile: Object.fromEntries(Object.entries({ architect: state.architectModel.trim(), writer: state.writerModel.trim() }).filter(([, value]) => value !== '')),")
# Additive OpenAPI document; no existing contract is silently overwritten.
props={k:{'type':'integer'} for k in ['chapter','completed_through','target_chapter','start_chapter','review_every','max_rewrites','max_retries','retries','revision']}
props.update({k:{'type':'string'} for k in ['id','project_id','stage','control','error_code','created_at','updated_at','next_run']})
props['state']={'type':'string','enum':['pending','running','paused','retrying','failed','completed','cancelled']}
props['actions']={'type':'object','required':['pause','stop','resume'],'properties':{k:{'type':'boolean'} for k in ['pause','stop','resume']},'additionalProperties':False}
job={'type':'object','properties':props,'required':list(props),'additionalProperties':False}
ref={'$ref':'#/components/schemas/AutopilotJob'}
errschema={'type':'object','description':'Safe API error envelope; no credentials or host paths.'}
params=lambda detail:[{'name':n,'in':'path','required':True,'schema':{'type':'string','minLength':1}} for n in (['id','job'] if detail else ['id'])]
def response(schema):return {'description':'Response','content':{'application/json':{'schema':schema}}}
def op(name,method,detail,schema,body=None):
 d={'operationId':name,'parameters':params(detail),'responses':{('202' if method=='post' else '200'):response(schema),**{str(n):response(errschema) for n in [400,404,409,500,503]}}}
 if method=='post':
  d['parameters'].append({'name':'Idempotency-Key','in':'header','required':True,'schema':{'type':'string','minLength':1}})
  d['requestBody']={'required':True,'content':{'application/json':{'schema':body or {'type':'object','additionalProperties':False}}}}
 return d
start={'type':'object','additionalProperties':False,'properties':{k:{'type':'integer','minimum':lo,'maximum':hi} for k,lo,hi in [('start_chapter',0,1000),('target_chapter',0,1000),('review_every',0,100),('max_rewrites',0,5),('max_retries',0,5)]}}
page={'type':'object','required':['jobs','worker_available','model_available','limit','offset'],'properties':{'jobs':{'type':'array','items':ref},'worker_available':{'type':'boolean'},'model_available':{'type':'boolean'},'limit':{'type':'integer'},'offset':{'type':'integer'}}}
detail={'type':'object','required':['job','foundation','chapter_plan','candidate_text','quality'],'properties':{'job':ref,'foundation':{},'chapter_plan':{},'candidate_text':{'type':'string'},'quality':{}}}
paths={'/api/projects/{id}/autopilot':{'get':op('listAutopilot','get',False,page),'post':op('startAutopilot','post',False,{'type':'object','properties':{'job':ref}},start)},'/api/projects/{id}/autopilot/{job}':{'get':op('getAutopilot','get',True,detail)}}
paths['/api/projects/{id}/autopilot']['get']['parameters'] += [{'name':n,'in':'query','schema':{'type':'integer','minimum':v,**({'maximum':100} if n=='limit' else {})}} for n,v in [('limit',1),('offset',0)]]
for action in ['pause','stop','resume']:paths['/api/projects/{id}/autopilot/{job}/'+action]={'post':op(action+'Autopilot','post',True,{'type':'object','properties':{'job':ref}})}
p='internal/server/openapi_phase9.json';(root/p).write_text(json.dumps({'paths':paths,'components':{'schemas':{'AutopilotJob':job}}},indent=2)+'\n');touched.add(p)
edit('internal/server/openapi.go','var openAPISpec []byte','//go:embed openapi_phase9.json\nvar phase9OpenAPI []byte\n\nvar openAPISpec []byte')
edit('internal/server/openapi.go','\topenAPISpec = mergeOpenAPIDocuments(openAPISpec, phase8OpenAPI)','\topenAPISpec = mergeOpenAPIDocuments(openAPISpec, phase8OpenAPI)\n\topenAPISpec = mergeOpenAPIDocuments(openAPISpec, phase9OpenAPI)')
# Current documentation changes; historical archives are left byte-identical.
edit('README.md','**只维护 Phase 1–8；Phase 9–13 暂停开发和专项维护。** 不开发 Durable Autopilot、不建立新任务队列、不发布新 Release。Foundation 请求仍只保存，`worker_available=false`。','**Phase 9 已按本次明确请求接入；Phase 10–13 继续暂停。** 默认 Web 启用可恢复 Autopilot，复用前八阶段的质量门禁与版本定稿。新建向导仍只保存请求，用户在 Autopilot 页面单独启动有界任务，不会自动产生模型费用。当前只做定向验证，不发布新 Release。详见 [Autopilot](docs/AUTOPILOT.md)。')
edit('README.md','| ChapterVersion | 版本历史、Diff、人工修订、同步、接受、定稿、重建与恢复 |','| ChapterVersion | 版本历史、Diff、人工修订、同步、接受、定稿、重建与恢复 |\n| Autopilot | 持久化任务、Foundation/章节规划、连续生成、审阅暂停、停止、重启恢复；只复用已接受的定稿路径 |')
edit('README.md','不代表 Worker 可用或 Provider 已联网验证。','不代表 Provider 已联网验证；Worker 就绪状态由独立能力字段报告。')
edit('docs/README.md','当前只维护 Phase 1–8，Phase 9–13 继续暂停。','当前维护 Phase 1–9，Phase 9 已由用户明确恢复实施；Phase 10–13 继续暂停。')
edit('docs/README.md','| 上下文 | [Context Compiler](CONTEXT_COMPILER.md) |','| 上下文 | [Context Compiler](CONTEXT_COMPILER.md) |\n| 持续创作 | [Phase 9 Autopilot](AUTOPILOT.md) |')
edit('docs/ROADMAP.md','## Current scope — 2026-09-05','## Current scope — 2026-09-05\n\nPhase 9 has been explicitly reopened for durable Autopilot delivery. Its implementation and limited verification boundary are documented in [AUTOPILOT.md](AUTOPILOT.md). Phase 10–13 stay paused. The earlier Phase 1–8 consolidation policy below is historical context, not an instruction to disable this newly requested worker.')
edit('docs/ROADMAP.md','## Phase 9 — Durable Autopilot — Deferred; frozen','## Phase 9 — Durable Autopilot — Implemented; targeted validation only')
edit('docs/ROADMAP.md','- Fake-LLM 300-chapter simulation.','- Foundation and ChapterPlan generation use the configured model and persisted selected context.\n- Chapter work reuses the actual quality/version coordinators; Web has real controls and review text.\n- The previously planned Fake-LLM 300-chapter simulation is not run under the current limited-check scope; do not infer large-scale acceptance.')
edit('docs/ENGINE.md','# Engine\n','# Engine\n\nPhase 9 wraps the existing deterministic chapter-quality engine and immutable Final bridge in a durable worker; it does not replace these with the legacy Host writer loop. See [AUTOPILOT.md](AUTOPILOT.md) for state, locking and recovery boundaries.\n')
edit('docs/CONTEXT_COMPILER.md','These are Phase 1–8 paths; later-phase workers remain deferred.','These are Phase 1–8 paths, now reused by the Phase 9 worker; Phase 10–13 remain deferred.')
for p in sorted(touched):
 if p.endswith('.go'):subprocess.run(['gofmt','-w',str(root/p)],check=True)
subprocess.run(['git','add','--']+sorted(touched),cwd=root,check=True)
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
# Caller builds web/dist and then stages it; no helper or workflow enters product.
(root/'.phase9-paths.json').write_text(json.dumps(sorted(touched)))
print(json.dumps({'base':BASE,'changed_paths':sorted(touched)},indent=2))
