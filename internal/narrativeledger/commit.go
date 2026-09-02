package narrativeledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

// CommitAcceptedFinal applies only the Narrative Ledger portion of an accepted
// Phase 5 proposal. The whole operation is one SQLite transaction and is keyed
// by the chapter transaction ID. Truth events are committed by the existing
// coordinator; retrying the saga safely replays both sides without duplicate
// Ledger events.
func (s *Store) CommitAcceptedFinal(ctx context.Context, input AcceptedFinalInput) (CommitResult, error) {
	if strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.TransactionID) == "" || strings.TrimSpace(input.ProposalID) == "" || strings.TrimSpace(input.CandidateID) == "" || input.Chapter < 0 || strings.TrimSpace(input.SourceVersion) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return CommitResult{}, fmt.Errorf("%w: accepted final metadata is incomplete", ErrValidation)
	}
	requestHash, err := hashJSON(input)
	if err != nil {
		return CommitResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommitResult{}, err
	}
	defer tx.Rollback()
	var storedHash, commitID string
	var foreshadowCount, secretCount int
	err = tx.QueryRowContext(ctx, `SELECT request_hash,commit_id,foreshadow_count,secret_count FROM narrative_ledger_commits WHERE transaction_id=?`, input.TransactionID).Scan(&storedHash, &commitID, &foreshadowCount, &secretCount)
	if err == nil {
		if storedHash != requestHash {
			return CommitResult{}, ErrIdempotencyConflict
		}
		return CommitResult{CommitID: commitID, TransactionID: input.TransactionID, ForeshadowCount: foreshadowCount, SecretCount: secretCount, Replayed: true}, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CommitResult{}, err
	}
	commitID = stableID("ledger-commit", input.ProjectID, input.TransactionID, requestHash)
	for index, change := range input.ForeshadowUpdates {
		if err := s.applyAcceptedForeshadow(ctx, tx, input, change, index); err != nil {
			return CommitResult{}, err
		}
		foreshadowCount++
	}
	for index, change := range input.Secrets {
		if err := s.applyAcceptedSecret(ctx, tx, input, change, index); err != nil {
			return CommitResult{}, err
		}
		secretCount++
	}
	now := utcText(s.now())
	if _, err := tx.ExecContext(ctx, `INSERT INTO narrative_ledger_commits(transaction_id,project_id,chapter,proposal_id,candidate_id,request_hash,commit_id,foreshadow_count,secret_count,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, input.TransactionID, input.ProjectID, input.Chapter, input.ProposalID, input.CandidateID, requestHash, commitID, foreshadowCount, secretCount, now); err != nil {
		return CommitResult{}, classifyWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE narrative_ledger_meta SET last_commit_chapter=MAX(last_commit_chapter,?),updated_at=? WHERE id=1`, input.Chapter, now); err != nil {
		return CommitResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommitResult{}, classifyWrite(err)
	}
	return CommitResult{CommitID: commitID, TransactionID: input.TransactionID, ForeshadowCount: foreshadowCount, SecretCount: secretCount}, nil
}

func (s *Store) applyAcceptedForeshadow(ctx context.Context, tx *sql.Tx, parent AcceptedFinalInput, change AcceptedChange, index int) error {
	if err := validateAcceptedChange(parent, change); err != nil {
		return err
	}
	var payload struct {
		ID                  string     `json:"id"`
		Title               string     `json:"title"`
		Description         string     `json:"description"`
		Importance          Importance `json:"importance"`
		PlantedChapter      *int       `json:"planted_chapter"`
		ExpectedPayoffMin   int        `json:"expected_payoff_min"`
		ExpectedPayoffMax   int        `json:"expected_payoff_max"`
		ActualPayoff        *int       `json:"actual_payoff"`
		Status              Status     `json:"status"`
		RelatedEntities     []string   `json:"related_entities"`
		RelatedArcs         []string   `json:"related_arcs"`
		LastProgressChapter *int       `json:"last_progress_chapter"`
		Urgency             Urgency    `json:"urgency"`
	}
	if err := json.Unmarshal(change.Object, &payload); err != nil {
		return fmt.Errorf("%w: accepted foreshadow payload is invalid", ErrValidation)
	}
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		id = subjectID(change.Subject)
	}
	if id == "" {
		id = stableID("foreshadow", parent.ProjectID, parent.TransactionID, fmt.Sprint(index))
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM foreshadows WHERE id=? AND project_id=?`, id, parent.ProjectID).Scan(&exists); err != nil {
		return err
	}
	source := truthstore.Source{Type: "chapter_final", ID: parent.CandidateID, Chapter: parent.Chapter, Version: parent.SourceVersion, Extractor: change.Extractor}
	if exists == 0 {
		planted := parent.Chapter
		if payload.PlantedChapter != nil {
			planted = *payload.PlantedChapter
		}
		lastProgress := planted
		if payload.LastProgressChapter != nil {
			lastProgress = *payload.LastProgressChapter
		}
		input := ForeshadowInput{ID: id, Title: payload.Title, Description: payload.Description, Importance: payload.Importance, PlantedChapter: planted, ExpectedPayoffMin: payload.ExpectedPayoffMin, ExpectedPayoffMax: payload.ExpectedPayoffMax, ActualPayoff: payload.ActualPayoff, Status: payload.Status, RelatedEntities: payload.RelatedEntities, RelatedArcs: payload.RelatedArcs, LastProgressChapter: lastProgress, Urgency: payload.Urgency, SourceVersion: change.SourceVersion, Authority: truthstore.AuthorityGeneratedFinal}
		normalizeForeshadowInput(&input)
		if input.Status == StatusPlanned && payload.PlantedChapter == nil {
			input.PlantedChapter = 0
			input.LastProgressChapter = 0
		}
		if err := input.Validate(); err != nil {
			return err
		}
		entities, _ := json.Marshal(input.RelatedEntities)
		arcs, _ := json.Marshal(input.RelatedArcs)
		now := utcText(s.now())
		if _, err := tx.ExecContext(ctx, `INSERT INTO foreshadows(id,project_id,title,description,importance,planted_chapter,expected_payoff_min,expected_payoff_max,actual_payoff,status,related_entities_json,related_arcs_json,last_progress_chapter,urgency,source_version,authority,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, input.ID, parent.ProjectID, input.Title, input.Description, input.Importance, input.PlantedChapter, input.ExpectedPayoffMin, input.ExpectedPayoffMax, nullableInt(input.ActualPayoff), input.Status, string(entities), string(arcs), input.LastProgressChapter, input.Urgency, input.SourceVersion, input.Authority, now, now); err != nil {
			return classifyWrite(err)
		}
		if err := replaceForeshadowLinks(ctx, tx, id, input.RelatedEntities, input.RelatedArcs); err != nil {
			return err
		}
		return appendForeshadowEvent(ctx, tx, id, parent.ProjectID, "accepted_create", parent.Chapter, payload, change.SourceVersion, truthstore.AuthorityGeneratedFinal, source, s.now)
	}
	current, err := getForeshadow(ctx, tx, parent.ProjectID, id, parent.Chapter)
	if err != nil {
		return err
	}
	input := ForeshadowInput{ID: current.ID, Title: current.Title, Description: current.Description, Importance: current.Importance, PlantedChapter: current.PlantedChapter, ExpectedPayoffMin: current.ExpectedPayoffMin, ExpectedPayoffMax: current.ExpectedPayoffMax, ActualPayoff: current.ActualPayoff, Status: current.Status, RelatedEntities: current.RelatedEntities, RelatedArcs: current.RelatedArcs, LastProgressChapter: current.LastProgressChapter, Urgency: current.Urgency, SourceVersion: change.SourceVersion, Authority: truthstore.AuthorityGeneratedFinal}
	if payload.Title != "" {
		input.Title = payload.Title
	}
	if payload.Description != "" {
		input.Description = payload.Description
	}
	if payload.Importance != "" {
		input.Importance = payload.Importance
	}
	if payload.ExpectedPayoffMin > 0 {
		input.ExpectedPayoffMin = payload.ExpectedPayoffMin
	}
	if payload.ExpectedPayoffMax > 0 {
		input.ExpectedPayoffMax = payload.ExpectedPayoffMax
	}
	if payload.ActualPayoff != nil {
		input.ActualPayoff = payload.ActualPayoff
	}
	if payload.Status != "" {
		if !transitionAllowed(current.Status, payload.Status) {
			return fmt.Errorf("%w: accepted foreshadow transition", ErrStateConflict)
		}
		input.Status = payload.Status
	}
	if payload.RelatedEntities != nil {
		input.RelatedEntities = payload.RelatedEntities
	}
	if payload.RelatedArcs != nil {
		input.RelatedArcs = payload.RelatedArcs
	}
	if payload.LastProgressChapter != nil {
		input.LastProgressChapter = *payload.LastProgressChapter
	} else if input.Status == StatusProgressing {
		input.LastProgressChapter = parent.Chapter
	}
	if input.Status == StatusResolved && input.ActualPayoff == nil {
		v := parent.Chapter
		input.ActualPayoff = &v
	}
	if err := input.Validate(); err != nil {
		return err
	}
	entities, _ := json.Marshal(cleanSorted(input.RelatedEntities))
	arcs, _ := json.Marshal(cleanSorted(input.RelatedArcs))
	now := utcText(s.now())
	if _, err := tx.ExecContext(ctx, `UPDATE foreshadows SET title=?,description=?,importance=?,expected_payoff_min=?,expected_payoff_max=?,actual_payoff=?,status=?,related_entities_json=?,related_arcs_json=?,last_progress_chapter=?,urgency=?,source_version=?,authority=?,updated_at=? WHERE id=? AND project_id=?`, input.Title, input.Description, input.Importance, input.ExpectedPayoffMin, input.ExpectedPayoffMax, nullableInt(input.ActualPayoff), input.Status, string(entities), string(arcs), input.LastProgressChapter, input.Urgency, input.SourceVersion, input.Authority, now, id, parent.ProjectID); err != nil {
		return classifyWrite(err)
	}
	if err := replaceForeshadowLinks(ctx, tx, id, input.RelatedEntities, input.RelatedArcs); err != nil {
		return err
	}
	return appendForeshadowEvent(ctx, tx, id, parent.ProjectID, "accepted_update", parent.Chapter, payload, change.SourceVersion, truthstore.AuthorityGeneratedFinal, source, s.now)
}

func (s *Store) applyAcceptedSecret(ctx context.Context, tx *sql.Tx, parent AcceptedFinalInput, change AcceptedChange, index int) error {
	if err := validateAcceptedChange(parent, change); err != nil {
		return err
	}
	var payload struct {
		ID                string        `json:"id"`
		Description       string        `json:"description"`
		Truth             string        `json:"truth"`
		CreatedChapter    *int          `json:"created_chapter"`
		RevealedChapter   *int          `json:"revealed_chapter"`
		PublicStatus      PublicStatus  `json:"public_status"`
		RelatedForeshadow string        `json:"related_foreshadow"`
		Holders           []HolderInput `json:"holders"`
	}
	if err := json.Unmarshal(change.Object, &payload); err != nil {
		return fmt.Errorf("%w: accepted secret payload is invalid", ErrValidation)
	}
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		id = subjectID(change.Subject)
	}
	if id == "" {
		id = stableID("secret", parent.ProjectID, parent.TransactionID, fmt.Sprint(index))
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secrets WHERE id=? AND project_id=?`, id, parent.ProjectID).Scan(&exists); err != nil {
		return err
	}
	source := truthstore.Source{Type: "chapter_final", ID: parent.CandidateID, Chapter: parent.Chapter, Version: parent.SourceVersion, Extractor: change.Extractor}
	if exists == 0 {
		created := parent.Chapter
		if payload.CreatedChapter != nil {
			created = *payload.CreatedChapter
		}
		input := SecretInput{ID: id, Description: payload.Description, Truth: payload.Truth, CreatedChapter: created, RevealedChapter: payload.RevealedChapter, PublicStatus: payload.PublicStatus, RelatedForeshadow: payload.RelatedForeshadow, SourceVersion: change.SourceVersion, Authority: truthstore.AuthorityGeneratedFinal, Holders: payload.Holders}
		normalizeSecretInput(&input)
		for i := range input.Holders {
			if input.Holders[i].SourceVersion == "" {
				input.Holders[i].SourceVersion = change.SourceVersion
			}
			if input.Holders[i].Authority == "" {
				input.Holders[i].Authority = truthstore.AuthorityGeneratedFinal
			}
			if input.Holders[i].Provenance.Type == "" {
				input.Holders[i].Provenance = source
			}
		}
		if err := input.Validate(); err != nil {
			return err
		}
		now := utcText(s.now())
		if _, err := tx.ExecContext(ctx, `INSERT INTO secrets(id,project_id,description,truth,created_chapter,revealed_chapter,public_status,related_foreshadow,source_version,authority,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, parent.ProjectID, input.Description, input.Truth, input.CreatedChapter, nullableInt(input.RevealedChapter), input.PublicStatus, input.RelatedForeshadow, input.SourceVersion, input.Authority, now, now); err != nil {
			return classifyWrite(err)
		}
		for _, holder := range input.Holders {
			if err := insertHolder(ctx, tx, id, holder); err != nil {
				return err
			}
		}
		return appendSecretEvent(ctx, tx, id, parent.ProjectID, "accepted_create", parent.Chapter, payload, change.SourceVersion, truthstore.AuthorityGeneratedFinal, source, s.now)
	}
	current, err := getSecret(ctx, tx, parent.ProjectID, id, parent.Chapter, true)
	if err != nil {
		return err
	}
	description := current.Description
	if payload.Description != "" {
		description = payload.Description
	}
	truth := current.Truth
	if payload.Truth != "" {
		truth = payload.Truth
	}
	public := current.PublicStatus
	if payload.PublicStatus != "" {
		public = payload.PublicStatus
	}
	revealed := current.RevealedChapter
	if payload.RevealedChapter != nil {
		revealed = payload.RevealedChapter
	}
	if public == PublicPublic && revealed == nil {
		v := parent.Chapter
		revealed = &v
	}
	related := current.RelatedForeshadow
	if payload.RelatedForeshadow != "" {
		related = payload.RelatedForeshadow
	}
	input := SecretInput{ID: id, Description: description, Truth: truth, CreatedChapter: current.CreatedChapter, RevealedChapter: revealed, PublicStatus: public, RelatedForeshadow: related, SourceVersion: change.SourceVersion, Authority: truthstore.AuthorityGeneratedFinal}
	if err := input.Validate(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE secrets SET description=?,truth=?,revealed_chapter=?,public_status=?,related_foreshadow=?,source_version=?,authority=?,updated_at=? WHERE id=? AND project_id=?`, description, truth, nullableInt(revealed), public, related, change.SourceVersion, truthstore.AuthorityGeneratedFinal, utcText(s.now()), id, parent.ProjectID); err != nil {
		return classifyWrite(err)
	}
	for _, holder := range payload.Holders {
		if holder.SourceVersion == "" {
			holder.SourceVersion = change.SourceVersion
		}
		if holder.Authority == "" {
			holder.Authority = truthstore.AuthorityGeneratedFinal
		}
		if holder.Provenance.Type == "" {
			holder.Provenance = source
		}
		if err := holder.Validate(current.CreatedChapter); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO secret_holders(secret_id,entity_id,valid_from_chapter,valid_to_chapter,source_version,authority,provenance_json) VALUES(?,?,?,?,?,?,?)`, id, holder.EntityID, holder.ValidFromChapter, nullableInt(holder.ValidToChapter), holder.SourceVersion, holder.Authority, mustJSON(holder.Provenance)); err != nil {
			return err
		}
	}
	return appendSecretEvent(ctx, tx, id, parent.ProjectID, "accepted_update", parent.Chapter, payload, change.SourceVersion, truthstore.AuthorityGeneratedFinal, source, s.now)
}

func validateAcceptedChange(parent AcceptedFinalInput, change AcceptedChange) error {
	if change.SourceChapter != parent.Chapter || strings.TrimSpace(change.SourceVersion) == "" || change.SourceVersion != parent.SourceVersion || strings.TrimSpace(change.SourceSHA) == "" || strings.TrimSpace(change.Extractor) == "" || len(change.Object) == 0 || !json.Valid(change.Object) {
		return fmt.Errorf("%w: accepted change provenance is invalid", ErrValidation)
	}
	return nil
}
func subjectID(subject string) string {
	parts := strings.SplitN(strings.TrimSpace(subject), ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(subject)
}
func mustJSON(value any) string { data, _ := json.Marshal(value); return string(data) }
