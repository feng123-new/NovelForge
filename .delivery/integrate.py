from pathlib import Path
import shutil,subprocess,sys
root=Path(sys.argv[1]).resolve();source=Path(__file__).parent/'files';changed=[]
def rep(name,old,new):
 p=root/name;s=p.read_text();assert s.count(old)==1,(name,old,s.count(old));p.write_text(s.replace(old,new));changed.append(name)
for p in source.rglob('*'):
 if p.is_file():
  dest=root/p.relative_to(source);assert not dest.exists(),str(dest);dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(p,dest);changed.append(str(p.relative_to(source)))
rep('internal/bootstrap/models.go','resp, err := current.model.Generate(ctx, messages, tools, opts...)','resp, err := observedGenerate(ctx, current, messages, tools, opts...)')
rep('internal/bootstrap/models.go','return next.model.Generate(ctx, messages, tools, opts...)','return observedGenerate(ctx, next, messages, tools, opts...)')
rep('internal/bootstrap/models.go','if !agentcore.IsFailoverEligible(err) {','if code := observability.ControlCode(err); code != "" && code != "PROVIDER_PAUSED" && code != "PROVIDER_COOLDOWN" { return modelTarget{}, "", false }\n\tif !agentcore.IsFailoverEligible(err) && observability.ControlCode(err) == "" {')
rep('internal/bootstrap/models.go','"github.com/voocel/ainovel-cli/internal/errs"','"github.com/voocel/ainovel-cli/internal/errs"\n "github.com/voocel/ainovel-cli/internal/observability"')
rep('internal/server/qualityruntime/runtime.go','"github.com/voocel/ainovel-cli/internal/qualitygate"','"github.com/voocel/ainovel-cli/internal/qualitygate"\n "github.com/voocel/ainovel-cli/internal/observability"')
rep('internal/server/qualityruntime/runtime.go','model := r.models.ForRoleWithFailover(role, nil)','model := r.models.ForRoleTracked(role, func(e bootstrap.FailoverEvent){ usage.Provider=e.ToProvider; usage.Model=e.ToModel })')
rep('internal/server/qualityruntime/runtime.go','retryable := errors.Is(err, context.DeadlineExceeded)','if observability.ControlCode(err)!="" { return nil, usage, err }\n\t\tretryable := errors.Is(err, context.DeadlineExceeded)')
rep('internal/server/qualityruntime/runtime.go','Code: "QUALITY_PROVIDER_FAILED", Retryable: retryable','Code: bootstrap.ObservedErrorCode(err), Retryable: retryable')
rep('internal/qualitygate/services.go','"time"','"time"\n "github.com/voocel/ainovel-cli/internal/observability"')
rep('internal/qualitygate/services.go','type IdempotentModelCaller struct {','type IdempotentModelCaller struct {\n Observer *observability.Store')
rep('internal/qualitygate/services.go','if existing.Status == "completed" {','if existing.Status == "completed" {\n if c.Observer!=nil { c.Observer.Replay(ctx,request.IdempotencyKey) }')
rep('internal/qualitygate/services.go','started := now().UTC()','if c.Observer!=nil { ctx=observability.WithCall(ctx,c.Observer,observability.Identity{LogicalKey:request.IdempotencyKey,RequestHash:requestHash,TransactionID:request.TransactionID,Chapter:request.Chapter,Agent:request.Agent,Operation:request.Operation}) }\n started := now().UTC()')
p=root/'internal/qualitygate/store_core.go';s=p.read_text();assert 'func (s *Store) Database()' not in s;s+='\n// Database is borrowed by project-scoped observation recording; Store owns Close.\nfunc (s *Store) Database() *sql.DB { return s.db }\n';p.write_text(s);changed.append('internal/qualitygate/store_core.go')
rep('internal/server/quality.go','caller := &qualitygate.IdempotentModelCaller{Repository: store, Invoker: invoker}','caller := &qualitygate.IdempotentModelCaller{Repository: store, Invoker: invoker, Observer:s.observationStore(store,projectID)}')
rep('internal/server/quality.go','func qualityFailure(err error) *apiFailure {','func qualityFailure(err error) *apiFailure {\n if observability.ControlCode(err)!="" {return observationFailure(err)}')
rep('internal/server/quality.go','"github.com/voocel/ainovel-cli/internal/project"','"github.com/voocel/ainovel-cli/internal/project"\n "github.com/voocel/ainovel-cli/internal/observability"')
rep('internal/server/autopilot_engine.go','"github.com/voocel/ainovel-cli/internal/project"','"github.com/voocel/ainovel-cli/internal/project"\n "github.com/voocel/ainovel-cli/internal/observability"')
rep('internal/server/autopilot_engine.go','func (e chapterJobEngine) Step(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {','func (e chapterJobEngine) Step(ctx context.Context, j autopilot.Job) (next autopilot.Job, resultErr error) {\n ctx=observability.WithTask(ctx,j.ID)\n defer func(){ if code:=observability.ControlCode(resultErr);code!="" {next=j;next.State=autopilot.Paused;next.ErrorCode=code;resultErr=nil} }()')
rep('internal/server/autopilot_engine.go','caller := qualitygate.IdempotentModelCaller{Repository: quality.Store, Invoker: invoker}','caller := qualitygate.IdempotentModelCaller{Repository: quality.Store, Invoker: invoker,Observer:e.s.observationStore(quality.Store,j.ProjectID)}')
rep('internal/server/autopilot_engine.go','var ce *qualitygate.ModelCallError','if observability.ControlCode(err)!="" {return nil,err}\n var ce *qualitygate.ModelCallError')
rep('internal/server/autopilot_engine.go','return j, autopilot.Retry("QUALITY_STEP_ERROR")','if observability.ControlCode(err)!="" {return j,err}\n return j, autopilot.Retry("QUALITY_STEP_ERROR")')
rep('internal/server/server.go','s.registerWorkspaceRoutes(mux)','s.registerWorkspaceRoutes(mux)\n s.registerObservationRoutes(mux)')
rep('internal/server/openapi.go','var openAPISpec []byte','//go:embed openapi_observability.json\nvar observationOpenAPI []byte\n\nvar openAPISpec []byte')
rep('internal/server/openapi.go','func init() {','func init() {') if False else None
rep('internal/server/openapi.go','openAPISpec = mergeOpenAPIDocuments(openAPISpec, phase11OpenAPI)','openAPISpec = mergeOpenAPIDocuments(openAPISpec, phase11OpenAPI)\n openAPISpec = mergeOpenAPIDocuments(openAPISpec, observationOpenAPI)')
rep('internal/project/lifecycle_database.go','m.Schema > 10','m.Schema > CurrentDatabaseSchema()')
# Derive new backup snapshots from the current migration set, preserving old SQL.
for name in ['internal/project/lifecycle_backup.go','internal/project/lifecycle_restore.go']:
 p=root/name;s=p.read_text();s=s.replace('schema > 10','schema > CurrentDatabaseSchema()').replace('schema == 10','schema == CurrentDatabaseSchema()').replace('schema < 10','schema < CurrentDatabaseSchema()').replace('m.Schema > 10','m.Schema > CurrentDatabaseSchema()');p.write_text(s);changed.append(name)
# Complete the tracker hook before compilation.
p=root/'internal/bootstrap/observed.go';s=p.read_text().replace('ForRoleTracked(role string)','ForRoleTracked(role string, reporters ...FailoverReporter)').replace('return &failoverModel{role:role,primary:primary,set:ms}','var report FailoverReporter;if len(reporters)>0{report=reporters[0]};return &failoverModel{role:role,primary:primary,set:ms,report:report}').replace('if err=ticket.Finish(ctx,in,out,known,code);err!=nil{return response,err}','if err=ticket.Finish(ctx,in,out,known,code);err!=nil{if callErr==nil && response!=nil {return response,nil};return response,err}');p.write_text(s)
p=root/'internal/observability/store.go';s=p.read_text().replace(' "strings"\n','').replace('\nvar _ = strings.TrimSpace\n','');anchor='if err=tx.Commit();err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}';assert s.count(anchor)==1;s=s.replace(anchor,'if _,err=tx.ExecContext(ctx,`INSERT INTO observation_links(call_key,logical_id) VALUES(?,?) ON CONFLICT(call_key) DO NOTHING`,id.LogicalKey,a.LogicalID);err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")};'+anchor);p.write_text(s)
subprocess.run(['gofmt','-w',*[str(root/n) for n in sorted(set(changed)) if n.endswith('.go')]],check=True)
subprocess.run(['git','add','--',*sorted(set(changed))],cwd=root,check=True)
print('INTEGRATED',len(set(changed)))
