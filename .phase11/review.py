from pathlib import Path
import json,re,subprocess,sys
from urllib.parse import unquote
root=Path(sys.argv[1]).resolve()
changed=[]
def replace(name,old,new):
 p=root/name;s=p.read_text();assert s.count(old)==1,(name,old,s.count(old));p.write_text(s.replace(old,new));changed.append(name)
# All schema objects are compared, including internal-name lookalikes.
replace('internal/project/lifecycle_database.go'," WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name"," ORDER BY type,name")
replace('internal/server/lifecycle_test.go','\t"net/http"\n','')
replace('internal/server/lifecycle_test.go','var _ = http.MethodGet','')
p=root/'internal/project/manuscript.go';s=p.read_text();anchor='\tb, err := lifecycle.Parse(name, data)';assert s.count(anchor)==1
s=s.replace(anchor,'\tif start < 1 || start > 1000 { return lifecycle.Import{}, lifecycle.ErrInvalid }\n'+anchor);p.write_text(s);changed.append('internal/project/manuscript.go')
json.loads((root/'internal/server/openapi_phase11.json').read_text())
tracked=subprocess.check_output(['git','ls-files'],cwd=root,text=True).splitlines()
collisions=[];seen={}
for n in tracked:
 if n.casefold() in seen and seen[n.casefold()]!=n:collisions.append([seen[n.casefold()],n])
 seen[n.casefold()]=n
missing=[]
for n in tracked:
 if not n.endswith('.md') or n.startswith('docs/archive/'):continue
 text=(root/n).read_text();text=re.sub(r'```.*?```','',text,flags=re.S)
 for target in re.findall(r'(?<!!)\[[^\]]*\]\(([^)\s]+)\)',text):
  if target.startswith(('#','http:','https:','mailto:','data:')):continue
  loc=unquote(target.split('#',1)[0].split('?',1)[0])
  if loc and not (root/n).parent.joinpath(loc).exists():missing.append([n,target])
print('LIFECYCLE_STATIC='+json.dumps({'casefold_collisions':collisions,'missing_local_targets':missing}))
assert not collisions and not missing
subprocess.run(['gofmt','-w',*[str(root/n) for n in changed if n.endswith('.go')]],check=True)
subprocess.run(['git','add','--',*changed],cwd=root,check=True)
