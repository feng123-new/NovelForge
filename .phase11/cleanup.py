import json,os,subprocess
MERGE='4484a7397ef0494ce5113163c845d131f685ed7d'
HEAD='1e5ad564ca0483664e2db51e8de96fdd322985ea'
TREE='3c47d67e435607f2a58aa95d806742e5e2c8161a'
TAG='archive/phase-11-delivery-20260905'
def git(*args,input=None):
 return subprocess.check_output(['git',*args],input=input,text=True).strip()
def remote(ref):
 value=git('ls-remote','origin',ref)
 return value.split()[0] if value else ''
self_sha=git('rev-parse','HEAD')
assert self_sha==os.environ['GITHUB_SHA']
assert remote('refs/heads/main')==MERGE
assert git('rev-parse',MERGE+'^{tree}')==TREE
assert git('rev-parse',HEAD+'^{tree}')==TREE
assert remote('refs/heads/ops/phase11-delivery')==self_sha
product_ref=remote('refs/heads/feat/phase11-lifecycle')
assert product_ref in ('',HEAD)
assert not remote('refs/tags/'+TAG)
record={'schema':1,'repository':'feng123-new/NovelForge','pr':40,'merge':MERGE,'product_head':HEAD,'product_tree':TREE,'refs':{'ops/phase11-delivery':self_sha,'feat/phase11-lifecycle':HEAD},'checks':[33976836444],'state':'archived_before_delete'}
blob=git('hash-object','-w','--stdin',input=json.dumps(record,indent=2)+'\n')
tree=git('mktree',input='100644 blob '+blob+'\tdelivery-record.json\n')
git('config','user.name','feng123-new')
git('config','user.email','203426027+feng123-new@users.noreply.github.com')
archive=git('commit-tree',tree,'-p',self_sha,'-p',HEAD,'-p',MERGE,input='Archive completed Phase 11 product, verification lineage and temporary refs.\n')
git('tag',TAG,archive)
git('push','origin','refs/tags/'+TAG)
assert remote('refs/tags/'+TAG)==archive
refs=['ops/phase11-delivery']
if product_ref:refs.insert(0,'feat/phase11-lifecycle')
# Delete exactly the just-archived branch tips, never other historical refs.
assert remote('refs/heads/ops/phase11-delivery')==self_sha
if product_ref:assert remote('refs/heads/feat/phase11-lifecycle')==HEAD
git('push','--atomic','origin','--delete',*refs)
assert not remote('refs/heads/ops/phase11-delivery')
assert not remote('refs/heads/feat/phase11-lifecycle')
assert remote('refs/heads/main')==MERGE
print('PHASE11_CLEANUP='+json.dumps({'main':MERGE,'tree':TREE,'archive':TAG,'archive_commit':archive,'deleted_refs':refs,'remaining_branches':len(git('ls-remote','--heads','origin').splitlines())}))
