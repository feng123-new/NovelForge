#!/usr/bin/env python3
"""Archive and prune only the two completed Phase 9 delivery refs."""
import json,os,subprocess,urllib.request,urllib.parse
REPO='feng123-new/NovelForge'
MERGED='1a96ab344a8d337af38da51bc080810a63d0a72d'
HEAD='489ee1be6a5c4858dcff045a5a263a48e7eb67bc'
TREE='4583e0f5e995af938bd0d3474281d407026dfd14'
CONTROLLER='ops/phase-09-delivery'
PRODUCT='feat/phase-09-durable-autopilot'
TAG='archive/phase-09-delivery-20260905'
def api(path):
 req=urllib.request.Request('https://api.github.com/repos/'+REPO+'/'+path,headers={'Authorization':'Bearer '+os.environ['GH_TOKEN'],'Accept':'application/vnd.github+json','X-GitHub-Api-Version':'2022-11-28'})
 with urllib.request.urlopen(req,timeout=30) as r:return json.load(r)
def git(*args,data=None):return subprocess.check_output(['git',*args],input=data,text=True).strip()
def all_pages(path):
 out=[]
 for page in range(1,101):
  rows=api(path+('&' if '?' in path else '?')+'per_page=100&page='+str(page));out+=rows
  if len(rows)<100:return out
 raise RuntimeError('pagination limit exceeded')
def branches():return {b['name']:b for b in all_pages('branches')}
current=os.environ['GITHUB_SHA'];expected={CONTROLLER:current,PRODUCT:HEAD}
assert git('rev-parse','HEAD')==current
assert os.environ['GITHUB_REPOSITORY']==REPO
pr=api('pulls/37')
assert pr['merged'] and pr['merge_commit_sha']==MERGED and pr['head']['sha']==HEAD and pr['base']['ref']=='main'
assert api('git/ref/heads/main')['object']['sha']==MERGED
assert api('git/commits/'+MERGED)['tree']['sha']==TREE
assert git('rev-parse',HEAD+'^{tree}')==TREE
before=branches()
for name,sha in expected.items():
 assert name in before and before[name]['commit']['sha']==sha and not before[name]['protected'],name
for p in all_pages('pulls?state=open'):
 assert p['head']['ref'] not in expected and p['base']['ref'] not in expected
assert not git('ls-remote','--tags','origin','refs/tags/'+TAG)
manifest={'schema':1,'state':'archived_before_delete','repository':REPO,'pr':37,'main':MERGED,'product_tree':TREE,'tested_source':'3cf18bb1ed8393f794e0b6753baaaee6f9fbac6e','deletion_refs':expected,'branches_before':{n:b['commit']['sha'] for n,b in before.items()},'reason':'completed merged product and completed isolated delivery controller; all other branches retained'}
blob=git('hash-object','-w','--stdin',data=json.dumps(manifest,indent=2)+'\n')
tree=git('mktree',data='100644 blob '+blob+'\tphase9-delivery-snapshot.json\n')
git('config','user.name','feng123-new');git('config','user.email','203426027+feng123-new@users.noreply.github.com')
archive=git('commit-tree',tree,'-p',current,'-p',HEAD,'-p',MERGED,'-m','Archive completed Phase 9 delivery refs; not a product release')
git('push','origin',archive+':refs/tags/'+TAG)
assert git('ls-remote','--tags','origin','refs/tags/'+TAG).split()[0]==archive
for sha in expected.values():subprocess.run(['git','merge-base','--is-ancestor',sha,archive],check=True)
# Re-read preconditions after archival; exact leases protect concurrent updates.
assert api('git/ref/heads/main')['object']['sha']==MERGED
latest=branches()
for name,sha in expected.items():assert latest[name]['commit']['sha']==sha and not latest[name]['protected'],name
for p in all_pages('pulls?state=open'):assert p['head']['ref'] not in expected and p['base']['ref'] not in expected
args=['git','push','--atomic']+['--force-with-lease=refs/heads/'+name+':'+sha for name,sha in expected.items()]+['origin']+[':refs/heads/'+name for name in expected]
subprocess.run(args,check=True)
after=branches()
assert all(name not in after for name in expected)
for name,entry in before.items():
 if name not in expected:assert name in after and after[name]['commit']['sha']==entry['commit']['sha'],name
assert api('git/ref/heads/main')['object']['sha']==MERGED
print('PHASE9_CLEANUP_RESULT='+json.dumps({'state':'deleted_and_verified','main':MERGED,'product_tree':TREE,'archive_tag':TAG,'archive_commit':archive,'deleted':expected,'remaining_count':len(after),'remaining':sorted(after)},sort_keys=True))
