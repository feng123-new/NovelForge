package narrativeledger

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

func scanSecret(ctx context.Context, queryer rowQueryer, key string, chapter int) (Secret, bool, error) {
	var result Secret
	var publicFrom sql.NullInt64
	var createdAt, updatedAt string
	err := queryer.QueryRowContext(ctx, `SELECT id, key, title, description, status,
		public_from_chapter, source_transaction_id, updated_chapter, created_at, updated_at
	FROM secrets WHERE key = ?`, normalizeKey(key)).Scan(
		&result.ID,
		&result.Key,
		&result.Title,
		&result.Description,
		&result.Status,
		&publicFrom,
		&result.SourceTransaction,
		&result.UpdatedChapter,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Secret{}, false, nil
	}
	if err != nil {
		return Secret{}, false, newError("LEDGER_DATABASE_READ_FAILED", "secret could not be read", err)
	}
	result.PublicFromChapter = nullIntPointer(publicFrom)
	result.Public = result.PublicFromChapter != nil && *result.PublicFromChapter <= chapter
	result.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Secret{}, false, err
	}
	result.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Secret{}, false, err
	}
	result.Holders, err = readSecretHolders(ctx, queryer, result.ID, chapter)
	if err != nil {
		return Secret{}, false, err
	}
	return result, true, nil
}

func readSecretHolders(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, secretID string, chapter int) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT holder FROM secret_knowledge
		WHERE secret_id = ? AND known_from_chapter <= ?
			AND (known_until_chapter IS NULL OR known_until_chapter >= ?)
		ORDER BY LOWER(holder), holder`, secretID, chapter, chapter)
	if err != nil {
		return nil, newError("LEDGER_DATABASE_READ_FAILED", "secret holders could not be read", err)
	}
	defer rows.Close()
	holders := []string{}
	for rows.Next() {
		var holder string
		if err := rows.Scan(&holder); err != nil {
			return nil, newError("LEDGER_DATABASE_READ_FAILED", "secret holder could not be decoded", err)
		}
		holders = append(holders, holder)
	}
	if err := rows.Err(); err != nil {
		return nil, newError("LEDGER_DATABASE_READ_FAILED", "secret holders could not be completed", err)
	}
	return stableHolders(holders), nil
}

// GetSecret returns the holder and public state as it existed at Chapter N.
func (s *Store) GetSecret(ctx context.Context, key string, asOfChapter int) (Secret, error) {
	if asOfChapter < 0 {
		return Secret{}, newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
	}
	if asOfChapter == 0 {
		current, err := s.currentChapter(ctx)
		if err != nil {
			return Secret{}, err
		}
		asOfChapter = current
	}
	result, found, err := scanSecret(ctx, s.db, key, asOfChapter)
	if err != nil {
		return Secret{}, err
	}
	if !found {
		return Secret{}, newError("LEDGER_SECRET_NOT_FOUND", "secret was not found", ErrNotFound)
	}
	return result, nil
}

// ListSecrets performs bounded, deterministic Chapter-N pagination.
func (s *Store) ListSecrets(ctx context.Context, input ListOptions) (SecretPage, error) {
	options, err := normalizeListOptions(input)
	if err != nil {
		return SecretPage{}, err
	}
	if options.AsOfChapter == 0 {
		options.AsOfChapter, err = s.currentChapter(ctx)
		if err != nil {
			return SecretPage{}, err
		}
	}
	where := []string{"1 = 1"}
	args := []any{}
	if options.Status != "" {
		switch SecretStatus(options.Status) {
		case SecretHidden, SecretHinted, SecretRevealed, SecretRetired:
			where = append(where, "s.status = ?")
			args = append(args, options.Status)
		default:
			if options.Status == "public" {
				where = append(where, "s.public_from_chapter IS NOT NULL AND s.public_from_chapter <= ?")
				args = append(args, options.AsOfChapter)
			} else if options.Status == "private" {
				where = append(where, "(s.public_from_chapter IS NULL OR s.public_from_chapter > ?)")
				args = append(args, options.AsOfChapter)
			} else {
				return SecretPage{}, newError("LEDGER_SECRET_STATUS_INVALID", "secret status filter is invalid", ErrValidation)
			}
		}
	}
	if options.Query != "" {
		where = append(where, "(LOWER(s.key) LIKE ? OR LOWER(s.title) LIKE ? OR LOWER(s.description) LIKE ?)")
		needle := "%" + strings.ToLower(options.Query) + "%"
		args = append(args, needle, needle, needle)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM secrets AS s WHERE `+clause, args...).Scan(&total); err != nil {
		return SecretPage{}, newError("LEDGER_DATABASE_READ_FAILED", "secret count could not be read", err)
	}
	query := `SELECT s.key FROM secrets AS s WHERE ` + clause + `
		ORDER BY CASE s.status WHEN 'hidden' THEN 0 WHEN 'hinted' THEN 1 WHEN 'revealed' THEN 2 ELSE 3 END,
			COALESCE(s.public_from_chapter, 2147483647), s.key LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), options.Limit, options.Offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return SecretPage{}, newError("LEDGER_DATABASE_READ_FAILED", "secret page could not be read", err)
	}
	defer rows.Close()
	keys := make([]string, 0, options.Limit)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return SecretPage{}, newError("LEDGER_DATABASE_READ_FAILED", "secret row could not be decoded", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return SecretPage{}, newError("LEDGER_DATABASE_READ_FAILED", "secret page could not be completed", err)
	}
	items := make([]Secret, 0, len(keys))
	for _, key := range keys {
		item, err := s.GetSecret(ctx, key, options.AsOfChapter)
		if err != nil {
			return SecretPage{}, err
		}
		items = append(items, item)
	}
	return SecretPage{
		Items:      items,
		Total:      total,
		Limit:      options.Limit,
		Offset:     options.Offset,
		NextOffset: nextOffset(total, options.Offset, options.Limit),
	}, nil
}

// SecretBoundary returns only public/holder metadata and never the secret description.
func (s *Store) SecretBoundary(ctx context.Context, key string, chapter int) (map[string]any, error) {
	secret, err := s.GetSecret(ctx, key, chapter)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"key":       secret.Key,
		"title":     secret.Title,
		"chapter":   chapter,
		"public":    secret.Public,
		"holders":   append([]string{}, secret.Holders...),
		"status":    secret.Status,
	}, nil
}

// ExplainSecretBoundary returns the index-backed Chapter-N holder plan.
func (s *Store) ExplainSecretBoundary(ctx context.Context, chapter int) ([]string, error) {
	if chapter < 0 {
		return nil, newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
	}
	rows, err := s.db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT holder FROM secret_knowledge
		INDEXED BY idx_secret_knowledge_temporal
		WHERE secret_id = ? AND known_from_chapter <= ?
			AND (known_until_chapter IS NULL OR known_until_chapter >= ?)
		ORDER BY holder`, "probe", chapter, chapter)
	if err != nil {
		return nil, newError("LEDGER_DATABASE_READ_FAILED", "secret boundary plan could not be read", err)
	}
	defer rows.Close()
	plans := []string{}
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			return nil, err
		}
		plans = append(plans, strconv.Itoa(id)+":"+detail)
	}
	return plans, rows.Err()
}
