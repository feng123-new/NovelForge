#!/usr/bin/env python3
from pathlib import Path
import re
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
start = text.index("def discover_entry_root()")
end = text.index("\ndef patch_server()", start)
replacement = r'''def discover_entry_access() -> tuple[str, str]:
    source = "\n".join(path.read_text(encoding="utf-8") for path in (ROOT / "internal/project").glob("*.go"))
    signature = re.search(
        r"func\s+\(r\s+\*Repository\)\s+find\s*\((.*?)\)\s*\(\s*\*?([A-Za-z_]\w*)\s*,\s*error\s*\)",
        source,
        re.S,
    )
    if not signature:
        raise RuntimeError("cannot discover project find signature")
    parameters, type_name = signature.groups()
    find_call = "r.find(ctx, id)" if "context.Context" in parameters else "r.find(id)"
    structure = re.search(r"type\s+" + re.escape(type_name) + r"\s+struct\s*\{(.*?)\n\}", source, re.S)
    if not structure:
        raise RuntimeError(f"cannot discover {type_name} structure")
    candidates: list[str] = []
    for name, type_expr in re.findall(r"^\s*([A-Za-z_]\w*)\s+([^\n`]+)", structure.group(1), re.M):
        if "string" in type_expr and any(token in name.lower() for token in ("root", "path", "dir")):
            candidates.append(name)
    if not candidates:
        raise RuntimeError(f"cannot discover root field in {type_name}")
    return "entry." + candidates[0], find_call
'''
text = text[:start] + replacement + text[end:]
old_loop = '''for relative, content in FILES.items():
    if relative == "internal/project/truth.go":
        content = content.replace("ENTRY_ROOT", discover_entry_root())
    write(relative, content)
'''
new_loop = '''for relative, content in FILES.items():
    if relative == "internal/project/truth.go":
        entry_root, find_call = discover_entry_access()
        content = content.replace("ENTRY_ROOT", entry_root)
        content = content.replace("r.find(id)", find_call)
    write(relative, content)
'''
if old_loop not in text:
    raise RuntimeError("cannot locate generator write loop")
text = text.replace(old_loop, new_loop, 1)
path.write_text(text, encoding="utf-8")
