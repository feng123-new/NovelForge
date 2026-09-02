package narrativeledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type Option func(*Store)

func WithClock(now func() time.Time) Option {
	return func(s *Store) {
		if now != nil {
			s.now = now
		}
	}
}

func OpenExisting(path string, busyTimeout time.Duration, options ...Option) (*Store, error) {
	db, err := migrate.Open(path, busyTimeout)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, now: time.Now}
	for _, option := range options {
		option(store)
	}
	if err := store.requireSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) requireSchema(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=4 AND name='narrative_ledger'`).Scan(&count); err != nil {
		return fmt.Errorf("inspect narrative ledger schema: %w", err)
	}
	if count != 1 {
		return errors.New("narrative ledger migration is not applied")
	}
	return nil
}

func (s *Store) CreateForeshadow(ctx context.Context, projectID, idempotencyKey string, input ForeshadowInput) (Foreshadow, error) {
	projectID = strings.TrimSpace(projectID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	normalizeForeshadowInput(&input)
	if projectID == "" || idempotencyKey == "" {
		return Foreshadow{}, fmt.Errorf("%w: project and idempotency key are required", ErrValidation)
	}
	if input.ID == "" {
		input.ID = stableID("foreshadow", projectID, idempotencyKey)
	}
	if err := input.Validate(); err != nil {
		return Foreshadow{}, err
	}
	requestHash, err := hashJSON(struct {
		Project string          `json:"project"`
		Input   ForeshadowInput `json:"input"`
	}{projectID, input})
	if err != nil {
		return Foreshadow{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Foreshadow{}, err
	}
	defer tx.Rollback()
	if replayID, replay, err := checkOperation(ctx, tx, idempotencyKey, requestHash); err != nil {
		return Foreshadow{}, err
	} else if replay {
		item, err := getForeshadow(ctx, tx, projectID, replayID, input.ExpectedPayoffMax)
		if err != nil {
			return Foreshadow{}, err
		}
		return item, tx.Commit()
	}
	now := utcText(s.now())
	entities, _ := json.Marshal(input.RelatedEntities)
	arcs, _ := json.Marshal(input.RelatedArcs)
	_, err = tx.ExecContext(ctx, `INSERT INTO foreshadows(
		id,project_id,title,description,importance,planted_chapter,expected_payoff_min,expected_payoff_max,
		actual_payoff,status,related_entities_json,related_arcs_json,last_progress_chapter,urgency,source_version,authority,created_at,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		input.ID, projectID, input.Title, input.Description, input.Importance, input.PlantedChapter,
		input.ExpectedPayoffMin, input.ExpectedPayoffMax, nullableInt(input.ActualPayoff), input.Status,
		string(entities), string(arcs), input.LastProgressChapter, input.Urgency, input.SourceVersion, input.Authority, now, now)
	if err != nil {
		return Foreshadow{}, classifyWrite(err)
	}
	if err := replaceForeshadowLinks(ctx, tx, input.ID, input.RelatedEntities, input.RelatedArcs); err != nil {
		return Foreshadow{}, err
	}
	if err := appendForeshadowEvent(ctx, tx, input.ID, projectID, "create", input.PlantedChapter, input, input.SourceVersion, input.Authority, truthstore.Source{Type: "human", ID: idempotencyKey, Chapter: input.PlantedChapter, Version: input.SourceVersion}, s.now); err != nil {
		return Foreshadow{}, err
	}
	if err := saveOperation(ctx, tx, idempotencyKey, requestHash, "foreshadow.create", input.ID, now); err != nil {
		return Foreshadow{}, err
	}
	if err := tx.Commit(); err != nil {
		return Foreshadow{}, classifyWrite(err)
	}
	return s.GetForeshadow(ctx, projectID, input.ID, input.ExpectedPayoffMax)
}

func (s *Store) UpdateForeshadow(ctx context.Context, projectID, id, idempotencyKey string, patch ForeshadowPatch) (Foreshadow, error) {
	projectID, id, idempotencyKey = strings.TrimSpace(projectID), strings.TrimSpace(id), strings.TrimSpace(idempotencyKey)
	if projectID == "" || id == "" || idempotencyKey == "" || patch.Chapter < 0 || strings.TrimSpace(patch.Reason) == "" {
		return Foreshadow{}, fmt.Errorf("%w: project, resource, chapter, reason and idempotency key are required", ErrValidation)
	}
	requestHash, err := hashJSON(struct {
		Project string          `json:"project"`
		ID      string          `json:"id"`
		Patch   ForeshadowPatch `json:"patch"`
	}{projectID, id, patch})
	if err != nil {
		return Foreshadow{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Foreshadow{}, err
	}
	defer tx.Rollback()
	if replayID, replay, err := checkOperation(ctx, tx, idempotencyKey, requestHash); err != nil {
		return Foreshadow{}, err
	} else if replay {
		item, err := getForeshadow(ctx, tx, projectID, replayID, patch.Chapter)
		if err != nil {
			return Foreshadow{}, err
		}
		return item, tx.Commit()
	}
	current, err := getForeshadow(ctx, tx, projectID, id, patch.Chapter)
	if err != nil {
		return Foreshadow{}, err
	}
	input := ForeshadowInput{
		ID: current.ID, Title: current.Title, Description: current.Description, Importance: current.Importance,
		PlantedChapter: current.PlantedChapter, ExpectedPayoffMin: current.ExpectedPayoffMin,
		ExpectedPayoffMax: current.ExpectedPayoffMax, ActualPayoff: current.ActualPayoff, Status: current.Status,
		RelatedEntities: append([]string(nil), current.RelatedEntities...), RelatedArcs: append([]string(nil), current.RelatedArcs...),
		LastProgressChapter: current.LastProgressChapter, Urgency: current.Urgency,
		SourceVersion: current.SourceVersion, Authority: current.Authority,
	}
	if patch.Title != nil {
		input.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Description != nil {
		input.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Importance != nil {
		input.Importance = *patch.Importance
	}
	if patch.PlantedChapter != nil {
		input.PlantedChapter = *patch.PlantedChapter
	}
	if patch.ExpectedPayoffMin != nil {
		input.ExpectedPayoffMin = *patch.ExpectedPayoffMin
	}
	if patch.ExpectedPayoffMax != nil {
		input.ExpectedPayoffMax = *patch.ExpectedPayoffMax
	}
	if patch.ActualPayoff != nil {
		value := *patch.ActualPayoff
		input.ActualPayoff = &value
	}
	if patch.ClearActualPayoff {
		input.ActualPayoff = nil
	}
	if patch.RelatedEntities != nil {
		input.RelatedEntities = append([]string(nil), (*patch.RelatedEntities)...)
	}
	if patch.RelatedArcs != nil {
		input.RelatedArcs = append([]string(nil), (*patch.RelatedArcs)...)
	}
	if patch.LastProgressChapter != nil {
		input.LastProgressChapter = *patch.LastProgressChapter
	}
	if patch.Urgency != nil {
		input.Urgency = *patch.Urgency
	}
	if patch.SourceVersion != nil {
		input.SourceVersion = strings.TrimSpace(*patch.SourceVersion)
	}
	if patch.Status != nil {
		if !transitionAllowed(current.Status, *patch.Status) {
			return Foreshadow{}, fmt.Errorf("%w: %s to %s is not allowed", ErrStateConflict, current.Status, *patch.Status)
		}
		input.Status = *patch.Status
		switch input.Status {
		case StatusPlanted:
			input.PlantedChapter = patch.Chapter
			input.LastProgressChapter = patch.Chapter
		case StatusProgressing:
			input.LastProgressChapter = patch.Chapter
		case StatusResolved:
			if input.ActualPayoff == nil {
				value := patch.Chapter
				input.ActualPayoff = &value
			}
		}
	}
	normalizeForeshadowInput(&input)
	if err := input.Validate(); err != nil {
		return Foreshadow{}, err
	}
	entities, _ := json.Marshal(input.RelatedEntities)
	arcs, _ := json.Marshal(input.RelatedArcs)
	now := utcText(s.now())
	_, err = tx.ExecContext(ctx, `UPDATE foreshadows SET title=?,description=?,importance=?,planted_chapter=?,expected_payoff_min=?,expected_payoff_max=?,actual_payoff=?,status=?,related_entities_json=?,related_arcs_json=?,last_progress_chapter=?,urgency=?,source_version=?,updated_at=? WHERE id=? AND project_id=?`,
		input.Title, input.Description, input.Importance, input.PlantedChapter, input.ExpectedPayoffMin, input.ExpectedPayoffMax, nullableInt(input.ActualPayoff), input.Status, string(entities), string(arcs), input.LastProgressChapter, input.Urgency, input.SourceVersion, now, id, projectID)
	if err != nil {
		return Foreshadow{}, classifyWrite(err)
	}
	if err := replaceForeshadowLinks(ctx, tx, id, input.RelatedEntities, input.RelatedArcs); err != nil {
		return Foreshadow{}, err
	}
	eventType := "update"
	if patch.Status != nil {
		eventType = string(*patch.Status)
	}
	if err := appendForeshadowEvent(ctx, tx, id, projectID, eventType, patch.Chapter, patch, input.SourceVersion, input.Authority, truthstore.Source{Type: "human", ID: idempotencyKey, Chapter: patch.Chapter, Version: input.SourceVersion}, s.now); err != nil {
		return Foreshadow{}, err
	}
	if err := saveOperation(ctx, tx, idempotencyKey, requestHash, "foreshadow.update", id, now); err != nil {
		return Foreshadow{}, err
	}
	if err := tx.Commit(); err != nil {
		return Foreshadow{}, classifyWrite(err)
	}
	return s.GetForeshadow(ctx, projectID, id, patch.Chapter)
}

func (s *Store) CreateSecret(ctx context.Context, projectID, idempotencyKey string, input SecretInput) (Secret, error) {
	projectID, idempotencyKey = strings.TrimSpace(projectID), strings.TrimSpace(idempotencyKey)
	normalizeSecretInput(&input)
	if projectID == "" || idempotencyKey == "" {
		return Secret{}, fmt.Errorf("%w: project and idempotency key are required", ErrValidation)
	}
	if input.ID == "" {
		input.ID = stableID("secret", projectID, idempotencyKey)
	}
	if err := input.Validate(); err != nil {
		return Secret{}, err
	}
	requestHash, err := hashJSON(struct {
		Project string      `json:"project"`
		Input   SecretInput `json:"input"`
	}{projectID, input})
	if err != nil {
		return Secret{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Secret{}, err
	}
	defer tx.Rollback()
	if replayID, replay, err := checkOperation(ctx, tx, idempotencyKey, requestHash); err != nil {
		return Secret{}, err
	} else if replay {
		item, err := getSecret(ctx, tx, projectID, replayID, input.CreatedChapter, true)
		if err != nil {
			return Secret{}, err
		}
		return item, tx.Commit()
	}
	if input.RelatedForeshadow != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM foreshadows WHERE id=? AND project_id=?`, input.RelatedForeshadow, projectID).Scan(&exists); err != nil || exists != 1 {
			return Secret{}, fmt.Errorf("%w: related foreshadow is not in this project", ErrValidation)
		}
	}
	now := utcText(s.now())
	_, err = tx.ExecContext(ctx, `INSERT INTO secrets(id,project_id,description,truth,created_chapter,revealed_chapter,public_status,related_foreshadow,source_version,authority,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		input.ID, projectID, input.Description, input.Truth, input.CreatedChapter, nullableInt(input.RevealedChapter), input.PublicStatus, input.RelatedForeshadow, input.SourceVersion, input.Authority, now, now)
	if err != nil {
		return Secret{}, classifyWrite(err)
	}
	for _, holder := range input.Holders {
		if err := insertHolder(ctx, tx, input.ID, holder); err != nil {
			return Secret{}, err
		}
	}
	if err := appendSecretEvent(ctx, tx, input.ID, projectID, "create", input.CreatedChapter, input, input.SourceVersion, input.Authority, truthstore.Source{Type: "human", ID: idempotencyKey, Chapter: input.CreatedChapter, Version: input.SourceVersion}, s.now); err != nil {
		return Secret{}, err
	}
	if err := saveOperation(ctx, tx, idempotencyKey, requestHash, "secret.create", input.ID, now); err != nil {
		return Secret{}, err
	}
	if err := tx.Commit(); err != nil {
		return Secret{}, classifyWrite(err)
	}
	return s.GetSecret(ctx, projectID, input.ID, input.CreatedChapter, true)
}

func (s *Store) UpdateSecret(ctx context.Context, projectID, id, idempotencyKey string, patch SecretPatch) (Secret, error) {
	projectID, id, idempotencyKey = strings.TrimSpace(projectID), strings.TrimSpace(id), strings.TrimSpace(idempotencyKey)
	if projectID == "" || id == "" || idempotencyKey == "" || patch.Chapter < 0 || strings.TrimSpace(patch.Reason) == "" {
		return Secret{}, fmt.Errorf("%w: incomplete secret update", ErrValidation)
	}
	requestHash, err := hashJSON(struct {
		Project, ID string
		Patch       SecretPatch
	}{projectID, id, patch})
	if err != nil {
		return Secret{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Secret{}, err
	}
	defer tx.Rollback()
	if replayID, replay, err := checkOperation(ctx, tx, idempotencyKey, requestHash); err != nil {
		return Secret{}, err
	} else if replay {
		item, err := getSecret(ctx, tx, projectID, replayID, patch.Chapter, true)
		if err != nil {
			return Secret{}, err
		}
		return item, tx.Commit()
	}
	current, err := getSecret(ctx, tx, projectID, id, patch.Chapter, true)
	if err != nil {
		return Secret{}, err
	}
	input := SecretInput{ID: current.ID, Description: current.Description, Truth: current.Truth, CreatedChapter: current.CreatedChapter, RevealedChapter: current.RevealedChapter, PublicStatus: current.PublicStatus, RelatedForeshadow: current.RelatedForeshadow, SourceVersion: current.SourceVersion, Authority: current.Authority}
	if patch.Description != nil {
		input.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Truth != nil {
		input.Truth = strings.TrimSpace(*patch.Truth)
	}
	if patch.RevealedChapter != nil {
		v := *patch.RevealedChapter
		input.RevealedChapter = &v
	}
	if patch.ClearReveal {
		input.RevealedChapter = nil
	}
	if patch.PublicStatus != nil {
		input.PublicStatus = *patch.PublicStatus
	}
	if patch.RelatedForeshadow != nil {
		input.RelatedForeshadow = strings.TrimSpace(*patch.RelatedForeshadow)
	}
	if patch.SourceVersion != nil {
		input.SourceVersion = strings.TrimSpace(*patch.SourceVersion)
	}
	if input.PublicStatus == PublicPublic && input.RevealedChapter == nil {
		v := patch.Chapter
		input.RevealedChapter = &v
	}
	if err := input.Validate(); err != nil {
		return Secret{}, err
	}
	if input.RelatedForeshadow != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM foreshadows WHERE id=? AND project_id=?`, input.RelatedForeshadow, projectID).Scan(&exists); err != nil || exists != 1 {
			return Secret{}, fmt.Errorf("%w: related foreshadow is not in this project", ErrValidation)
		}
	}
	now := utcText(s.now())
	_, err = tx.ExecContext(ctx, `UPDATE secrets SET description=?,truth=?,revealed_chapter=?,public_status=?,related_foreshadow=?,source_version=?,updated_at=? WHERE id=? AND project_id=?`, input.Description, input.Truth, nullableInt(input.RevealedChapter), input.PublicStatus, input.RelatedForeshadow, input.SourceVersion, now, id, projectID)
	if err != nil {
		return Secret{}, classifyWrite(err)
	}
	eventType := "update"
	if input.PublicStatus == PublicPublic && current.PublicStatus != PublicPublic {
		eventType = "reveal"
	}
	if err := appendSecretEvent(ctx, tx, id, projectID, eventType, patch.Chapter, patch, input.SourceVersion, input.Authority, truthstore.Source{Type: "human", ID: idempotencyKey, Chapter: patch.Chapter, Version: input.SourceVersion}, s.now); err != nil {
		return Secret{}, err
	}
	if err := saveOperation(ctx, tx, idempotencyKey, requestHash, "secret.update", id, now); err != nil {
		return Secret{}, err
	}
	if err := tx.Commit(); err != nil {
		return Secret{}, classifyWrite(err)
	}
	return s.GetSecret(ctx, projectID, id, patch.Chapter, true)
}

func (s *Store) AddHolder(ctx context.Context, projectID, secretID, idempotencyKey string, holder HolderInput) (Secret, error) {
	projectID, secretID, idempotencyKey = strings.TrimSpace(projectID), strings.TrimSpace(secretID), strings.TrimSpace(idempotencyKey)
	if projectID == "" || secretID == "" || idempotencyKey == "" {
		return Secret{}, fmt.Errorf("%w: incomplete holder request", ErrValidation)
	}
	requestHash, err := hashJSON(struct {
		Project, Secret string
		Holder          HolderInput
	}{projectID, secretID, holder})
	if err != nil {
		return Secret{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Secret{}, err
	}
	defer tx.Rollback()
	if replayID, replay, err := checkOperation(ctx, tx, idempotencyKey, requestHash); err != nil {
		return Secret{}, err
	} else if replay {
		item, err := getSecret(ctx, tx, projectID, replayID, holder.ValidFromChapter, true)
		if err != nil {
			return Secret{}, err
		}
		return item, tx.Commit()
	}
	secret, err := getSecret(ctx, tx, projectID, secretID, holder.ValidFromChapter, true)
	if err != nil {
		return Secret{}, err
	}
	if err := holder.Validate(secret.CreatedChapter); err != nil {
		return Secret{}, err
	}
	if err := insertHolder(ctx, tx, secretID, holder); err != nil {
		return Secret{}, err
	}
	if err := appendSecretEvent(ctx, tx, secretID, projectID, "holder_add", holder.ValidFromChapter, holder, holder.SourceVersion, holder.Authority, holder.Provenance, s.now); err != nil {
		return Secret{}, err
	}
	if err := saveOperation(ctx, tx, idempotencyKey, requestHash, "secret.holder.add", secretID, utcText(s.now())); err != nil {
		return Secret{}, err
	}
	if err := tx.Commit(); err != nil {
		return Secret{}, classifyWrite(err)
	}
	return s.GetSecret(ctx, projectID, secretID, holder.ValidFromChapter, true)
}

func (s *Store) CloseHolder(ctx context.Context, projectID, secretID, entityID, idempotencyKey string, validTo int, sourceVersion string, authority truthstore.Authority) (Secret, error) {
	projectID, secretID, entityID, idempotencyKey = strings.TrimSpace(projectID), strings.TrimSpace(secretID), strings.TrimSpace(entityID), strings.TrimSpace(idempotencyKey)
	if projectID == "" || secretID == "" || entityID == "" || idempotencyKey == "" || validTo < 0 || strings.TrimSpace(sourceVersion) == "" || !validAuthority(authority) {
		return Secret{}, fmt.Errorf("%w: invalid holder close", ErrValidation)
	}
	payload := struct {
		Project, Secret, Entity string
		ValidTo                 int
		SourceVersion           string
		Authority               truthstore.Authority
	}{projectID, secretID, entityID, validTo, sourceVersion, authority}
	requestHash, err := hashJSON(payload)
	if err != nil {
		return Secret{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Secret{}, err
	}
	defer tx.Rollback()
	if replayID, replay, err := checkOperation(ctx, tx, idempotencyKey, requestHash); err != nil {
		return Secret{}, err
	} else if replay {
		item, err := getSecret(ctx, tx, projectID, replayID, validTo, true)
		if err != nil {
			return Secret{}, err
		}
		return item, tx.Commit()
	}
	result, err := tx.ExecContext(ctx, `UPDATE secret_holders SET valid_to_chapter=? WHERE secret_id=? AND entity_id=? AND valid_from_chapter<=? AND (valid_to_chapter IS NULL OR valid_to_chapter>?)`, validTo, secretID, entityID, validTo, validTo)
	if err != nil {
		return Secret{}, classifyWrite(err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Secret{}, fmt.Errorf("%w: active holder range not found", ErrNotFound)
	}
	if err := appendSecretEvent(ctx, tx, secretID, projectID, "holder_close", validTo, payload, sourceVersion, authority, truthstore.Source{Type: "human", ID: idempotencyKey, Chapter: validTo, Version: sourceVersion}, s.now); err != nil {
		return Secret{}, err
	}
	if err := saveOperation(ctx, tx, idempotencyKey, requestHash, "secret.holder.close", secretID, utcText(s.now())); err != nil {
		return Secret{}, err
	}
	if err := tx.Commit(); err != nil {
		return Secret{}, classifyWrite(err)
	}
	return s.GetSecret(ctx, projectID, secretID, validTo, true)
}

func checkOperation(ctx context.Context, tx *sql.Tx, key, requestHash string) (string, bool, error) {
	var storedHash, resourceID string
	err := tx.QueryRowContext(ctx, `SELECT request_hash,resource_id FROM narrative_ledger_operations WHERE idempotency_key=?`, key).Scan(&storedHash, &resourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if storedHash != requestHash {
		return "", false, ErrIdempotencyConflict
	}
	return resourceID, true, nil
}

func saveOperation(ctx context.Context, tx *sql.Tx, key, requestHash, operation, resourceID, created string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO narrative_ledger_operations(idempotency_key,request_hash,operation,resource_id,created_at) VALUES(?,?,?,?,?)`, key, requestHash, operation, resourceID, created)
	if err != nil {
		return classifyWrite(err)
	}
	return nil
}

func appendForeshadowEvent(ctx context.Context, tx *sql.Tx, id, projectID, eventType string, chapter int, payload any, sourceVersion string, authority truthstore.Authority, source truthstore.Source, now func() time.Time) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventID := stableID("foreshadow-event", id, eventType, fmt.Sprint(chapter), hexHash(data))
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO foreshadow_events(id,foreshadow_id,project_id,event_type,chapter,payload_json,source_version,authority,source_type,source_id,source_chapter,source_extractor,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, eventID, id, projectID, eventType, chapter, string(data), sourceVersion, authority, source.Type, source.ID, source.Chapter, source.Extractor, utcText(now()))
	return classifyWrite(err)
}

func appendSecretEvent(ctx context.Context, tx *sql.Tx, id, projectID, eventType string, chapter int, payload any, sourceVersion string, authority truthstore.Authority, source truthstore.Source, now func() time.Time) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventID := stableID("secret-event", id, eventType, fmt.Sprint(chapter), hexHash(data))
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO secret_events(id,secret_id,project_id,event_type,chapter,payload_json,source_version,authority,source_type,source_id,source_chapter,source_extractor,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, eventID, id, projectID, eventType, chapter, string(data), sourceVersion, authority, source.Type, source.ID, source.Chapter, source.Extractor, utcText(now()))
	return classifyWrite(err)
}

func insertHolder(ctx context.Context, tx *sql.Tx, secretID string, holder HolderInput) error {
	provenance, err := json.Marshal(holder.Provenance)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO secret_holders(secret_id,entity_id,valid_from_chapter,valid_to_chapter,source_version,authority,provenance_json) VALUES(?,?,?,?,?,?,?)`, secretID, holder.EntityID, holder.ValidFromChapter, nullableInt(holder.ValidToChapter), holder.SourceVersion, holder.Authority, string(provenance))
	return classifyWrite(err)
}

func replaceForeshadowLinks(ctx context.Context, tx *sql.Tx, id string, entities, arcs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM foreshadow_entities WHERE foreshadow_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM foreshadow_arcs WHERE foreshadow_id=?`, id); err != nil {
		return err
	}
	for _, entity := range entities {
		if _, err := tx.ExecContext(ctx, `INSERT INTO foreshadow_entities(foreshadow_id,entity_id) VALUES(?,?)`, id, entity); err != nil {
			return err
		}
	}
	for _, arc := range arcs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO foreshadow_arcs(foreshadow_id,arc_id) VALUES(?,?)`, id, arc); err != nil {
			return err
		}
	}
	return nil
}

func normalizeForeshadowInput(input *ForeshadowInput) {
	input.ID = strings.TrimSpace(input.ID)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.SourceVersion = strings.TrimSpace(input.SourceVersion)
	if input.Status == "" {
		input.Status = StatusPlanned
	}
	if input.Importance == "" {
		input.Importance = ImportanceMedium
	}
	if input.Urgency == "" {
		input.Urgency = UrgencyNormal
	}
	if input.Authority == "" {
		input.Authority = truthstore.AuthorityHumanFinal
	}
	input.RelatedEntities = cleanSorted(input.RelatedEntities)
	input.RelatedArcs = cleanSorted(input.RelatedArcs)
}

func normalizeSecretInput(input *SecretInput) {
	input.ID = strings.TrimSpace(input.ID)
	input.Description = strings.TrimSpace(input.Description)
	input.Truth = strings.TrimSpace(input.Truth)
	input.RelatedForeshadow = strings.TrimSpace(input.RelatedForeshadow)
	input.SourceVersion = strings.TrimSpace(input.SourceVersion)
	if input.PublicStatus == "" {
		input.PublicStatus = PublicPrivate
	}
	if input.Authority == "" {
		input.Authority = truthstore.AuthorityHumanFinal
	}
}

func cleanSorted(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
func hashJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hexHash(data), nil
}
func hexHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func stableID(prefix string, values ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "_" + hex.EncodeToString(sum[:12])
}
func utcText(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
func intPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}
func classifyWrite(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "constraint") {
		return fmt.Errorf("%w: %v", ErrStateConflict, err)
	}
	return err
}
