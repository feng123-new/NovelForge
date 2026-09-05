// Package autopilot owns durable orchestration, not chapter authority.
package autopilot

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "bytes"
 "strings"
 "time"

 "github.com/voocel/ainovel-cli/internal/qualitygate"
)

var ErrConflict = errors.New("autopilot state conflict")
var ErrNotFound = errors.New("autopilot job not found")

const (
 Pending = "pending"
 Running = "running"
 Paused = "paused"
 Retrying = "retrying"
 Failed = "failed"
 Completed = "completed"
 Cancelled = "cancelled"
)

type Input struct {
 FoundationID string `json:"foundation_id"`
 Idea string `json:"idea"`
 Style string `json:"style"`
 Language string `json:"language"`
 WordsPerChapter int `json:"words_per_chapter"`
 StartChapter int `json:"start_chapter"`
 TargetChapter int `json:"target_chapter"`
 ReviewEvery int `json:"review_every"`
 MaxRewrites int `json:"max_rewrites"`
 MaxRetries int `json:"max_retries"`
}

func (in Input) Validate() error {
 if strings.TrimSpace(in.Idea)=="" || len(in.Idea)>40000 || len(in.Style)>16000 || len(in.Language)>80 || in.StartChapter<1 || in.TargetChapter<in.StartChapter || in.TargetChapter>1000 || in.ReviewEvery<0 || in.ReviewEvery>100 || in.MaxRewrites<0 || in.MaxRewrites>5 || in.MaxRetries<0 || in.MaxRetries>5 || in.WordsPerChapter<0 || in.WordsPerChapter>20000 { return errors.New("invalid bounded autopilot input") }
 return nil
}

type Character struct { ID string `json:"id"`; Name string `json:"name"`; InitialState string `json:"initial_state"` }
type Arc struct { Title string `json:"title"`; Summary string `json:"summary"`; FirstChapter int `json:"first_chapter"`; LastChapter int `json:"last_chapter"` }
// Foundation is a planning artifact. It is never promoted to Final Truth.
type Foundation struct {
 StoryCompass string `json:"story_compass"`
 WorldRules []string `json:"world_rules"`
 Characters []Character `json:"characters"`
 POV string `json:"pov"`
 Arcs []Arc `json:"arcs"`
 Ending string `json:"ending"`
}
func (f Foundation) Validate(target int) error {
 if strings.TrimSpace(f.StoryCompass)=="" || len(f.StoryCompass)>12000 || len(f.Ending)>6000 || len(f.Characters)==0 || len(f.Characters)>100 || len(f.WorldRules)>100 || len(f.Arcs)==0 || len(f.Arcs)>100 { return errors.New("invalid foundation") }
 seen:=map[string]bool{}; found:=false
 for _,c:=range f.Characters { if c.ID=="" || len(c.ID)>128 || c.Name=="" || c.InitialState=="" || len(c.InitialState)>2000 || seen[c.ID] {return errors.New("invalid foundation character")}; seen[c.ID]=true; found=found||c.ID==f.POV }
 if !found {return errors.New("foundation POV must identify a character")}
 for _,rule:=range f.WorldRules {if rule=="" || len(rule)>2000{return errors.New("invalid world rule")}}
 for _,a:=range f.Arcs {if a.Title=="" || a.Summary=="" || len(a.Summary)>4000 || a.FirstChapter<1 || a.LastChapter<a.FirstChapter || a.LastChapter>target {return errors.New("invalid arc range")}}
 return nil
}

type Job struct {
 ID string `json:"id"`
 ProjectID string `json:"project_id"`
 State string `json:"state"`
 Stage string `json:"stage"`
 Chapter int `json:"chapter"`
 CompletedThrough int `json:"completed_through"`
 Control string `json:"control,omitempty"`
 ErrorCode string `json:"error_code,omitempty"`
 Retries int `json:"retries"`
 NextRun time.Time `json:"next_run"`
 Revision int `json:"revision"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
 Input Input `json:"input"`
 Foundation *Foundation `json:"foundation,omitempty"`
 Plan *qualitygate.ChapterPlan `json:"plan,omitempty"`
 PlanningContext json.RawMessage `json:"planning_context,omitempty"`
 ReviewApproved bool `json:"review_approved"`
}
// View omits model prompts, foundation secrets and prose from task listings/SSE.
func (j Job) View() map[string]any {
 return map[string]any{"id":j.ID,"project_id":j.ProjectID,"state":j.State,"stage":j.Stage,"chapter":j.Chapter,"completed_through":j.CompletedThrough,"target_chapter":j.Input.TargetChapter,"start_chapter":j.Input.StartChapter,"review_every":j.Input.ReviewEvery,"max_rewrites":j.Input.MaxRewrites,"max_retries":j.Input.MaxRetries,"retries":j.Retries,"control":j.Control,"error_code":j.ErrorCode,"revision":j.Revision,"created_at":j.CreatedAt,"updated_at":j.UpdatedAt,"next_run":j.NextRun}
}
func (j Job) Terminal() bool {return j.State==Completed || j.State==Cancelled}
func Identity(project,key string) string { h:=sha256.Sum256([]byte(project+"\x00"+key)); return "job_"+hex.EncodeToString(h[:16]) }
func (j Job) CallKey(stage string) string {return fmt.Sprintf("%s:%s:%d",j.ID,stage,j.Chapter)}

// Engine.Step must reuse the existing deterministic quality/version services.
// A successful return describes one durable boundary, not an entire book.
type Engine interface {Step(context.Context,Job)(Job,error)}
type Failure struct {Code string; Retryable bool}
func (e *Failure) Error() string {return e.Code}
func Stop(code string) error {return &Failure{Code:code}}
func Retry(code string) error {return &Failure{Code:code,Retryable:true}}

func Decode(data []byte, target any) error {
 if len(data)==0 || len(data)>256*1024 {return errors.New("invalid structured output size")}
 d:=json.NewDecoder(bytes.NewReader(data)); d.DisallowUnknownFields()
 if err:=d.Decode(target); err!=nil{return err}
 var trailing any
 if err:=d.Decode(&trailing); err!=io.EOF {return errors.New("structured output has trailing data")}
 return nil
}
