from pathlib import Path
import ast,json,re,shutil,subprocess,sys
from urllib.parse import unquote
root=Path(sys.argv[1]).resolve();source=Path(__file__).parent/'files';changed=[]
def replace(name,old,new):
 p=root/name;s=p.read_text();assert s.count(old)==1,(name,old,s.count(old));p.write_text(s.replace(old,new));changed.append(name)
for p in source.rglob('*'):
 if p.is_file():
  name=str(p.relative_to(source));dest=root/name;assert not dest.exists(),name;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(p,dest);changed.append(name)
replace('cmd/novelforge/server.go','configPath := flags.String("config", "", "explicit server model configuration file")','configPath := flags.String("config", "", "explicit server model configuration file")\n noAutopilot := flags.Bool("no-autopilot", false, "disable the durable task worker for inspection; explicit manual model actions remain available")')
replace('cmd/novelforge/server.go','[--config FILE]','[--config FILE] [--no-autopilot]')
replace('cmd/novelforge/server.go','AutopilotEnabled:     true,','AutopilotEnabled:     !*noAutopilot,')
# Preserve the complete original job body/strict assertions. Only change dispatch.
replace('.github/workflows/ci.yml','name: CI\n\non:\n  push:\n    branches: [main]\n  pull_request:\n  workflow_dispatch:','name: Full acceptance (manual)\n\non:\n  workflow_dispatch:')
replace('internal/project/lifecycle_test.go','got.Schema != 10','got.Schema != CurrentDatabaseSchema()')
replace('internal/project/lifecycle_test.go','schema != 10','schema != CurrentDatabaseSchema()')
# Local preimage assertions remain schema 9 and format 1, not rewritten.
assert 'm.Schema != 9' in (root/'internal/project/lifecycle_test.go').read_text()
p=root/'.gitignore';s=p.read_text();s+='\n# Candidate builds and local acceptance evidence (may contain private paths)\n/candidate-dist/\n/verification-results/\n';p.write_text(s);changed.append('.gitignore')
# Tooling syntax and fixtures are checked without executing full/scale/paid modes.
for name in changed:
 if name.endswith('.py'):ast.parse((root/name).read_text(),filename=name)
json.loads((root/'examples/server-config.json').read_text())
for name in ['README.md','docs/README.md','docs/ROADMAP.md','docs/DEVELOPMENT.md','docs/WEB.md','docs/AUTHORING.md','docs/AUTOPILOT.md','docs/LIFECYCLE.md']:
 p=root/name;s=p.read_text();s=s.replace('Phase 12–13 remain paused.','Phase 12 is delivered. Phase 13A supplies candidate delivery; Phase 13B full acceptance remains local.').replace('Phase 12–13 remain paused;', 'Phase 12 is delivered; Phase 13B remains local;').replace('Phase 12–13 stay paused.','Phase 13B remains local acceptance.')
 if name=='docs/ROADMAP.md':
  s=s.replace('## Phase 13 — v0.1.0 hardening — Deferred; frozen','## Phase 13B — v0.1.0 full acceptance — Deferred to local environment')
  s+='\n\n## Phase 13A — Default entry, verification and candidate delivery\n\nNormal server startup retains its worker; --no-autopilot provides explicit inspection mode. Quick PR checks, manually triggered original full CI, local verification modes, native executable smoke, six-target candidate packaging and prerelease publication are separate. See [deployment](DEPLOYMENT.md), [local acceptance](LOCAL_ACCEPTANCE.md) and [release process](RELEASING.md). Candidate publication is confirmed by the actual GitHub Release and exact manifest, not by this status heading. No Phase 13B full or scale success is implied.\n'
 elif name in ['README.md','docs/README.md','docs/DEVELOPMENT.md']:
  links=('docs/' if name=='README.md' else '')
  block='\n## Current delivery: Phase 12 + Phase 13A\n\nPhase 1–12 functionality is delivered; Phase 13A supplies default-entry smoke, local verification tools and an explicit release-candidate pipeline. Phase 13B full/platform/scale/real-provider acceptance is pending local execution.\n\nStart with [local deployment]('+links+'DEPLOYMENT.md), follow [local acceptance]('+links+'LOCAL_ACCEPTANCE.md), and use the [candidate process]('+links+'RELEASING.md). A prerelease is not stable/latest, and cross-compilation is not target-platform runtime validation.\n'
  first,sep,rest=s.partition('\n');s=first+'\n'+block+'\n'+rest
 p.write_text(s);changed.append(name)
# Do not expose clean checkout paths or test sample content in the published
# verification summary; detailed private logs remain workflow artifacts.
p=root/'scripts/package_candidate.py';s=p.read_text();old="(out/'verification-summary.json').write_text(json.dumps(verification,ensure_ascii=False,indent=2)+'\\n',encoding='utf-8')";assert s.count(old)==1
new="summary={k:v for k,v in verification.items() if k not in ('checks','error')};summary['checks']=[{k:v for k,v in check.items() if k in ('name','status','seconds')} for check in verification['checks']]\n    (out/'verification-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\\n',encoding='utf-8')"
s=s.replace(old,new);p.write_text(s)
# Normalize wrappers and new test code, preserving all old migrations/dependencies.
subprocess.run(['gofmt','-w',*[str(root/n) for n in changed if n.endswith('.go')]],check=True)
subprocess.run(['git','add','--',*sorted(set(changed))],cwd=root,check=True)
tracked=subprocess.check_output(['git','ls-files'],cwd=root,text=True).splitlines();seen={};missing=[]
for name in tracked:
 assert name.casefold() not in seen or seen[name.casefold()]==name, name
 seen[name.casefold()]=name
 if not name.endswith('.md') or name.startswith('docs/archive/'):continue
 text=re.sub(r'```.*?```','',(root/name).read_text(),flags=re.S)
 for target in re.findall(r'(?<!!)\[[^\]]*\]\(([^)\s]+)\)',text):
  if target.startswith(('#','https:','http:','mailto:','data:')):continue
  local=unquote(target.split('#',1)[0].split('?',1)[0])
  if local and not (root/name).parent.joinpath(local).exists():missing.append([name,target])
print('LOCAL_LINK_CHECK',json.dumps(missing));assert not missing
print('PHASE13A_CHANGED',len(set(changed)))
