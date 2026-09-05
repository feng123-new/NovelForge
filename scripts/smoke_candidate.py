#!/usr/bin/env python3
"""Run the real executable in a fresh workspace, never against a user's novel.

No model is configured or called. Only local HTTP and temporary files are used.
Python 3.10+; standard library only. Works with an extracted release binary.
"""
from __future__ import annotations
import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid
import zipfile
import io


def isolated_environment(home: Path) -> dict[str, str]:
    env = dict(os.environ)
    for key in list(env):
        upper = key.upper()
        if any(word in upper for word in ('API_KEY', 'API_TOKEN', 'AUTH_TOKEN')) or upper.startswith(('NOVELFORGE_', 'AINOVEL_')):
            env.pop(key, None)
    env.update(HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(home / 'config'), XDG_DATA_HOME=str(home / 'data'), APPDATA=str(home / 'appdata'), LOCALAPPDATA=str(home / 'local'))
    return env


def smoke(binary: Path, no_autopilot: bool = False) -> dict:
    binary = binary.resolve(strict=True)
    with tempfile.TemporaryDirectory(prefix='novelforge-smoke-') as temporary:
        root = Path(temporary)
        home = root / 'home'
        home.mkdir()
        env = isolated_environment(home)
        version = subprocess.check_output([str(binary), '--version'], cwd=root, env=env, text=True, timeout=15).strip()
        with socket.socket() as port_socket:
            port_socket.bind(('127.0.0.1', 0))
            port = port_socket.getsockname()[1]
        command = [str(binary), 'server', '--host', '127.0.0.1', '--port', str(port), '--workspace', str(root / 'workspace')]
        if no_autopilot:
            command.append('--no-autopilot')
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        base = f'http://127.0.0.1:{port}'
        def request(path: str, body: dict | None = None):
            headers = {'Accept': 'application/json'}
            data = None
            if body is not None:
                data = json.dumps(body).encode()
                headers.update({'Content-Type': 'application/json', 'Idempotency-Key': 'smoke-' + uuid.uuid4().hex})
            with opener.open(urllib.request.Request(base + path, data=data, headers=headers), timeout=10) as response:
                return response.status, response.headers, response.read(4 * 1024 * 1024)
        with (root / 'server.log').open('wb') as log:
            process = subprocess.Popen(command, cwd=root, env=env, stdout=log, stderr=subprocess.STDOUT)
            try:
                deadline = time.monotonic() + 30
                while True:
                    if process.poll() is not None:
                        raise RuntimeError(f'server exited before readiness (status={process.returncode})')
                    try:
                        status, _, content = request('/api/health')
                        health = json.loads(content)
                        assert status == 200 and health.get('status') == 'ok', 'health response'
                        break
                    except (urllib.error.URLError, TimeoutError, ConnectionError):
                        if time.monotonic() >= deadline:
                            raise RuntimeError('server readiness deadline exceeded')
                        time.sleep(0.1)
                assert request('/')[0] == 200, 'embedded HTML'
                assert len(request('/assets/app.js')[2]) > 1000, 'embedded JavaScript'
                _, _, content = request('/api/projects', {'title': 'Candidate smoke only', 'target_chapters': 2})
                project = json.loads(content)
                project_id = project.get('id')
                assert isinstance(project_id, str) and project_id, 'project creation'
                prefix = '/api/projects/' + project_id
                status, _, content = request(prefix + '/observability')
                observations = json.loads(content)
                assert status == 200 and observations['totals']['calls'] == 0, 'no hidden calls'
                assert observations['settings']['policy']['prices'] == [], 'no invented model price'
                status, _, content = request(prefix + '/observability/diagnostics')
                assert status == 200 and isinstance(json.loads(content)['findings'], list), 'diagnostics'
                status, _, content = request(prefix + '/observability/report')
                assert status == 200 and 'prompts' in json.loads(content)['excludes'], 'redacted report'
                status, _, archive = request(prefix + '/lifecycle/backup')
                assert status == 200, 'portable backup'
                with zipfile.ZipFile(io.BytesIO(archive)) as zipped:
                    assert not any(n.endswith('config.json') or n.endswith('server.db') for n in zipped.namelist()), 'backup credentials or workspace tasks'
                    assert '.novelforge/project.db' in zipped.namelist(), 'project snapshot'
                return {'status': 'passed', 'scope': 'real executable / fresh local workspace / no model calls', 'version': version, 'worker_disabled': no_autopilot, 'checks': ['version', 'health', 'embedded-web', 'project-create', 'observation-state', 'diagnostics', 'redacted-report', 'portable-backup']}
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--no-autopilot', action='store_true', help='also verify the inspection-only entry')
    parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    try:
        result = smoke(args.binary, args.no_autopilot)
    except (OSError, RuntimeError, AssertionError, ValueError, subprocess.SubprocessError, zipfile.BadZipFile) as error:
        result = {'status': 'failed', 'error': str(error), 'scope': 'local no-model executable smoke'}
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result['status'] == 'passed' else 1

if __name__ == '__main__':
    raise SystemExit(main())
