#!/usr/bin/env python3
from pathlib import Path
import json,re,subprocess,sys
root=Path(sys.argv[1]);base='39e67336d5615066cf5180309475aa79ff468af5'
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()==base
touched=[]
def edit(p,old,new):
 f=root/p;s=f.read_text();assert s.count(old)==1,(p,old[:80],s.count(old));f.write_text(s.replace(old,new));touched.append(p)
p='internal/server/autopilot_engine.go'
edit(p,'\tswitch j.Stage {','''	// Before planning, do not rebrand an older unfinished quality transaction
	// with this job's new plan/fingerprint. Explicit completed human work can
	// take over even when this cursor was paused before draft generation.
	if j.Stage == "plan_context" || j.Stage == "plan" {
		complete, proofErr := e.s.projects.AutopilotFinalComplete(ctx,j.ProjectID,j.Chapter)
		if proofErr != nil { return j,autopilot.Stop("FINAL_PROOF_UNAVAILABLE") }
		if complete { return e.advance(ctx,j) }
		if _, snapshotErr := quality.Store.Snapshot(ctx,j.ProjectID,j.Chapter); snapshotErr == nil {
			return j,autopilot.Stop("EXISTING_DRAFT_REQUIRES_REVIEW")
		} else if !errors.Is(snapshotErr,qualitygate.ErrNotFound) { return j,autopilot.Stop("QUALITY_STATE_UNAVAILABLE") }
	}
	switch j.Stage {''')
edit(p,'func (e chapterJobEngine) advance(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {','''func (e chapterJobEngine) advance(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {
	if err := e.checkBoundary(ctx,j,false); err != nil { return j,err }''')
p='internal/project/autopilot_boundary.go'
edit(p,"AND s.final_version_id=a.version_id AND s.state='completed')`","AND s.final_version_id=a.version_id AND s.state='completed')\n AND NOT EXISTS (SELECT 1 FROM derived_state_rebuilds r WHERE r.project_id=a.project_id AND r.boundary_chapter<=a.chapter AND r.state!='completed')`")
p='internal/server/autopilot_closure_test.go'
edit(p,'\t"fmt"\n','')
edit(p,'\t"github.com/voocel/ainovel-cli/internal/autopilot"','\t"github.com/voocel/ainovel-cli/internal/autopilot"\n\t"github.com/voocel/ainovel-cli/internal/chapterversion"\n\t"bytes"')
edit(p,'\tfmt.Sprint(path) // parsed contract is tested in addition to the real HTTP flow','\tif !bytes.Contains(path,[]byte("#/components/schemas/AutopilotApproval")) { t.Fatal("resume schema not referenced") }')
with (root/p).open('a') as f:f.write('''
func TestAutopilotClosureForeignDraftNotReused(t *testing.T) {
 model:=&phase9Model{calls:map[string]int{}};s,id,foundation,_:=closureFixture(t,model)
 req:=httptest.NewRequest(http.MethodGet,"http://localhost/internal",nil).WithContext(t.Context())
 q,cleanup,failure:=s.qualityCoordinator(req,id);if failure!=nil{t.Fatal(failure)}
 var plan qualitygate.ChapterPlan;if err:=json.Unmarshal(apiPlan(1),&plan);err!=nil{t.Fatal(err)}
 snapshot,err:=q.Generate(t.Context(),id,1,plan);cleanup();if err!=nil{t.Fatal(err)}
 if len(snapshot.Candidates)!=1{t.Fatal("missing original draft")}
 closureEnqueue(t,s,id,foundation,"new-job-over-old-draft",1,1)
 if _,err=closureStep(t,s);err!=nil{t.Fatal(err)}
 stopped,err:=closureStep(t,s);var failureErr *autopilot.Failure
 if !errors.As(err,&failureErr)||failureErr.Code!="EXISTING_DRAFT_REQUIRES_REVIEW"||stopped.State!=autopilot.Failed{t.Fatal("old draft was adopted under new plan",stopped,err)}
 if model.calls["writer:draft"]!=1||model.calls["planner:chapter"]!=0{t.Fatal("old draft silently regenerated",model.calls)}
}
func TestAutopilotClosureHumanFinalResumesPlanning(t *testing.T) {
 for _,stage:=range []string{"plan_context","plan","generate","check"}{t.Run(stage,func(t *testing.T){
  model:=&phase9Model{calls:map[string]int{}};s,id,foundation,_:=closureFixture(t,model)
  job:=closureEnqueue(t,s,id,foundation,"human-takeover",1,1)
  for n:=0;n<6 && job.Stage!=stage;n++{var err error;job,err=closureStep(t,s);if err!=nil{t.Fatal(err)}}
  if job.Stage!=stage{t.Fatal("fixture did not reach stage",job.Stage)}
  if _,err:=s.jobs.Control(t.Context(),job.ID,"pause");err!=nil{t.Fatal(err)}
  lease,err:=s.projects.AcquireExecution(t.Context(),id);if err!=nil{t.Fatal(err)}
  request:=httptest.NewRequest(http.MethodGet,"http://localhost/internal",nil).WithContext(t.Context())
  coordinator,cleanup,failure:=s.chapterVersionCoordinator(request,id);if failure!=nil{lease.Close();t.Fatal(failure)}
  service:=&chapterversion.Service{Store:coordinator.Store}
  revision,err:=service.SaveHuman(t.Context(),1,"human-save","Mira entered the gate. She found a hand-written note.");if err!=nil{t.Fatal(err)}
  if _,err=coordinator.Check(t.Context(),1,"human-check",revision.ID);err!=nil{t.Fatal(err)}
  if _,err=service.Accept(t.Context(),1,"human-accept",revision.ID,"Author approved replacement");err!=nil{t.Fatal(err)}
  if _,err=coordinator.Finalize(t.Context(),1,"human-final",revision.ID);err!=nil{t.Fatal(err)}
  active,err:=coordinator.Store.ActiveFinal(t.Context(),1,true);if err!=nil||active==nil{t.Fatal("human final missing",err)}
  finalID:=active.ID;cleanup();lease.Close()
  callsBefore:=model.calls["writer:draft"]
  if _,err=s.jobs.Control(t.Context(),job.ID,"resume");err!=nil{t.Fatal(err)}
  done,err:=closureStep(t,s);if err!=nil||done.State!=autopilot.Completed||done.CompletedThrough!=1{t.Fatal("human-final takeover stuck",done,err)}
  versions,err:=s.projects.OpenChapterVersionStore(t.Context(),id);if err!=nil{t.Fatal(err)};defer versions.Close()
  active,err=versions.ActiveFinal(t.Context(),1,true)
  if err!=nil||active==nil||active.ID!=finalID||model.calls["writer:draft"]!=callsBefore{t.Fatal("accepted human prose was replaced",err)}
 })}
}
''')
p='docs/AUTOPILOT.md'
with (root/p).open('a') as f:f.write('''\nA newly started task encountering an existing unfinished chapter transaction stops with `EXISTING_DRAFT_REQUIRES_REVIEW`: resume the original paused/failed task, or explicitly review/finalize the retained content. It does not bind a newly generated plan to another task's old draft. Completed Human Finals can take over at planning or generation stages; a pending rebuild cannot satisfy Final completion. These are conservative recovery boundaries, not automatic re-planning or implicit acceptance.\n\nClosure build and the initial 18 named Go / 3 frontend tests passed in Actions `33965208527` on product commit `39e67336d5615066cf5180309475aa79ff468af5`. Follow-up takeover checks are recorded with their exact final product in the delivery PR; this is not full-suite or scale acceptance.\n''')
touched.append(p)
for p in set(touched):
 if p.endswith('.go'):subprocess.run(['gofmt','-w',str(root/p)],check=True)
subprocess.run(['git','add','--']+sorted(set(touched)),cwd=root,check=True)
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
# Check local Markdown file targets and case-fold conflicts without web/network.
paths=subprocess.check_output(['git','ls-files'],cwd=root,text=True).splitlines()
fold={};collisions=[]
for p in paths:
 k=p.casefold()
 if k in fold:collisions.append((fold[k],p))
 fold[k]=p
assert not collisions,collisions
broken=[]
for p in paths:
 if not p.endswith('.md'):continue
 for target in re.findall(r'\]\(([^\s)]+)(?:\s+[^)]*)?\)',(root/p).read_text()):
  if target.startswith(('#','http:','https:','mailto:','data:')):continue
  target=target.split('#',1)[0]
  if target and not (root/p).parent.joinpath(target).exists():broken.append((p,target))
print('STATIC_CLOSURE='+json.dumps({'casefold_collisions':collisions,'local_missing_targets':broken}))
assert not broken,broken
