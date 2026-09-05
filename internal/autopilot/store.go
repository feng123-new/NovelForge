package autopilot

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "time"

 "github.com/voocel/ainovel-cli/internal/db/migrate"
)

func Migration() migrate.Migration {return migrate.Migration{Version:3,Name:"durable_autopilot_jobs",SQL:`
CREATE TABLE autopilot_jobs (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('pending','running','paused','retrying','failed','completed','cancelled')),
 next_run TEXT NOT NULL, payload_json TEXT NOT NULL CHECK(json_valid(payload_json)), created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX autopilot_one_live_project ON autopilot_jobs(project_id) WHERE state NOT IN ('completed','cancelled');
CREATE INDEX autopilot_runnable ON autopilot_jobs(state,next_run,created_at,id);
CREATE INDEX autopilot_project_jobs ON autopilot_jobs(project_id,created_at,id);
`}}

type Event struct {ID uint64; Project string; Time time.Time; Data map[string]any}
type Store struct {Path string; Emit func(Event)}
type queryer interface {ExecContext(context.Context,string,...any)(sql.Result,error); QueryRowContext(context.Context,string,...any)*sql.Row}
func load(ctx context.Context,q queryer,id string)(Job,error){
 var raw string; err:=q.QueryRowContext(ctx,"SELECT payload_json FROM autopilot_jobs WHERE id=?",id).Scan(&raw)
 if errors.Is(err,sql.ErrNoRows){return Job{},ErrNotFound}; if err!=nil{return Job{},err}
 var j Job; err=json.Unmarshal([]byte(raw),&j); return j,err
}
func (s Store) Get(ctx context.Context,id string)(Job,error){db,err:=migrate.Open(s.Path,5*time.Second); if err!=nil{return Job{},err};defer db.Close();return load(ctx,db,id)}
func (s Store) List(ctx context.Context,project string,limit,offset int)([]Job,error){
 if limit<1 || limit>100 || offset<0{return nil,errors.New("invalid page")}
 db,err:=migrate.Open(s.Path,5*time.Second);if err!=nil{return nil,err};defer db.Close()
 rows,err:=db.QueryContext(ctx,`SELECT payload_json FROM autopilot_jobs WHERE (?='' OR project_id=?) ORDER BY created_at DESC,id LIMIT ? OFFSET ?`,project,project,limit,offset);if err!=nil{return nil,err};defer rows.Close()
 out:=[]Job{};for rows.Next(){var raw string;if err=rows.Scan(&raw);err!=nil{return nil,err};var j Job;if err=json.Unmarshal([]byte(raw),&j);err!=nil{return nil,err};out=append(out,j)};return out,rows.Err()
}
func (s Store) Active(ctx context.Context,project string)(bool,error){
 db,err:=migrate.Open(s.Path,5*time.Second);if err!=nil{return false,err};defer db.Close()
 var n int;err=db.QueryRowContext(ctx,`SELECT count(*) FROM autopilot_jobs WHERE project_id=? AND state IN ('pending','running','retrying')`,project).Scan(&n);return n>0,err
}
// change serializes only short state transactions; no LLM call holds this lock.
// The task update and its replayable SSE record commit together.
func (s Store) change(ctx context.Context,id string,fn func(Job,error)(Job,error))(Job,error){
 db,err:=migrate.Open(s.Path,5*time.Second);if err!=nil{return Job{},err};defer db.Close()
 conn,err:=db.Conn(ctx);if err!=nil{return Job{},err};defer conn.Close()
 if _,err=conn.ExecContext(ctx,"BEGIN IMMEDIATE");err!=nil{return Job{},err}
 defer conn.ExecContext(context.Background(),"ROLLBACK")
 old,loadErr:=load(ctx,conn,id);j,err:=fn(old,loadErr);if err!=nil{return Job{},err}
 now:=time.Now().UTC();j.UpdatedAt=now;j.Revision=old.Revision+1
 raw,err:=json.Marshal(j);if err!=nil{return Job{},err}
 if loadErr==ErrNotFound {
  var n int;if err=conn.QueryRowContext(ctx,"SELECT count(*) FROM autopilot_jobs WHERE project_id=? AND state NOT IN ('completed','cancelled')",j.ProjectID).Scan(&n);err!=nil{return Job{},err};if n>0{return Job{},ErrConflict}
  _,err=conn.ExecContext(ctx,"INSERT INTO autopilot_jobs VALUES(?,?,?,?,?,?,?)",j.ID,j.ProjectID,j.State,j.NextRun.Format(time.RFC3339Nano),string(raw),j.CreatedAt.Format(time.RFC3339Nano),now.Format(time.RFC3339Nano))
 }else{_,err=conn.ExecContext(ctx,"UPDATE autopilot_jobs SET state=?,next_run=?,payload_json=?,updated_at=? WHERE id=?",j.State,j.NextRun.Format(time.RFC3339Nano),string(raw),now.Format(time.RFC3339Nano),id)}
 if err!=nil{return Job{},err}
 data:=j.View();payload,err:=json.Marshal(data);if err!=nil{return Job{},err}
 result,err:=conn.ExecContext(ctx,"INSERT INTO events(event_type,project_id,payload_json,created_at) VALUES(?,?,?,?)","autopilot.changed",j.ProjectID,payload,now.Format(time.RFC3339Nano));if err!=nil{return Job{},err}
 eventID,err:=result.LastInsertId();if err!=nil{return Job{},err}
 if _,err=conn.ExecContext(ctx,"COMMIT");err!=nil{return Job{},err}
 if s.Emit!=nil{s.Emit(Event{ID:uint64(eventID),Project:j.ProjectID,Time:now,Data:data})}
 return j,nil
}
func (s Store) Enqueue(ctx context.Context,project,key string,in Input)(Job,error){
 if err:=in.Validate();err!=nil{return Job{},err};if project=="" || key==""{return Job{},errors.New("identity required")}
 id:=Identity(project,key)
 return s.change(ctx,id,func(j Job,err error)(Job,error){
  if err==nil {a,_:=json.Marshal(j.Input);b,_:=json.Marshal(in);if string(a)!=string(b){return Job{},ErrConflict};return j,nil}
  if !errors.Is(err,ErrNotFound){return Job{},err}
  now:=time.Now().UTC();return Job{ID:id,ProjectID:project,State:Pending,Stage:"foundation",Chapter:in.StartChapter,CompletedThrough:in.StartChapter-1,CreatedAt:now,NextRun:now,Input:in},nil
 })
}
func (s Store) Control(ctx context.Context,id,action string)(Job,error){return s.change(ctx,id,func(j Job,err error)(Job,error){
 if err!=nil{return j,err}
 switch action {
 case "pause","stop":
  target:=Paused;if action=="stop"{target=Cancelled}
  if j.Terminal(){if j.State==target{return j,nil};return j,ErrConflict}
  if j.State==Running {j.Control=action}else{j.State=target;j.Control=""}
 case "resume":
  if j.State==Pending || j.State==Retrying{return j,nil}
  if j.State!=Paused && j.State!=Failed{return j,ErrConflict}
  if j.ErrorCode=="REVIEW_REQUIRED"{j.ReviewApproved=true}
  j.State=Pending;j.Control="";j.ErrorCode="";j.Retries=0;j.NextRun=time.Now().UTC()
 default:return j,errors.New("unsupported action")
 };return j,nil
})}
// Recover is called only after acquiring the process-wide workspace lock.
func (s Store) Recover(ctx context.Context)error{
 db,err:=migrate.Open(s.Path,5*time.Second);if err!=nil{return err}
 rows,err:=db.QueryContext(ctx,"SELECT id FROM autopilot_jobs WHERE state='running'");if err!=nil{db.Close();return err}
 var ids []string;for rows.Next(){var id string;if err=rows.Scan(&id);err!=nil{rows.Close();db.Close();return err};ids=append(ids,id)};err=rows.Err();rows.Close();db.Close();if err!=nil{return err}
 for _,id:=range ids {_,err=s.change(ctx,id,func(j Job,e error)(Job,error){if e!=nil{return j,e};if j.State!=Running{return j,ErrConflict};j.State=Pending;j.ErrorCode="RECOVERED_INTERRUPTION";if j.Control=="pause"{j.State=Paused};if j.Control=="stop"{j.State=Cancelled};j.Control="";return j,nil});if err!=nil{return err}};return nil
}
func (s Store) Next(ctx context.Context)(Job,error){
 db,err:=migrate.Open(s.Path,5*time.Second);if err!=nil{return Job{},err};defer db.Close()
 var id string;err=db.QueryRowContext(ctx,"SELECT id FROM autopilot_jobs WHERE state IN ('pending','retrying') AND next_run<=? ORDER BY created_at,id LIMIT 1",time.Now().UTC().Format(time.RFC3339Nano)).Scan(&id)
 if errors.Is(err,sql.ErrNoRows){return Job{},ErrNotFound};if err!=nil{return Job{},err}
 return s.change(ctx,id,func(j Job,e error)(Job,error){if e!=nil{return j,e};if j.State!=Pending&&j.State!=Retrying{return j,ErrConflict};j.State=Running;return j,nil})
}
func (s Store) Finish(ctx context.Context,before,after Job,stepErr error)(Job,error){return s.change(ctx,before.ID,func(current Job,err error)(Job,error){
 if err!=nil{return current,err};if current.State!=Running || current.Stage!=before.Stage || current.Chapter!=before.Chapter{return current,ErrConflict}
 if stepErr==nil {after.ID=current.ID;after.ProjectID=current.ProjectID;after.Input=current.Input;after.CreatedAt=current.CreatedAt;after.Control=current.Control;after.Retries=0;current=after;if current.State==Running{current.State=Pending};current.NextRun=time.Now().UTC()}else{
  current.ErrorCode="AUTOPILOT_STEP_FAILED";var f *Failure
  if errors.As(stepErr,&f){current.ErrorCode=f.Code}
  if errors.Is(stepErr,context.Canceled){current.State=Pending;current.ErrorCode="INTERRUPTED"}else if f!=nil&&f.Retryable&&current.Retries<current.Input.MaxRetries {current.Retries++;current.State=Retrying;current.NextRun=time.Now().UTC().Add(time.Duration(1<<current.Retries)*time.Second)}else{current.State=Failed}
 }
 if current.Control=="stop" {current.State=Cancelled;current.Control=""}else if current.Control=="pause" {if current.State!=Completed{current.State=Paused};current.Control=""}
 return current,nil
})}
func (s Store) String()string{return fmt.Sprintf("autopilot store")}
