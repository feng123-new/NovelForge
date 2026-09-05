#!/usr/bin/env python3
"""One-round repository cleanup. This controller never enters the product tree."""
from __future__ import annotations
import collections
import hashlib
import json
import os
import pathlib
import posixpath
import re
import subprocess
import sys
import urllib.parse
import urllib.request

REPO = 'feng123-new/NovelForge'
BASE = '48fb03aa1d9d91ee994edc935317d6bbfe6d8b06'
CONTROL = 'ops/phase-01-08-cleanup-20260905'
PRODUCT = 'chore/phase-01-08-cleanup-20260905'
ARCHIVE = 'archive/phase-01-08-cleanup-20260905'
FROZEN = 'feature/phase-09-autopilot'
MOVES = {
    'docs/architecture.md': 'docs/upstream/ainovel-runtime-architecture.md',
    'docs/IMPLEMENTATION_STATUS.md': 'docs/archive/phase-01-08/IMPLEMENTATION_STATUS.md',
    'docs/PHASE_01_08_REVIEW.md': 'docs/archive/phase-01-08/PHASE_01_08_REVIEW.md',
    'docs/PHASE_08_ACCEPTANCE.md': 'docs/archive/phase-01-08/PHASE_08_ACCEPTANCE.md',
    'scripts/novel.png': 'docs/assets/upstream/novel.png',
    'scripts/sample.gif': 'docs/assets/upstream/sample.gif',
}
SYMBOL = 'attachContextCompilerDiagnostics'
TOOL = 'internal/tools/novel_context.go'
LINK = re.compile(r'(?P<prefix>!?\[[^\]\n]*\]\()(?P<url><[^>\n]+>|[^\s()]+)(?P<suffix>(?:[ \t]+[\"\'][^\n]*?[\"\'])?\))')
REFERENCE = re.compile(r'(?m)^(?P<prefix>[ \t]*\[[^\]\n]+\]:[ \t]*)(?P<url><[^>\n]+>|[^\s]+)(?P<suffix>[^\n]*)$')
ROOT = pathlib.Path.cwd()
TMP = pathlib.Path(os.environ.get('RUNNER_TEMP', '/tmp')) / 'nf-cleanup'
TMP.mkdir(parents=True, exist_ok=True)


def git(*args, cwd=None, data=None, check=True):
    p = subprocess.run(['git', *args], cwd=cwd or ROOT, input=data,
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if check and p.returncode:
        raise RuntimeError(f'git {args[0]} failed ({p.returncode}): {p.stderr.decode(errors="replace")}')
    return p


def text(*args, cwd=None):
    return git(*args, cwd=cwd).stdout.decode().strip()


def api(path):
    # Only public repository metadata; no user, organization or secret endpoints.
    req = urllib.request.Request(f'https://api.github.com/repos/{REPO}/{path}', headers={
        'Accept': 'application/vnd.github+json',
        'Authorization': 'Bearer ' + os.environ['GH_TOKEN'],
        'X-GitHub-Api-Version': '2022-11-28',
    })
    with urllib.request.urlopen(req, timeout=30) as response:
        return json.load(response)


def pages(path):
    result = []
    for page in range(1, 11):
        sep = '&' if '?' in path else '?'
        batch = api(f'{path}{sep}per_page=100&page={page}')
        assert isinstance(batch, list), path
        result.extend(batch)
        if len(batch) < 100:
            return result
    raise RuntimeError('metadata pagination exceeded bounded limit')


def ancestor(a, b):
    p = git('merge-base', '--is-ancestor', a, b, check=False)
    assert p.returncode in (0, 1), (a, b, p.stderr.decode())
    return p.returncode == 0


def temp_name(name):
    return name.startswith(('ops/', 'tmp/', 'probe-', '__probe'))


def snapshot():
    branches = pages('branches')
    prs = pages('pulls?state=all')
    active = set()
    delivered = {}
    for pr in prs:
        if pr['state'] == 'open':
            for side in ('head', 'base'):
                ref = pr[side]
                if ref.get('repo') and ref['repo']['full_name'] == REPO:
                    active.add(ref['ref'])
        if (pr.get('merged_at') and pr.get('merge_commit_sha') and
            pr['head'].get('repo') and pr['head']['repo']['full_name'] == REPO and
            ancestor(pr['merge_commit_sha'], BASE)):
            delivered[pr['head']['sha']] = pr['number']
    records = []
    for b in sorted(branches, key=lambda b: b['name']):
        name, sha = b['name'], b['commit']['sha']
        r = {'name': name, 'sha': sha, 'protected': bool(b['protected']), 'decision': 'retain'}
        if name in ('main', FROZEN, CONTROL, PRODUCT) or b['protected'] or name in active:
            r['reason'] = 'protected_active_or_frozen'
        elif ancestor(sha, BASE):
            r.update(decision='delete_after_merge', reason='ancestor_of_main')
        elif sha in delivered:
            r.update(decision='delete_after_merge', reason='exact_delivered_pr_head', pr=delivered[sha])
        else:
            r['reason'] = 'unique_work_retained'
        records.append(r)
    groups = collections.defaultdict(list)
    for r in records:
        groups[r['sha']].append(r)
    for group in groups.values():
        group.sort(key=lambda r: (temp_name(r['name']), len(r['name']), r['name']))
        canonical = group[0]
        for r in group[1:]:
            if r['reason'] == 'unique_work_retained' and temp_name(r['name']):
                r.update(decision='delete_after_merge', reason='duplicate_commit_alias',
                         same_commit_as=canonical['name'])
    for r in records:
        if r['reason'] != 'unique_work_retained' or not temp_name(r['name']):
            continue
        merge_base = text('merge-base', BASE, r['sha'])
        changed = text('diff', '--name-only', merge_base, r['sha']).splitlines()
        if changed and all(p.startswith(('.github/workflows/', '.maintenance/')) for p in changed):
            r.update(decision='delete_after_merge', reason='archived_temporary_workflow_only',
                     changed_paths=changed)
    result = {'schema': 1, 'repository': REPO, 'base': BASE, 'archive_tag': ARCHIVE,
              'rules': ['Never delete main, frozen later-phase, protected or open-PR refs.',
                        'Only merged ancestry, exact delivered PR heads, duplicate temporary aliases, or workflow-only temporary changes qualify.',
                        'Archive every original head as a parent before deleting; recheck exact remote SHA immediately before deletion.'],
              'branches': records}
    (TMP / 'snapshot.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    return result


def resolve_link(url, source):
    raw = url[1:-1] if url.startswith('<') and url.endswith('>') else url
    split = urllib.parse.urlsplit(raw)
    if split.scheme or split.netloc or not split.path or split.path.startswith('/'):
        return None
    target = posixpath.normpath(posixpath.join(posixpath.dirname(source), urllib.parse.unquote(split.path)))
    return target, split


def relocate_links(content, old_source, new_source):
    def replace(m):
        resolved = resolve_link(m['url'], old_source)
        if resolved is None:
            return m.group(0)
        target, split = resolved
        destination = MOVES.get(target, target)
        new = posixpath.relpath(destination, posixpath.dirname(new_source) or '.')
        new = urllib.parse.quote(new, safe='/._-~')
        if split.query:
            new += '?' + split.query
        if split.fragment:
            new += '#' + split.fragment
        if m['url'].startswith('<'):
            new = '<' + new + '>'
        if target not in MOVES and old_source == new_source:
            return m.group(0)
        return m['prefix'] + new + m['suffix']
    content = LINK.sub(replace, content)
    return REFERENCE.sub(replace, content)


def bad_links(content, source, files):
    bad = set()
    for pattern in (LINK, REFERENCE):
        for m in pattern.finditer(content):
            resolved = resolve_link(m['url'], source)
            if resolved is None:
                continue
            target, _ = resolved
            if target not in files and not any(p.startswith(target.rstrip('/') + '/') for p in files):
                bad.add((source, target))
    return bad


def single_replace(content, before, after):
    assert content.count(before) == 1, before[:100]
    return content.replace(before, after, 1)


def prepare():
    assert api('git/ref/heads/main')['object']['sha'] == BASE, 'main advanced; review a new base'
    snap = snapshot()
    original_count = sum(r['name'] != CONTROL for r in snap['branches'])
    proposed = [r for r in snap['branches'] if r['decision'] == 'delete_after_merge']
    print('BRANCH_PLAN=' + json.dumps({'before': original_count, 'delete_candidates': len(proposed),
                                      'retained': [r['name'] for r in snap['branches'] if r['decision'] == 'retain' and r['name'] != CONTROL]}, ensure_ascii=False))
    root = TMP / 'product'
    git('worktree', 'add', '--detach', str(root), BASE)
    tracked = git('ls-files', '-z', cwd=root).stdout.decode().split('\0')[:-1]
    before = {p: (root / p).read_bytes() for p in tracked}
    assert hashlib.sha1(b'blob ' + str(len(before[TOOL])).encode() + b'\0' + before[TOOL]).hexdigest() == '0b740474be0ac7db868bed989e01ff75357033d2'
    references = [(p, len(re.findall(r'\b' + SYMBOL + r'\b', data.decode())))
                  for p, data in before.items() if p.endswith('.go') and SYMBOL.encode() in data]
    assert references == [(TOOL, 1)], references
    print('UNUSED_FUNCTION_REFS=' + json.dumps(references))
    old_code = before[TOOL].decode()
    start = old_code.index('func ' + SYMBOL + '(')
    end = old_code.index('// buildLoadingSummary', start)
    dead = old_code[start:end]
    assert dead.rstrip().endswith('}')
    (root / TOOL).write_text(old_code[:start] + old_code[end:])
    print('REMOVED_FUNCTION_LINES=' + str(len(dead.splitlines())))
    for old, new in MOVES.items():
        assert old in before and not (root / new).exists(), (old, new)
        (root / new).parent.mkdir(parents=True, exist_ok=True)
        (root / old).rename(root / new)
    baseline_bad = set()
    for old, raw in before.items():
        new = MOVES.get(old, old)
        if not old.endswith('.md'):
            continue
        content = raw.decode()
        baseline_bad |= {(MOVES.get(s, s), MOVES.get(t, t)) for s, t in bad_links(content, old, set(tracked))}
        content = relocate_links(content, old, new)
        # Explicit repository-root references in inline code are not Markdown links.
        def code_ref(m):
            value = m[1]
            for src, dst in MOVES.items():
                value = value.replace(src, dst)
            return '`' + value + '`'
        content = re.sub(r'`([^`\n]+)`', code_ref, content)
        (root / new).write_text(content)
    # Go comments may refer to the old architecture path; never change executable lines.
    comment_updates = []
    for path, raw in before.items():
        if not path.endswith('.go'):
            continue
        current = (root / path).read_text()
        lines = current.splitlines(keepends=True)
        changed = False
        for i, line in enumerate(lines):
            if line.lstrip().startswith('//'):
                revised = line
                for src, dst in MOVES.items():
                    revised = revised.replace(src, dst)
                changed |= revised != line
                lines[i] = revised
        if changed:
            (root / path).write_text(''.join(lines))
            comment_updates.append(path)
    path = root / 'docs/CONTEXT_COMPILER.md'
    content = path.read_text()
    content = single_replace(content,
        '> Maintenance update (2026-09-05): `novel_context` now returns compiler-selected records, and Web Writer hashes include compiled project context. Compilation errors stop the call. Additive Migration 8 supports character search without changing Migration 5. Older diagnostic-only integration notes below describe historical Phase 7 delivery; see [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md).',
        '> Current integration: compiler-selected context is used by `novel_context` and the configured Web Writer. These are Phase 1–8 paths; later-phase workers remain deferred. Delivery evidence and limitations are in [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md).')
    content = single_replace(content,
        'The existing `novel_context` path calls `CompileLegacyMap` and emits a compact `_context_compiler` diagnostic object. Existing fields and regression tests remain intact, allowing consumers to migrate incrementally rather than replacing the mature context path in one destructive change.',
        '`novel_context` calls `CompileLegacyMap` followed by `SelectLegacyMap`: only selected records are returned, preserving their legacy field shape. Compilation errors stop the request rather than returning the original unselected map. `_context_compiler` reports `version: 2` and `status: applied`; detailed layer diagnostics are optional and are compacted before content is trimmed again when the byte budget is tight. `_loading_summary` and `_trimmed` describe the actual returned selection.\n\nThe configured Web Writer separately receives compiled project Truth, POV-filtered Ledger, recent documents and FTS evidence as `compiled_context`, before the idempotent model-request hash is calculated. The adapters retain estimated token budgets and bounded queries; this does not imply that every legacy Agent path or whole-book retrieval has been revalidated.')
    sentence = 'Migration 5 adds `context_documents`, its project/chapter index, an external-content FTS5 table, and synchronization triggers. Searches are project-scoped, bounded, stable, and include `source_chapter <= Chapter N`; future chapters and other projects cannot leak into the result.'
    content = single_replace(content, sentence, sentence + '\n\nAdditive Migration 8 maintains a separate character index for Chinese substring terms, including two-character names. The original text and English token index remain. Backfill and synchronization triggers use the deterministic Go-registered `novelforge_search_characters` function. External SQLite clients must not write `context_documents` without registering that function. Alias, simplified/traditional conversion and synonym recall are not guaranteed by this index.')
    path.write_text(content)
    path = root / 'docs/ARCHITECTURE.md'
    path.write_text(single_replace(path.read_text(), '# NovelForge Architecture\n', '# NovelForge Architecture\n\nThis document describes NovelForge architecture and design targets. For current delivery status use the [documentation index](README.md); the retained [ainovel runtime architecture](upstream/ainovel-runtime-architecture.md) documents the existing TUI/Headless runtime, not a competing current specification.\n'))
    path = root / 'docs/PHASE_01_08_FIXES.md'
    content = path.read_text()
    content = single_replace(content, '本轮通过 GitHub 维护分支提交源码修复；是否已合并以修复 PR 和主分支状态为准。原修复包中“连接器只读”“未推送”是此前记录，不是本轮的工具或交付状态。', '五项源码修复已通过 PR #35 合并，合并提交为 `48fb03aa1d9d91ee994edc935317d6bbfe6d8b06`。对应产品树 `582edd5d920e4f1346d2b57073cfd3e9c64a6c9c` 在 Actions `33939847858` 完成入口构建和 11 个命名测试。原修复包中“连接器只读”“未推送”是此前记录，不是当前交付状态。后续目录清理见[文档导航](README.md)，不重新计作功能验收。')
    path.write_text(content)
    path = root / 'README.md'
    path.write_text(single_replace(path.read_text(), '## 当前维护范围\n', '## 当前维护范围\n\n[文档导航与当前维护记录](docs/README.md) · [历史交付档案](docs/archive/README.md)\n'))
    (root / 'docs/README.md').write_text(f'''# NovelForge 文档导航

当前只维护 Phase 1–8，Phase 9–13 继续暂停。源码功能交付、局部验证与正式发布是不同状态。

## 当前使用与设计

| 主题 | 当前入口 |
| --- | --- |
| 安装、配置与使用 | [项目 README](../README.md)、[Web 工作台](WEB.md)、[配置与迁移](MIGRATION.md) |
| 项目和接口 | [Project API](PROJECT_API.md)、[API 说明](API.md) |
| 事实与叙事 | [Truth Store](TRUTH_STORE.md)、[Narrative Ledger](NARRATIVE_LEDGER.md) |
| 创作与版本 | [Quality Gate](QUALITY_GATE.md)、[ChapterVersion](CHAPTER_VERSIONS.md)、[Agent 边界](AGENTS.md) |
| 上下文 | [Context Compiler](CONTEXT_COMPILER.md) |
| 架构与演进 | [NovelForge 架构](ARCHITECTURE.md)、[保留的上游运行时](upstream/ainovel-runtime-architecture.md)、[上游同步](UPSTREAM_SYNC.md) |
| 当前范围与修复 | [路线图](ROADMAP.md)、[五项源码修复](PHASE_01_08_FIXES.md) |
| 历史证据 | [归档导航](archive/README.md)；不以历史 CI 冒充当前运行结果 |

## 2026-09-05 目录与冗余清理

基线：`{BASE}`。本轮不改变生成、审核、定稿、迁移、模型配置或后续阶段功能。

将旧运行时架构移动到独立名称，消除仅大小写不同的文件路径；历史交付、早期复核与 Phase 8 验收归档；两份上游演示素材移动到 `docs/assets/upstream/`，内容不删减。同步仓库内文档链接和源码注释中的显式路径。删除经全仓 Go 引用检查确认只有定义的旧诊断辅助函数，保留现用编译、诊断、字节预算与测试。

清理前业务仓库共有 {original_count} 个分支；静态规则选出 {len(proposed)} 个候选。删除前将完整分支清单、提交对象和理由保存到独立归档标签 `{ARCHIVE}`。只有本 PR 合并后才执行删除，并重新核对 HEAD、保护状态和未关闭 PR；发生变化的分支保留。独有且未确认可清理的实现以及 Phase 9 分支不动。实际删除数量以清理 PR 的操作记录为准，不把计划数量冒充已完成。

验证限于文件路径大小写检查、移动后链接检查、素材字节一致性、无引用检查、入口编译及两个上下文命名测试；实际结果在清理 PR 记录。没有全量测试、前端重建、数据库迁移或历史改写。`web/dist`、兼容入口、依赖锁、许可证、旧迁移及既有 CI 均保留。临时清理控制器不进入主分支。
''')
    (root / 'docs/archive/README.md').write_text(f'''# 历史档案

[返回当前文档导航](../README.md)。本目录保留当时的事实和验证范围，不将旧的 complete、未修复或后续交接说明当作当前操作指令。

| 档案 | 用途 |
| --- | --- |
| [阶段交付记录](phase-01-08/IMPLEMENTATION_STATUS.md) | 保留历史 PR、提交和 CI 证据 |
| [前八阶段原始复核](phase-01-08/PHASE_01_08_REVIEW.md) | 保留 `5dbcadfa` 基线的断点与回补计划 |
| [Phase 8 验收](phase-01-08/PHASE_08_ACCEPTANCE.md) | 保留章节版本阶段证据 |

当前五项修复状态见 [PHASE_01_08_FIXES.md](../PHASE_01_08_FIXES.md)。归档只调整位置和本地引用，不删除验收内容。上游运行时架构位于 [upstream](../upstream/ainovel-runtime-architecture.md)，仍用于解释保留的 TUI/Headless 路径。

## 分支快照与恢复

清理使用独立 Git 标签 `{ARCHIVE}`，不创建 Release，也不改写 main 历史。标签指向专用档案提交：树中保存 `branch-snapshot.json`，父提交保留全部原分支 HEAD 以及本轮清理提交，避免 squash 合并或删除最后一个引用导致旧实现失去持久引用。

读取档案：

```sh
git fetch origin refs/tags/{ARCHIVE}:refs/tags/{ARCHIVE}
git show {ARCHIVE}:branch-snapshot.json
```

按快照中记录的完整 SHA 新建恢复分支即可。不要强制覆盖已有同名分支。没有确认可清理的独有实现、受保护分支、仍被开放 PR 使用的分支和冻结的后续阶段分支均不删除。实际分支操作结果保存在清理 PR 讨论记录中。
''')
    # Only dead-function deletion and comment path updates are permitted Go changes.
    for p in tracked:
        if p.endswith('.go'):
            expected = before[p].decode()
            if p == TOOL:
                expected = old_code[:start] + old_code[end:]
            for src, dst in MOVES.items():
                expected = ''.join(line.replace(src, dst) if line.lstrip().startswith('//') else line
                                   for line in expected.splitlines(keepends=True))
            assert (root / p).read_text() == expected, p
    git('add', '-A', cwd=root)
    paths = git('ls-files', '-z', cwd=root).stdout.decode().split('\0')[:-1]
    lower = collections.defaultdict(list)
    for p in paths:
        lower[p.casefold()].append(p)
    assert not [v for v in lower.values() if len(v) > 1], 'case-fold path collision'
    after_bad = set()
    for p in paths:
        if p.endswith('.md'):
            after_bad |= bad_links((root/p).read_text(), p, set(paths))
    new_bad = after_bad - baseline_bad
    assert not new_bad, ('new broken local Markdown paths', sorted(new_bad))
    # Every moved document link that was valid must still resolve; existing bad links are reported.
    for old, new in MOVES.items():
        if old.endswith(('.png', '.gif')):
            assert (root / new).read_bytes() == before[old], old
    assert not any(SYMBOL.encode() in (root/p).read_bytes() for p in paths if p.endswith('.go'))
    immutable = ['.github', 'web', 'go.mod', 'go.sum', 'LICENSE', 'THIRD_PARTY_NOTICES.md',
                 'internal/db', 'internal/compat', 'cmd/ainovel-cli']
    git('diff', '--cached', '--exit-code', BASE, '--', *immutable, cwd=root)
    git('diff', '--cached', '--check', cwd=root)
    changed = text('diff', '--cached', '--name-only', cwd=root).splitlines()
    for p in changed:
        assert p.endswith('.md') or p in MOVES or p in MOVES.values() or p == TOOL or p in comment_updates, p
    report = {'base': BASE, 'branch_count_before': original_count, 'branch_candidates': len(proposed),
              'removed_function_lines': len(dead.splitlines()), 'moves': MOVES,
              'go_comment_updates': comment_updates, 'new_broken_local_links': 0,
              'preexisting_broken_local_links': sorted(after_bad), 'changed_paths': changed}
    (TMP / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print('STATIC_REPORT=' + json.dumps(report, ensure_ascii=False))


def publish():
    root = TMP / 'product'
    assert api('git/ref/heads/main')['object']['sha'] == BASE
    assert not text('diff', '--name-only', cwd=root), 'unrecorded changes after checks'
    assert not text('ls-remote', '--heads', 'origin', PRODUCT)
    assert not text('ls-remote', '--tags', 'origin', 'refs/tags/' + ARCHIVE)
    git('config', 'user.name', 'NovelForge maintenance', cwd=root)
    git('config', 'user.email', '203426027+feng123-new@users.noreply.github.com', cwd=root)
    git('commit', '-m', 'chore: clean Phase 1-8 docs and unused diagnostics [skip ci]',
        '-m', 'Archive historical documentation, resolve case-only paths, relocate preserved media, remove an unreferenced helper. Limited entry build and named tests only. Phase 9-13 remain frozen.', cwd=root)
    product_sha = text('rev-parse', 'HEAD', cwd=root)
    product_tree = text('rev-parse', 'HEAD^{tree}', cwd=root)
    snap = json.loads((TMP / 'snapshot.json').read_text())
    snap['cleanup_product_commit'] = product_sha
    snap['cleanup_product_tree'] = product_tree
    content = (json.dumps(snap, ensure_ascii=False, indent=2)+'\n').encode()
    blob = git('hash-object', '-w', '--stdin', cwd=root, data=content).stdout.decode().strip()
    archive_tree = git('mktree', cwd=root, data=f'100644 blob {blob}\tbranch-snapshot.json\n'.encode()).stdout.decode().strip()
    parents = sorted({r['sha'] for r in snap['branches']} | {BASE, product_sha})
    args = ['commit-tree', archive_tree]
    for sha in parents:
        args += ['-p', sha]
    archive_sha = git(*args, cwd=root, data=b'Archive Phase 1-8 branch heads and cleanup plan; not a product merge.\n').stdout.decode().strip()
    # Both new refs must be absent; never overwrite an existing branch or tag.
    git('push', '--atomic', f'--force-with-lease=refs/heads/{PRODUCT}:',
        f'--force-with-lease=refs/tags/{ARCHIVE}:', 'origin',
        f'{product_sha}:refs/heads/{PRODUCT}', f'{archive_sha}:refs/tags/{ARCHIVE}', cwd=root)
    print('PUBLISHED=' + json.dumps({'commit': product_sha, 'tree': product_tree,
                                    'archive_tag': ARCHIVE, 'archive_commit': archive_sha}))
    print(text('diff', '--stat', BASE, 'HEAD', cwd=root))
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as stream:
        stream.write(f'## Cleanup prepared\nProduct: `{product_sha}`\nTree: `{product_tree}`\nArchive: `{ARCHIVE}` → `{archive_sha}`\n\nOnly entry build and two named context tests were run. No branch deletion until the product PR is merged.\n')


def prune():
    candidates = pages('pulls?state=closed&head=feng123-new:' + PRODUCT)
    merged = [p for p in candidates if p.get('merged_at') and p['head']['ref'] == PRODUCT]
    assert len(merged) == 1, 'cleanup PR not uniquely merged'
    pr = merged[0]
    current_main = api('git/ref/heads/main')['object']['sha']
    assert current_main == pr['merge_commit_sha'], 'main changed since cleanup merge; review before deleting refs'
    archive_sha = text('rev-parse', 'refs/tags/' + ARCHIVE)
    snap = json.loads(text('show', f'{archive_sha}:branch-snapshot.json'))
    assert snap['base'] == BASE and snap['repository'] == REPO
    assert pr['head']['sha'] == snap['cleanup_product_commit']
    assert text('rev-parse', current_main+'^{tree}') == snap['cleanup_product_tree']
    current = {b['name']: b for b in pages('branches')}
    active = set()
    for p in pages('pulls?state=open'):
        for side in ('head', 'base'):
            if p[side].get('repo') and p[side]['repo']['full_name'] == REPO:
                active.add(p[side]['ref'])
    planned = [r for r in snap['branches'] if r['decision'] == 'delete_after_merge']
    control = next(r for r in snap['branches'] if r['name'] == CONTROL)
    planned += [dict(control, reason='completed_cleanup_controller'),
                {'name': PRODUCT, 'sha': snap['cleanup_product_commit'], 'reason': 'merged_cleanup_pr'}]
    deletions, skipped = [], []
    for r in planned:
        b = current.get(r['name'])
        if (not b or b['protected'] or r['name'] in active or b['commit']['sha'] != r['sha']):
            skipped.append(r['name'])
            continue
        assert r['name'] not in ('main', FROZEN)
        assert ancestor(r['sha'], archive_sha), 'head is not anchored in immutable archive'
        git('check-ref-format', 'refs/heads/' + r['name'])
        deletions.append(r)
    assert deletions, 'no safe refs remain eligible'
    command = ['push', '--atomic']
    command += [f'--force-with-lease=refs/heads/{r["name"]}:{r["sha"]}' for r in deletions]
    command += ['origin'] + [':refs/heads/' + r['name'] for r in deletions]
    # Exact-ref leases reject concurrent ref moves. No wildcard or history rewrite.
    git(*command)
    remaining = pages('branches')
    remaining_names = {b['name'] for b in remaining}
    assert not remaining_names.intersection(r['name'] for r in deletions)
    assert api('git/ref/heads/main')['object']['sha'] == current_main
    assert api('git/ref/heads/'+FROZEN)['object']['sha'] == next(r['sha'] for r in snap['branches'] if r['name'] == FROZEN)
    report = {'pr': pr['number'], 'main': current_main, 'archive': archive_sha,
              'deleted': deletions, 'skipped_changed_or_active': skipped,
              'remaining_count': len(remaining), 'remaining': sorted(remaining_names)}
    print('PRUNE_RESULT=' + json.dumps(report, ensure_ascii=False))
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as stream:
        stream.write(f'## Branch cleanup complete\nPR #{pr["number"]}; main `{current_main}`.\nDeleted {len(deletions)} refs with exact-head leases; {len(remaining)} branches remain. Archive `{ARCHIVE}` preserves original commits.\n\n```json\n'+json.dumps(report, ensure_ascii=False, indent=2)+'\n```\n')


def mode():
    existing = text('ls-remote', '--heads', 'origin', PRODUCT)
    chosen = 'prune' if existing else 'prepare'
    with open(os.environ['GITHUB_OUTPUT'], 'a') as stream:
        stream.write('mode='+chosen+'\n')
    print('MODE='+chosen)


if __name__ == '__main__':
    action = sys.argv[1]
    assert action in ('mode', 'prepare', 'publish', 'prune')
    globals()[action]()
