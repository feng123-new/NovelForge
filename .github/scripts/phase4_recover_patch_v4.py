#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
text = text.replace('time.UTC) })))', 'time.UTC) }))', 1)
path.write_text(text, encoding="utf-8")
