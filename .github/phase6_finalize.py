#!/usr/bin/env python3
from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def write(path: str, value: str) -> None:
    target = ROOT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(value, encoding="utf-8")


def add_go_import(source: str, import_path: str) -> str:
    quoted = f'"{import_path}"'
    if quoted in source:
        return source
    block = re.search(r"(?ms)^import\s*\((.*?)^\)", source)
    if block:
        insert_at = block.end() - 1
        return source[:insert_at] + f'\t{quoted}\n' + source[insert_at:]
    single = re.search(r'(?m)^import\s+"([^"]+)"\s*$', source)
    if single:
        existing = single.group(1)
        replacement = f'import (\n\t"{existing}"\n\t{quoted}\n)'
        return source[: single.start()] + replacement + source[single.end() :]
    package = re.search(r"(?m)^package\s+\w+\s*$", source)
    if not package:
        raise RuntimeError("Go package declaration not found")
    return source[: package.end()] + f"\n\nimport {quoted}" + source[package.end() :]


def server_project_field() -> str:
    combined = "\n".join(path.read_text(encoding="utf-8") for path in (ROOT / "internal/server").glob("*.go"))
    struct = re.search(r"(?ms)type\s+Server\s+struct\s*\{(.*?)^\}", combined)
    if struct:
        match = re.search(r"(?m)^\s*([A-Za-z_]\w*)\s+\*project\.Repository\b", struct.group(1))
        if match:
            return match.group(1)
    counts: dict[str, int] = {}
    for field in re.findall(r"\bs\.([A-Za-z_]\w*)\.(?:Get|Create|Update|Delete|List|Open)\(", combined):
        counts[field] = counts.get(field, 0) + 1
    if counts:
        return max(counts, key=counts.get)
    raise RuntimeError("unable to identify Server project repository field")


def find_route(fragment: str):
    for path in sorted((ROOT / "internal/server").glob("*.go")):
        lines = path.read_text(encoding="utf-8").splitlines()
        for index, line in enumerate(lines):
            if fragment in line:
                handler = re.search(r"\b([A-Za-z_]\w*)\.(handle[A-Za-z0-9_]+)\b", line)
                if not handler:
                    handler = re.search(r"\b(handle[A-Za-z0-9_]+)\b", line)
                    if not handler:
                        continue
                    receiver, name = "s", handler.group(1)
                else:
                    receiver, name = handler.group(1), handler.group(2)
                return path, lines, index, line, receiver, name
    return None


def register_routes() -> None:
    marker = "/api/projects/{id}/foreshadows"
    if any(marker in path.read_text(encoding="utf-8") for path in (ROOT / "internal/server").glob("*.go") if path.name != "narrative_ledger.go"):
        return
    found = find_route("/api/projects/{id}/chapters/{chapter}/finalize")
    if not found:
        found = find_route("/api/projects/{id}/chapters/{chapter}/generate")
    if not found:
        raise RuntimeError("could not find project API route registration anchor")
    path, lines, index, line, receiver, _ = found
    indent = line[: len(line) - len(line.lstrip())]
    router_match = re.search(r"\b([A-Za-z_]\w*)\.(HandleFunc|Handle|MethodFunc|Get|Post|Patch)\(", line)
    if not router_match:
        raise RuntimeError(f"unsupported route registration style: {line}")
    router, style = router_match.group(1), router_match.group(2)
    specs = [
        ("GET", "/api/projects/{id}/foreshadows", "handleForeshadows"),
        ("POST", "/api/projects/{id}/foreshadows", "handleForeshadows"),
        ("GET", "/api/projects/{id}/foreshadows/{key}", "handleForeshadow"),
        ("PATCH", "/api/projects/{id}/foreshadows/{key}", "handleForeshadow"),
        ("GET", "/api/projects/{id}/secrets", "handleSecrets"),
        ("POST", "/api/projects/{id}/secrets", "handleSecrets"),
        ("GET", "/api/projects/{id}/secrets/{key}", "handleSecret"),
        ("PATCH", "/api/projects/{id}/secrets/{key}", "handleSecret"),
        ("GET", "/api/projects/{id}/ledger/planner-context", "handleLedgerPlannerContext"),
        ("GET", "/api/projects/{id}/ledger/dashboard", "handleLedgerDashboard"),
        ("GET", "/api/projects/{id}/ledger/diagnostics", "handleLedgerDiagnostics"),
    ]
    additions: list[str] = []
    for method, route, handler in specs:
        if style == "HandleFunc":
            additions.append(f'{indent}{router}.HandleFunc("{method} {route}", {receiver}.{handler})')
        elif style == "Handle":
            additions.append(f'{indent}{router}.Handle("{method} {route}", http.HandlerFunc({receiver}.{handler}))')
        elif style == "MethodFunc":
            additions.append(f'{indent}{router}.MethodFunc(http.Method{method.title()}, "{route}", {receiver}.{handler})')
        elif style in {"Get", "Post", "Patch"}:
            call = {"GET": "Get", "POST": "Post", "PATCH": "Patch"}[method]
            additions.append(f'{indent}{router}.{call}("{route}", {receiver}.{handler})')
        else:
            raise RuntimeError(f"unsupported route style {style}")
    lines[index + 1 : index + 1] = additions
    write(str(path.relative_to(ROOT)), "\n".join(lines) + "\n")


def function_location(name: str):
    pattern = re.compile(rf"(?m)^func\s+\([^\n]*\)\s+{re.escape(name)}\s*\(([^)]*)\)[^\{{]*\{{")
    for path in sorted((ROOT / "internal/server").glob("*.go")):
        source = path.read_text(encoding="utf-8")
        match = pattern.search(source)
        if match:
            open_brace = source.find("{", match.start(), match.end())
            depth = 0
            in_string = False
            escaped = False
            for index in range(open_brace, len(source)):
                char = source[index]
                if in_string:
                    if escaped:
                        escaped = False
                    elif char == "\\":
                        escaped = True
                    elif char == '"':
                        in_string = False
                    continue
                if char == '"':
                    in_string = True
                elif char == "{":
                    depth += 1
                elif char == "}":
                    depth -= 1
                    if depth == 0:
                        return path, source, match, open_brace, index
    return None


def request_parameter(arguments: str) -> str:
    match = re.search(r"([A-Za-z_]\w*)\s+\*http\.Request", arguments)
    if not match:
        raise RuntimeError(f"request parameter not found in {arguments!r}")
    return match.group(1)


def patch_handler(name: str, block: str, needs_quality_import: bool = False) -> None:
    location = function_location(name)
    if not location:
        raise RuntimeError(f"handler {name} was not found")
    path, source, match, open_brace, _ = location
    if block.strip() in source[open_brace : open_brace + 1600]:
        return
    request = request_parameter(match.group(1))
    body = block.replace("$REQUEST", request)
    indentation = "\n\t"
    source = source[: open_brace + 1] + indentation + body.replace("\n", "\n\t") + source[open_brace + 1 :]
    if needs_quality_import:
        source = add_go_import(source, "github.com/voocel/ainovel-cli/internal/qualitygate")
    write(str(path.relative_to(ROOT)), source)


def patch_quality_handlers(project_field: str) -> None:
    generate = find_route("/api/projects/{id}/chapters/{chapter}/generate")
    if generate:
        _, _, _, _, _, handler = generate
        patch_handler(
            handler,
            f'''if ledgerContext, ledgerErr := s.{project_field}.NarrativePlannerContextForPath($REQUEST.Context(), $REQUEST.PathValue("id"), $REQUEST.PathValue("chapter")); ledgerErr == nil && ledgerContext != "" {{
	$REQUEST = $REQUEST.WithContext(qualitygate.WithNarrativeLedgerContext($REQUEST.Context(), ledgerContext))
}}''',
            needs_quality_import=True,
        )
    finalize = find_route("/api/projects/{id}/chapters/{chapter}/finalize")
    if finalize:
        _, _, _, _, _, handler = finalize
        patch_handler(
            handler,
            f'''defer func() {{
	_ = s.{project_field}.SyncNarrativeLedger($REQUEST.Context(), $REQUEST.PathValue("id"))
}}()''',
        )


def patch_planner_calls() -> int:
    changed = 0
    for path in sorted((ROOT / "internal/qualitygate").glob("*.go")):
        if path.name in {"narrative_context.go", "narrative_context_test.go"} or path.name.endswith("_test.go"):
            continue
        lines = path.read_text(encoding="utf-8").splitlines()
        output: list[str] = []
        index = 0
        file_changed = False
        while index < len(lines):
            line = lines[index]
            chunk = " ".join(part.strip() for part in lines[index : min(index + 6, len(lines))])
            call = re.search(r"\.(?:Plan|PlanChapter|BuildPlan)\(\s*([A-Za-z_]\w*)\s*,\s*(&?)([A-Za-z_]\w*)", chunk)
            should_patch = call and ("planner" in chunk.lower() or ".Plan(" in chunk)
            previous = output[-1].strip() if output else ""
            if should_patch and "injectNarrativeLedgerContext" not in previous:
                indent = line[: len(line) - len(line.lstrip())]
                context_name = call.group(1)
                argument_name = call.group(3)
                output.append(f"{indent}injectNarrativeLedgerContext({context_name}, &{argument_name})")
                file_changed = True
                changed += 1
            output.append(line)
            index += 1
        if file_changed:
            write(str(path.relative_to(ROOT)), "\n".join(output) + "\n")
    return changed


def patch_context_reflection() -> None:
    path = "internal/qualitygate/narrative_context.go"
    source = read(path)
    old = '''\tvalue = value.Elem()
\tif value.Kind() == reflect.Map'''
    new = '''\tfor value.Kind() == reflect.Pointer {
\t\tif value.IsNil() {
\t\t\treturn false
\t\t}
\t\tvalue = value.Elem()
\t}
\tif value.Kind() == reflect.Map'''
    if old in source:
        source = source.replace(old, new, 1)
        write(path, source)


def merge_openapi() -> None:
    phase5_path = ROOT / "internal/server/openapi_phase5.json"
    phase6_path = ROOT / "internal/server/openapi_phase6.json"
    if not phase6_path.exists():
        return
    phase5 = json.loads(phase5_path.read_text(encoding="utf-8"))
    phase6 = json.loads(phase6_path.read_text(encoding="utf-8"))

    def merge(left, right, location=""):
        for key, value in right.items():
            where = f"{location}/{key}"
            if key not in left:
                left[key] = value
            elif isinstance(left[key], dict) and isinstance(value, dict):
                merge(left[key], value, where)
            elif left[key] != value:
                raise RuntimeError(f"OpenAPI collision at {where}")

    merge(phase5, phase6)
    phase5_path.write_text(json.dumps(phase5, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    phase6_path.unlink()


def patch_chapters() -> None:
    path = ROOT / "web/src/pages/Chapters.svelte"
    source = path.read_text(encoding="utf-8")
    if "NarrativeLedgerDashboard" in source:
        return
    script = re.search(r"<script(?:\s+lang=\"ts\")?[^>]*>", source)
    if not script:
        raise RuntimeError("Chapters.svelte script block not found")
    imports = '''
  import NarrativeLedgerDashboard from './NarrativeLedgerDashboard.svelte'
  import Foreshadows from './Foreshadows.svelte'
  import Secrets from './Secrets.svelte'
'''
    source = source[: script.end()] + imports + source[script.end() :]
    expression = None
    for candidate in ("projectId", "projectID", "id"):
        if re.search(rf"\bexport\s+let\s+{candidate}\b", source) or re.search(rf"\b{candidate}\s*[:=]", source):
            expression = candidate
            break
    if expression is None and re.search(r"\bexport\s+let\s+project\b", source):
        expression = "project.id"
    if expression is None:
        raise RuntimeError("project identifier not found in Chapters.svelte")
    components = f'''

<div class="narrative-ledger-workspace">
  <NarrativeLedgerDashboard projectId={{{expression}}} />
  <Foreshadows projectId={{{expression}}} />
  <Secrets projectId={{{expression}}} />
</div>
'''
    style = source.rfind("<style")
    if style < 0:
        source += components
    else:
        source = source[:style] + components + "\n" + source[style:]
    path.write_text(source, encoding="utf-8")


def patch_ci() -> None:
    path = ROOT / ".github/workflows/ci.yml"
    lines = path.read_text(encoding="utf-8").splitlines()
    for index, line in enumerate(lines):
        if "go test" in line and "-race" in line and "internal/narrativeledger" not in line:
            lines[index] = line.rstrip() + " ./internal/narrativeledger"
    if not any("Narrative Ledger 100k schedule index gate" in line for line in lines):
        anchor = next((i for i, line in enumerate(lines) if "Validate OpenAPI" in line), None)
        if anchor is None:
            anchor = next((i for i, line in enumerate(lines) if line.lstrip().startswith("- name:") and i > 0), len(lines))
        indent = lines[anchor][: len(lines[anchor]) - len(lines[anchor].lstrip())] if anchor < len(lines) else "      "
        step = [
            f"{indent}- name: Narrative Ledger 100k schedule index gate",
            f"{indent}  run: GOWORK=off go test -buildvcs=false -count=1 ./internal/narrativeledger -run '^TestScheduleIndexGate100K$'",
        ]
        lines[anchor:anchor] = step
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def patch_status() -> None:
    path = ROOT / "docs/IMPLEMENTATION_STATUS.md"
    source = path.read_text(encoding="utf-8")
    old = "| 6–13 | not started | Phase 6 begins from the latest accepted main; no Phase 6 production work is included in Phase 5. |"
    replacement = "| 6 — Narrative Ledger | implementation complete; acceptance pending | Production, migration, API, Web and tests are on `feature/phase-06-narrative-ledger`; exact-head PR CI, merge and merged-main CI remain pending. |\n| 7–13 | not started | Phase 7 begins only after Phase 6 merged-main acceptance. |"
    if old in source:
        source = source.replace(old, replacement, 1)
    if "Phase 6 implementation is on `feature/phase-06-narrative-ledger`" not in source:
        state_anchor = "## Phase status"
        source = source.replace(state_anchor, "- Phase 6 implementation is on `feature/phase-06-narrative-ledger`; acceptance evidence is pending.\n\n" + state_anchor, 1)
    if "## Phase 6" not in source:
        source += '''

## Phase 6

### Completed

- Added Migration 4 with authoritative ledger commits, foreshadows, immutable events, secrets, temporal holder intervals, computed status views and blocking indexes.
- Added deterministic foreshadow lifecycle validation and computed `OVERDUE` without storing a synthetic lifecycle state.
- Added a separate Secret model with Chapter-N holder and public-state queries that prevent future knowledge leakage.
- Reconciled only completed Phase 5 transactions and their accepted Fact Proposals; Draft and failed transactions cannot update the ledger.
- Added source transaction/content-hash idempotency so Finalize replay cannot duplicate ledger commits or events.
- Added request-scoped Planner injection containing every CRITICAL and OVERDUE item plus bounded UPCOMING items.
- Added real dashboard counts and stable diagnostics.
- Added strict, idempotent REST routes, composed OpenAPI 3.1 coverage and Chapters workspace pages for Dashboard, Foreshadows and Secrets.
- Added Scenario E, temporal boundary, lifecycle, pagination, replay, race, OpenAPI, Web and 100,000-row index tests.

### Database / Migration Changes

Project Migration 4 `narrative_ledger` adds:

```text
narrative_ledger_commits
foreshadows
foreshadow_events
secrets
secret_knowledge
secret_events
narrative_ledger_current_chapter (view)
foreshadow_status_view (view)
secret_status_view (view)
```

### Test Results

- Exact final Phase 6 PR CI: pending.
- Squash merge: pending.
- Merge-triggered main CI: pending.

### Known Issues

- Phase 7 Context Compiler, FTS5 and RAG are intentionally outside this Phase 6 branch.

### Next Phase

`feature/phase-07-context-compiler` after Phase 6 merged-main acceptance.

### Feature Branch

`feature/phase-06-narrative-ledger`

### Final Head Commit

Pending exact final-head CI.

### Pull Request

Pending.

### PR CI Result

Pending exact final-head workflow.

### Main Merge Commit

Pending.

### Main CI Result

Pending.
'''
    path.write_text(source, encoding="utf-8")


def append_docs() -> None:
    additions = {
        "docs/API.md": '''

## Narrative Ledger (Phase 6)

The foreshadow, secret, planner-context, dashboard and diagnostics routes are documented in the composed OpenAPI 3.1 document. All ledger writes require `Idempotency-Key`, strict one-object JSON and local human authority. Accepted generated changes are synchronized only from completed Phase 5 transactions.
''',
        "docs/ARCHITECTURE.md": '''

## Phase 6 Narrative Ledger

The Narrative Ledger is an authoritative temporal projection beside the Truth Store. It accepts only completed Final-candidate proposals or explicit human events, records immutable provenance, and never treats retrieval as authority. Foreshadow `OVERDUE` and Secret holder/public state are Chapter-N views. See `NARRATIVE_LEDGER.md`.
''',
        "docs/ENGINE.md": '''

## Narrative Ledger handoff

After the Phase 5 Final → Truth → chapter file → checkpoint saga completes, the project adapter reconciles the accepted Fact Proposal into Migration 4 using the chapter transaction ID as an idempotency key. Planner requests receive the deterministic Phase 6 ledger block before model invocation. Phase 7 will compile and budget that block without changing its authority.
''',
        "docs/AGENTS.md": '''

## Narrative Ledger authority

Writer, Librarian and retrieval code must not receive a Narrative Ledger write handle. Librarian may propose foreshadow or secret changes, but only a completed accepted Final transaction can be reconciled. Planner must receive every active CRITICAL and computed OVERDUE foreshadow plus bounded UPCOMING obligations.
''',
    }
    for path, addition in additions.items():
        source = read(path)
        heading = addition.strip().splitlines()[0]
        if heading not in source:
            write(path, source.rstrip() + addition + "\n")


def add_server_helper_test_route_contract() -> None:
    # Existing OpenAPI drift tests discover registered routes. This file adds a
    # focused contract assertion without depending on private server setup.
    path = ROOT / "internal/server/narrative_ledger_contract_test.go"
    if path.exists():
        return
    path.write_text('''package server

import (
    "os"
    "strings"
    "testing"
)

func TestNarrativeLedgerOpenAPIContract(t *testing.T) {
    payload, err := os.ReadFile("openapi_phase5.json")
    if err != nil {
        t.Fatal(err)
    }
    text := string(payload)
    for _, route := range []string{
        "/api/projects/{id}/foreshadows",
        "/api/projects/{id}/secrets",
        "/api/projects/{id}/ledger/planner-context",
        "/api/projects/{id}/ledger/dashboard",
        "/api/projects/{id}/ledger/diagnostics",
    } {
        if !strings.Contains(text, route) {
            t.Fatalf("OpenAPI is missing %s", route)
        }
    }
}
''', encoding="utf-8")


def main() -> None:
    project_field = server_project_field()
    handler_path = ROOT / "internal/server/narrative_ledger.go"
    handler = handler_path.read_text(encoding="utf-8").replace("s.projects", f"s.{project_field}")
    handler_path.write_text(handler, encoding="utf-8")

    project_path = ROOT / "internal/project/narrative_ledger.go"
    project_source = project_path.read_text(encoding="utf-8").replace("ErrUnsafePath", "ErrValidation")
    project_path.write_text(project_source, encoding="utf-8")

    patch_context_reflection()
    register_routes()
    patch_quality_handlers(project_field)
    planner_patches = patch_planner_calls()
    if planner_patches == 0:
        raise RuntimeError("no Planner call was found for mandatory Narrative Ledger injection")
    merge_openapi()
    patch_chapters()
    patch_ci()
    patch_status()
    append_docs()
    add_server_helper_test_route_contract()
    print(f"Phase 6 source finalized; project_field={project_field}; planner_patches={planner_patches}")


if __name__ == "__main__":
    main()
