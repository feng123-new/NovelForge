#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
old = "Migrations: []migrate.Migration{Migration()}"
new = "Migrations: []migrate.Migration{{Version: 1, Name: \"truth_test_base\", SQL: \"CREATE TABLE truth_test_base(id INTEGER PRIMARY KEY);\"}, Migration()}"
if old not in text:
    raise RuntimeError("cannot locate Truth Store test migration lists")
text = text.replace(old, new)
path.write_text(text, encoding="utf-8")
