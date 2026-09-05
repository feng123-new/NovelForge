package server

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "testing"
 "time"

 "github.com/voocel/ainovel-cli/internal/autopilot"
 "github.com/voocel/ainovel-cli/internal/db/migrate"
 "github.com/voocel/ainovel-cli/internal/project"
 "github.com/voocel/ainovel-cli/internal/qualitygate"
 "github.com/voocel/ainovel-cli/internal/truthstore"
)

type closureModel struct {
 base *phase9Model
 switchPOV bool
 observedHorizon int
}
func (m *closureModel) Invoke(ctx context.Context,op string,payload []byte)([]byte,qualitygate.ModelUsage,error){
 if op=="architect:foundation"{var in autopilot.Input;if err:=json.Unmarshal(payload,&in);err!=nil{return nil,qualitygate.ModelUsage{},err};m.observedHorizon=in.TargetChapter}
 out,usage,err:=m.base.Invoke(ctx,op,payload);if err!=nil{return out,usage,err}
 if m.switchPOV && op=="architect:foundation" {var f autopilot.Foundation;_ = json.Unmarshal(out,&f);f.Characters=append(f.Characters,autopilot.Character{ID:"Other",Name:"Other",InitialState:"at sea"});out,err=json.Marshal(f)}
 if m.switchPOV && op=="planner:chapter" {var p qualitygate.ChapterPlan;_ = json.Unmarshal(out,&p);p.POV="Other";out,err=json.Marshal(p)}
 return out,usage,err
}
func closureFixture(t *testing.T,model qualitygate.ModelInvoker)(*Server,string,string,string){
 t.Helper();workspace:=t.TempDir();s,err:=New(Config{Workspace:workspace,QualityModel:model});if err!=nil{t.Fatal(err)};t.Cleanup(func(){s.Close()})
 p,err:=s.projects.Create(t.Context(),project.CreateInput{Title:"Closure",Slug:"closure",TargetChapters:2,WordsPerChapter:500,Language:"en"});if err!=nil{t.Fatal(err)}
 f,err:=s.projects.SaveFoundationRequest(t.Context(),p.ID,project.FoundationRequestInput{Idea:"Mira finds the bell",Automation:project.AutomationSettings{Mode:"autopilot",ReviewPolicy:"full_automatic",MaxRewrites:2}});if err!=nil{t.Fatal(err)}
 return s,p.ID,f.ID,filepath.Join(workspace,"closure",".novelforge","project.db")
}
func closureEnqueue(t *testing.T,s *Server,id,foundation,key string,from,to int) autopilot.Job{
 t.Helper();j,err:=s.jobs.Enqueue(t.Context(),id,key,autopilot.Input{FoundationID:foundation,Idea:"Mira finds the bell",Language:"en",WordsPerChapter:500,BookTargetChapter:2,StartChapter:from,TargetChapter:to,ReviewEvery:0,MaxRewrites:2});if err!=nil{t.Fatal(err)};return j
}
func closureStep(t *testing.T,s *Server) (autopilot.Job,error){
 t.Helper();before,err:=s.jobs.Next(t.Context());if err!=nil{t.Fatal(err)}
 lease,err:=s.projects.AcquireExecution(t.Context(),before.ProjectID);if err!=nil{t.Fatal(err)}
 after,stepErr:=(chapterJobEngine{s:s}).Step(t.Context(),before);lease.Close()
 stored,err:=s.jobs.Finish(t.Context(),before,after,stepErr);if err!=nil{t.Fatal(err)}
 return stored,stepErr
}
func closureComplete(t *testing.T,s *Server,id string) autopilot.Job {
 t.Helper();for n:=0;n<30;n++{j,err:=s.jobs.Get(t.Context(),id);if err!=nil{t.Fatal(err)};if j.State==autopilot.Completed{return j};if j.State==autopilot.Failed||j.State==autopilot.Paused{t.Fatal("unexpected stop",j.ErrorCode,j.Stage)};j,err=closureStep(t,s);if err!=nil{t.Fatal("step failed",j.ErrorCode,err)}};t.Fatal("bounded short flow made no completion");return autopilot.Job{}
}
func TestAutopilotClosureBatchHorizonAndNextChapter(t *testing.T){
 model:=&closureModel{base:&phase9Model{calls:map[string]int{}}};s,id,foundation,_:=closureFixture(t,model)
 first:=closureEnqueue(t,s,id,foundation,"batch-one",1,1);closureComplete(t,s,first.ID)
 if model.observedHorizon!=2{t.Fatal("one-chapter batch truncated book foundation",model.observedHorizon)}
 next,err:=s.projects.AutopilotNextChapter(t.Context(),id);if err!=nil||next!=2{t.Fatal("progress did not use Final checkpoint",next,err)}
 second:=closureEnqueue(t,s,id,foundation,"batch-two",2,2);closureComplete(t,s,second.ID)
 if model.base.calls["architect:foundation"]!=1||model.base.calls["writer:draft"]!=2{t.Fatal("saved foundation not reused",model.base.calls)}
 next,err=s.projects.AutopilotNextChapter(t.Context(),id);if err!=nil||next!=3{t.Fatal(next,err)}
 response:=httptest.NewRecorder();s.Handler().ServeHTTP(response,httptest.NewRequest(http.MethodGet,"/api/projects/"+id+"/autopilot",nil))
 if response.Code!=200{t.Fatal(response.Body.String())};var body struct{Next int `json:"next_chapter"`};if json.Unmarshal(response.Body.Bytes(),&body)!=nil||body.Next!=3{t.Fatal("UI recommended progress is stale",response.Body.String())}
}
func TestAutopilotClosureRejectsPOVScopeSwitch(t *testing.T){
 model:=&closureModel{base:&phase9Model{calls:map[string]int{}},switchPOV:true};s,id,f,_:=closureFixture(t,model);closureEnqueue(t,s,id,f,"pov",1,2)
 for i:=0;i<2;i++{if _,err:=closureStep(t,s);err!=nil{t.Fatal(err)}}
 j,err:=closureStep(t,s);var failure *autopilot.Failure
 if !errors.As(err,&failure)||failure.Code!="PLAN_POV_SCOPE_MISMATCH"||j.State!=autopilot.Failed{t.Fatal("unscoped plan accepted",j,err)}
 if model.base.calls["writer:draft"]!=0{t.Fatal("writer received another POV's selected context")}
}
func TestAutopilotClosureAuthorityEditInvalidatesPlan(t *testing.T){
 model:=&phase9Model{calls:map[string]int{}};s,id,f,_:=closureFixture(t,model);closureEnqueue(t,s,id,f,"stale",1,2)
 for i:=0;i<2;i++{if _,err:=closureStep(t,s);err!=nil{t.Fatal(err)}}
 truth,err:=s.projects.OpenTruthStore(t.Context(),id);if err!=nil{t.Fatal(err)}
 _,err=truth.Append(t.Context(),truthstore.AppendInput{IdempotencyKey:"closure-edit",Kind:truthstore.EventAssert,SubjectType:"world_rule",SubjectID:"gate",Predicate:"locked",Value:json.RawMessage(`true`),ValidFromChapter:0,KnownFromChapter:0,Authority:truthstore.AuthorityHumanFinal,Confidence:1,Source:truthstore.Source{Type:"human",ID:"closure",Chapter:0,Version:"manual-v1",ConfirmedBy:"author"}});truth.Close();if err!=nil{t.Fatal(err)}
 j,stepErr:=closureStep(t,s);var failure *autopilot.Failure
 if !errors.As(stepErr,&failure)||failure.Code!="CHAPTER_CONTEXT_CHANGED"||j.State!=autopilot.Failed{t.Fatal("stale planning snapshot accepted",j,stepErr)}
 if model.calls["planner:chapter"]!=0||model.calls["writer:draft"]!=0{t.Fatal("stale model calls escaped guard",model.calls)}
}
func TestAutopilotClosureRequiresFinalCheckpointProof(t *testing.T){
 model:=&phase9Model{calls:map[string]int{}};s,id,f,path:=closureFixture(t,model)
 j:=closureEnqueue(t,s,id,f,"proof",1,1);done:=closureComplete(t,s,j.ID)
 db,err:=migrate.Open(path,5*time.Second);if err!=nil{t.Fatal(err)}
 if _,err=db.ExecContext(t.Context(),"DELETE FROM chapter_version_checkpoints WHERE project_id=? AND chapter=1",id);err!=nil{db.Close();t.Fatal(err)};db.Close()
 ok,err:=s.projects.AutopilotFinalComplete(t.Context(),id,1);if err!=nil||ok{t.Fatal("pointer substituted for proof",ok,err)}
 done.Chapter=1
 if _,err=(chapterJobEngine{s:s}).advance(t.Context(),done);err==nil{t.Fatal("advanced without checkpoint")}
 next,err:=s.projects.AutopilotNextChapter(t.Context(),id);if err!=nil||next!=1{t.Fatal("unproved chapter counted",next,err)}
}
func TestAutopilotClosureInterruptedRewriteReplaysRequest(t *testing.T){
 model:=&phase9Model{calls:map[string]int{}};s,id,_,path:=closureFixture(t,model)
 s.cfg.QualityPolicy.QualityThreshold=10
 request:=httptest.NewRequest(http.MethodGet,"http://localhost/internal",nil).WithContext(t.Context())
 quality,cleanup,failure:=s.qualityCoordinator(request,id);if failure!=nil{t.Fatal(failure)};defer cleanup()
 var plan qualitygate.ChapterPlan;if err:=json.Unmarshal(apiPlan(1),&plan);err!=nil{t.Fatal(err)}
 if _,err:=quality.Generate(t.Context(),id,1,plan);err!=nil{t.Fatal(err)}
 checked,err:=quality.Check(t.Context(),id,1);if err!=nil||checked.Transaction.State!=qualitygate.StateRewritePending{t.Fatal("rewrite fixture",checked.Transaction,err)}
 db,err:=migrate.Open(path,5*time.Second);if err!=nil{t.Fatal(err)};defer db.Close()
 _,err=db.ExecContext(t.Context(),`CREATE TRIGGER closure_fail_candidate BEFORE INSERT ON chapter_candidates WHEN NEW.attempt=1 BEGIN SELECT RAISE(ABORT,'injected failure after model response'); END;`);if err!=nil{t.Fatal(err)}
 if _,err=quality.Rewrite(t.Context(),id,1,plan);err==nil{t.Fatal("failure not injected")}
 if _,err=db.ExecContext(t.Context(),"DROP TRIGGER closure_fail_candidate");err!=nil{t.Fatal(err)}
 recovered,err:=quality.Generate(t.Context(),id,1,plan)
 if err!=nil||recovered.Transaction.State!=qualitygate.StateDraftReady||recovered.Transaction.Attempt!=1{t.Fatal("rewrite did not resume original attempt",recovered.Transaction,err)}
 if model.calls["writer:draft"]!=2{t.Fatal("persisted rewrite response was called again",model.calls)}
 if len(recovered.Candidates)!=2{t.Fatal("drafts lost or duplicated",len(recovered.Candidates))}
}
func TestAutopilotClosureOpenAPIApprovalContract(t *testing.T){
 var spec struct {Components struct{Schemas map[string]json.RawMessage `json:"schemas"`} `json:"components"`;Paths map[string]json.RawMessage `json:"paths"`}
 if err:=json.Unmarshal(openAPISpec,&spec);err!=nil{t.Fatal(err)}
 if !json.Valid(spec.Components.Schemas["AutopilotApproval"]){t.Fatal("approval schema absent")}
 path:=spec.Paths["/api/projects/{id}/autopilot/{job}/resume"]
 if !json.Valid(path)||len(path)==0{t.Fatal("resume contract absent")}
 fmt.Sprint(path) // parsed contract is tested in addition to the real HTTP flow
}
