package autopilot_test

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "testing"
 "time"

 "github.com/voocel/ainovel-cli/internal/autopilot"
)

func TestAutopilotStopCannotBeDowngraded(t *testing.T){
 s:=testStore(t);ctx:=t.Context();j,err:=s.Enqueue(ctx,"p","start",input());if err!=nil{t.Fatal(err)}
 before,err:=s.Next(ctx);if err!=nil{t.Fatal(err)}
 stopped,err:=s.Control(ctx,j.ID,"stop");if err!=nil{t.Fatal(err)}
 if _,err=s.Control(ctx,j.ID,"pause");!errors.Is(err,autopilot.ErrConflict){t.Fatal("pause replaced stop",err)}
 replay,err:=s.Control(ctx,j.ID,"stop");if err!=nil||replay.Revision!=stopped.Revision{t.Fatal("no-op control changed revision",replay,err)}
 after:=before;after.Stage="plan"
 final,err:=s.Finish(ctx,before,after,nil);if err!=nil||final.State!=autopilot.Cancelled{t.Fatal("stop not acknowledged",final,err)}
}
func TestAutopilotReviewRequiresExactDisplayedCandidate(t *testing.T){
 s:=testStore(t);ctx:=t.Context();j,err:=s.Enqueue(ctx,"p","start",input());if err!=nil{t.Fatal(err)}
 before,err:=s.Next(ctx);if err!=nil{t.Fatal(err)}
 after:=before;after.State=autopilot.Paused;after.Stage="finalize";after.ErrorCode="REVIEW_REQUIRED";after.ReviewCandidateID="candidate-1"
 paused,err:=s.Finish(ctx,before,after,nil);if err!=nil{t.Fatal(err)}
 for _,a:=range []autopilot.Approval{{},{Revision:paused.Revision-1,CandidateID:"candidate-1"},{Revision:paused.Revision,CandidateID:"candidate-2"}}{
  if _,err=s.ControlApproved(ctx,j.ID,"resume",a);!errors.Is(err,autopilot.ErrConflict){t.Fatal("stale/unseen candidate accepted",a,err)}
 }
 got,err:=s.ControlApproved(ctx,j.ID,"resume",autopilot.Approval{Revision:paused.Revision,CandidateID:"candidate-1"})
 if err!=nil||!got.ReviewApproved||got.State!=autopilot.Pending{t.Fatal(got,err)}
}
func TestAutopilotOldClaimCannotFinishNewAttempt(t *testing.T){
 s:=testStore(t);ctx:=t.Context();_,err:=s.Enqueue(ctx,"p","start",input());if err!=nil{t.Fatal(err)}
 old,err:=s.Next(ctx);if err!=nil{t.Fatal(err)}
 if err=s.Recover(ctx);err!=nil{t.Fatal(err)}
 fresh,err:=s.Next(ctx);if err!=nil{t.Fatal(err)}
 after:=old;after.Stage="plan"
 if _,err=s.Finish(ctx,old,after,nil);!errors.Is(err,autopilot.ErrConflict){t.Fatal("old claim completed new work",err)}
 after=fresh;after.Stage="plan"
 if _,err=s.Finish(ctx,fresh,after,nil);err!=nil{t.Fatal(err)}
}
func TestAutopilotStopsNoProgressAndPreservesReplay(t *testing.T){
 s:=testStore(t);ctx:=t.Context();j,err:=s.Enqueue(ctx,"p","start",input());if err!=nil{t.Fatal(err)}
 replay,err:=s.Enqueue(ctx,"p","start",input());if err!=nil||replay.Revision!=j.Revision{t.Fatal("idempotent enqueue changed event cursor",replay,err)}
 before,err:=s.Next(ctx);if err!=nil{t.Fatal(err)}
 after,err:=s.Finish(ctx,before,before,nil)
 if err!=nil||after.State!=autopilot.Failed||after.ErrorCode!="AUTOPILOT_NO_PROGRESS"{t.Fatal("silent infinite loop",after,err)}
}
func TestAutopilotRejectsWorkerSymlink(t *testing.T){
 s:=testStore(t);path:=filepath.Join(filepath.Dir(s.Path),"autopilot-worker.lock")
 target:=filepath.Join(t.TempDir(),"do-not-touch");if err:=os.WriteFile(target,[]byte("keep"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(target,path);err!=nil{t.Skip("symlinks unavailable")}
 e:=&blockingEngine{entered:make(chan struct{})}
 if r,err:=autopilot.Start(s,e,func(context.Context,string)(io.Closer,error){return nopLease{},nil});err==nil{r.Close();t.Fatal("worker followed symlink")}
 data,err:=os.ReadFile(target);if err!=nil||string(data)!="keep"{t.Fatal("symlink target changed")}
}
func TestAutopilotFoundationHorizonAndBounds(t *testing.T){
 f:=autopilot.Foundation{StoryCompass:"Story",Characters:[]autopilot.Character{{ID:"m",Name:"M",InitialState:"alive"}},POV:"m",Arcs:[]autopilot.Arc{{Title:"A",Summary:"start",FirstChapter:1,LastChapter:2}}}
 if f.Validate(2)!=nil||!f.Covers(2)||f.Covers(3){t.Fatal("bad coverage")}
 f.Arcs=append(f.Arcs,autopilot.Arc{Title:"gap",Summary:"bad",FirstChapter:4,LastChapter:5})
 if f.Validate(5)==nil{t.Fatal("gap accepted")}
 in:=input();in.BookTargetChapter=100;if in.Validate()!=nil||in.PlanningTarget()!=100{t.Fatal("batch truncated horizon")}
 in.BookTargetChapter=1;if in.Validate()==nil{t.Fatal("horizon before batch")}
}
func TestAutopilotControlSurvivesRecovery(t *testing.T){
 for _,action:=range []string{"pause","stop"}{t.Run(action,func(t *testing.T){
  s:=testStore(t);ctx:=t.Context();j,err:=s.Enqueue(ctx,"p","start",input());if err!=nil{t.Fatal(err)}
  if _,err=s.Next(ctx);err!=nil{t.Fatal(err)};if _,err=s.Control(ctx,j.ID,action);err!=nil{t.Fatal(err)}
  if err=s.Recover(ctx);err!=nil{t.Fatal(err)};got,err:=s.Get(ctx,j.ID);if err!=nil{t.Fatal(err)}
  target:=autopilot.Paused;if action=="stop"{target=autopilot.Cancelled};if got.State!=target||got.Control!=""{t.Fatal(got)}
 })}
}
// A small deterministic queue sequence, not a 300-chapter capacity benchmark.
func TestAutopilotBoundedRetryEventuallyFails(t *testing.T){
 s:=testStore(t);in:=input();in.MaxRetries=1;j,err:=s.Enqueue(t.Context(),"p","start",in);if err!=nil{t.Fatal(err)}
 before,err:=s.Next(t.Context());if err!=nil{t.Fatal(err)}
 after,err:=s.Finish(t.Context(),before,before,autopilot.Retry("TEMPORARY"));if err!=nil||after.State!=autopilot.Retrying||after.Retries!=1{t.Fatal(after,err)}
 if _,err=s.Next(t.Context());!errors.Is(err,autopilot.ErrNotFound){t.Fatal("backoff ignored",err)}
 time.Sleep(time.Until(after.NextRun)+5*time.Millisecond)
 before,err=s.Next(t.Context());if err!=nil{t.Fatal(err)}
 after,err=s.Finish(t.Context(),before,before,autopilot.Retry("TEMPORARY"));if err!=nil||after.State!=autopilot.Failed{t.Fatal("retry unbounded",j.ID,after,err)}
}
