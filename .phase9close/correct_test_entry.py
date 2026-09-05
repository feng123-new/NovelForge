#!/usr/bin/env python3
from pathlib import Path
import subprocess,sys
root=Path(sys.argv[1]);p=root/'internal/server/autopilot_closure_test.go'
s=p.read_text();old='service.Accept(t.Context(), 1, "human-accept", revision.ID, "Author approved replacement")';new='coordinator.Accept(t.Context(), 1, "human-accept", revision.ID, "Author approved replacement")'
assert s.count(old)==1
p.write_text(s.replace(old,new))
subprocess.run(['gofmt','-w',str(p)],check=True)
subprocess.run(['git','add','internal/server/autopilot_closure_test.go'],cwd=root,check=True)
print('Human takeover test now uses the production semantic Accept coordinator; assertions unchanged.')
