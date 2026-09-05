from pathlib import Path
import json,os,subprocess
REPO='feng123-new/NovelForge'
MAIN='12c6f9fd2ace38826f3cd2f62f51d3c93bb710c4'
TREE='a2f58936b7417fe732a9d7218cd119002c109676'
TAG='v0.1.0-rc.1'
ARCHIVE='archive/phase-12-13a-delivery-20260906'
SELF=os.environ['GITHUB_SHA']
REFS={
 'feat/phase12-observability':'0520d91283f4135760560e02f169113d5b44ee86',
 'ops/phase12-20260906':'5f2e9234a302b3384d8215c3fc6103ab7a424e0c',
 'feat/phase13a-candidate':'9d4210052b985db48ea0e4a89388bea16e0e9221',
 'fix/candidate-draft-publication':'2135a3811392873f69b6cc757e136129715b549f',
 'ops/phase13a-20260906':SELF,
 'release/v0.1.0-rc.1':MAIN,
}
def git(*args):return subprocess.check_output(['git',*args],text=True).strip()
def api(path):return json.loads(subprocess.check_output(['gh','api',f'repos/{REPO}/{path}'],text=True))
def heads():return {line.split()[1].removeprefix('refs/heads/'):line.split()[0] for line in git('ls-remote','--heads','origin').splitlines()}
before=heads();assert before.get('main')==MAIN
for name,sha in REFS.items():assert before.get(name)==sha,(name,before.get(name),sha)
for number in [41,42,43]:assert api(f'pulls/{number}')['merged']
assert api('pulls/43')['merge_commit_sha']==MAIN
assert api(f'git/commits/{MAIN}')['tree']['sha']==TREE
release=api(f'releases/tags/{TAG}')
assert release['prerelease'] and not release['draft'] and release['published_at'] and len(release['assets'])==9
assert release['target_commitish']==MAIN
run=api('actions/runs/33982629041');assert run['status']=='completed' and run['conclusion']=='success' and run['head_sha']==MAIN
subprocess.run(['git','fetch','origin',f'refs/tags/{TAG}:refs/tags/{TAG}'],check=True)
assert git('rev-parse',f'{TAG}^{{commit}}')==MAIN
assert not git('ls-remote','--tags','origin',f'refs/tags/{ARCHIVE}')
# Additional parents on this GitHub-created controller commit preserve the
# original feature/operation histories even though main used squash merges.
for sha in set(REFS.values()):subprocess.run(['git','merge-base','--is-ancestor',sha,SELF],check=True)
record={'schema':1,'repository':REPO,'phases':['12','13A'],'phase13b':'pending local full acceptance','main':MAIN,'tree':TREE,'prs':[41,42,43],'release_tag':TAG,'release_url':release['html_url'],'release_id':release['id'],'release_assets':[{'name':a['name'],'size':a['size'],'digest':a.get('digest')} for a in release['assets']],'refs':REFS,'checks':[33981142111,33981850927,33982005438,33982083798,33982551498,33982629041],'first_publication_failure_retained':33982101675,'draft_cleanup':33982592900,'state':'archived_before_exact_deletion'}
Path('delivery-record.json').write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n')
subprocess.run(['git','config','user.name','feng123-new'],check=True)
subprocess.run(['git','config','user.email','203426027+feng123-new@users.noreply.github.com'],check=True)
subprocess.run(['git','tag','-a',ARCHIVE,SELF,'-F','delivery-record.json'],check=True)
subprocess.run(['git','push','origin',f'refs/tags/{ARCHIVE}'],check=True)
assert git('ls-remote','--tags','origin',f'refs/tags/{ARCHIVE}^{{}}').split()[0]==SELF
command=['git','push','--atomic','origin']
command += [f'--force-with-lease=refs/heads/{name}:{sha}' for name,sha in REFS.items()]
command += [f':refs/heads/{name}' for name in REFS]
subprocess.run(command,check=True)
after=heads();assert after=={name:sha for name,sha in before.items() if name not in REFS}
assert len(after)==12 and after['main']==MAIN
print(json.dumps({'main':MAIN,'tree':TREE,'archive':ARCHIVE,'archive_commit':SELF,'removed':list(REFS),'remaining_branches':len(after),'release':release['html_url'],'assets':len(release['assets'])},ensure_ascii=False,indent=2))
