#!/usr/bin/env python3
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")

old = '''\tif input.KnownToChapter != nil && *input.KnownToChapter < input.KnownFromChapter {
\t\treturn normalizedInput{}, newError(CodeValidation, "known_to_chapter must not precede known_from_chapter", false, nil)
\t}
\tif _, ok := input.Authority.rank(); !ok {'''
new = '''\tif input.KnownToChapter != nil && *input.KnownToChapter < input.KnownFromChapter {
\t\treturn normalizedInput{}, newError(CodeValidation, "known_to_chapter must not precede known_from_chapter", false, nil)
\t}
\teffectiveFrom, effectiveTo := effectiveBounds(input.ValidFromChapter, input.ValidToChapter, input.KnownFromChapter, input.KnownToChapter)
\tif effectiveTo != nil && *effectiveTo < effectiveFrom {
\t\treturn normalizedInput{}, newError(CodeValidation, "valid and knowledge chapter ranges must overlap", false, nil)
\t}
\tif _, ok := input.Authority.rank(); !ok {'''
if old not in text:
    raise RuntimeError("cannot locate temporal validation block")
text = text.replace(old, new, 1)

old = '''\t\tvar already sql.NullString
\t\tif err := tx.QueryRowContext(ctx, `SELECT superseded_by_event_id FROM truth_facts WHERE event_id = ?`, target.ID).Scan(&already); errors.Is(err, sql.ErrNoRows) {
\t\t\treturn AppendResult{}, newError(CodeConflict, "supersede target is not projected as a fact", false, err)
\t\t} else if err != nil {
\t\t\treturn AppendResult{}, classifyStorageError("supersede target projection could not be read", err)
\t\t}
\t\tif already.Valid {'''
new = '''\t\tvar targetEffectiveFrom int
\t\tvar already sql.NullString
\t\tif err := tx.QueryRowContext(ctx, `SELECT effective_from_chapter, superseded_by_event_id FROM truth_facts WHERE event_id = ?`, target.ID).Scan(&targetEffectiveFrom, &already); errors.Is(err, sql.ErrNoRows) {
\t\t\treturn AppendResult{}, newError(CodeConflict, "supersede target is not projected as a fact", false, err)
\t\t} else if err != nil {
\t\t\treturn AppendResult{}, classifyStorageError("supersede target projection could not be read", err)
\t\t}
\t\treplacementFrom, _ := effectiveBounds(normalized.ValidFromChapter, normalized.ValidToChapter, normalized.KnownFromChapter, normalized.KnownToChapter)
\t\tif replacementFrom < targetEffectiveFrom {
\t\t\treturn AppendResult{}, newError(CodeConflict, "supersede cannot begin before the target fact becomes effective", false, nil)
\t\t}
\t\tif already.Valid {'''
if old not in text:
    raise RuntimeError("cannot locate supersede target projection block")
text = text.replace(old, new, 1)

old = '''\t\t\teffectiveFrom, _ := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
\t\t\tend := effectiveFrom - 1'''
new = '''\t\t\teffectiveFrom, _ := effectiveBounds(event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter, event.KnownToChapter)
\t\t\tif effectiveFrom < target.EffectiveFromChapter {
\t\t\t\treturn nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s begins before its supersede target", event.ID), false, nil)
\t\t\t}
\t\t\tend := effectiveFrom - 1'''
if old not in text:
    raise RuntimeError("cannot locate rebuild supersede temporal block")
text = text.replace(old, new, 1)

path.write_text(text, encoding="utf-8")
