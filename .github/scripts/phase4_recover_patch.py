#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")

text = text.replace(
    'import (\n\t"context"\n\t"crypto/rand"\n\t"database/sql"',
    'import (\n\t"context"\n\t"crypto/rand"\n\t"crypto/sha256"\n\t"database/sql"',
    1,
)

text = text.replace(
    'byID := make(map[string]*projectedFact, len(events))',
    'byID := make(map[string]int, len(events))',
    1,
)
text = text.replace(
    'target := byID[event.SupersedesEventID]\n\t\t\tif target == nil || target.Kind == EventRetract || !sameKey(target.Event, event) {',
    'targetIndex, exists := byID[event.SupersedesEventID]\n\t\t\tif !exists {\n\t\t\t\treturn nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s has an invalid supersede target", event.ID), false, nil)\n\t\t\t}\n\t\t\ttarget := &facts[targetIndex]\n\t\t\tif target.Kind == EventRetract || !sameKey(target.Event, event) {',
    1,
)
text = text.replace(
    'byID[event.ID] = &facts[len(facts)-1]',
    'byID[event.ID] = len(facts) - 1',
    1,
)

text = text.replace(
    'if decoder.More() {\n\t\treturn nil, "", newError(CodeValidation, "value must contain one JSON value", false, nil)\n\t}',
    'var trailing any\n\tif err := decoder.Decode(&trailing); err == nil {\n\t\treturn nil, "", newError(CodeValidation, "value must contain one JSON value", false, nil)\n\t}',
    1,
)

text = text.replace(
    'if \'"github.com/voocel/ainovel-cli/internal/project"\' not in text:\n        text = text.replace("import (", \'import (\\n\\t"github.com/voocel/ainovel-cli/internal/project"\', 1)',
    'if \'"fmt"\' not in text:\n        text = text.replace("import (", \'import (\\n\\t"fmt"\', 1)\n    if \'"github.com/voocel/ainovel-cli/internal/project"\' not in text:\n        text = text.replace("import (", \'import (\\n\\t"github.com/voocel/ainovel-cli/internal/project"\', 1)',
    1,
)

text = text.replace(
    r'(\s*([A-Za-z_]\w*)\s*,\s*error\s*\)',
    r'(\s*\*?([A-Za-z_]\w*)\s*,\s*error\s*\)',
    1,
)

path.write_text(text, encoding="utf-8")
