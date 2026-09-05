package autopilot

import (
 "context"
 "errors"
 "io"
 "path/filepath"
 "sync"
 "sync/atomic"
 "time"

 "github.com/gofrs/flock"
)

// Runner is deliberately a single local worker per workspace. The OS lease,
// unlike an expiring timestamp alone, cannot admit a second live writer after
// a stalled heartbeat. A killed process releases it automatically.
type Runner struct {
 Store *Store
 Engine Engine
 Acquire func(context.Context,string)(io.Closer,error)
 lease *flock.Flock
 cancel context.CancelFunc
 done chan struct{}
 wake chan struct{}
 once sync.Once
 ready atomic.Bool
}
func Start(store *Store,engine Engine,acquire func(context.Context,string)(io.Closer,error))(*Runner,error){
 if store==nil || engine==nil || acquire==nil{return nil,errors.New("autopilot dependencies required")}
 lock:=flock.New(filepath.Join(filepath.Dir(store.Path),"autopilot-worker.lock"),flock.SetPermissions(0600))
 ok,err:=lock.TryLock();if err!=nil || !ok{lock.Close();return nil,errors.New("autopilot workspace already has a worker")}
 if err=store.Recover(context.Background());err!=nil{lock.Close();return nil,err}
 ctx,cancel:=context.WithCancel(context.Background())
 r:=&Runner{Store:store,Engine:engine,Acquire:acquire,lease:lock,cancel:cancel,done:make(chan struct{}),wake:make(chan struct{},1)}
 r.ready.Store(true);go r.loop(ctx);return r,nil
}
func (r *Runner) Ready()bool{return r!=nil&&r.ready.Load()}
func (r *Runner) Wake(){if r!=nil{select{case r.wake<-struct{}{}:default:}}}
func (r *Runner) Close()error{if r==nil{return nil};r.once.Do(func(){r.ready.Store(false);r.cancel();<-r.done});return nil}
func (r *Runner) loop(ctx context.Context){
 defer close(r.done);defer r.lease.Close();defer r.ready.Store(false)
 timer:=time.NewTicker(300*time.Millisecond);defer timer.Stop()
 for {
  if ctx.Err()!=nil{return}
  job,err:=r.Store.Next(ctx)
  if err!=nil{select{case<-ctx.Done():return;case<-r.wake:case<-timer.C:};continue}
  after,stepErr:=r.step(ctx,job)
  finishCtx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
  _,err=r.Store.Finish(finishCtx,job,after,stepErr);cancel()
  // If a checkpoint cannot be saved, stop the worker. Leaving 'running'
  // durable is safer than making another call with uncertain progress.
  if err!=nil{return}
 }
}
func(r *Runner) step(ctx context.Context,j Job)(next Job,err error){
 next=j
 defer func(){if recover()!=nil{next=j;err=Stop("AUTOPILOT_PANIC")}}()
 lease,err:=r.Acquire(ctx,j.ProjectID);if err!=nil{return j,Retry("PROJECT_BUSY")};defer lease.Close()
 callCtx,cancel:=context.WithTimeout(ctx,10*time.Minute);defer cancel()
 // Controls are durable requests and take effect between bounded steps.
 // We do not report a job paused/cancelled while it can still write.
 current,err:=r.Store.Get(callCtx,j.ID);if err!=nil{return j,err}
 if current.Control!=""{return j,nil}
 next,err=r.Engine.Step(callCtx,j)
 if ctx.Err()!=nil{return j,context.Canceled}
 if errors.Is(err,context.DeadlineExceeded){return j,Retry("STEP_TIMEOUT")}
 return next,err
}
