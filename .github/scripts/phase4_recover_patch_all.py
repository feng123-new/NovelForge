#!/usr/bin/env python3
from pathlib import Path
import re
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")

# Production imports and canonical JSON trailing-input validation.
text = text.replace(
    'import (\n\t"context"\n\t"crypto/rand"\n\t"database/sql"',
    'import (\n\t"context"\n\t"crypto/rand"\n\t"crypto/sha256"\n\t"database/sql"',
    1,
)
text = text.replace(
    '\t"fmt"\n\t"regexp"',
    '\t"fmt"\n\t"io"\n\t"regexp"',
    1,
)
text = text.replace(
    '''\tif decoder.More() {
\t\treturn nil, "", newError(CodeValidation, "value must contain one JSON value", false, nil)
\t}''',
    '''\tvar trailing any
\tif err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
\t\treturn nil, "", newError(CodeValidation, "value must contain one JSON value", false, err)
\t}''',
    1,
)

# JSON contract for batch query payloads.
text = text.replace(
    '''type StateQuery struct {
\tChapter     int
\tSubjectType string
\tSubjectID   string
\tPredicate   string
\tLimit       int
\tOffset      int
}''',
    '''type StateQuery struct {
\tChapter     int    `json:"chapter"`
\tSubjectType string `json:"subject_type,omitempty"`
\tSubjectID   string `json:"subject_id,omitempty"`
\tPredicate   string `json:"predicate,omitempty"`
\tLimit       int    `json:"limit,omitempty"`
\tOffset      int    `json:"offset,omitempty"`
}''',
    1,
)

# Event checksums are computed before insertion. The append-only trigger never needs
# to be removed, and the database-assigned sequence remains projection metadata.
text = text.replace(
    '''func eventChecksum(event Event) string {
\tvalue := struct {
\t\tSequence          int64           `json:"sequence"`''',
    '''func eventChecksum(event Event) string {
\tvalue := struct {''',
    1,
)
text = text.replace(
    '''\t}{event.Sequence, event.ID, event.IdempotencyKey, event.RequestHash, event.Kind,''',
    '''\t}{event.ID, event.IdempotencyKey, event.RequestHash, event.Kind,''',
    1,
)
text = text.replace(
    '''\t\tSupersedesEventID: normalized.SupersedesEventID, CreatedAt: createdAt,
\t}
\tresult, err := tx.ExecContext''',
    '''\t\tSupersedesEventID: normalized.SupersedesEventID, CreatedAt: createdAt,
\t}
\tevent.Checksum = eventChecksum(event)
\tresult, err := tx.ExecContext''',
    1,
)
text = text.replace(
    ''') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,''',
    ''') VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,''',
    1,
)
text = text.replace(
    '''nullableString(event.SupersedesEventID), event.CreatedAt.Format(time.RFC3339Nano))''',
    '''nullableString(event.SupersedesEventID), event.CreatedAt.Format(time.RFC3339Nano), event.Checksum)''',
    1,
)
start = text.find("\tevent.Sequence = sequence\n\tevent.Checksum = eventChecksum(event)\n")
if start >= 0:
    end = text.find("\teffectiveFrom, effectiveTo :=", start)
    if end < 0:
        raise RuntimeError("cannot locate checksum update block end")
    text = text[:start] + "\tevent.Sequence = sequence\n\n" + text[end:]

# Idempotency remains correct across independently opened Store instances.
text = text.replace(
    '''\tif err != nil {
\t\treturn AppendResult{}, classifyStorageError("truth event could not be appended", err)
\t}
\tsequence, err := result.LastInsertId()''',
    '''\tif err != nil {
\t\t_ = tx.Rollback()
\t\texisting, lookupErr := eventByIdempotencyDB(ctx, s.db, normalized.IdempotencyKey)
\t\tif lookupErr == nil {
\t\t\tif existing.RequestHash != normalized.RequestHash {
\t\t\t\treturn AppendResult{}, newError(CodeIdempotencyConflict, "Idempotency-Key was already used with a different truth event", false, nil)
\t\t\t}
\t\t\treturn AppendResult{Event: existing, Replayed: true}, nil
\t\t}
\t\treturn AppendResult{}, classifyStorageError("truth event could not be appended", err)
\t}
\tsequence, err := result.LastInsertId()''',
    1,
)
text = text.replace(
    '''func eventByIdempotencyTx(ctx context.Context, tx *sql.Tx, key string) (Event, error) {
\treturn scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE idempotency_key=?`, key))
}
''',
    '''func eventByIdempotencyTx(ctx context.Context, tx *sql.Tx, key string) (Event, error) {
\treturn scanEvent(tx.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE idempotency_key=?`, key))
}

func eventByIdempotencyDB(ctx context.Context, db *sql.DB, key string) (Event, error) {
\treturn scanEvent(db.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM truth_events WHERE idempotency_key=?`, key))
}
''',
    1,
)

# Closing a fact never extends a naturally bounded interval.
text = text.replace(
    '''result, err := tx.ExecContext(ctx, `UPDATE truth_facts SET effective_to_chapter=?, superseded_by_event_id=? WHERE event_id=?`, end, byID, targetID)''',
    '''result, err := tx.ExecContext(ctx, `UPDATE truth_facts SET
\t\teffective_to_chapter=CASE WHEN effective_to_chapter IS NULL OR ? < effective_to_chapter THEN ? ELSE effective_to_chapter END,
\t\tsuperseded_by_event_id=? WHERE event_id=?`, end, end, byID, targetID)''',
    1,
)

# Rebuild projections use stable slice indexes rather than pointers invalidated by append.
text = text.replace(
    'byID := make(map[string]*projectedFact, len(events))',
    'byID := make(map[string]int, len(events))',
    1,
)
text = text.replace(
    '''\t\t\ttarget := byID[event.SupersedesEventID]
\t\t\tif target == nil || target.Kind == EventRetract || !sameKey(target.Event, event) {''',
    '''\t\t\ttargetIndex, exists := byID[event.SupersedesEventID]
\t\t\tif !exists {
\t\t\t\treturn nil, nil, newError(CodeCorrupt, fmt.Sprintf("truth event %s has an invalid supersede target", event.ID), false, nil)
\t\t\t}
\t\t\ttarget := &facts[targetIndex]
\t\t\tif target.Kind == EventRetract || !sameKey(target.Event, event) {''',
    1,
)
text = text.replace(
    '''\t\t\tend := effectiveFrom - 1
\t\t\ttarget.EffectiveToChapter = &end
\t\t\ttarget.SupersededByEventID = event.ID''',
    '''\t\t\tend := effectiveFrom - 1
\t\t\tif target.EffectiveToChapter == nil || end < *target.EffectiveToChapter {
\t\t\t\ttarget.EffectiveToChapter = &end
\t\t\t}
\t\t\ttarget.SupersededByEventID = event.ID''',
    1,
)
text = text.replace(
    'byID[event.ID] = &facts[len(facts)-1]',
    'byID[event.ID] = len(facts) - 1',
    1,
)

# Tests use production crypto/rand. Repeated deterministic bytes would create duplicate IDs.
text = text.replace('\t"bytes"\n\t"context"\n\t"database/sql"', '\t"context"\n\t"database/sql"', 1)
text = text.replace(
    ''',
\t\tWithRandom(bytes.NewReader(bytes.Repeat([]byte{0x42}, 4096))))''',
    ''')''',
    1,
)

# Structure-aware server integration.
server_start = text.index("def patch_server() -> None:")
server_end = text.index("\ndef patch_openapi() -> None:", server_start)
server_replacement = r'''def patch_server() -> None:
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
text = text[:server_start] + server_replacement + text[server_end:]

path.write_text(text, encoding="utf-8")
