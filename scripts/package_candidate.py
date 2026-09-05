#!/usr/bin/env python3
"""Build release-candidate archives without silently invoking full acceptance.

Packaging is not publication. Requires a clean source tree and a passed quick
report for this exact commit/tree. Uses Go and the Python standard library.
"""
from __future__ import annotations
import argparse
import datetime as dt
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = [('linux','amd64','Linux','x86_64'),('linux','arm64','Linux','arm64'),('darwin','amd64','Darwin','x86_64'),('darwin','arm64','Darwin','arm64'),('windows','amd64','Windows','x86_64'),('windows','arm64','Windows','arm64')]
INCLUDED = ['README.md','LICENSE','THIRD_PARTY_NOTICES.md','docs/DEPLOYMENT.md','docs/LOCAL_ACCEPTANCE.md','docs/DIAGNOSTICS.md','docs/LIFECYCLE.md','docs/AUTHORING.md','docs/AUTOPILOT.md','docs/RELEASING.md','docs/DEPENDENCY_LICENSES.md','docs/FRONTEND_DEPENDENCY_LICENSES.md','examples/server-config.json','scripts/smoke_candidate.py','scripts/start-web.sh','scripts/start-web.ps1']


def git(*arguments: str) -> str:
    return subprocess.check_output(['git',*arguments],cwd=ROOT,text=True).strip()


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def archive(path: Path, files: dict[str, bytes], stamp: int, windows: bool) -> None:
    if windows:
        date = dt.datetime.fromtimestamp(max(stamp,315532800),dt.timezone.utc)
        with zipfile.ZipFile(path,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=9) as writer:
            for name,data in sorted(files.items()):
                info=zipfile.ZipInfo(name,date.timetuple()[:6]);info.compress_type=zipfile.ZIP_DEFLATED;info.create_system=3;info.external_attr=(0o100644 << 16)
                writer.writestr(info,data)
    else:
        with path.open('wb') as raw, gzip.GzipFile(filename='',fileobj=raw,mode='wb',mtime=stamp) as compressed, tarfile.open(fileobj=compressed,mode='w') as writer:
            for name,data in sorted(files.items()):
                info=tarfile.TarInfo(name);info.size=len(data);info.mtime=stamp;info.mode=0o755 if name=='novelforge' or name.endswith('.sh') else 0o644;info.uid=info.gid=0;info.uname=info.gname=''
                writer.addfile(info,io.BytesIO(data))


def main() -> int:
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version',required=True)
    parser.add_argument('--verification',type=Path,required=True)
    parser.add_argument('--output',type=Path,default=ROOT/'candidate-dist')
    args=parser.parse_args()
    if not re.fullmatch(r'v\d+\.\d+\.\d+-rc\.\d+',args.version):
        parser.error('candidate tag must be vMAJOR.MINOR.PATCH-rc.N')
    if git('status','--porcelain','--untracked-files=no'):
        raise SystemExit('refusing to package modified tracked source')
    commit,tree=git('rev-parse','HEAD'),git('rev-parse','HEAD^{tree}')
    verification=json.loads(args.verification.read_text(encoding='utf-8'))
    if verification.get('status')!='passed' or verification.get('mode')!='quick' or verification.get('commit')!=commit or verification.get('tree')!=tree or verification.get('dirty'):
        raise SystemExit('a clean, passing quick report for this exact commit and tree is required')
    passed={c['name'] for c in verification.get('checks',[]) if c.get('status')=='passed'}
    if not {'frontend-types','frontend-tests','frontend-build','frontend-reproducibility','api-contracts','default-entry-smoke','inspection-entry-smoke'} <= passed:
        raise SystemExit('verification report is missing mandatory candidate checks')
    stamp=int(git('show','-s','--format=%ct','HEAD'));date=dt.datetime.fromtimestamp(stamp,dt.timezone.utc).isoformat()
    out=args.output.resolve();out.mkdir(parents=True,exist_ok=False)
    included={n:(ROOT/n).read_bytes() for n in INCLUDED}
    manifest={'schema':1,'version':args.version,'commit':commit,'tree':tree,'commit_date':date,'go':subprocess.check_output(['go','version'],text=True).strip(),'channel':'release-candidate','full_local_acceptance':'pending Phase 13B','signature':'SHA-256 checksums only; not OS code signing or notarization','verification_mode':'quick','verification_sha256':sha(args.verification.read_bytes()),'frontend_sha256':{str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in sorted((ROOT/'web/dist').rglob('*')) if p.is_file()},'artifacts':[]}
    host_os=subprocess.check_output(['go','env','GOHOSTOS'],text=True).strip();host_arch=subprocess.check_output(['go','env','GOHOSTARCH'],text=True).strip()
    from smoke_candidate import smoke
    with tempfile.TemporaryDirectory(prefix='novelforge-package-') as temporary:
        for goos,goarch,label,arch in TARGETS:
            executable='novelforge.exe' if goos=='windows' else 'novelforge'
            binary=Path(temporary)/(goos+'-'+goarch+'-'+executable)
            flags=f'-s -w -X main.version={args.version} -X main.commit={commit} -X main.date={date}'
            print(f'Build {goos}/{goarch}',flush=True)
            env={**os.environ,'CGO_ENABLED':'0','GOWORK':'off','GOFLAGS':'-mod=readonly','GOOS':goos,'GOARCH':goarch}
            subprocess.run(['go','build','-trimpath','-ldflags',flags,'-o',str(binary),'./cmd/novelforge'],cwd=ROOT,env=env,check=True,timeout=900)
            metadata={'version':args.version,'commit':commit,'tree':tree,'os':goos,'arch':goarch,'binary_sha256':sha(binary.read_bytes()),'runtime_verification':'not run on target platform','phase13b':'pending'}
            if goos==host_os and goarch==host_arch:
                metadata['smoke']=smoke(binary)
                if args.version not in metadata['smoke']['version'] or commit not in metadata['smoke']['version']:
                    raise SystemExit('binary version does not match the candidate source')
                metadata['runtime_verification']='native no-model smoke passed; not full acceptance'
            files={**included,executable:binary.read_bytes(),'BUILD_INFO.json':(json.dumps(metadata,indent=2)+'\n').encode()}
            filename=f'novelforge_{args.version[1:]}_{label}_{arch}'+('.zip' if goos=='windows' else '.tar.gz')
            archive(out/filename,files,stamp,goos=='windows')
            manifest['artifacts'].append({**metadata,'name':filename,'size':(out/filename).stat().st_size,'sha256':sha((out/filename).read_bytes())})
    (out/'release-manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    # Share only structured verification metadata; test logs stay in Actions.
    summary={k:v for k,v in verification.items() if k not in ('checks','error')};summary['checks']=[{k:v for k,v in check.items() if k in ('name','status','seconds')} for check in verification['checks']]
    (out/'verification-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    sums=[]
    for file in sorted(out.iterdir()):
        if file.is_file():sums.append(f'{sha(file.read_bytes())}  {file.name}')
    (out/'novelforge_checksums.txt').write_text('\n'.join(sums)+'\n',encoding='utf-8')
    assert len(manifest['artifacts'])==6
    print(json.dumps({'output':str(out),'commit':commit,'archives':6,'release_files':len(list(out.iterdir()))},indent=2))
    return 0

if __name__=='__main__':
    raise SystemExit(main())
