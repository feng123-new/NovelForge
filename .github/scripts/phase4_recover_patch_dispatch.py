#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
old = '''name: CI

on:
  push:'''
new = '''name: CI

on:
  workflow_dispatch:
  push:'''
if old not in text:
    raise RuntimeError("cannot locate CI trigger block in generator")
text = text.replace(old, new, 1)
path.write_text(text, encoding="utf-8")
