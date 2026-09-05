package server

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "mime/multipart"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/voocel/ainovel-cli/internal/lifecycle"
 "github.com/voocel/ainovel-cli/internal/project"
 "github.com/voocel/ainovel-cli/internal/qualitygate"
)

type lifecycleWriter struct{}
func(lifecycleWriter) Write(context.Context,qualitygate.WriterRequest)(qualitygate.WriterResult,error){return qualitygate.WriterResult{},fmt.Errorf("import must not call Writer")}
type lifecycleLibrarian struct{calls *int}
func(l lifecycleLibrarian) Propose(_ context.Context,r qualitygate.LibrarianRequest)(qualitygate.FactProposal,error){
 (*l.calls)++
 change:=qualitygate.FactChange{Subject:"location:harbor",Predicate:"weather",Object:json.RawMessage(`"rain"`),SourceChapter:r.Chapter,SourceVersion:r.Candidate.SourceVersion,SourceSHA:r.Candidate.TextSHA,Extractor:"lifecycle-test",Confidence:1,ProposedAuthority:"llm_suggestion",ValidFromChapter:r.Chapter,KnownFromChapter:r.Chapter,Reason:"from imported candidate"}
 return qualitygate.FactProposal{ProposalID:"import-"+r.Candidate.ID,ProjectID:r.ProjectID,Chapter:r.Chapter,SourceVersion:r.Candidate.SourceVersion,SourceSHA:r.Candidate.TextSHA,Extractor:"lifecycle-test",Authority:"llm_suggestion",WorldFacts:[]qualitygate.FactChange{change}},nil
}
type lifecycleEditor struct{}
func(lifecycleEditor) Review(context.Context,qualitygate.EditorRequest)(qualitygate.EditorReview,error){return qualitygate.EditorReview{Score:9,Summary:"reviewed"},nil}
func lifecycleServer(t *testing.T,workspace string,calls *int)*Server{t.Helper();s,err:=New(Config{Workspace:workspace,QualityWriter:lifecycleWriter{},QualityLibrarian:lifecycleLibrarian{calls},QualityEditor:lifecycleEditor{}});if err!=nil{t.Fatal(err)};return s}
func lifeJSON(t *testing.T,s *Server,method,url,key string,value any)*httptest.ResponseRecorder{t.Helper();var b []byte;if value!=nil{b,_=json.Marshal(value)};r:=httptest.NewRequest(method,url,bytes.NewReader(b));if key!=""{r.Header.Set("Idempotency-Key",key)};r.Header.Set("Content-Type","application/json");w:=httptest.NewRecorder();s.Handler().ServeHTTP(w,r);return w}
func lifeUpload(t *testing.T,s *Server,url,name,key string,data []byte)*httptest.ResponseRecorder{t.Helper();var b bytes.Buffer;m:=multipart.NewWriter(&b);f,err:=m.CreateFormFile("file",name);if err!=nil{t.Fatal(err)};_,_=f.Write(data);_=m.Close();r:=httptest.NewRequest("POST",url,&b);r.Header.Set("Content-Type",m.FormDataContentType());r.Header.Set("Idempotency-Key",key);w:=httptest.NewRecorder();s.Handler().ServeHTTP(w,r);return w}
func requireLifeStatus(t *testing.T,w *httptest.ResponseRecorder,status int){t.Helper();if w.Code!=status{t.Fatalf("status %d want %d: %s",w.Code,status,w.Body.String())}}

func TestLifecycleImportAnalysisExportRestoreFlow(t *testing.T){
 ctx:=context.Background();workspace:=t.TempDir();calls:=0;s:=lifecycleServer(t,workspace,&calls)
 p,err:=s.projects.Create(ctx,project.CreateInput{Title:"导入测试",Language:"zh"});if err!=nil{t.Fatal(err)};prefix:="/api/projects/"+p.ID+"/lifecycle"
 source:=[]byte("第1章 风雨\n张三在港口躲雨。")
 w:=lifeUpload(t,s,prefix+"/imports","story.txt","upload-1",source);requireLifeStatus(t,w,201)
 var upload struct{Import lifecycle.Import `json:"import"`};if err=json.Unmarshal(w.Body.Bytes(),&upload);err!=nil{t.Fatal(err)};if calls!=0{t.Fatal("upload called a model")}
 db,err:=s.projects.OpenLifecycle(ctx,p.ID);if err!=nil{t.Fatal(err)}
 _,err=db.DB.Exec(`CREATE TRIGGER fail_import_progress BEFORE UPDATE OF state ON manuscript_import_chapters WHEN NEW.state='analyzed' BEGIN SELECT RAISE(ABORT,'test boundary'); END`);if err!=nil{t.Fatal(err)}
 step:=prefix+"/imports/"+upload.Import.ID+"/step"
 w=lifeJSON(t,s,"POST",step,"analyze-1",map[string]any{"chapter":1,"analyze":true});requireLifeStatus(t,w,500);if calls!=1{t.Fatal("analysis was not reached")}
 if _,err=db.DB.Exec(`DROP TRIGGER fail_import_progress`);err!=nil{t.Fatal(err)};db.DB.Close()
 // New server and HTTP operation key, same durable semantic Check identity.
 s=lifecycleServer(t,workspace,&calls)
 w=lifeJSON(t,s,"POST",step,"analyze-recovered",map[string]any{"chapter":1,"analyze":true});requireLifeStatus(t,w,200)
 var response struct{Chapter lifecycle.ImportChapter `json:"chapter"`};if err=json.Unmarshal(w.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
 if response.Chapter.State!="analyzed" || calls!=1{t.Fatalf("analysis replay: %+v calls=%d",response,calls)}
 versions,err:=s.projects.OpenChapterVersionStore(ctx,p.ID);if err!=nil{t.Fatal(err)};active,err:=versions.ActiveFinal(ctx,1,false);if err!=nil || active!=nil{t.Fatal("analysis promoted a Final",err)};var n int
 if err=versions.Database().QueryRow(`SELECT count(*) FROM truth_events`).Scan(&n);err!=nil || n!=0{t.Fatal("analysis wrote Truth",err)};versions.Close()
 c,cleanup,failure:=s.chapterVersionCoordinator(httptest.NewRequest("POST","/",nil),p.ID);if failure!=nil{t.Fatal(failure)}
 if _,err=c.Accept(ctx,1,"accept-import",response.Chapter.VersionID,"explicit human review");err!=nil{t.Fatal(err)}
 if _,err=c.Finalize(ctx,1,"final-import",response.Chapter.VersionID);err!=nil{t.Fatal(err)};cleanup()
 w=lifeJSON(t,s,"GET",prefix+"/export?format=epub&from=1&to=1","",nil);requireLifeStatus(t,w,200)
 book,err:=lifecycle.Parse("book.epub",w.Body.Bytes());if err!=nil || len(book.Chapters)!=1 || !strings.Contains(book.Chapters[0].Text,"张三"){t.Fatal("EPUB export",err,book)}
 // Secrets in configuration files must never enter a portable archive.
 config:=filepath.Join(workspace,filepath.FromSlash(p.Path),".novelforge","config.json");if err=os.WriteFile(config,[]byte(`{"api_key":"test-only-config-secret"}`),0600);err!=nil{t.Fatal(err)}
 w=lifeJSON(t,s,"GET",prefix+"/backup","",nil);requireLifeStatus(t,w,200);backup:=append([]byte(nil),w.Body.Bytes()...)
 _,files,err:=lifecycle.Unpack(backup);if err!=nil{t.Fatal(err)};for name,b:=range files{if strings.Contains(name,"config") || bytes.Contains(b,[]byte("test-only-config-secret")){t.Fatal("configuration leaked",name)}}
 restored:=lifecycleServer(t,t.TempDir(),&calls)
 w=lifeUpload(t,restored,"/api/lifecycle/restore","project.zip","restore-1",backup);requireLifeStatus(t,w,201)
 var result project.LifecycleRestoreResult;if err=json.Unmarshal(w.Body.Bytes(),&result);err!=nil{t.Fatal(err)};if result.Project.ID!=p.ID || result.JobsResumed || !result.RequiresConfiguration{t.Fatal("restore contract",result)}
 w=lifeJSON(t,restored,"GET",prefix+"/export?format=txt","",nil);requireLifeStatus(t,w,200);if !strings.Contains(w.Body.String(),"张三在港口躲雨。"){t.Fatal("restored Final changed")}
 w=lifeUpload(t,restored,"/api/lifecycle/restore","project.zip","restore-again",backup);requireLifeStatus(t,w,200)
 w=lifeUpload(t,s,prefix+"/imports","story.txt","upload-again",source);requireLifeStatus(t,w,201)
 if calls!=1{t.Fatal("export/backup/restore repeated analysis")}
}

func TestLifecycleTransportIsolationAndGuards(t *testing.T){
 ctx:=context.Background();calls:=0;s:=lifecycleServer(t,t.TempDir(),&calls)
 p,_:=s.projects.Create(ctx,project.CreateInput{Title:"one"});q,_:=s.projects.Create(ctx,project.CreateInput{Title:"two"})
 prefix:="/api/projects/"+p.ID+"/lifecycle"
 w:=lifeUpload(t,s,prefix+"/imports","book.md","import-a",[]byte("# Start\nSome text."));requireLifeStatus(t,w,201);var v struct{Import lifecycle.Import `json:"import"`};_=json.Unmarshal(w.Body.Bytes(),&v)
 w=lifeJSON(t,s,"GET","/api/projects/"+q.ID+"/lifecycle/imports/"+v.Import.ID,"",nil);requireLifeStatus(t,w,404)
 w=lifeJSON(t,s,"POST",prefix+"/imports/"+v.Import.ID+"/step","bad-fields",map[string]any{"chapter":1,"execute":"anything"});requireLifeStatus(t,w,400)
 lease,err:=s.projects.AcquireExecution(ctx,p.ID);if err!=nil{t.Fatal(err)}
 w=lifeJSON(t,s,"GET",prefix+"/backup","",nil);requireLifeStatus(t,w,409);lease.Close()
 w=lifeJSON(t,s,"GET",prefix+"/export?format=txt","",nil);if w.Code==200{t.Fatal("exported unaccepted staged manuscript")}
 var spec struct{Paths map[string]json.RawMessage `json:"paths"`};if err=json.Unmarshal(openAPISpec,&spec);err!=nil{t.Fatal(err)}
 for _,route:=range []string{"/api/lifecycle/restore","/api/projects/{id}/lifecycle/imports","/api/projects/{id}/lifecycle/imports/{import}/step","/api/projects/{id}/lifecycle/export","/api/projects/{id}/lifecycle/backup","/api/projects/{id}/lifecycle/migrate"}{if len(spec.Paths[route])==0{t.Fatal("missing contract",route)}}
 if calls!=0{t.Fatal("transport guard called model")}
}

var _ = http.MethodGet
