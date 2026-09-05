#!/usr/bin/env python3
"""Static final review, then reversible exact-ref cleanup after PR #36 merge."""
import collections, json, os, pathlib, posixpath, re, subprocess, sys, urllib.parse, urllib.request
REPO='feng123-new/NovelForge'
BASE='48fb03aa1d9d91ee994edc935317d6bbfe6d8b06'
TESTED='e3cf8595ca1dadd613d9cae08411a52444efb800'
HEAD='0e06a07328a355e1c507c983444c1f5e78089f1e'
CONTROL='ops/phase-01-08-cleanup-20260905'
PRODUCT='chore/phase-01-08-cleanup-20260905'
FROZEN='feature/phase-09-autopilot'
ARCHIVE='archive/phase-01-08-cleanup-20260905'
FINAL=ARCHIVE+'-final'
ALLOWED={'docs/DEVELOPMENT.md','docs/archive/phase-01-08/PHASE_08_ACCEPTANCE.md'}

def git(*args,data=None):
 p=subprocess.run(['git',*args],input=data,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 if p.returncode: raise RuntimeError(f'git {args[0]} failed: '+p.stderr.decode(errors='replace'))
 return p.stdout

def text(*args): return git(*args).decode().strip()
def api(path):
 req=urllib.request.Request(f'https://api.github.com/repos/{REPO}/{path}',headers={'Accept':'application/vnd.github+json','Authorization':'Bearer '+os.environ['GH_TOKEN'],'X-GitHub-Api-Version':'2022-11-28'})
 with urllib.request.urlopen(req,timeout=30) as r: return json.load(r)
def pages(path):
 out=[]
 for n in range(1,11):
  b=api(path+('&' if '?' in path else '?')+f'per_page=100&page={n}')
  assert isinstance(b,list)
  out+=b
  if len(b)<100: return out
 raise RuntimeError('pagination limit')
def summary(s):
 print(s)
 with open(os.environ['GITHUB_STEP_SUMMARY'],'a') as f: f.write(s+'\n')

def run():
 pr=api('pulls/36')
 assert pr['head']['ref']==PRODUCT and pr['head']['sha']==HEAD
 main=api('git/ref/heads/main')['object']['sha']
 assert main==(pr['merge_commit_sha'] if pr['merged'] else BASE), 'unexpected main change'
 changes=set(text('diff','--name-only',TESTED,HEAD).splitlines())
 assert changes==ALLOWED, changes
 git('diff','--check',TESTED,HEAD)
 git('diff','--exit-code',TESTED,HEAD,'--','internal','cmd','go.mod','go.sum','web','.github')
 assert text('rev-parse',HEAD+':docs/archive/phase-01-08/PHASE_08_ACCEPTANCE.md')=='463e7972f753dba4c2a05e65456bb20979e9c539'
 paths=set(git('ls-tree','-r','--name-only','-z',HEAD).decode().split('\0')[:-1])
 assert len({p.casefold() for p in paths})==len(paths)
 # Validate inline/reference-style local Markdown file targets, not remote URLs or anchors.
 patterns=[re.compile(r'!?\[[^\]\n]*\]\((<[^>\n]+>|[^\s()]+)(?:[ \t]+[\"\'][^\n]*?[\"\'])?\)'),re.compile(r'(?m)^[ \t]*\[[^\]\n]+\]:[ \t]*(<[^>\n]+>|[^\s]+)')]
 bad=[]
 for p in sorted(paths):
  if not p.endswith('.md'): continue
  content=git('show',HEAD+':'+p).decode()
  for pattern in patterns:
   for m in pattern.finditer(content):
    u=urllib.parse.urlsplit(m[1].strip('<>'))
    if u.scheme or u.netloc or not u.path or u.path.startswith('/'): continue
    target=posixpath.normpath(posixpath.join(posixpath.dirname(p),urllib.parse.unquote(u.path)))
    if target not in paths and not any(q.startswith(target.rstrip('/')+'/') for q in paths): bad.append((p,target))
 assert not bad,bad
 summary('FINAL_STATIC_REVIEW='+json.dumps({'head':HEAD,'tested_code':TESTED,'documentation_only_followup':sorted(changes),'local_missing_targets':0,'casefold_collisions':0,'historical_acceptance_bytes':'identical to pre-cleanup baseline'}))
 if not pr['merged']:
  summary('PR #36 is open. Review checks passed; no branch or tag mutation performed in this attempt.')
  return
 assert text('rev-parse',main+'^{tree}')==text('rev-parse',HEAD+'^{tree}')
 old_archive=text('rev-parse','refs/tags/'+ARCHIVE)
 assert old_archive=='84dcc9177ca9d67719e901a224941aa4c8ffba9b'
 snap=json.loads(text('show',old_archive+':branch-snapshot.json'))
 assert snap['base']==BASE and snap['repository']==REPO and snap['cleanup_product_commit']==TESTED
 current={b['name']:b for b in pages('branches')}
 active=set()
 for p in pages('pulls?state=open'):
  for side in ('head','base'):
   if p[side].get('repo') and p[side]['repo']['full_name']==REPO: active.add(p[side]['ref'])
 control_sha=os.environ['GITHUB_SHA']
 assert current[CONTROL]['commit']['sha']==control_sha
 planned=[r for r in snap['branches'] if r['decision']=='delete_after_merge']
 planned += [{'name':CONTROL,'sha':control_sha,'reason':'completed_cleanup_controller'}, {'name':PRODUCT,'sha':HEAD,'reason':'merged_cleanup_pr'}]
 deletions=[]; skipped=[]
 for r in planned:
  b=current.get(r['name'])
  if not b or b['protected'] or r['name'] in active or b['commit']['sha']!=r['sha']:
   skipped.append(r['name']); continue
  assert r['name'] not in ('main',FROZEN)
  git('check-ref-format','refs/heads/'+r['name'])
  deletions.append(r)
 assert deletions
 result={'schema':1,'state':'archived_before_delete','repository':REPO,'pr':36,'main':main,'reviewed_head':HEAD,'original_archive':old_archive,'controller':control_sha,'deletions':deletions,'skipped_changed_or_active':skipped}
 data=(json.dumps(result,ensure_ascii=False,indent=2)+'\n').encode()
 # Supplemental immutable archive retains the documentation review and this final controller,
 # while the original snapshot/tag remains untouched.
 blob=git('hash-object','-w','--stdin',data=data).decode().strip()
 tree=git('mktree',data=f'100644 blob {blob}\tcleanup-result.json\n'.encode()).decode().strip()
 git('config','user.name','NovelForge maintenance')
 git('config','user.email','203426027+feng123-new@users.noreply.github.com')
 final=git('commit-tree',tree,'-p',old_archive,'-p',HEAD,'-p',control_sha,'-p',main,data=b'Archive reviewed cleanup refs before deleting exact branch names.\n').decode().strip()
 assert not text('ls-remote','--tags','origin','refs/tags/'+FINAL), 'do not replace archival tag'
 git('push',f'--force-with-lease=refs/tags/{FINAL}:','origin',f'{final}:refs/tags/{FINAL}')
 for r in deletions: git('merge-base','--is-ancestor',r['sha'],final)
 assert api('git/ref/heads/main')['object']['sha']==main
 command=['push','--atomic']+[f'--force-with-lease=refs/heads/{r["name"]}:{r["sha"]}' for r in deletions]+['origin']+[':refs/heads/'+r['name'] for r in deletions]
 git(*command)
 remaining=pages('branches'); names={b['name'] for b in remaining}
 assert not names.intersection(r['name'] for r in deletions)
 assert api('git/ref/heads/main')['object']['sha']==main
 assert api('git/ref/heads/'+FROZEN)['object']['sha']==next(r['sha'] for r in snap['branches'] if r['name']==FROZEN)
 result.update(state='deleted_and_verified',final_archive=final,deleted_count=len(deletions),remaining_count=len(remaining),remaining=sorted(names))
 summary('PRUNE_RESULT='+json.dumps(result,ensure_ascii=False))

if __name__=='__main__':
 if sys.argv[1]=='mode':
  with open(os.environ['GITHUB_OUTPUT'],'a') as f: f.write('mode=prune\n')
 elif sys.argv[1]=='prune': run()
 else: raise RuntimeError('final controller only performs reviewed static checks and post-merge pruning')
