package server

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "net/http"
 "strings"

 "github.com/voocel/ainovel-cli/internal/autopilot"
 "github.com/voocel/ainovel-cli/internal/bootstrap"
 "github.com/voocel/ainovel-cli/internal/project"
 "github.com/voocel/ainovel-cli/internal/qualitygate"
 "github.com/voocel/ainovel-cli/internal/server/qualityruntime"
)

// chapterJobEngine adapts the existing deterministic Chapter Engine (Phase 5)
// and immutable Final coordinator (Phase 8). It does not invoke the old host
// writing loop, which would bypass this authority boundary.
type chapterJobEngine struct{s *Server}

func(s *Server) autopilotModel(ctx context.Context,id string,profile map[string]string)(qualitygate.ModelInvoker,error){
 if s.cfg.QualityModel!=nil{return s.cfg.QualityModel,nil}
 if !s.cfg.QualityConfigEnabled{return nil,autopilot.Stop("MODEL_NOT_CONFIGURED")}
 cfg,err:=s.projects.LoadModelConfig(ctx,id,s.cfg.QualityConfigPath);if err!=nil{return nil,autopilot.Stop("MODEL_NOT_CONFIGURED")}
 if cfg.Roles==nil{cfg.Roles=map[string]bootstrap.RoleConfig{}}
 for role,value:=range profile {
  if role!="architect"&&role!="writer"&&role!="librarian"&&role!="editor"{return nil,autopilot.Stop("MODEL_PROFILE_INVALID")}
  provider,model,ok:=strings.Cut(value,"/");if !ok||model==""{return nil,autopilot.Stop("MODEL_PROFILE_INVALID")}
  if _,ok=cfg.Providers[provider];!ok{return nil,autopilot.Stop("MODEL_PROFILE_NOT_CONFIGURED")}
  selection:=cfg.Roles[role];selection.Provider=provider;selection.Model=model;cfg.Roles[role]=selection
 }
 model,err:=qualityruntime.New(cfg);if err!=nil{return nil,autopilot.Stop("MODEL_NOT_CONFIGURED")};return model,nil
}

func(e chapterJobEngine) Step(ctx context.Context,j autopilot.Job)(autopilot.Job,error){
 if _,err:=e.s.projects.Get(ctx,j.ProjectID);err!=nil{return j,autopilot.Stop("PROJECT_UNAVAILABLE")}
 model,err:=e.s.autopilotModel(ctx,j.ProjectID,j.Input.ModelProfile);if err!=nil{return j,err}
 // Copy configuration, not Server ownership; this adapter never closes Server.
 local:=*e.s;local.cfg=e.s.cfg;local.cfg.QualityModel=model;local.cfg.QualityPolicy.MaxRewrites=j.Input.MaxRewrites
 req,_:=http.NewRequestWithContext(ctx,http.MethodGet,"http://localhost/internal-autopilot",nil)
 quality,cleanup,failure:=local.qualityCoordinator(req,j.ProjectID);if failure!=nil{return j,autopilot.Stop("QUALITY_SERVICE_UNAVAILABLE")};defer cleanup()
 if writer,ok:=quality.Writer.(qualitygate.ModelWriterService);ok {
  writer.Context=project.FoundationContext{Repository:e.s.projects,Foundation:j.Foundation,Style:fmt.Sprintf("%s\nLanguage: %s. Target chapter length: %d characters/words.",j.Input.Style,j.Input.Language,j.Input.WordsPerChapter)}
  quality.Writer=writer
 }
 call:=func(agent,operation string,payload any)([]byte,error){
  data,err:=json.Marshal(payload);if err!=nil{return nil,err}
  invoker:=qualitygate.RetryingModelInvoker{Invoker:model,MaxRetries:e.s.cfg.QualityMaxRetries}
  caller:=qualitygate.IdempotentModelCaller{Repository:quality.Store,Invoker:invoker}
  out,_,_,err:=caller.Call(ctx,qualitygate.CallRequest{IdempotencyKey:j.CallKey(agent+":"+operation),ProjectID:j.ProjectID,Chapter:j.Chapter,TransactionID:j.ID,Agent:agent,Operation:operation,Payload:data})
  if err!=nil {var ce *qualitygate.ModelCallError;if errors.As(err,&ce)&&ce.Retryable{return nil,autopilot.Retry("PLANNING_PROVIDER_RETRY")};return nil,autopilot.Stop("PLANNING_CALL_FAILED")};return out,nil
 }
 switch j.Stage {
 case "foundation":
  data,err:=call("architect","foundation",j.Input);if err!=nil{return j,err}
  var f autopilot.Foundation;if err=autopilot.Decode(data,&f);err!=nil{return j,autopilot.Stop("FOUNDATION_SCHEMA_INVALID")};if f.Validate(j.Input.TargetChapter)!=nil{return j,autopilot.Stop("FOUNDATION_INVALID")}
  j.Foundation=&f;j.Stage="plan_context";j.ErrorCode="";return j,nil
 case "plan_context":
  if j.Foundation==nil{return j,autopilot.Stop("FOUNDATION_MISSING")}
  if err=e.checkBoundary(ctx,j,true);err!=nil{return j,err}
  data,err:=e.s.projects.PlanningContext(ctx,j.ProjectID,j.Chapter,j.Foundation.POV);if err!=nil{return j,autopilot.Stop("PLANNING_CONTEXT_FAILED")}
  j.PlanningContext=data;j.Stage="plan";j.ErrorCode="";return j,nil
 case "plan":
  data,err:=call("planner","chapter",map[string]any{"chapter":j.Chapter,"target_chapter":j.Input.TargetChapter,"language":j.Input.Language,"foundation":j.Foundation,"selected_context":j.PlanningContext})
  if err!=nil{return j,err}
  var p qualitygate.ChapterPlan;if autopilot.Decode(data,&p)!=nil||p.Validate()!=nil||p.Chapter!=j.Chapter{return j,autopilot.Stop("CHAPTER_PLAN_INVALID")}
  known:=false;for _,c:=range j.Foundation.Characters {known=known||c.ID==p.POV};if !known{return j,autopilot.Stop("PLAN_POV_UNKNOWN")}
  j.Plan=&p;j.Stage="generate";j.ErrorCode="";return j,nil
 case "generate","check","rewrite","finalize":
  if j.Plan==nil||j.Plan.Chapter!=j.Chapter{return j,autopilot.Stop("CHAPTER_PLAN_MISSING")}
  snapshot,err:=quality.Store.Snapshot(ctx,j.ProjectID,j.Chapter)
  if err!=nil&&!errors.Is(err,qualitygate.ErrNotFound){return j,autopilot.Stop("QUALITY_STATE_UNAVAILABLE")}
  if errors.Is(err,qualitygate.ErrNotFound){
   if err=e.checkBoundary(ctx,j,true);err!=nil{return j,err}
   snapshot,err=quality.Generate(ctx,j.ProjectID,j.Chapter,*j.Plan)
  }else{
   switch snapshot.Transaction.State {
   case qualitygate.StateCompleted:
    return e.advance(ctx,j)
   case qualitygate.StateFinalCandidate:
    due:=j.Input.ReviewEvery>0&&((j.Chapter-j.Input.StartChapter+1)%j.Input.ReviewEvery==0||j.Chapter==j.Input.TargetChapter)
    if due&&!j.ReviewApproved{j.State=autopilot.Paused;j.Stage="finalize";j.ErrorCode="REVIEW_REQUIRED";return j,nil}
    if err=e.checkBoundary(ctx,j,true);err!=nil{return j,err}
    snapshot,err=local.finalizeQualityPhase8(req,quality,j.ProjectID,j.Chapter,j.CallKey("final"))
   case qualitygate.StateTruthCommitPending,qualitygate.StateCheckpointPending:
    // An incomplete Final saga must be replayed, never treated as completed
    // merely because the Active Final pointer has already switched.
    snapshot,err=local.finalizeQualityPhase8(req,quality,j.ProjectID,j.Chapter,j.CallKey("final"))
   case qualitygate.StateRewritePending:
    snapshot,err=quality.Rewrite(ctx,j.ProjectID,j.Chapter,*j.Plan)
   case qualitygate.StatePlanned,qualitygate.StateDrafting:
    snapshot,err=quality.Generate(ctx,j.ProjectID,j.Chapter,*j.Plan)
   case qualitygate.StateHold:
    if snapshot.Continuity!=nil&&snapshot.Continuity.Status==qualitygate.ContinuityFail&&snapshot.Transaction.Attempt>=snapshot.Transaction.MaxRewrites{return j,autopilot.Stop("CONTINUITY_REWRITE_EXHAUSTED")}
    if len(snapshot.Candidates)==0{snapshot,err=quality.Generate(ctx,j.ProjectID,j.Chapter,*j.Plan)}else{snapshot,err=quality.Check(ctx,j.ProjectID,j.Chapter)}
   case qualitygate.StateFailed:return j,autopilot.Stop("QUALITY_FAILED")
   default:snapshot,err=quality.Check(ctx,j.ProjectID,j.Chapter)
   }
  }
  if err!=nil {if ctx.Err()!=nil{return j,ctx.Err()};return j,autopilot.Retry("QUALITY_STEP_ERROR")}
  if snapshot.Transaction.State==qualitygate.StateHold||snapshot.Transaction.State==qualitygate.StateFailed{return j,autopilot.Stop("QUALITY_HOLD")}
  if snapshot.Transaction.State==qualitygate.StateCompleted{return e.advance(ctx,j)}
  switch snapshot.Transaction.State{case qualitygate.StateRewritePending:j.Stage="rewrite";case qualitygate.StateFinalCandidate,qualitygate.StateTruthCommitPending,qualitygate.StateCheckpointPending:j.Stage="finalize";default:j.Stage="check"}
  j.ErrorCode="";return j,nil
 default:return j,autopilot.Stop("AUTOPILOT_STAGE_INVALID")
 }
}

func(e chapterJobEngine) checkBoundary(ctx context.Context,j autopilot.Job,rejectCurrent bool)error{
 versions,err:=e.s.projects.OpenChapterVersionStore(ctx,j.ProjectID);if err!=nil{return autopilot.Stop("VERSION_STORE_UNAVAILABLE")};defer versions.Close()
 if j.Chapter>1 {
  active,err:=versions.ActiveFinal(ctx,j.Chapter-1,false);if err!=nil||active==nil{return autopilot.Stop("PRIOR_FINAL_REQUIRED")}
  status,err:=versions.DetectExternal(ctx,j.Chapter-1);if err!=nil||status.SyncRequired{return autopilot.Stop("PRIOR_CHAPTER_SYNC_REQUIRED")}
 }
 if rejectCurrent {
  active,err:=versions.ActiveFinal(ctx,j.Chapter,false);if err!=nil{return autopilot.Stop("VERSION_STORE_UNAVAILABLE")};if active!=nil{return autopilot.Stop("EXISTING_FINAL_PROTECTED")}
  // Do not overwrite legacy or externally created prose that has not passed
  // the immutable version/sync workflow.
  for offset:=0;;offset+=100{page,err:=e.s.projects.ListChapters(ctx,j.ProjectID,100,offset);if err!=nil{return autopilot.Stop("CHAPTER_INSPECTION_FAILED")};for _,c:=range page.Chapters{if c.Chapter==j.Chapter{return autopilot.Stop("EXISTING_CHAPTER_REQUIRES_SYNC")}};if page.NextOffset==nil{break}}
 }
 return nil
}
func(e chapterJobEngine) advance(ctx context.Context,j autopilot.Job)(autopilot.Job,error){
 versions,err:=e.s.projects.OpenChapterVersionStore(ctx,j.ProjectID);if err!=nil{return j,autopilot.Stop("VERSION_STORE_UNAVAILABLE")};defer versions.Close()
 active,err:=versions.ActiveFinal(ctx,j.Chapter,false);if err!=nil||active==nil{return j,autopilot.Stop("FINAL_CHECKPOINT_MISSING")}
 sync,err:=versions.DetectExternal(ctx,j.Chapter);if err!=nil||sync.SyncRequired{return j,autopilot.Stop("CHAPTER_SYNC_REQUIRED")}
 j.CompletedThrough=j.Chapter;j.Chapter++;j.Plan=nil;j.PlanningContext=nil;j.ReviewApproved=false;j.ErrorCode="";j.Stage="plan_context"
 if j.Chapter>j.Input.TargetChapter{j.State=autopilot.Completed;j.Stage="completed"}
 return j,nil
}
