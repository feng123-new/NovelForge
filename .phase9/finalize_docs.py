#!/usr/bin/env python3
"""Documentation-only review after the exact product code passed checks."""
import json,pathlib,re,subprocess,os,posixpath,urllib.parse
TESTED='3cf18bb1ed8393f794e0b6753baaaee6f9fbac6e'
BRANCH='feat/phase-09-durable-autopilot'
root=pathlib.Path(os.environ['RUNNER_TEMP'])/'phase9-reviewed'
def git(*args):return subprocess.check_output(['git',*args],cwd=root,text=True).strip()
subprocess.run(['git','worktree','add','--detach',str(root),TESTED],check=True)
assert git('rev-parse','HEAD')==TESTED
changed=set()
def edit(path,old,new,count=1):
 p=root/path;s=p.read_text();assert s.count(old)==count,(path,old,s.count(old));p.write_text(s.replace(old,new));changed.add(path)
p='docs/ROADMAP.md';s=(root/p).read_text();a=s.index('## Current scope');b=s.index('## Phase 0')
intro='''## Current scope — 2026-09-05

Phase 1–8 delivery and the cleanup remain the baseline. Phase 9 has been explicitly reopened and implemented as a durable local Autopilot over the existing quality/version coordinators. Phase 10–13 remain paused; finishing this phase does not start them automatically.

[AUTOPILOT.md](AUTOPILOT.md) is the current Phase 9 behavior and verification boundary. [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md) records the earlier integration repair. Original reviews and acceptance evidence remain in [the archive](archive/README.md); they are not rewritten as new acceptance claims.

Verification is limited to the affected entry build, named short-flow tests, changed frontend type checking and asset build. No full Go/frontend/race suites, platform matrices, paid-model run or 300-chapter simulation is claimed. Actual run and exact source references are recorded in the delivery PR. Broader scale acceptance remains unverified.

'''
(root/p).write_text(s[:a]+intro+s[b:]);changed.add(p)
edit('docs/ROADMAP.md','New Novel stores a Foundation request; its worker remains unavailable and is outside this consolidation.','New Novel stores a Foundation request; Phase 9 now executes it after an explicit start from the Autopilot page.')
edit('docs/DEVELOPMENT.md','for Phase 1–8 maintenance. Phase 9–13 remain paused.','for Phase 1–9 maintenance. Phase 9 was explicitly reopened; Phase 10–13 remain paused.')
edit('docs/WEB.md','Workers remain unavailable. See [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md) for current scope and verification limits.','The normal CLI entry now enables the Phase 9 worker. Worker readiness and model configuration are distinct conditions. See [AUTOPILOT.md](AUTOPILOT.md) for current controls and verification limits.')
edit('docs/WEB.md','Projects, requests, events and all future job state remain authoritative on the local server.','Projects, requests, events and durable job state remain authoritative on the local server.')
edit('docs/WEB.md','The Foundation write stores a validated request and emits `foundation.requested`. It explicitly returns `worker_available=false`; it does not claim that the Phase 9 worker has started.','The Foundation write stores a validated request and emits `foundation.requested`. Its transport availability field reflects worker readiness, not whether a job has been started. The user explicitly opens Autopilot and starts a finite task; saving the wizard alone never initiates a paid model call.')
p='docs/WEB.md';s=(root/p).read_text();s+='''
## Phase 9 Autopilot workspace

The Autopilot route lists persisted jobs for the selected project and supports real Start, Pause, Stop and Resume commands through the typed API client. It displays the current boundary, completed/target chapter, bounded retries, pending control intent and safe failure code. Events and polling refresh server state; a button click is not treated as completion.

Every-chapter/every-N policies pause before the selected candidate is committed as Final. The page exposes candidate text and the chapter plan for review, then offers explicit approval. Approvals are bound to the selected candidate ID. Read-only details remain available while a task runs; edits require a paused/stopped job and the shared project lease. Archiving/deleting requires stopping any unfinished task.

The changed UI passed Svelte/TypeScript checking, one focused component test and a production asset build. The existing broad suites remain in the repository but were not run for this limited-scope delivery. New dependencies and local-storage credentials were not added. Task semantics, recovery and API contracts are in [AUTOPILOT.md](AUTOPILOT.md).
''';(root/p).write_text(s);changed.add(p)
edit('docs/PHASE_01_08_FIXES.md','# Phase 1–8 五项问题修复记录\n','# Phase 1–8 五项问题修复记录\n\n> 本文记录前八阶段修复时的范围。Phase 9 已由后续明确请求恢复实施，当前行为见 [AUTOPILOT.md](AUTOPILOT.md)；下文暂停说明属于当时记录，Phase 10–13 仍暂停。\n')
p='docs/AUTOPILOT.md';s=(root/p).read_text();s+='''
### Delivery evidence

Tested source: `3cf18bb1ed8393f794e0b6753baaaee6f9fbac6e`; source tree: `aff66ee85e2056d4d594f640c8fed79fcfed9f17`. GitHub Actions run `33962880165`, job `101297733300`, passed with Go 1.25.5 and Node 22. The final delivery PR may add documentation-only review; any such difference is checked separately and is not described as another full runtime test.

The configured two-chapter flow uses a temporary provider HTTP endpoint, not an injected quality service: it exercises project/server configuration, both new planning operations, the actual HTTP model adapter, persisted model-call replay, current context compilation, existing continuity/editor checks, human review and immutable Finalization, plus rebuilding the Server between chapters. Six named Go tests and one focused frontend test passed. The final executable was built again after the changed `web/dist` was generated.

Source whitespace checks and JavaScript syntax checks passed. Vite's exact generated JavaScript contains whitespace inside a string literal; that generated file is intentionally not rewritten by a whitespace trimmer. The first staging attempt stopped on that generic whitespace warning after its checks; no failing test was removed, no permanent CI configuration was weakened, and no product branch was published by that attempt.
''';(root/p).write_text(s);changed.add(p)
# Validate current files and all local Markdown file targets, without network
# checks or claiming to validate every heading anchor.
files=git('ls-files').splitlines();collisions={};missing=[]
for f in files:
 folded=f.casefold();assert folded not in collisions,(f,collisions.get(folded));collisions[folded]=f
 if not f.endswith('.md'):continue
 text=(root/f).read_text()
 for m in re.finditer(r'(?<!!)\[[^\]\n]*\]\(([^)\s]+)(?:\s+[^)]*)?\)',text):
  link=m.group(1).strip('<>');u=urllib.parse.urlsplit(link)
  if u.scheme or u.netloc or not u.path:continue
  target=root/posixpath.normpath(posixpath.join(posixpath.dirname(f),urllib.parse.unquote(u.path)))
  if not target.exists():missing.append((f,link))
assert not missing,missing
subprocess.run(['git','add','--']+sorted(changed),cwd=root,check=True)
assert all(p.startswith('docs/') and not p.startswith('docs/archive/') for p in git('diff','--cached','--name-only').splitlines())
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
subprocess.run(['git','config','user.name','feng123-new'],cwd=root,check=True)
subprocess.run(['git','config','user.email','203426027+feng123-new@users.noreply.github.com'],cwd=root,check=True)
assert git('ls-remote','--heads','origin',BRANCH).split()[0]==TESTED
subprocess.run(['git','commit','-m','docs: align Phase 9 current scope and targeted delivery evidence [skip ci]'],cwd=root,check=True)
subprocess.run(['git','push','origin','HEAD:refs/heads/'+BRANCH],cwd=root,check=True)
print(json.dumps({'tested_code':TESTED,'reviewed_head':git('rev-parse','HEAD'),'reviewed_tree':git('rev-parse','HEAD^{tree}'),'documentation_only':sorted(changed),'missing_local_targets':0,'casefold_collisions':0},indent=2))
