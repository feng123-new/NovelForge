#!/usr/bin/env python3
"""Apply exact-base Phase 9 closure changes to an isolated product worktree."""
from pathlib import Path
import json, shutil, subprocess, sys
root=Path(sys.argv[1]).resolve()
base='1a96ab344a8d337af38da51bc080810a63d0a72d'
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()==base
changed=set()
def edit(path, old, new, count=1):
    p=root/path; s=p.read_text(); assert s.count(old)==count,(path,old[:120],s.count(old)); p.write_text(s.replace(old,new)); changed.add(path)
def append(path,text):
    p=root/path;p.write_text(p.read_text()+text);changed.add(path)
# Durable control: monotonic STOP, compare-and-set review, and fresh worker claims.
p='internal/autopilot/model.go'
edit(p,'type Input struct {','type Input struct {\n\tBookTargetChapter int `json:"book_target_chapter,omitempty"`')
edit(p,'func (in Input) Validate() error {','func (in Input) Validate() error {\n\tif in.BookTargetChapter != 0 && (in.BookTargetChapter < in.TargetChapter || in.BookTargetChapter > 1000) { return errors.New("invalid book horizon") }')
edit(p,'type Job struct {','type Job struct {\n\tClaimRevision int `json:"claim_revision,omitempty"`\n\tAuthorityFingerprint string `json:"authority_fingerprint,omitempty"`')
edit(p,'"next_run": j.NextRun}', '"next_run": j.NextRun, "review_candidate_id": j.ReviewCandidateID}')
edit(p,'func (f Foundation) Validate(target int) error {','func (f Foundation) Validate(target int) error {\n\traw, err := json.Marshal(f)\n\tif err != nil || len(raw) > 48*1024 || target < 1 || target > 1000 { return errors.New("foundation exceeds bounded planning budget") }')
edit(p,'c.Name == "" || c.InitialState', 'c.Name == "" || len(c.Name)>256 || c.InitialState')
edit(p,'\tfor _, a := range f.Arcs {','\tnext := 1\n\tfor _, a := range f.Arcs {\n\t\tif a.FirstChapter != next || len(a.Title)>512 { return errors.New("arcs must be ordered and contiguous") }\n\t\tnext = a.LastChapter+1')
append(p,'\n// PlanningTarget is the book horizon, not the finite execution batch.\nfunc (in Input) PlanningTarget() int { if in.BookTargetChapter>0 { return in.BookTargetChapter }; return in.TargetChapter }\nfunc (f Foundation) Covers(chapter int) bool { return len(f.Arcs)>0 && f.Arcs[len(f.Arcs)-1].LastChapter>=chapter }\n\n// Approval binds the explicit human decision to the detail that was displayed.\ntype Approval struct { Revision int `json:"expected_revision,omitempty"`; CandidateID string `json:"review_candidate_id,omitempty"` }\n')
p='internal/autopilot/store.go'
edit(p,'\tnow := time.Now().UTC()\n\tj.UpdatedAt = now','\tif loadErr == nil {\n\t\ta, _ := json.Marshal(old); b, _ := json.Marshal(j)\n\t\tif string(a)==string(b) { return old,nil } // no-op replay must not invalidate approvals or append events\n\t}\n\tnow := time.Now().UTC()\n\tj.UpdatedAt = now')
edit(p,'func (s Store) Control(ctx context.Context, id, action string) (Job, error) {','func (s Store) Control(ctx context.Context, id, action string) (Job, error) {\n\treturn s.ControlApproved(ctx,id,action,Approval{})\n}\nfunc (s Store) ControlApproved(ctx context.Context, id, action string, approval Approval) (Job, error) {')
edit(p,'\t\t\tif j.State == Running {\n\t\t\t\tj.Control = action','\t\t\tif j.State == Running {\n\t\t\t\tif j.Control == "stop" && action == "pause" { return j, ErrConflict }\n\t\t\t\tj.Control = action')
edit(p,'\t\t\tif j.ErrorCode == "REVIEW_REQUIRED" {\n\t\t\t\tj.ReviewApproved = true','\t\t\tif j.ErrorCode == "REVIEW_REQUIRED" {\n\t\t\t\tif approval.Revision != j.Revision || approval.CandidateID == "" || approval.CandidateID != j.ReviewCandidateID { return j, ErrConflict }\n\t\t\t\tj.ReviewApproved = true')
edit(p,'\t\tj.State = Running\n\t\treturn j, nil','\t\tif j.NextRun.After(time.Now().UTC()) { return j, ErrConflict }\n\t\tj.State = Running\n\t\tj.ClaimRevision = j.Revision + 1\n\t\treturn j, nil')
edit(p,'current.State != Running || current.Stage != before.Stage || current.Chapter != before.Chapter','current.State != Running || before.State != Running || current.ClaimRevision != before.ClaimRevision || current.Stage != before.Stage || current.Chapter != before.Chapter')
edit(p,'\t\tif stepErr == nil {\n\t\t\tafter.ID', '\t\tif stepErr == nil && after.State == Running && after.Stage == before.Stage && after.Chapter == before.Chapter && current.Control == "" { stepErr = Stop("AUTOPILOT_NO_PROGRESS") }\n\t\tif stepErr == nil {\n\t\t\tafter.ID')
edit(p,'\t\t\tafter.Input = current.Input','\t\t\tafter.ClaimRevision = current.ClaimRevision\n\t\t\tafter.Input = current.Input')
append(p,'\n// Unfinished queries the unique live slot directly, not a truncated history page.\nfunc (s Store) Unfinished(ctx context.Context, project string) (bool,error) {\n db,err:=migrate.Open(s.Path,5*time.Second); if err!=nil{return false,err}; defer db.Close()\n var n int; err=db.QueryRowContext(ctx,`SELECT count(*) FROM autopilot_jobs WHERE project_id=? AND state NOT IN (\'completed\',\'cancelled\')`,project).Scan(&n); return n>0,err\n}\n')
p='internal/autopilot/runner.go'
edit(p,'\t"io"','\t"io"\n\t"os"')
edit(p,'\tlock := flock.New(filepath.Join(filepath.Dir(store.Path), "autopilot-worker.lock"), flock.SetPermissions(0600))','\tlockPath := filepath.Join(filepath.Dir(store.Path), "autopilot-worker.lock")\n\tif info, err := os.Lstat(lockPath); err == nil { if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink!=0 { return nil,errors.New("unsafe worker lock") } } else if !errors.Is(err,os.ErrNotExist) { return nil,err }\n\tlock := flock.New(lockPath, flock.SetPermissions(0600))')
# A resumed rewrite must use precisely the original attempt and payload.
p='internal/qualitygate/coordinator.go'
edit(p,'\tresult, err := c.Writer.Write(ctx, WriterRequest{ProjectID: projectID, Chapter: chapter, TransactionID: tx.ID, Attempt: tx.Attempt, Plan: plan})','\trequest := WriterRequest{ProjectID: projectID, Chapter: chapter, TransactionID: tx.ID, Attempt: tx.Attempt, Plan: plan}\n\tif tx.Attempt>0 {\n\t\tcandidates, readErr := c.Store.Candidates(ctx,tx.ID); if readErr!=nil { return Snapshot{},readErr }\n\t\tvar previous *Candidate\n\t\tfor i:=range candidates { if candidates[i].Attempt<tx.Attempt && (previous==nil || candidates[i].Attempt>previous.Attempt) { previous=&candidates[i] } }\n\t\tif previous==nil { return Snapshot{},errors.New("rewrite source candidate is missing") }\n\t\trequest.PreviousDraft=previous.Text\n\t\trequest.Feedback=[]string{}\n\t\tif continuity,e:=c.Store.Continuity(ctx,tx.ID,previous.ID);e==nil { for _,issue:=range continuity.Issues { request.Feedback=append(request.Feedback,issue.IssueCode+": "+issue.SuggestedAction) } }\n\t\tif review,e:=c.Store.Editor(ctx,tx.ID,previous.ID);e==nil { request.Feedback=append(request.Feedback,review.Weaknesses...) }\n\t}\n\tresult, err := c.Writer.Write(ctx, request)')
# API input / review, finite-batch foundation horizon and fresh project guards.
p='internal/server/autopilot.go'
edit(p,'\t\twriteJSON(w, 200, map[string]any{"jobs": views,', '\t\tnextChapter, nextErr := s.projects.AutopilotNextChapter(r.Context(),id)\n\t\tif nextErr!=nil { writeFailure(w,r,*projectFailure(nextErr)); return }\n\t\twriteJSON(w, 200, map[string]any{"next_chapter": nextChapter, "jobs": views,')
edit(p,'options.StartChapter = p.CompletedChapters + 1','options.StartChapter, err = s.projects.AutopilotNextChapter(r.Context(),id)\n\t\t\t\tif err!=nil { f:=projectFailure(err); return f.Status,nil,f }')
edit(p,'in := autopilot.Input{FoundationID:', 'in := autopilot.Input{BookTargetChapter: max(p.TotalChapters, options.TargetChapter), FoundationID:')
edit(p,'\ttext := ""','\ttext := ""\n\tdisplayedCandidateID := ""')
edit(p,'\t\t\ttext = c.Text','\t\t\ttext = c.Text\n\t\t\tdisplayedCandidateID = c.ID')
edit(p,'"candidate_text": text, "quality": snap','"candidate_text": text, "candidate_id": displayedCandidateID, "quality": snap')
edit(p,'\t\tvar empty struct{}\n\t\tif f := decodeJSONBody(body, &empty, true); f != nil {','\t\tvar approval autopilot.Approval\n\t\tif f := decodeJSONBody(body, &approval, true); f != nil {')
edit(p,'s.jobs.Control(r.Context(), j.ID, action)','s.jobs.ControlApproved(r.Context(), j.ID, action, approval)')
old='''			jobs, err := s.jobs.List(r.Context(), id, 100, 0)
			if err != nil {
				writeFailure(w, r, *jobFailure(err))
				return
			}
			for _, job := range jobs {
				if !job.Terminal() {
					writeFailure(w, r, apiFailure{Status: 409, Code: "PROJECT_JOB_UNFINISHED", Message: "stop the unfinished job before archiving or deleting this project"})
					return
				}
			}'''
new='''			unfinished, err := s.jobs.Unfinished(r.Context(),id)
			if err!=nil { writeFailure(w,r,*jobFailure(err)); return }
			if unfinished { writeFailure(w,r,apiFailure{Status:409,Code:"PROJECT_JOB_UNFINISHED",Message:"stop the unfinished job before archiving or deleting this project"}); return }'''
edit(p,old,new)
# Recheck after obtaining the OS lease: START/RESUME may have won the race.
edit(p,'\t\tdefer lease.Close()\n\t\tnext.ServeHTTP(w, r)','\t\tdefer lease.Close()\n\t\tactive, err = s.jobs.Active(r.Context(),id)\n\t\tif destructive { active,err=s.jobs.Unfinished(r.Context(),id) }\n\t\tif err!=nil { writeFailure(w,r,*jobFailure(err));return }\n\t\tif active { writeFailure(w,r,apiFailure{Status:409,Code:"PROJECT_AUTOPILOT_BUSY",Message:"project task changed while acquiring the write lock"});return }\n\t\tnext.ServeHTTP(w, r)')
# Engine: bind planning POV, freeze authority snapshot, prove Final completion.
p='internal/server/autopilot_engine.go'
edit(p,'role != "architect" && role != "writer"','role != "architect" && role != "planner" && role != "writer"')
edit(p,'\t\tif loadErr == nil {\n\t\t\tj.Foundation = saved','\t\tif loadErr == nil {\n\t\t\tif saved.Validate(1000)!=nil || !saved.Covers(j.Input.PlanningTarget()) { return j,autopilot.Stop("FOUNDATION_HORIZON_MISMATCH") }\n\t\t\tj.Foundation = saved')
edit(p,'data, err := call("architect", "foundation", j.Input)','foundationInput:=j.Input\n\t\tfoundationInput.TargetChapter=j.Input.PlanningTarget()\n\t\tdata, err := call("architect", "foundation", foundationInput)')
edit(p,'if f.Validate(j.Input.TargetChapter) != nil {','if f.Validate(j.Input.PlanningTarget()) != nil || !f.Covers(j.Input.PlanningTarget()) {')
edit(p,'\t\tj.PlanningContext = data','\t\tfingerprint,err:=e.s.projects.AutopilotFingerprint(ctx,j.ProjectID,j.Chapter)\n\t\tif err!=nil{return j,autopilot.Stop("AUTHORITY_SNAPSHOT_FAILED")}\n\t\tj.AuthorityFingerprint=fingerprint\n\t\tj.PlanningContext = data')
edit(p,'\tcase "plan":\n\t\tdata, err := call(', '\tcase "plan":\n\t\tif j.Foundation==nil || len(j.PlanningContext)==0 { return j,autopilot.Stop("PLANNING_CONTEXT_MISSING") }\n\t\tif err:=e.checkAuthority(ctx,j);err!=nil{return j,err}\n\t\tdata, err := call(')
edit(p,'"target_chapter": j.Input.TargetChapter, "language": j.Input.Language,','"target_chapter": j.Input.PlanningTarget(), "batch_target_chapter": j.Input.TargetChapter, "planning_pov": j.Foundation.POV, "style": j.Input.Style, "words_per_chapter": j.Input.WordsPerChapter, "language": j.Input.Language,')
edit(p,'\t\tif !known {','\t\tif !known || p.POV != j.Foundation.POV {')
edit(p,'autopilot.Stop("PLAN_POV_UNKNOWN")','autopilot.Stop("PLAN_POV_SCOPE_MISMATCH")')
marker='''		if errors.Is(err, qualitygate.ErrNotFound) {
			if err = e.checkBoundary'''
replacement='''		// Partial generated Final commits must converge before any takeover or stale-input check.
		partial := err==nil && (snapshot.Transaction.State==qualitygate.StateTruthCommitPending || snapshot.Transaction.State==qualitygate.StateCheckpointPending)
		if !partial {
			complete,proofErr:=e.s.projects.AutopilotFinalComplete(ctx,j.ProjectID,j.Chapter)
			if proofErr!=nil{return j,autopilot.Stop("FINAL_PROOF_UNAVAILABLE")}
			if complete { return e.advance(ctx,j) } // explicit Human Final may finish a paused chapter
			if authorityErr:=e.checkAuthority(ctx,j);authorityErr!=nil{return j,authorityErr}
		}
		if errors.Is(err, qualitygate.ErrNotFound) {
			if err = e.checkBoundary'''
edit(p,marker,replacement)
edit(p,'\t\t\tcase qualitygate.StateRewritePending:\n\t\t\t\tsnapshot, err = quality.Rewrite', '\t\t\tcase qualitygate.StateRewritePending:\n\t\t\t\tif snapshot.Transaction.Attempt>=j.Input.MaxRewrites{return j,autopilot.Stop("REWRITE_BUDGET_EXHAUSTED")}\n\t\t\t\tsnapshot, err = quality.Rewrite')
edit(p,'if len(snapshot.Candidates) == 0 {','if len(snapshot.Candidates) == 0 || snapshot.Candidates[len(snapshot.Candidates)-1].Attempt < snapshot.Transaction.Attempt {')
edit(p,'\tif j.Chapter > 1 {\n\t\tactive, err := versions.ActiveFinal','\tif j.Chapter > 1 {\n\t\tcomplete,proofErr:=e.s.projects.AutopilotFinalComplete(ctx,j.ProjectID,j.Chapter-1)\n\t\tif proofErr!=nil || !complete{return autopilot.Stop("PRIOR_FINAL_CHECKPOINT_REQUIRED")}\n\t\tactive, err := versions.ActiveFinal')
edit(p,'func (e chapterJobEngine) advance(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {','func (e chapterJobEngine) advance(ctx context.Context, j autopilot.Job) (autopilot.Job, error) {\n\tcomplete,proofErr:=e.s.projects.AutopilotFinalComplete(ctx,j.ProjectID,j.Chapter)\n\tif proofErr!=nil || !complete{return j,autopilot.Stop("FINAL_CHECKPOINT_MISSING")}')
edit(p,'\tj.PlanningContext = nil','\tj.PlanningContext = nil\n\tj.AuthorityFingerprint = ""')
append(p,'\n// Conservatively reject stale plans/reviews after any authoritative edit.\n// No prompt, fact or secret is exposed by this digest. A completed explicit\n// Human Final may take over the chapter; it is verified before this guard.\nfunc (e chapterJobEngine) checkAuthority(ctx context.Context,j autopilot.Job) error {\n if j.AuthorityFingerprint=="" { return autopilot.Stop("CONTEXT_BASELINE_REQUIRED") }\n fingerprint,err:=e.s.projects.AutopilotFingerprint(ctx,j.ProjectID,j.Chapter)\n if err!=nil{return autopilot.Stop("AUTHORITY_SNAPSHOT_FAILED")}\n if fingerprint!=j.AuthorityFingerprint{return autopilot.Stop("CHAPTER_CONTEXT_CHANGED")}\n return e.checkBoundary(ctx,j,false)\n}\n')
# The existing tests keep their original assertions, but now send explicit approval.
p='internal/autopilot/store_test.go'
edit(p,'\trunning.Stage = "plan"\n\tafter, err := s.Finish(ctx, j, running, nil)','\tbefore := running\n\trunning.Stage = "plan"\n\tafter, err := s.Finish(ctx, before, running, nil)')
p='internal/server/autopilot_test.go'
edit(p,'"approve1", `{}`','"approve1", approvalBody(j)')
edit(p,'\tawaitJob(t, s, id, autopilot.Paused, 2)','\tj = awaitJob(t, s, id, autopilot.Paused, 2)')
edit(p,'"approve2", `{}`','"approve2", approvalBody(j)')
append(p,'\nfunc approvalBody(j autopilot.Job) string { raw,_:=json.Marshal(autopilot.Approval{Revision:j.Revision,CandidateID:j.ReviewCandidateID});return string(raw) }\n')
# API contract and UI must travel together with server-side candidate CAS.
p='web/src/lib/autopilot.ts'
edit(p,'  revision: number;','  revision: number;\n  review_candidate_id?: string;')
edit(p,'offset: number }','offset: number; next_chapter?: number }')
edit(p,'candidate_text: string; quality: unknown','candidate_text: string; candidate_id: string; quality: unknown')
append(p,"\nexport interface AutopilotApproval { expected_revision?: number; review_candidate_id?: string }\n")
p='web/src/lib/api.ts'
edit(p,'AutopilotDetail, AutopilotJob }','AutopilotDetail, AutopilotJob, AutopilotApproval }')
edit(p,"action: 'pause' | 'stop' | 'resume'): Promise<{ job: AutopilotJob }> { return this.write(`/projects/${encodeURIComponent(id)}/autopilot/${encodeURIComponent(job)}/${action}`, 'POST', {}); }", "action: 'pause' | 'stop' | 'resume', approval: AutopilotApproval = {}): Promise<{ job: AutopilotJob }> { return this.write(`/projects/${encodeURIComponent(id)}/autopilot/${encodeURIComponent(job)}/${action}`, 'POST', approval); }")
p='web/src/pages/Autopilot.svelte'
edit(p,'  await refresh();\n }','  await refresh();\n  if (page?.next_chapter) startChapter = page.next_chapter;\n }')
edit(p,"  try { await api.controlAutopilot(projectID, id, action); detail = null; await refresh(); }", "  try {\n   const job = page?.jobs.find((item) => item.id === id);\n   const approval = action === 'resume' && job?.error_code === 'REVIEW_REQUIRED' ? { expected_revision: detail?.job.revision, review_candidate_id: detail?.candidate_id } : {};\n   if (action === 'resume' && job?.error_code === 'REVIEW_REQUIRED' && (!detail || detail.job.id !== id || !detail.candidate_id)) throw new Error('请先查看当前候选稿');\n   await api.controlAutopilot(projectID, id, action, approval); detail = null; await refresh();\n  }")
edit(p,'disabled={pending || !job.actions.resume}', "disabled={pending || !job.actions.resume || (job.error_code === 'REVIEW_REQUIRED' && (!detail || detail.job.id !== job.id || detail.job.revision !== job.revision || detail.candidate_id !== job.review_candidate_id))}")
edit(p,'  try { detail = await api.autopilotDetail(projectID, id); }','  const project = projectID, generation = epoch;\n  try { const result = await api.autopilotDetail(project, id); if (!disposed && generation === epoch && project === projectID) detail = result; }')
# Additive OpenAPI schema; route count is unchanged.
p='internal/server/openapi_phase9.json'
f=root/p; spec=json.loads(f.read_text());schemas=spec.setdefault('components',{}).setdefault('schemas',{})
schemas['AutopilotApproval']={'type':'object','additionalProperties':False,'properties':{'expected_revision':{'type':'integer','minimum':1},'review_candidate_id':{'type':'string','minLength':1}},'description':'For REVIEW_REQUIRED both fields are mandatory and must match the inspected task revision and selected candidate. Stale approvals return 409.'}
resume=spec['paths']['/api/projects/{id}/autopilot/{job}/resume']['post']
resume['requestBody']={'required':False,'content':{'application/json':{'schema':{'$ref':'#/components/schemas/AutopilotApproval'}}}}
for name,sc in schemas.items():
    props=sc.get('properties',{})
    if 'revision' in props and 'state' in props: props['review_candidate_id']={'type':'string'}
    if 'candidate_text' in props: props['candidate_id']={'type':'string'}
    if 'worker_available' in props and 'jobs' in props: props['next_chapter']={'type':'integer','minimum':1}
f.write_text(json.dumps(spec,ensure_ascii=False,indent=2)+'\n');changed.add(p)
# Preserve all existing production/tests except the exact changes above.
files=Path(__file__).parent/'files'
if files.exists():
    for source in files.rglob('*'):
        if source.is_file():
            rel=source.relative_to(files);dest=root/rel;assert not dest.exists(),str(rel);dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(source,dest);changed.add(str(rel))
for path in sorted(changed):
    if path.endswith('.go'):subprocess.run(['gofmt','-w',str(root/path)],check=True)
subprocess.run(['git','add','--']+sorted(changed),cwd=root,check=True)
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
print('CLOSURE_PATHS='+json.dumps(sorted(changed)))
