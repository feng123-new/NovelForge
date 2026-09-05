"""Remove only the verified unpublished draft from the failed first publication.
Published tags, releases, source branches and Actions artifacts are untouched.
"""
import json, subprocess
REPO='feng123-new/NovelForge'
ID=383325874
TAG='v0.1.0-rc.1'
SOURCE='31f23e8f444f1a5f2639a6d4d56d29409880e385'
def api(path):return json.loads(subprocess.check_output(['gh','api',f'repos/{REPO}/{path}'],text=True))
release=api(f'releases/{ID}')
assert release['id']==ID and release['tag_name']==TAG
assert release['draft'] is True and release['prerelease'] is True and release['published_at'] is None
assert release['target_commitish']==SOURCE and len(release['assets'])==9
jobs=api('actions/runs/33982101675/jobs')['jobs']
assert any(j['id']==101349032105 and j['conclusion']=='success' for j in jobs)
assert any(j['id']==101349497725 and j['conclusion']=='failure' for j in jobs)
lookup=subprocess.run(['gh','api',f'repos/{REPO}/git/ref/tags/{TAG}'],capture_output=True,text=True)
assert lookup.returncode!=0 and ('404' in lookup.stderr or '404' in lookup.stdout),'unexpected tag: stop without deletion'
print(json.dumps({'action':'remove_only_unpublished_failed_draft','id':ID,'tag':TAG,'source':SOURCE,'assets':[{'name':a['name'],'size':a['size'],'digest':a.get('digest')} for a in release['assets']],'original_build_run_retained':33982101675},indent=2))
subprocess.run(['gh','api','--method','DELETE',f'repos/{REPO}/releases/{ID}'],check=True)
verify=subprocess.run(['gh','api',f'repos/{REPO}/releases/{ID}'],capture_output=True,text=True)
assert verify.returncode!=0 and ('404' in verify.stderr or '404' in verify.stdout)
print('UNPUBLISHED_DRAFT_REMOVED; source, tags and original Actions evidence retained')
