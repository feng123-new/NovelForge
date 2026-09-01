#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
text = text.replace('\t"bytes"\n\t"context"', '\t"context"', 1)
text = text.replace(',\n\t\tWithRandom(bytes.NewReader(bytes.Repeat([]byte{0x42}, 4096))))', '))', 1)
path.write_text(text, encoding="utf-8")
