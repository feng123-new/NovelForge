#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
needle = '''    if "Truth Store 100k temporal index gate" not in text:
        needle = "      - name: Validate OpenAPI routes and schemas\\n"
        step = "      - name: Truth Store 100k temporal index gate\\n        env:\\n          GOWORK: \\\"off\\\"\\n          NOVELFORGE_SCALE_TEST: \\\"1\\\"\\n        run: go test -buildvcs=false -count=1 ./internal/truthstore -run '^TestHundredThousandFactTemporalQueryUsesIndex$'\\n"
        if needle not in text:
            raise RuntimeError("cannot find CI OpenAPI step")
        text = text.replace(needle, step + needle, 1)
    path.write_text(text, encoding="utf-8")'''
replacement = '''    if "Truth Store 100k temporal index gate" not in text:
        needle = "      - name: Validate OpenAPI routes and schemas\\n"
        step = "      - name: Truth Store 100k temporal index gate\\n        env:\\n          GOWORK: \\\"off\\\"\\n          NOVELFORGE_SCALE_TEST: \\\"1\\\"\\n        run: go test -buildvcs=false -count=1 ./internal/truthstore -run '^TestHundredThousandFactTemporalQueryUsesIndex$'\\n"
        if needle not in text:
            raise RuntimeError("cannot find CI OpenAPI step")
        text = text.replace(needle, step + needle, 1)
    if "workflow_dispatch:" not in text:
        trigger = "on:\\n  push:"
        if trigger not in text:
            raise RuntimeError("cannot find CI trigger block")
        text = text.replace(trigger, "on:\\n  workflow_dispatch:\\n  push:", 1)
    path.write_text(text, encoding="utf-8")'''
if needle not in text:
    raise RuntimeError("cannot locate generator patch_ci body")
text = text.replace(needle, replacement, 1)
path.write_text(text, encoding="utf-8")
