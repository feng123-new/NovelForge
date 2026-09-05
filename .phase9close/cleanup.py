#!/usr/bin/env python3
"""Archive and prune only the two exact merged Phase 9 closure refs."""
import json,os,subprocess
REPO='feng123-new/NovelForge'
MERGE='c0c07201087b0e56526e58995586dee3f697c2bc'
HEAD='10bd8f3e3265b643ec924c41048b16505f0979ff'
TREE='ed7c296e5a30c025c391c5e9ae9c5e6bca586054'
TAG='archive/phase-09-closure-20260905'
def cmd(*args,input=None):return subprocess.check_output(args,input=input,text=True).strip()
def api(path):return json.loads(cmd('gh','api',f'repos/{REPO}/{path}'))
def pages(path):
 out=[]
 for n in range(1,100):
  part=api(path+('&' if '?' in path else '?')+f'per_page=100&page={n}');out+=part
  if len(part)<100:return out
 raise RuntimeError('pagination safety limit')
pr=api('pulls/38');assert pr['merged'] and pr['merge_commit_sha']==MERGE and pr['head']['sha']==HEAD
assert api('git/ref/heads/main')['object']['sha']==MERGE
assert cmd('git','rev-parse',MERGE+'^{tree}')==TREE
assert cmd('git','rev-parse',HEAD+'^{tree}')==TREE
# Permanent CI, credentials, old migrations/archives and dependency inputs unchanged.
assert not cmd('git','diff','--name-only','1a96ab344a8d337af38da51bc080810a63d0a72d',MERGE,'--','.github','go.mod','go.sum','web/package-lock.json','docs/archive')
controller=cmd('git','rev-parse','HEAD')
targets={'fix/phase9-closure':HEAD,'ops/phase9-closure':controller}
original=pages('branches');byname={x['name']:x for x in original}
for branch,sha in targets.items():
 assert branch in byname and byname[branch]['commit']['sha']==sha and not byname[branch]['protected']
for p in pages('pulls?state=open'):
 assert not (p['head']['repo'] and p['head']['repo']['full_name']==REPO and p['head']['ref'] in targets)
assert not cmd('git','ls-remote','origin','refs/tags/'+TAG),'archive tag already exists; refusing overwrite'
record={'schema':1,'repository':REPO,'pr':38,'merge':MERGE,'product_head':HEAD,'product_tree':TREE,'deleted_refs':targets,'state':'archived_before_delete','checks':[33965208527,33965606038]}
blob=cmd('git','hash-object','-w','--stdin',input=json.dumps(record,ensure_ascii=False,indent=2)+'\n')
tree=cmd('git','mktree',input=f'100644 blob {blob}\tclosure-record.json\n')
subprocess.run(['git','config','user.name','feng123-new'],check=True)
subprocess.run(['git','config','user.email','203426027+feng123-new@users.noreply.github.com'],check=True)
archive=cmd('git','commit-tree',tree,'-p',MERGE,'-p',HEAD,'-p',controller,input='Archive merged Phase 9 closure source, verification and cleanup refs. Not a Release.\n')
subprocess.run(['git','push','origin',archive+':refs/tags/'+TAG],check=True)
assert cmd('git','ls-remote','origin','refs/tags/'+TAG).split()[0]==archive
# Re-check main immediately before deletion; leases refuse changed refs.
assert api('git/ref/heads/main')['object']['sha']==MERGE
leases=['--force-with-lease=refs/heads/'+name+':'+sha for name,sha in targets.items()]
subprocess.run(['git','push','--atomic',*leases,'origin',*[':refs/heads/'+name for name in targets]],check=True)
after=pages('branches');kept={x['name']:x['commit']['sha'] for x in after}
expected={x['name']:x['commit']['sha'] for x in original if x['name'] not in targets}
assert kept==expected, 'branch state changed concurrently; inspect before reporting'
result={**record,'state':'deleted_and_verified','archive_tag':TAG,'archive_commit':archive,'remaining_count':len(after),'remaining':sorted(kept)}
print('PHASE9_CLOSURE_RESULT='+json.dumps(result,ensure_ascii=False))
