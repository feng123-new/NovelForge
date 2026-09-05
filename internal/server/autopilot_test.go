package server

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "sync"
 "testing"
 "time"

 "github.com/voocel/ainovel-cli/internal/autopilot"
 "github.com/voocel/ainovel-cli/internal/project"
 "github.com/voocel/ainovel-cli/internal/qualitygate"
)
type phase9Model struct{mu sync.Mutex;calls map[string]int}
func(m *phase9Model)Invoke(_ context.Context,op string,data []byte)([]byte,qualitygate.ModelUsage,error){
 m.mu.Lock();m.calls[op]++;m.mu.Unlock()
 var out any
 switch op {
 case "architect:foundation":out=autopilot.Foundation{StoryCompass:"Mira seeks the bell",WorldRules:[]string{"No teleportation"},Characters:[]autopilot.Character{{ID:"Mira",Name:"Mira",InitialState:"Alive at the gate"}},POV:"Mira",Arcs:[]autopilot.Arc{{Title:"Arrival",Summary:"Find the bell",FirstChapter:1,LastChapter:2}},Ending:"The bell is found"}
 case "planner:chapter":var p struct{Chapter int `json:"chapter"`};_ = json.Unmarshal(data,&p);var plan qualitygate.ChapterPlan;_ = json.Unmarshal(apiPlan(p.Chapter),&plan);out=plan
 case "writer:draft":var p map[string]json.RawMessage;_ = json.Unmarshal(data,&p);if len(p["compiled_context"])==0{return nil,qualitygate.ModelUsage{},fmt.Errorf("missing compiled context")};return []byte("Mira entered the gate. She heard the bell."),qualitygate.ModelUsage{InputTokens:10,OutputTokens:10},nil
 case "librarian:fact_proposal":
  var p struct{ProjectID string `json:"project_id"`;Chapter int `json:"chapter"`;Candidate qualitygate.Candidate `json:"candidate"`};_ = json.Unmarshal(data,&p)
  proposal,err:=(&apiLibrarian{}).Propose(context.Background(),qualitygate.LibrarianRequest{ProjectID:p.ProjectID,Chapter:p.Chapter,Candidate:p.Candidate});if err!=nil{return nil,qualitygate.ModelUsage{},err}
  proposal.CharacterChanges=[]qualitygate.FactChange{{Subject:"Mira",Predicate:"state",Object:json.RawMessage(`"alive"`),SourceChapter:p.Chapter,SourceVersion:p.Candidate.SourceVersion,SourceSHA:p.Candidate.TextSHA,Extractor:proposal.Extractor,Confidence:1,ProposedAuthority:"llm_suggestion",ValidFromChapter:p.Chapter,KnownFromChapter:p.Chapter,Reason:"Mira acts in the text"}};out=proposal
 case "editor:review":review,_:=(&apiEditor{}).Review(context.Background(),qualitygate.EditorRequest{});out=review
 default:return nil,qualitygate.ModelUsage{},fmt.Errorf("unexpected operation %s",op)
 }
 raw,err:=json.Marshal(out);return raw,qualitygate.ModelUsage{InputTokens:10,OutputTokens:10},err
}
func postJob(t *testing.T,s *Server,url,key,body string)*httptest.ResponseRecorder{t.Helper();r:=httptest.NewRequest(http.MethodPost,url,bytes.NewBufferString(body));r.Header.Set("Idempotency-Key",key);w:=httptest.NewRecorder();s.Handler().ServeHTTP(w,r);return w}
func awaitJob(t *testing.T,s *Server,id,state string,chapter int)autopilot.Job{t.Helper();deadline:=time.Now().Add(15*time.Second);var j autopilot.Job;for time.Now().Before(deadline){var err error;j,err=s.jobs.Get(t.Context(),id);if err!=nil{t.Fatal(err)};if j.State==state&&(chapter==0||j.Chapter==chapter){return j};if j.State==autopilot.Failed{t.Fatalf("job failed: %s stage=%s chapter=%d",j.ErrorCode,j.Stage,j.Chapter)};time.Sleep(20*time.Millisecond)};t.Fatalf("job did not reach %s: %+v",state,j);return j}
func TestAutopilotRealChapterFlowReviewAndRestart(t *testing.T){
 workspace:=t.TempDir();model:=&phase9Model{calls:map[string]int{}}
 cfg:=Config{Workspace:workspace,QualityModel:model,AutopilotEnabled:true}
 s,err:=New(cfg);if err!=nil{t.Fatal(err)};defer s.Close()
 p,err:=s.projects.Create(t.Context(),project.CreateInput{Title:"Durable two chapters",TargetChapters:2,Language:"en",WordsPerChapter:500});if err!=nil{t.Fatal(err)}
 _,err=s.projects.SaveFoundationRequest(t.Context(),p.ID,project.FoundationRequestInput{Idea:"Mira finds a bell",Automation:project.AutomationSettings{Mode:"copilot",ReviewPolicy:"every_chapter",MaxRewrites:2}});if err!=nil{t.Fatal(err)}
 base:="/api/projects/"+p.ID+"/autopilot"
 created:=postJob(t,s,base,"start",`{"start_chapter":1,"target_chapter":2}`);if created.Code!=202{t.Fatal(created.Code,created.Body.String())}
 id:=autopilot.Identity(p.ID,"start");j:=awaitJob(t,s,id,autopilot.Paused,1);if j.ErrorCode!="REVIEW_REQUIRED"{t.Fatal("not awaiting explicit review",j)}
 versions,err:=s.projects.OpenChapterVersionStore(t.Context(),p.ID);if err!=nil{t.Fatal(err)};active,err:=versions.ActiveFinal(t.Context(),1,false);versions.Close();if err!=nil||active!=nil{t.Fatal("review policy allowed early Final",err)}
 detail:=httptest.NewRecorder();s.Handler().ServeHTTP(detail,httptest.NewRequest(http.MethodGet,base+"/"+id,nil));if detail.Code!=200||!bytes.Contains(detail.Body.Bytes(),[]byte("Mira entered")){t.Fatal("candidate not inspectable",detail.Body.String())}
 s.Close();s,err=New(cfg);if err!=nil{t.Fatal(err)};defer s.Close()
 resumed:=postJob(t,s,base+"/"+id+"/resume","approve1",`{}`);if resumed.Code!=202{t.Fatal(resumed.Body.String())}
 awaitJob(t,s,id,autopilot.Paused,2)
 resumed=postJob(t,s,base+"/"+id+"/resume","approve2",`{}`);if resumed.Code!=202{t.Fatal(resumed.Body.String())}
 j=awaitJob(t,s,id,autopilot.Completed,0);if j.CompletedThrough!=2{t.Fatal("wrong completion",j)}
 replay:=postJob(t,s,base,"start",`{"start_chapter":1,"target_chapter":2}`);if replay.Code!=202||replay.Header().Get("Idempotency-Replayed")!="true"{t.Fatal("start did not replay",replay.Code,replay.Body.String())}
 versions,err=s.projects.OpenChapterVersionStore(t.Context(),p.ID);if err!=nil{t.Fatal(err)};defer versions.Close()
 for n:=1;n<=2;n++{v,err:=versions.ActiveFinal(t.Context(),n,false);if err!=nil||v==nil{t.Fatal("missing active Final",n,err)}}
 model.mu.Lock();defer model.mu.Unlock();if model.calls["architect:foundation"]!=1||model.calls["writer:draft"]!=2{t.Fatal("restart repeated model calls",model.calls)}
}
func TestAutopilotTransportIsolationAndContracts(t *testing.T){
 s,err:=New(Config{Workspace:t.TempDir()});if err!=nil{t.Fatal(err)};defer s.Close()
 p,err:=s.projects.Create(t.Context(),project.CreateInput{Title:"Locked project"});if err!=nil{t.Fatal(err)}
 j,err:=s.jobs.Enqueue(t.Context(),p.ID,"queued",autopilot.Input{Idea:"Story",StartChapter:1,TargetChapter:1,MaxRewrites:2,ReviewEvery:1});if err!=nil{t.Fatal(err)}
 blocked:=postJob(t,s,"/api/projects/"+p.ID+"/archive","archive",`{}`);if blocked.Code!=409{t.Fatal("mutation not blocked",blocked.Code,blocked.Body.String())}
 wrong:=httptest.NewRecorder();s.Handler().ServeHTTP(wrong,httptest.NewRequest(http.MethodGet,"/api/projects/another/autopilot/"+j.ID,nil));if wrong.Code!=404{t.Fatal("cross-project job exposed",wrong.Code)}
 stopped:=postJob(t,s,"/api/projects/"+p.ID+"/autopilot/"+j.ID+"/stop","stop",`{}`);if stopped.Code!=202{t.Fatal(stopped.Body.String())}
 var spec map[string]any;if err=json.Unmarshal(openAPISpec,&spec);err!=nil{t.Fatal(err)}
 paths:=spec["paths"].(map[string]any);for _,path:=range []string{"/api/projects/{id}/autopilot","/api/projects/{id}/autopilot/{job}","/api/projects/{id}/autopilot/{job}/pause","/api/projects/{id}/autopilot/{job}/stop","/api/projects/{id}/autopilot/{job}/resume"}{if paths[path]==nil{t.Fatal("missing OpenAPI",path)}}
}
