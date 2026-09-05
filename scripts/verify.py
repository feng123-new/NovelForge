#!/usr/bin/env python3
"""Explicit verification tiers. Default quick never runs full suites or paid models.

Each execution writes a JSON manifest and individual logs. Failure is nonzero;
missing prerequisites are blocked, not passed. Use a source checkout for tests.
"""
from __future__ import annotations
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
SERVER_QUICK = '^(TestObservationConfiguredAttemptsFallbackReplayAndPause|TestObservationAPIRevisionContractAndUnknownUsage|TestAutopilotRealChapterFlowReviewAndRestart|TestAutopilotClosureInterruptedRewriteReplaysRequest|TestAuthoringActualModelRequestsAndReplay|TestLifecycleImportAnalysisExportRestoreFlow)$'
RECOVERY = '^(TestAutopilotRealChapterFlowReviewAndRestart|TestAutopilotClosureInterruptedRewriteReplaysRequest|TestLifecycleImportAnalysisExportRestoreFlow|TestConfiguredQualityHTTPAndHumanRevision)$'
FRONTEND_QUICK = ['src/pages/Observability.test.ts', 'src/pages/Autopilot.test.ts', 'src/pages/Lifecycle.test.ts']


def command_path(name: str) -> str:
    result = shutil.which(name + '.cmd') if os.name == 'nt' and name in ('npm', 'npx') else shutil.which(name)
    if not result:
        raise FileNotFoundError(f'required tool is missing: {name}')
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mode', choices=['doctor', 'quick', 'full', 'recovery', 'scale', 'smoke'], default='quick')
    parser.add_argument('--binary', type=Path, help='required for smoke-only on an extracted release')
    parser.add_argument('--output', type=Path, help='new directory for private logs and evidence')
    parser.add_argument('--skip-frontend', action='store_true', help='explicit partial scope; never means frontend passed')
    args = parser.parse_args()
    stamp = dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    output = (args.output or ROOT / 'verification-results' / (stamp + '-' + args.mode)).resolve()
    output.mkdir(parents=True, exist_ok=False)
    report = {'schema': 1, 'mode': args.mode, 'started_at': stamp, 'platform': platform.platform(), 'python': sys.version.split()[0], 'status': 'running', 'checks': [], 'paid_model_calls': 'not permitted', 'unexecuted': ['paid-model literary evaluation', 'other-platform runtime acceptance', 'browser manual acceptance', 'power-loss filesystem verification']}
    if args.mode != 'full':
        report['unexecuted'].append('complete unit/integration/race suites')
    if args.mode != 'scale':
        report['unexecuted'].append('100/500/1000-chapter synthetic scale')
    def flush():
        (output / 'result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    try:
        report['commit'] = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
        report['tree'] = subprocess.check_output(['git', 'rev-parse', 'HEAD^{tree}'], cwd=ROOT, text=True).strip()
        report['dirty'] = bool(subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=no'], cwd=ROOT, text=True).strip())
    except (OSError, subprocess.SubprocessError):
        report['commit'] = 'unavailable (release binary or non-Git source)'
    flush()
    with tempfile.TemporaryDirectory(prefix='novelforge-verify-') as temporary:
        temporary_root = Path(temporary)
        env = dict(os.environ)
        for key in list(env):
            upper = key.upper()
            if any(word in upper for word in ('API_KEY', 'API_TOKEN', 'AUTH_TOKEN')) or upper.startswith(('NOVELFORGE_', 'AINOVEL_')):
                env.pop(key, None)
        try:
            if args.mode != 'smoke':
                go = command_path('go')
                # Preserve development caches while isolating application home/config.
                for name in ['GOMODCACHE', 'GOCACHE']:
                    env[name] = subprocess.check_output([go, 'env', name], cwd=ROOT, text=True).strip()
            home = temporary_root / 'home'
            home.mkdir()
            env.update(HOME=str(home), USERPROFILE=str(home), APPDATA=str(home / 'appdata'), LOCALAPPDATA=str(home / 'local'), XDG_CONFIG_HOME=str(home / 'config'), XDG_DATA_HOME=str(home / 'data'), GOWORK='off', GOFLAGS='-mod=readonly')
            def run(name: str, command: list[str], cwd: Path = ROOT, timeout: int = 180, extra: dict | None = None):
                started = time.monotonic()
                log = output / (f'{len(report["checks"]):02d}-' + name + '.log')
                record = {'name': name, 'command': command, 'log': log.name, 'status': 'running'}
                report['checks'].append(record)
                flush()
                print(f'[{args.mode}] {name}', flush=True)
                try:
                    with log.open('wb') as handle:
                        subprocess.run(command, cwd=cwd, env={**env, **(extra or {})}, stdout=handle, stderr=subprocess.STDOUT, timeout=timeout, check=True)
                    record['status'] = 'passed'
                except FileNotFoundError:
                    record['status'] = 'blocked'
                    raise
                except (subprocess.SubprocessError, OSError):
                    record['status'] = 'failed'
                    print(log.read_text(encoding='utf-8', errors='replace')[-8000:], file=sys.stderr)
                    raise
                finally:
                    record['seconds'] = round(time.monotonic() - started, 3)
                    flush()
            if args.mode == 'smoke':
                if not args.binary:
                    raise ValueError('--binary is required for smoke mode')
                run('executable-smoke', [sys.executable, str(ROOT / 'scripts/smoke_candidate.py'), '--binary', str(args.binary.resolve())])
            else:
                run('go-version', [go, 'version'])
                if args.mode == 'doctor':
                    expected = (ROOT / 'go.mod').read_text().split('\ngo ', 1)[1].splitlines()[0]
                    report['required_go'] = expected
                    for tool in ['git', 'node', 'npm']:
                        run(tool + '-version', [command_path(tool), '--version'])
                else:
                    binary = temporary_root / ('novelforge.exe' if os.name == 'nt' else 'novelforge')
                    run('entry-build', [go, 'build', '-trimpath', '-o', str(binary), './cmd/novelforge'], timeout=600, extra={'CGO_ENABLED': '0'})
                    test_base = [go, 'test', '-buildvcs=false', '-count=1', '-timeout=180s', '-v']
                    if args.mode == 'quick':
                        run('accounting', test_base + ['./internal/observability', '-run', '^TestObservation'])
                        run('connected-short-flows', test_base + ['./internal/server', '-run', SERVER_QUICK], timeout=240)
                        run('lifecycle-migration', test_base + ['./internal/project', '-run', '^TestLifecycle(MigrationKeepsPreimage|RejectsChangedSchemaAndSymlinks)$'])
                        run('api-contracts', test_base + ['./internal/server', '-run', '^TestOpenAPI'], timeout=240)
                    elif args.mode == 'recovery':
                        run('recovery-flows', test_base + ['./internal/server', '-run', RECOVERY], timeout=240)
                        run('version-authority', test_base + ['./internal/chapterversion/...', './internal/server/...', '-run', 'ScenarioB'], timeout=300)
                    elif args.mode == 'scale':
                        run('temporal-index-100k', test_base + ['./internal/truthstore', '-run', '^TestHundredThousandFactTemporalQueryUsesIndex$'], timeout=600, extra={'NOVELFORGE_SCALE_TEST': '1'})
                        run('synthetic-chapter-scale', test_base + ['./internal/server', '-run', '^TestCandidateSyntheticChapterScale$'], timeout=600, extra={'NOVELFORGE_SCALE_TEST': '1'})
                    elif args.mode == 'full':
                        run('vet-all', [go, 'vet', './...'], timeout=900)
                        run('go-full', [go, 'test', '-buildvcs=false', '-count=1', '-timeout=20m', './...'], timeout=1800)
                        run('race-core', [go, 'test', '-race', '-buildvcs=false', '-count=1', '-timeout=20m', './internal/autopilot', './internal/observability', './internal/project', './internal/qualitygate', './internal/truthstore', './internal/narrativeledger', './internal/contextcompiler', './internal/chapterversion/...', './internal/server/...'], timeout=1800, extra={'CGO_ENABLED': '1'})
                    if args.mode in ('quick', 'full') and not args.skip_frontend:
                        npm, npx = command_path('npm'), command_path('npx')
                        run('frontend-install', [npm, 'ci', '--ignore-scripts', '--no-audit', '--no-fund'], ROOT / 'web', timeout=600)
                        run('frontend-types', [npm, 'run', 'check'], ROOT / 'web')
                        run('frontend-tests', [npm, 'test'] if args.mode == 'full' else [npx, 'vitest', 'run', *FRONTEND_QUICK], ROOT / 'web', timeout=600)
                        run('frontend-build', [npm, 'run', 'build'], ROOT / 'web')
                        run('generated-js', [command_path('node'), '--check', 'dist/assets/app.js'], ROOT / 'web')
                        # Exact committed generated assets are the executable's build input.
                        run('frontend-reproducibility', [command_path('git'), 'diff', '--exit-code', '--', 'web/dist', 'web/package-lock.json'])
                        run('rebuilt-entry', [go, 'build', '-trimpath', '-o', str(binary), './cmd/novelforge'], timeout=600, extra={'CGO_ENABLED': '0'})
                    else:
                        report['unexecuted'].append('frontend checks in this invocation')
                    run('default-entry-smoke', [sys.executable, str(ROOT / 'scripts/smoke_candidate.py'), '--binary', str(binary)])
                    run('inspection-entry-smoke', [sys.executable, str(ROOT / 'scripts/smoke_candidate.py'), '--binary', str(binary), '--no-autopilot'])
            report['status'] = 'passed'
        except FileNotFoundError as error:
            report['status'] = 'blocked'
            report['error'] = str(error)
        except (ValueError, OSError, subprocess.SubprocessError) as error:
            report['status'] = 'failed'
            report['error'] = str(error)
        finally:
            report['finished_at'] = dt.datetime.now(dt.timezone.utc).isoformat()
            flush()
    print(f'{report["status"]}: {output / "result.json"}')
    return 0 if report['status'] == 'passed' else 2 if report['status'] == 'blocked' else 1

if __name__ == '__main__':
    raise SystemExit(main())
