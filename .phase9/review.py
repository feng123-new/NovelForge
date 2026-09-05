#!/usr/bin/env python3
"""Bounded integration corrections, run after the fixed-base application."""
import pathlib,subprocess,sys
root=pathlib.Path(sys.argv[1]).resolve()
touched=set()
def edit(p,old,new,count=1):
 f=root/p;s=f.read_text();assert s.count(old)==count,(p,old,s.count(old));f.write_text(s.replace(old,new));touched.add(p)
p='internal/autopilot/model.go'
s=(root/p).read_text()
line=next(x for x in s.splitlines(True) if '`json:"review_approved"`' in x)
edit(p,line,line+'\tReviewCandidateID string `json:"review_candidate_id,omitempty"`\n')
p='internal/server/autopilot_engine.go'
edit(p,'if due && !j.ReviewApproved {','if due && (!j.ReviewApproved || j.ReviewCandidateID != snapshot.Transaction.FinalCandidateID) {')
edit(p,'j.ErrorCode = "REVIEW_REQUIRED"','j.ErrorCode = "REVIEW_REQUIRED"\n\t\t\t\tj.ReviewApproved = false\n\t\t\t\tj.ReviewCandidateID = snapshot.Transaction.FinalCandidateID')
old='snapshot, err = local.finalizeQualityPhase8(req, quality, j.ProjectID, j.Chapter, j.CallKey("final"))'
edit(p,old,old+'\n\t\t\tif err == nil && snapshot.Transaction.State != qualitygate.StateCompleted { return j, autopilot.Retry("FINALIZE_INCOMPLETE") }',2)
edit(p,'j.ReviewApproved = false\n\tj.ErrorCode = ""','j.ReviewApproved = false\n\tj.ReviewCandidateID = ""\n\tj.ErrorCode = ""')
p='internal/autopilot/runner.go'
edit(p,'if err != nil {\n\t\t\tselect {','if err != nil {\n\t\t\tif !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrConflict) && ctx.Err()==nil { return }\n\t\t\tselect {')
# Paused/failed projects can be edited but must explicitly stop their job
# before destructive lifecycle operations; otherwise the durable job is orphaned.
p='internal/server/autopilot.go'
old='active, err := s.jobs.Active(r.Context(), id)'
edit(p,old,'''destructive := r.Method==http.MethodDelete || strings.HasSuffix(r.URL.Path,"/archive")
        if destructive {
            jobs,err:=s.jobs.List(r.Context(),id,100,0)
            if err!=nil{writeFailure(w,r,*jobFailure(err));return}
            for _,job:=range jobs {if !job.Terminal(){writeFailure(w,r,apiFailure{Status:409,Code:"PROJECT_JOB_UNFINISHED",Message:"stop the unfinished job before archiving or deleting this project"});return}}
        }
        active, err := s.jobs.Active(r.Context(), id)''')
# Resume must not race an ongoing paused-project edit. Pausing/stopping a
# running step remains non-blocking: those only persist the control intent.
old='next, err := s.jobs.Control(r.Context(), j.ID, action)'
edit(p,old,'''if action=="resume" {
            projectState,err:=s.projects.Get(r.Context(),j.ProjectID)
            if err!=nil{f:=projectFailure(err);return f.Status,nil,f}
            if projectState.Archived{return 409,nil,&apiFailure{Status:409,Code:"PROJECT_ARCHIVED",Message:"restore the project before resuming"}}
            lease,err:=s.projects.AcquireExecution(r.Context(),j.ProjectID)
            if err!=nil{return 409,nil,&apiFailure{Status:409,Code:"PROJECT_BUSY",Message:"project is being edited"}}
            defer lease.Close()
        }
        next, err := s.jobs.Control(r.Context(), j.ID, action)''')
# Upgrade the existing two-chapter test to exercise real configuration and
# the HTTP provider adapter, replacing only the external paid endpoint.
p='internal/server/autopilot_test.go'
edit(p,'"net/http/httptest"','"net/http/httptest"\n "os"\n "path/filepath"')
old='cfg := Config{Workspace: workspace, QualityModel: model, AutopilotEnabled: true}'
edit(p,old,'''provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        if r.Header.Get("Authorization")!="Bearer fixture-key"{http.Error(w,"missing configured credential",401);return}
        var wire struct{Messages []struct{Role string `json:"role"`;Content string `json:"content"`} `json:"messages"`}
        if err:=json.NewDecoder(r.Body).Decode(&wire);err!=nil{http.Error(w,"invalid model request",400);return}
        var payload []byte
        for _,message:=range wire.Messages {if message.Role=="user"{payload=[]byte(message.Content)}}
        var fields map[string]json.RawMessage
        if err:=json.Unmarshal(payload,&fields);err!=nil{http.Error(w,"invalid user payload",400);return}
        operation:=""
        switch {
        case fields["idea"]!=nil:operation="architect:foundation"
        case fields["selected_context"]!=nil:operation="planner:chapter"
        case fields["chapter_plan"]!=nil:operation="writer:draft"
        default:
            var schema string;_ = json.Unmarshal(fields["schema"],&schema)
            if schema=="FactProposal"{operation="librarian:fact_proposal"};if schema=="EditorReview"{operation="editor:review"}
        }
        result,_,err:=model.Invoke(r.Context(),operation,payload)
        if err!=nil{http.Error(w,err.Error(),400);return}
        w.Header().Set("Content-Type","application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{"id":"fixture","object":"chat.completion","model":"fixture","choices":[]any{map[string]any{"index":0,"message":map[string]any{"role":"assistant","content":string(result)},"finish_reason":"stop"}},"usage":map[string]any{"prompt_tokens":10,"completion_tokens":10,"total_tokens":20}})
    }))
    defer provider.Close()
    configPath:=filepath.Join(t.TempDir(),"models.json")
    configJSON,_:=json.Marshal(map[string]any{"provider":"fixture","model":"fixture","context_window":100000,"providers":map[string]any{"fixture":map[string]any{"type":"openai","api":"chat","api_key":"fixture-key","base_url":provider.URL+"/v1"}}})
    if err:=os.WriteFile(configPath,configJSON,0600);err!=nil{t.Fatal(err)}
    cfg := Config{Workspace: workspace, QualityConfigEnabled:true,QualityConfigPath:configPath, AutopilotEnabled: true}''')
# Current guide no longer instructs users to update archived acceptance or
# advertises frozen Phase 9; historical archives themselves remain untouched.
p='docs/AUTOPILOT.md'
edit(p,'Write operations require a paused/stopped task and an available project lock.','Write operations require a paused/stopped task and an available project lock. Archiving/deleting requires an explicitly stopped or completed job, preventing orphaned tasks.')
edit(p,'it explicitly approves the current selected candidate,','it explicitly approves the current selected candidate by its persisted candidate ID (a changed selection pauses for a new approval),')
for p in sorted(touched):
 if p.endswith('.go'):subprocess.run(['gofmt','-w',str(root/p)],check=True)
subprocess.run(['git','add','--']+sorted(touched),cwd=root,check=True)
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
print('Bounded Final replay, candidate-bound approval, lifecycle exclusion and configured HTTP checks applied.')
