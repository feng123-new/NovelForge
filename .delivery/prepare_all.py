from pathlib import Path
import runpy,subprocess,sys
here=Path(__file__).parent
script=here/'integrate.py'
lines=script.read_text().splitlines()
format_lines=[s for s in lines if s.startswith("subprocess.run(['gofmt'")]
assert len(format_lines)==1
# Apply all exact source transformations before the single formatting pass.
code='\n'.join(s for s in lines if s not in format_lines)
exec(compile(code,str(script),'exec'),{'__file__':str(script),'__name__':'__main__'})
runpy.run_path(str(here/'finish12.py'),run_name='__main__')
root=Path(sys.argv[1]).resolve()
p=root/'web/src/pages/Observability.svelte';s=p.read_text();assert s.count('if(e&&e.id!==last')==1;s=s.replace('if(e&&e.id!==last',"if(e&&typeof e.id==='number'&&e.id!==last");p.write_text(s)
p=root/'web/src/lib/observability.ts';s=p.read_text();assert s.count('last_attempt_at: string;')==1;p.write_text(s.replace('last_attempt_at: string;','last_attempt_at: string | null;'))
paths=subprocess.check_output(['git','diff','--cached','--name-only'],cwd=root,text=True).splitlines()
go=[str(root/p) for p in paths if p.endswith('.go')]
subprocess.run(['gofmt','-w',*go],check=True)
subprocess.run(['git','add','--',*paths],cwd=root,check=True)
