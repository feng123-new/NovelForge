package chapterversion

import "github.com/voocel/ainovel-cli/internal/db/migrate"

// CounterMigration serializes version-number allocation across independent
// Store instances. It is separate from Migration 6 so the immutable model
// remains reviewable while concurrent allocation receives its own checksum.
func CounterMigration() migrate.Migration {
	return migrate.Migration{
		Version: 7,
		Name:    "chapter_version_atomic_counters",
		SQL: `
CREATE TABLE chapter_version_counters (
    project_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter >= 1),
    next_version INTEGER NOT NULL CHECK(next_version >= 1),
    PRIMARY KEY(project_id, chapter)
);
INSERT INTO chapter_version_counters(project_id, chapter, next_version)
SELECT project_id, chapter, MAX(version_number)
FROM chapter_versions
GROUP BY project_id, chapter;
`,
	}
}
