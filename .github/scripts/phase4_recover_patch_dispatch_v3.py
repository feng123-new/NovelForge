#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
start = text.index("def patch_ci() -> None:")
end = text.index("\ndef patch_docs() -> None:", start)
section = text[start:end]
marker = '    path.write_text(text, encoding="utf-8")'
position = section.rfind(marker)
if position < 0:
    raise RuntimeError("cannot locate patch_ci write")
injection = '''    if "workflow_dispatch:" not in text:
        trigger = "on:\\n  push:"
        if trigger not in text:
            raise RuntimeError("cannot find CI trigger block")
        text = text.replace(trigger, "on:\\n  workflow_dispatch:\\n  push:", 1)
'''
section = section[:position] + injection + section[position:]
text = text[:start] + section + text[end:]
path.write_text(text, encoding="utf-8")
