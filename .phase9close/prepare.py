#!/usr/bin/env python3
"""Correct one exact integration anchor, then execute the reviewed fixed-base patch."""
from pathlib import Path
import sys
p=Path(__file__).with_name('apply.py')
s=p.read_text()
lines=s.splitlines(True)
needle="edit(p,'\\t\\tdefer lease.Close()\\n\\t\\tnext.ServeHTTP(w, r)'"
matched=[i for i,line in enumerate(lines) if line.startswith(needle)]
assert len(matched)==1
old='\t\tdefer lease.Close()\n\t\tactive, err = s.jobs.Active(r.Context(), id)'
new=old+'\n\t\tif destructive { active,err=s.jobs.Unfinished(r.Context(),id) }'
lines[matched[0]]=f'edit(p,{old!r},{new!r})\n'
s=''.join(lines)
# All remaining exact-base and path assertions are unchanged.
exec(compile(s,str(p),'exec'),{'__file__':str(p),'__name__':'__main__'})
root=Path(sys.argv[1])
# Svelte mutable state updated by an async helper needs an explicit annotation.
f=root/'web/src/pages/Autopilot.svelte'
source=f.read_text();old='  if (page?.next_chapter) startChapter = page.next_chapter;';assert source.count(old)==1
f.write_text(source.replace(old,'  const recommended = (page as AutopilotPage | null)?.next_chapter;\n  if (recommended) startChapter = recommended;'))
# HTTP regression must reject empty approvals as well as accept the explicit one.
f=root/'internal/server/autopilot_test.go';source=f.read_text()
old='\tresumed := postJob(t, s, base+"/"+id+"/resume", "approve1", approvalBody(j))'
new='\tmissing := postJob(t,s,base+"/"+id+"/resume","missing-approval",`{}`)\n\tif missing.Code!=409 { t.Fatal("empty review approval accepted",missing.Code,missing.Body.String()) }\n'+old
assert source.count(old)==1;f.write_text(source.replace(old,new))
import subprocess
subprocess.run(['gofmt','-w',str(f)],check=True)
subprocess.run(['git','add','web/src/pages/Autopilot.svelte','internal/server/autopilot_test.go'],cwd=root,check=True)
