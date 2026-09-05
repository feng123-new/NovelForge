import pathlib,sys,json,subprocess,re
root=pathlib.Path(sys.argv[1]);changed=set()
def edit(path,old,new,count=1):
 p=root/path;s=p.read_text();assert s.count(old)==count,(path,old[:80],s.count(old));p.write_text(s.replace(old,new));changed.add(path)
# Keep all model-facing lint data inside the same compilation budget.
p='internal/project/authoring.go'
edit(p,'s authoring.Selection) (json.RawMessage, error)', 's authoring.Selection, extra ...contextcompiler.Item) (json.RawMessage, error)')
edit(p,'authoringProviders(selectionItems(s))','authoringProviders(append(selectionItems(s), extra...))')
f=root/p;s=f.read_text();a=s.index('func (r *Repository) CompileEditorialContext(');b=s.index('func (r *Repository) AuthoringLint(',a)
s=s[:a]+'''func (r *Repository) CompileEditorialContext(ctx context.Context, req qualitygate.EditorRequest) (json.RawMessage, []string, error) {
 sel, err := r.authoringSelection(ctx,req.ProjectID,req.TransactionID+":editor","review","",req.Chapter,"")
 if err != nil { return nil,nil,err }
 report,err := r.AuthoringLint(ctx,req.ProjectID,req.Chapter,req.Candidate.Text,sel.Rules)
 if err != nil { return nil,nil,err }
 notes:=[]string{}
 for _,finding:=range report.Findings { notes=append(notes,"[Advisory style] "+finding.Message) }
 // A compact model-facing subset is still placed inside the compiler, never appended after it.
 modelReport:=report
 if len(modelReport.Findings)>8 { modelReport.Findings=append([]authoring.Finding(nil),modelReport.Findings[:8]...);modelReport.Truncated=true }
 raw,err:=json.Marshal(modelReport);if err!=nil{return nil,nil,err}
 compiled,err:=compileAuthoringSelection(ctx,req.ProjectID,req.Chapter,"",sel,contextcompiler.Item{ID:"authoring:rule-report",Layer:contextcompiler.LayerStyle,Kind:"advisory_rule_report",Content:string(raw),Priority:950,Mandatory:true})
 return compiled,notes,err
}

'''+s[b:];f.write_text(s);changed.add(p)
p='internal/authoring/rules.go';edit(p,'report.Findings = append(report.Findings, Finding{','runes:=[]rune(phrase); if len(runes)>100 { phrase=string(runes[:100])+"…" }\n\t\treport.Findings = append(report.Findings, Finding{')
# Assert actual invocations, not only distinct operation names.
p='internal/server/authoring_test.go';edit(p,'type craftCapture struct{ payloads map[string][]byte }','type craftCapture struct{ payloads map[string][]byte; count int }')
edit(p,'c.payloads[operation] = append([]byte(nil), payload...)','c.count++\n\tc.payloads[operation] = append([]byte(nil), payload...)')
edit(p,'callsBefore := len(model.payloads)','callsBefore := model.count');edit(p,'if len(model.payloads) != callsBefore {','if model.count != callsBefore {')
# OpenAPI extension uses explicit inputs/outputs and one-of mutation operations.
schemas={}
def ref(name):return {'$ref':'#/components/schemas/'+name}
def obj(properties,required=None):return {'type':'object','properties':properties,'required':list(properties) if required is None else required,'additionalProperties':False}
def integer(low=0,high=None):
 x={'type':'integer','minimum':low}
 if high is not None:x['maximum']=high
 return x
string={'type':'string'};boolean={'type':'boolean'}
schemas['AuthoringEntry']=obj({'id':{'type':'string','maxLength':96},'kind':{'enum':['skill','style','knowledge']},'role':{'enum':['','writing','review','polish','planning']},'title':{'type':'string','minLength':1,'maxLength':240},'markdown':{'type':'string','minLength':1,'maxLength':16384,'description':'UTF-8 bytes limited to 16 KiB by the server.'},'source':{'type':'string','maxLength':512},'enabled':boolean,'pinned':boolean,'priority':integer(0,100),'from_chapter':integer(0,1000),'pov':{'type':'string','maxLength':200}})
schemas['AuthoringRules']=obj({'enabled':boolean,'phrases':{'type':'array','maxItems':32,'items':{'type':'string','minLength':1,'maxLength':160}},'max_phrase_occurrences':integer(0,100),'max_sentence_repeats':integer(1,20),'min_sentence_runes':integer(4,200),'previous_chapters':integer(0,3)})
schemas['AuthoringMutation']=obj({'expected_revision':integer(1),'entry':ref('AuthoringEntry'),'delete_id':{'type':'string','minLength':1,'maxLength':96},'rules':ref('AuthoringRules')},['expected_revision']);schemas['AuthoringMutation']['oneOf']=[{'required':[name]} for name in ['entry','delete_id','rules']]
schemas['AuthoringChange']=obj({'revision':integer(1),'entry_id':string,'replayed':boolean},['revision','replayed'])
entries={'type':'array','items':ref('AuthoringEntry')}
schemas['AuthoringState']=obj({'revision':integer(1),'entries':entries,'builtins':entries,'rules':ref('AuthoringRules'),'total':integer(),'limit':integer(1,100),'offset':integer()})
schemas['AuthoringFinding']=obj({'code':string,'phrase':string,'count':integer(),'limit':integer(),'message':string})
schemas['AuthoringReport']=obj({'findings':{'type':'array','maxItems':64,'items':ref('AuthoringFinding')},'advisory':{'const':True},'truncated':boolean})
def param(name,where,schema,required=False):return {'name':name,'in':where,'required':required,'schema':schema}
project=param('id','path',{'type':'string','minLength':1},True)
page=[param('limit','query',integer(1,100)),param('offset','query',integer())]
def response(schema):return {'description':'Authoritative local response','content':{'application/json':{'schema':schema}}}
def op(name,output,params,body=None):
 d={'operationId':name,'parameters':[project]+params,'responses':{'200':response(output)}}
 for code in ['400','404','409','500']:d['responses'][code]=response({'type':'object','description':'Safe API error envelope; no paths, credentials or raw database errors.'})
 if body is not None:d['parameters'].append(param('Idempotency-Key','header',{'type':'string','minLength':1,'maxLength':256},True));d['requestBody']={'required':True,'content':{'application/json':{'schema':body}}}
 return d
paths={
 '/api/projects/{id}/authoring':{'get':op('getAuthoring',ref('AuthoringState'),page+[param('kind','query',{'enum':['','skill','style','knowledge']})]),'post':op('mutateAuthoring',ref('AuthoringChange'),[],ref('AuthoringMutation'))},
 '/api/projects/{id}/authoring/search':{'get':op('searchAuthoring',obj({'entries':entries,'limit':integer(),'offset':integer()}),page+[param('kind','query',{'enum':['skill','style','knowledge']},True),param('q','query',{'type':'string','maxLength':512},True),param('chapter','query',integer(0,1000),True),param('pov','query',{'type':'string','maxLength':200})])},
 '/api/projects/{id}/authoring/lint':{'post':op('lintAuthoring',obj({'revision':integer(1),'report':ref('AuthoringReport')}),[],obj({'chapter':integer(1,1000),'text':{'type':'string','maxLength':1048576}}))}}
p=root/'internal/server/openapi_phase10.json';p.write_text(json.dumps({'paths':paths,'components':{'schemas':schemas}},ensure_ascii=False,indent=2)+'\n');changed.add(str(p.relative_to(root)))
# Current docs, not archived history.
for name in ['README.md','docs/README.md','docs/ROADMAP.md','docs/DEVELOPMENT.md','docs/AUTOPILOT.md','docs/WEB.md']:
 p=root/name;s=p.read_text();s=s.replace('Phase 10–13','Phase 11–13').replace('Phase 10-13','Phase 11-13')
 if name=='docs/ROADMAP.md':s=s.replace('## Phase 10 — Skills, style and reference systems — Deferred; frozen','## Phase 10 — Skills, style and reference systems — Implemented; targeted validation only');s=s.replace('Phase 11–13 remain paused; finishing this phase','Phase 10 adds connected Markdown Skills, style/reference libraries and advisory rules. Phase 11–13 remain paused; finishing this phase')
 if name in ['README.md','docs/README.md','docs/ROADMAP.md','docs/DEVELOPMENT.md','docs/WEB.md']:
  link='docs/AUTHORING.md' if name=='README.md' else 'AUTHORING.md';s+='\n\n## Phase 10 authoring systems\n\nMarkdown Writing/Review/Polish/Planning Skills, separate style and reference libraries with Chinese-capable FTS, and configurable advisory phrase/repetition rules are connected to the Web/Autopilot model requests. See [Authoring systems]('+link+') for limits, storage and actual runtime behavior. Phase 11–13 remain paused; no full-suite or long-book acceptance is claimed.\n'
 p.write_text(s);changed.add(name)
for p in changed:
 if p.endswith('.go'):subprocess.run(['gofmt','-w',str(root/p)],check=True)
subprocess.run(['git','add','--']+sorted(changed),cwd=root,check=True)
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
print('Phase 10 input budgets, request replay assertions, contracts and current documentation prepared.')
