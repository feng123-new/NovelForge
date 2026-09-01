#!/usr/bin/env python3
from pathlib import Path
import re
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
start = text.index("def patch_server() -> None:")
end = text.index("\ndef patch_openapi() -> None:", start)
replacement = r'''def patch_server() -> None:
    path = ROOT / "internal/server/server.go"
    text = path.read_text(encoding="utf-8")
    if '"fmt"' not in text:
        text = text.replace("import (", 'import (\n\t"fmt"', 1)
    if '"github.com/voocel/ainovel-cli/internal/project"' not in text:
        text = text.replace("import (", 'import (\n\t"github.com/voocel/ainovel-cli/internal/project"', 1)
    if "truthProjects *project.Repository" not in text:
        match = re.search(r"type\s+Server\s+struct\s*\{", text)
        if not match:
            raise RuntimeError("cannot find Server struct")
        text = text[:match.end()] + "\n\ttruthProjects *project.Repository" + text[match.end():]
    if "phase4TruthProjects" not in text:
        match = re.search(r"func\s+New\s*\(\s*([A-Za-z_]\w*)\s+Config\s*\)\s*\(\s*\*Server\s*,\s*error\s*\)\s*\{", text)
        if not match:
            raise RuntimeError("cannot find server New")
        config_name = match.group(1)
        setup = f"\n\tphase4TruthProjects, truthProjectErr := project.NewRepository({config_name}.Workspace)\n\tif truthProjectErr != nil {{\n\t\treturn nil, fmt.Errorf(\"prepare truth project repository: %w\", truthProjectErr)\n\t}}\n"
        text = text[:match.end()] + setup + text[match.end():]
        literal = text.find("&Server{", match.end())
        if literal < 0:
            raise RuntimeError("cannot find Server literal")
        text = text[:literal + len("&Server{")] + "\n\t\ttruthProjects: phase4TruthProjects," + text[literal + len("&Server{"):]
    if '"/api/truth/"' not in text:
        lines = text.splitlines()
        inserted = False
        for index, line in enumerate(lines):
            if "/api/openapi.json" not in line or ".HandleFunc(" not in line:
                continue
            match = re.match(r"^(\s*)([A-Za-z_]\w*)\.HandleFunc\([^,]+,\s*([A-Za-z_]\w*)\.[A-Za-z_]\w*\)", line)
            if not match:
                continue
            indent, mux, receiver = match.groups()
            lines[index + 1:index + 1] = [
                f'{indent}{mux}.HandleFunc("/api/truth", {receiver}.handleTruth)',
                f'{indent}{mux}.HandleFunc("/api/truth/", {receiver}.handleTruth)',
            ]
            inserted = True
            break
        if not inserted:
            raise RuntimeError("cannot discover ServeMux registration point")
        text = "\n".join(lines) + "\n"
    path.write_text(text, encoding="utf-8")
'''
text = text[:start] + replacement + text[end:]
path.write_text(text, encoding="utf-8")
