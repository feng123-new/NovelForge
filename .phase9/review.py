#!/usr/bin/env python3
"""Bounded integration corrections, run after the fixed-base application."""
import pathlib,subprocess,sys
root=pathlib.Path(sys.argv[1]).resolve()
touched=set()
def edit(p,old,new,count=1):
 f=root/p;s=f.read_text();assert s.count(old)==count,(p,old,s.count(old));f.write_text(s.replace(old,new));touched.add(p)
# Once formatted, use exact source markers with gofmt-stable spacing.
p='internal/autopilot/model.go'
s=(root/p).read_text()
needle='ReviewApproved bool'
# gofmt aligns fields, so anchor through the complete field line.
line=next(x for x in s.splitlines(True) if '`json:"review_approved"`' in x)
edit(p,line,line+'\tReviewCandidateID string `json:"review_candidate_id,omitempty"`\n')
p='internal/server/autopilot_engine.go'
s=(root/p).read_text()
old='if due && !j.ReviewApproved {'
assert old in s
edit(p,old,'if due && (!j.ReviewApproved || j.ReviewCandidateID != snapshot.Transaction.FinalCandidateID) {')
edit(p,'j.ErrorCode = "REVIEW_REQUIRED"','j.ErrorCode = "REVIEW_REQUIRED"\n\t\t\t\tj.ReviewApproved = false\n\t\t\t\tj.ReviewCandidateID = snapshot.Transaction.FinalCandidateID')
# Both initial and resumed finalization must acknowledge Completed. The old
# bridge may return a pending snapshot with no Go error after storage faults.
old='snapshot, err = local.finalizeQualityPhase8(req, quality, j.ProjectID, j.Chapter, j.CallKey("final"))'
edit(p,old,old+'\n\t\t\tif err == nil && snapshot.Transaction.State != qualitygate.StateCompleted { return j, autopilot.Retry("FINALIZE_INCOMPLETE") }',2)
edit(p,'j.ReviewApproved = false\n\tj.ErrorCode = ""','j.ReviewApproved = false\n\tj.ReviewCandidateID = ""\n\tj.ErrorCode = ""')
# A persistent read failure stops the worker rather than advertising healthy
# execution while silently polling an unreadable queue forever.
p='internal/autopilot/runner.go'
edit(p,'if err != nil {\n\t\t\tselect {','if err != nil {\n\t\t\tif !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrConflict) && ctx.Err()==nil { return }\n\t\t\tselect {')
# Candidate approval is also checked by deterministic continuity/version
# guards; this ID binding is not a replacement for those checks.
for p in sorted(touched):subprocess.run(['gofmt','-w',str(root/p)],check=True)
subprocess.run(['git','add','--']+sorted(touched),cwd=root,check=True)
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
print('Bounded Final replay and candidate-bound approval corrections applied.')
