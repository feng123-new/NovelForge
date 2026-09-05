from pathlib import Path
import runpy,subprocess,sys
runpy.run_path(str(Path(__file__).parent/'prepare.py'),run_name='__main__')
root=Path(sys.argv[1]);p=root/'internal/project/lifecycle_backup.go';s=p.read_text();assert s.count('Schema: 10}')==1;p.write_text(s.replace('Schema: 10}','Schema: CurrentDatabaseSchema()}'))
p=root/'docs/LIFECYCLE.md';s=p.read_text().replace('Known project schemas 1–10 are supported','Known project schemas 1–11 are supported');p.write_text(s)
subprocess.run(['gofmt','-w',str(root/'internal/project/lifecycle_backup.go')],check=True)
subprocess.run(['git','add','internal/project/lifecycle_backup.go','docs/LIFECYCLE.md'],cwd=root,check=True)
