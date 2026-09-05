from pathlib import Path
import json, shutil, subprocess, sys
BASE='9816bb98f6f8cabfd00797a80ecf721e57c4c461'
root=Path(sys.argv[1]).resolve(); stage=Path.cwd().resolve()
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()==BASE
changed=set()
paths=subprocess.check_output(['git','diff','--name-only',BASE,'HEAD'],text=True).splitlines()
allowed=('internal/lifecycle/','internal/project/manuscript.go','internal/project/lifecycle_','internal/server/lifecycle','web/src/lib/lifecycle.ts','web/src/pages/Lifecycle')
for name in paths:
 if name.startswith(allowed):
  src=stage/name;dst=root/name
  assert src.is_file() and not src.is_symlink() and not dst.exists(),name
  dst.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(src,dst);changed.add(name)
 elif not name.startswith(('.phase11/','.github/workflows/phase11-')): raise RuntimeError('unexpected staged path '+name)
def edit(name,old,new):
 p=root/name;s=p.read_text();assert s.count(old)==1,(name,old[:90],s.count(old));p.write_text(s.replace(old,new));changed.add(name)
def write(name,text):
 p=root/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_text(text);changed.add(name)
edit('internal/project/manuscript.go','c.Number>=start && c.Number<start+len(b.Chapters)','c.Chapter>=start && c.Chapter<start+len(b.Chapters)')
edit('internal/lifecycle/text.go',r'(?:第[0-9零〇一二三四五六七八九十百千万两]+[章回节]|chapter[ \t]+[0-9ivxlcdm]+)(?:\s|[：:、.．—-]|$)',r'(?:第[0-9零〇一二三四五六七八九十百千万两]+[章回节]|chapter[ \t]+[0-9ivxlcdm]+(?:\s|[：:、.．—-]|$))')
edit('internal/lifecycle/text.go','  split:=!fenced',"  if numbered && !fenced && m!=nil && len(m[1])==1 && !chapterHeading.MatchString(h) && len(b.Chapters)==0 && strings.TrimSpace(strings.Join(buf,\"\\n\"))==\"\" { b.Title=h;buf=nil;continue }\n  split:=!fenced")
edit('internal/lifecycle/store.go',' Saved int `json:"saved"`; Analyzed int `json:"analyzed"`; Created string `json:"created_at"`',' Saved int `json:"saved"`; Analyzed int `json:"analyzed"`; NextSave int `json:"next_save"`; NextAnalysis int `json:"next_analysis"`; Created string `json:"created_at"`')
edit('internal/lifecycle/store.go'," (SELECT count(*) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.state='analyzed')"," (SELECT count(*) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.state='analyzed'),\n (SELECT coalesce(min(chapter),0) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.version_id=''),\n (SELECT coalesce(min(chapter),0) FROM manuscript_import_chapters c WHERE c.import_id=i.id AND c.state!='analyzed')")
edit('internal/lifecycle/store.go','&v.Created,&v.Saved,&v.Analyzed)','&v.Created,&v.Saved,&v.Analyzed,&v.NextSave,&v.NextAnalysis)')
# Deduplication precedes raw-prose conflict checks, including replay after Final.
edit('internal/project/manuscript.go',' // Raw/unversioned prose is evidence too;', ''' s,err:=r.OpenLifecycle(ctx,id);if err!=nil{return lifecycle.Import{},err};defer s.DB.Close()
 batch:="imp_"+lifecycle.SHA([]byte(id+"\\n"+name+"\\n"+lifecycle.SHA(data)+fmt.Sprint(start)))[:32]
 if prior,e:=s.Get(ctx,batch);e==nil{return prior,nil}else if !errors.Is(e,lifecycle.ErrNotFound){return lifecycle.Import{},e}
 // Raw/unversioned prose is evidence too;''')
edit('internal/project/manuscript.go',' s,err:=r.OpenLifecycle(ctx,id);if err!=nil{return lifecycle.Import{},err};defer s.DB.Close()\n return s.Begin',' return s.Begin')
edit('internal/server/lifecycle_upload.go','var _ = errors.Is','func lifecycleUploadError(err error) error { var size *http.MaxBytesError; if errors.As(err,&size){return lifecycle.ErrLimit}; return lifecycle.ErrInvalid }')
p=root/'internal/server/lifecycle_upload.go';p.write_text(p.read_text().replace('if err!=nil{return out,lifecycle.ErrInvalid}','if err!=nil{return out,lifecycleUploadError(err)}'))
edit('internal/project/lifecycle_restore.go',' if e,err:=r.find(meta.ID);err==nil {',' if e,err:=r.find(meta.ID);err==nil {\n  if _,err=r.lifecycleRoot(meta.ID);err!=nil{return result,err}')
edit('internal/project/lifecycle_backup.go','else if !errors.Is(err,os.ErrExist){return result,err}', '''else if !errors.Is(err,os.ErrExist){return result,err} else {
  prior,readErr:=readLifecycleFile(filepath.Join(dir,result.BackupID),lifecycle.MaxArchive);if readErr!=nil{return result,readErr}
  manifest,_,readErr:=lifecycle.Unpack(prior);if readErr!=nil || manifest.ProjectID!=id || manifest.Format!=expected{return result,lifecycle.ErrConflict}
 }''')
edit('internal/server/server.go','\ts.registerWorkspaceRoutes(mux)','\ts.registerWorkspaceRoutes(mux)\n\ts.registerLifecycleRoutes(mux)')
edit('internal/server/autopilot_engine.go','func (e chapterJobEngine) checkBoundary(ctx context.Context, j autopilot.Job, rejectCurrent bool) error {','''func (e chapterJobEngine) checkBoundary(ctx context.Context, j autopilot.Job, rejectCurrent bool) error {
 if rejectCurrent {
  pending,err:=e.s.projects.ImportedChapterPending(ctx,j.ProjectID,j.Chapter)
  if err!=nil{return autopilot.Stop("IMPORT_STATE_UNAVAILABLE")}
  if pending{return autopilot.Stop("IMPORTED_CHAPTER_REQUIRES_REVIEW")}
 }''')
edit('internal/server/openapi.go','var openAPISpec []byte','//go:embed openapi_phase11.json\nvar phase11OpenAPI []byte\n\nvar openAPISpec []byte')
edit('internal/server/openapi.go','openAPISpec = mergeOpenAPIDocuments(openAPISpec, phase10OpenAPI)','openAPISpec = mergeOpenAPIDocuments(openAPISpec, phase10OpenAPI)\n openAPISpec = mergeOpenAPIDocuments(openAPISpec, phase11OpenAPI)')
edit('web/src/App.svelte',"  import Dashboard from './pages/Dashboard.svelte';","  import Dashboard from './pages/Dashboard.svelte';\n  import Lifecycle from './pages/Lifecycle.svelte';")
edit('web/src/App.svelte','    dashboard: Dashboard,','    dashboard: Dashboard,\n    lifecycle: Lifecycle,')
edit('web/src/App.svelte',"    { name: 'projects', label: 'Projects', icon: '◇', href: '#/projects' },","    { name: 'projects', label: 'Projects', icon: '◇', href: '#/projects' },\n    { name: 'lifecycle', label: 'Import & Backup', icon: '⇄', href: '#/lifecycle' },")
edit('web/src/lib/router.ts','export type RouteName = ',"export type RouteName = 'lifecycle' | ")
edit('web/src/lib/router.ts',"  '/': 'dashboard',","  '/': 'dashboard',\n  '/lifecycle': 'lifecycle',")
api_methods='''
  manuscriptImports(id: string, offset=0): Promise<ImportCollection> { return this.request(`/projects/${encodeURIComponent(id)}/lifecycle/imports?limit=50&offset=${offset}`); }
  manuscriptImport(id: string, batch: string, offset=0): Promise<ImportDetail> { return this.request(`/projects/${encodeURIComponent(id)}/lifecycle/imports/${encodeURIComponent(batch)}?limit=50&offset=${offset}`); }
  importManuscript(id: string, file: File, start: number): Promise<{import: ManuscriptImport}> { return this.uploadLifecycle(`/projects/${encodeURIComponent(id)}/lifecycle/imports`,file,start); }
  stepManuscript(id: string, batch: string, chapter: number, analyze: boolean): Promise<{chapter: ImportedChapter}> { return this.write(`/projects/${encodeURIComponent(id)}/lifecycle/imports/${encodeURIComponent(batch)}/step`,'POST',{chapter,analyze}); }
  restoreLifecycle(file: File): Promise<RestoreResult> { return this.uploadLifecycle('/lifecycle/restore',file); }
  migrateLifecycle(id: string, expected: number): Promise<MigrationResult> { return this.write(`/projects/${encodeURIComponent(id)}/lifecycle/migrate`,'POST',{expected_format:expected,confirm:id}); }
  private uploadLifecycle<T>(path: string,file: File,start?: number): Promise<T> { const body=new FormData();body.set('file',file);if(start!==undefined)body.set('start_chapter',String(start));return this.request<T>(path,{method:'POST',headers:{'Idempotency-Key':createIdempotencyKey()},body}); }
  async downloadLifecycle(id: string,kind: string,from=1,to=0): Promise<Blob> {
    const prefix=`/projects/${encodeURIComponent(id)}/lifecycle`;
    let path: string;
    if(kind==='backup')path=`${prefix}/backup`;
    else if(kind.startsWith('lifecycle-migrate-'))path=`${prefix}/backups/${encodeURIComponent(kind)}`;
    else {const q=new URLSearchParams({format:kind,from:String(from)});if(to>0)q.set('to',String(to));path=`${prefix}/export?${q}`;}
    const fetcher=this.fetcher??globalThis.fetch.bind(globalThis);
    const response=await fetcher(`${this.baseURL}${path}`,{credentials:'same-origin'});
    if(!response.ok){const data=await response.json().catch(()=>undefined) as {error?: APIErrorPayload}|undefined;throw new APIClientError(data?.error?.message??'Download failed',response.status,data?.error);}
    return response.blob();
  }
'''
edit('web/src/lib/api.ts','export class APIClient {','export class APIClient {\n'+api_methods)
p=root/'web/src/lib/api.ts';p.write_text("import type { ManuscriptImport, ImportCollection, ImportDetail, ImportedChapter, RestoreResult, MigrationResult } from './lifecycle';\n"+p.read_text())
# Contracts are generated from explicit schemas, not an untyped route placeholder.
S=lambda **p:{'type':'object','properties':p,'required':list(p)}
st={'type':'string'};num={'type':'integer'};boolean={'type':'boolean'}
imp=S(id=st,project_id=st,filename=st,source_sha=st,start_chapter=num,total=num,saved=num,analyzed=num,next_save=num,next_analysis=num,created_at=st)
chapter=S(chapter=num,title=st,characters=num,version_id=st,state={'type':'string','enum':['pending','saved','analyzed']});chapter['properties']['error_code']=st
error=S(error=S(code=st,message=st,details={'type':'object'},retryable=boolean,trace_id=st))
ref=lambda n:{'$ref':'#/components/schemas/'+n}
arr=lambda v:{'type':'array','items':v}
response=lambda v:{'description':'Lifecycle response','content':{'application/json':{'schema':v}}}
paths={};prefix='/api/projects/{id}/lifecycle'
def add(path,method,op,result,body=None,upload=False,query=None,status='200',binary=False):
 params=[]
 for name in ['id','import','backup']:
  if '{'+name+'}' in path:params.append({'name':name,'in':'path','required':True,'schema':{'type':'string','minLength':1}})
 if method=='post':params.append({'name':'Idempotency-Key','in':'header','required':True,'schema':{'type':'string','pattern':'^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'}})
 for name,definition in (query or {}).items():params.append({'name':name,'in':'query','schema':definition})
 responses={status:response(result)}
 if binary:responses[status]={'description':'Attachment; active Finals only for manuscript export','content':{media:{'schema':{'type':'string','format':'binary'}} for media in ['application/zip','application/epub+zip','text/plain','text/markdown']}}
 for n in ['400','404','409','413','500','503']:responses[n]=response(ref('LifecycleError'))
 spec={'operationId':op,'parameters':params,'responses':responses}
 if body is not None:spec['requestBody']={'required':True,'content':{'multipart/form-data' if upload else 'application/json':{'schema':body}}}
 paths.setdefault(path,{})[method]=spec
pagination={'limit':{'type':'integer','minimum':1,'maximum':100},'offset':{'type':'integer','minimum':0}}
add(prefix+'/imports','get','listManuscriptImports',S(imports=arr(ref('ManuscriptImport')),limit=num,offset=num,model_available=boolean),query=pagination)
upload=S(file={'type':'string','format':'binary'});upload['properties']['start_chapter']={'type':'integer','minimum':1,'maximum':1000};upload['additionalProperties']=False
add(prefix+'/imports','post','stageManuscript',S(**{'import':ref('ManuscriptImport'),'model_called':boolean,'finalized':boolean}),upload,True,status='201')
add(prefix+'/imports/{import}','get','getManuscriptImport',S(**{'import':ref('ManuscriptImport'),'chapters':arr(ref('ImportedChapter')),'limit':num,'offset':num}),query=pagination)
step=S(chapter={'type':'integer','minimum':1,'maximum':1000},analyze=boolean);step['additionalProperties']=False
add(prefix+'/imports/{import}/step','post','stepManuscript',S(chapter=ref('ImportedChapter'),accepted=boolean,finalized=boolean),step)
add(prefix+'/export','get','exportManuscript',st,binary=True,query={'format':{'type':'string','enum':['txt','md','epub']},'from':{'type':'integer','minimum':1,'maximum':1000},'to':{'type':'integer','minimum':1,'maximum':1000}})
add(prefix+'/backup','get','backupProject',st,binary=True);add(prefix+'/backups/{backup}','get','downloadMigrationBackup',st,binary=True)
mi=S(expected_format={'type':'integer','minimum':1,'maximum':2},confirm=st);mi['additionalProperties']=False
add(prefix+'/migrate','post','migrateProjectFormat',S(from_format=num,to_format=num,schema_version=num,backup_id=st,changed=boolean),mi)
rest=S(file={'type':'string','format':'binary'});rest['additionalProperties']=False
add('/api/lifecycle/restore','post','restoreProjectBackup',S(project=S(id=st,title=st,format_version=num),replayed=boolean,requires_configuration=boolean,jobs_resumed=boolean),rest,True,status='201')
paths['/api/lifecycle/restore']['post']['responses']['200']=paths['/api/lifecycle/restore']['post']['responses']['201']
write('internal/server/openapi_phase11.json',json.dumps({'paths':paths,'components':{'schemas':{'ManuscriptImport':imp,'ImportedChapter':chapter,'LifecycleError':error}}},ensure_ascii=False,indent=2)+'\n')
for name in ['README.md','docs/README.md','docs/WEB.md','docs/DEVELOPMENT.md']:
 p=root/name;s=p.read_text();s=s.replace('Phase 11–13 remain paused','Phase 11 is now connected; Phase 12–13 remain paused',1)
 link='docs/LIFECYCLE.md' if name=='README.md' else 'LIFECYCLE.md'
 s+='\n## Phase 11 lifecycle\n\n[Manuscript import, export, backup and restore]('+link+') now connect to immutable versions and the existing semantic review path. Upload and restore do not start paid work. Phase 12–13 remain paused. Verification is bounded to named short flows and affected builds; no whole-book or full-suite acceptance is implied.\n'
 p.write_text(s);changed.add(name)
p=root/'docs/ROADMAP.md';s=p.read_text().replace('Phase 11–13 remain paused','Phase 12–13 remain paused').replace('Phase 11 — Novel lifecycle tooling — Deferred; frozen','Phase 11 — Novel lifecycle tooling — Implemented; targeted validation only');s+='\nPhase 11 current behavior and explicit recovery limits: [Lifecycle](LIFECYCLE.md). Historical test evidence is not rewritten.\n';p.write_text(s);changed.add('docs/ROADMAP.md')
for name in ['docs/AUTOPILOT.md','docs/AUTHORING.md']:
 p=root/name;s=p.read_text().replace('Phase 11–13 remain paused','Phase 12–13 remain paused',1);p.write_text(s);changed.add(name)
newdoc=stage/'.phase11/LIFECYCLE.md';write('docs/LIFECYCLE.md',newdoc.read_text())
# Do not reformat unrelated files or modify generated JS string contents.
gos=[str(root/n) for n in sorted(changed) if n.endswith('.go')]
subprocess.run(['gofmt','-w',*gos],check=True)
subprocess.run(['git','add','--',*sorted(changed)],cwd=root,check=True)
print('PHASE11_PATHS='+json.dumps(sorted(changed)))
