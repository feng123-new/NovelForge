#!/usr/bin/env python3
"""Archive before deleting only this delivery's exact two refs. Never move main."""
import json, os, re, subprocess, urllib.request
repo='feng123-new/NovelForge'
merge='9816bb98f6f8cabfd00797a80ecf721e57c4c461'
product='8a11910f1f68a6baa6e38026e74fb8a41e14c2cd'
tree='e96fe19862d1186110e61c2ffb55031e6f4a8678'
controller=os.environ['GITHUB_SHA']
tag='archive/phase-10-delivery-20260905'
assert re.fullmatch('[0-9a-f]{40}',controller)
assert os.environ.get('GITHUB_REPOSITORY')==repo
refs={'feat/phase10-authoring':product,'ops/phase10-delivery':controller}
def git(*args,input=None):
 return subprocess.check_output(['git',*args],input=input,text=True).strip()
def remote():
 return {ref:sha for sha,ref in (line.split() for line in git('ls-remote','--heads','origin').splitlines())}
def read(path):
 request=urllib.request.Request('https://api.github.com/repos/'+repo+'/'+path,headers={'Authorization':'Bearer '+os.environ['GH_TOKEN'],'Accept':'application/vnd.github+json','X-GitHub-Api-Version':'2022-11-28'})
 with urllib.request.urlopen(request,timeout=30) as response:return json.load(response)
pr=read('pulls/39')
assert pr['merged'] and pr['merge_commit_sha']==merge
before=remote()
assert before['refs/heads/main']==merge
for name,sha in refs.items():assert before.get('refs/heads/'+name)==sha,(name,'ref moved or absent')
assert git('rev-parse',merge+'^{tree}')==tree
assert git('rev-parse',product+'^{tree}')==tree
assert not git('ls-remote','--tags','origin','refs/tags/'+tag),'archive tag already exists'
record={'schema':1,'repository':repo,'pr':39,'merge':merge,'product_head':product,'product_tree':tree,'refs':refs,'checks':[33967464713,33967637352],'state':'archived_before_delete'}
blob=git('hash-object','-w','--stdin',input=json.dumps(record,ensure_ascii=False,indent=2)+'\n')
archive_tree=git('mktree',input='100644 blob '+blob+'\tdelivery-record.json\n')
git('config','user.name','feng123-new')
git('config','user.email','203426027+feng123-new@users.noreply.github.com')
archive=git('commit-tree',archive_tree,'-p',product,'-p',controller,'-p',merge,'-m','Archive Phase 10 delivery refs and exact verification evidence [skip ci]')
subprocess.run(['git','push','origin',archive+':refs/tags/'+tag],check=True)
# Check again, then atomically remove refs only if their exact values still match.
current=remote();assert current==before,'repository refs changed during archive; stop for review'
leases=['--force-with-lease=refs/heads/'+name+':'+sha for name,sha in refs.items()]
subprocess.run(['git','push','--atomic',*leases,'origin',*[':refs/heads/'+name for name in refs]],check=True)
after=remote()
expected={ref:sha for ref,sha in before.items() if ref not in {'refs/heads/'+n for n in refs}}
assert after==expected,'unexpected remote branch change'
result={'state':'deleted_and_verified','main':merge,'product_tree':tree,'archive_tag':tag,'archive_commit':archive,'deleted':refs,'remaining_count':len(after),'remaining':sorted(name.removeprefix('refs/heads/') for name in after)}
print('PHASE10_CLEANUP_RESULT='+json.dumps(result,ensure_ascii=False,sort_keys=True))
