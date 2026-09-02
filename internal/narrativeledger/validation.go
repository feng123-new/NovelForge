package narrativeledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode"
)

const (
	defaultPageLimit = 50
	maximumPageLimit = 100
)

func normalizeChangeSet(input ChangeSet) (ChangeSet, error) {
	input.Source.TransactionID = strings.TrimSpace(input.Source.TransactionID)
	input.Source.CandidateID = strings.TrimSpace(input.Source.CandidateID)
	if input.Source.Provenance == nil {
		input.Source.Provenance = map[string]string{}
	}
	if input.Source.TransactionID == "" {
		return ChangeSet{}, newError("LEDGER_SOURCE_REQUIRED", "source transaction is required", ErrValidation)
	}
	if len(input.Source.TransactionID) > 240 || len(input.Source.CandidateID) > 240 {
		return ChangeSet{}, newError("LEDGER_SOURCE_INVALID", "source identifier is too long", ErrValidation)
	}
	if input.Source.Chapter < 0 {
		return ChangeSet{}, newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
	}
	if input.Source.Authority != AuthorityAcceptedFinal && input.Source.Authority != AuthorityHuman {
		return ChangeSet{}, newError("LEDGER_AUTHORITY_REJECTED", "only accepted final or human authority may write the ledger", ErrAuthority)
	}
	seenForeshadows := map[string]struct{}{}
	for index := range input.Foreshadows {
		change := &input.Foreshadows[index]
		change.Action = strings.ToLower(strings.TrimSpace(change.Action))
		if change.Action == "" {
			change.Action = "upsert"
		}
		change.Key = normalizeKey(change.Key)
		change.Title = strings.TrimSpace(change.Title)
		change.Description = strings.TrimSpace(change.Description)
		if err := validateKey(change.Key, "foreshadow"); err != nil {
			return ChangeSet{}, err
		}
		if _, duplicate := seenForeshadows[change.Key]; duplicate {
			return ChangeSet{}, newError("LEDGER_DUPLICATE_CHANGE", "a foreshadow may be changed only once per source transaction", ErrValidation)
		}
		seenForeshadows[change.Key] = struct{}{}
		if change.Priority != nil && !validPriority(*change.Priority) {
			return ChangeSet{}, newError("LEDGER_PRIORITY_INVALID", "foreshadow priority is invalid", ErrValidation)
		}
		if change.Status != nil && !validStoredForeshadowStatus(*change.Status) {
			return ChangeSet{}, newError("LEDGER_FORESHADOW_STATUS_INVALID", "foreshadow status is invalid", ErrValidation)
		}
		if err := validateChapterPointers(change.PlantedChapter, change.DueChapter, change.RevealChapter); err != nil {
			return ChangeSet{}, err
		}
		if change.PlantedChapter != nil && change.DueChapter != nil && *change.DueChapter < *change.PlantedChapter {
			return ChangeSet{}, newError("LEDGER_FORESHADOW_SCHEDULE_INVALID", "due chapter must not precede planted chapter", ErrValidation)
		}
	}
	seenSecrets := map[string]struct{}{}
	for index := range input.Secrets {
		change := &input.Secrets[index]
		change.Action = strings.ToLower(strings.TrimSpace(change.Action))
		if change.Action == "" {
			change.Action = "upsert"
		}
		change.Key = normalizeKey(change.Key)
		change.Title = strings.TrimSpace(change.Title)
		change.Description = strings.TrimSpace(change.Description)
		if err := validateKey(change.Key, "secret"); err != nil {
			return ChangeSet{}, err
		}
		if _, duplicate := seenSecrets[change.Key]; duplicate {
			return ChangeSet{}, newError("LEDGER_DUPLICATE_CHANGE", "a secret may be changed only once per source transaction", ErrValidation)
		}
		seenSecrets[change.Key] = struct{}{}
		if change.Status != nil && !validSecretStatus(*change.Status) {
			return ChangeSet{}, newError("LEDGER_SECRET_STATUS_INVALID", "secret status is invalid", ErrValidation)
		}
		if err := validateChapterPointers(change.PublicFromChapter); err != nil {
			return ChangeSet{}, err
		}
		seenHolders := map[string]struct{}{}
		for knowledgeIndex := range change.Knowledge {
			knowledge := &change.Knowledge[knowledgeIndex]
			knowledge.Holder = strings.TrimSpace(knowledge.Holder)
			if knowledge.Holder == "" || len([]rune(knowledge.Holder)) > 200 {
				return ChangeSet{}, newError("LEDGER_SECRET_HOLDER_INVALID", "secret holder is required and must be at most 200 characters", ErrValidation)
			}
			if _, duplicate := seenHolders[strings.ToLower(knowledge.Holder)]; duplicate {
				return ChangeSet{}, newError("LEDGER_DUPLICATE_HOLDER", "a holder may occur only once per secret change", ErrValidation)
			}
			seenHolders[strings.ToLower(knowledge.Holder)] = struct{}{}
			if knowledge.KnownFromChapter < 0 || (knowledge.KnownUntilChapter != nil && *knowledge.KnownUntilChapter < knowledge.KnownFromChapter) {
				return ChangeSet{}, newError("LEDGER_SECRET_KNOWLEDGE_RANGE_INVALID", "secret knowledge interval is invalid", ErrValidation)
			}
		}
	}
	return input, nil
}

func normalizeListOptions(options ListOptions) (ListOptions, error) {
	if options.AsOfChapter < 0 || options.Offset < 0 {
		return ListOptions{}, newError("LEDGER_PAGINATION_INVALID", "chapter and offset must not be negative", ErrValidation)
	}
	if options.Limit <= 0 {
		options.Limit = defaultPageLimit
	}
	if options.Limit > maximumPageLimit {
		options.Limit = maximumPageLimit
	}
	options.Status = strings.ToLower(strings.TrimSpace(options.Status))
	options.Priority = strings.ToLower(strings.TrimSpace(options.Priority))
	options.Query = strings.TrimSpace(options.Query)
	if options.Priority != "" && !validPriority(Priority(options.Priority)) {
		return ListOptions{}, newError("LEDGER_PRIORITY_INVALID", "priority filter is invalid", ErrValidation)
	}
	return options, nil
}

func normalizeKey(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	builder.Grow(len(value))
	previousDash := false
	for _, current := range strings.ToLower(value) {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || current == '_' || current == '.' {
			builder.WriteRune(current)
			previousDash = false
			continue
		}
		if !previousDash {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func validateKey(key, kind string) error {
	if key == "" || len(key) > 200 {
		return newError("LEDGER_KEY_INVALID", kind+" key is required and must be at most 200 characters", ErrValidation)
	}
	return nil
}

func validateChapterPointers(values ...*int) error {
	for _, value := range values {
		if value != nil && *value < 0 {
			return newError("LEDGER_CHAPTER_INVALID", "chapter must not be negative", ErrValidation)
		}
	}
	return nil
}

func validPriority(value Priority) bool {
	switch value {
	case PriorityCritical, PriorityHigh, PriorityNormal, PriorityLow:
		return true
	default:
		return false
	}
}

func validStoredForeshadowStatus(value ForeshadowStatus) bool {
	switch value {
	case ForeshadowPlanned, ForeshadowPlanted, ForeshadowReinforced, ForeshadowRevealed, ForeshadowAbandoned:
		return true
	default:
		return false
	}
}

func validSecretStatus(value SecretStatus) bool {
	switch value {
	case SecretHidden, SecretHinted, SecretRevealed, SecretRetired:
		return true
	default:
		return false
	}
}

func contentHash(value any) (string, []byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), payload, nil
}

func deterministicID(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:16])
}

func foreshadowTransitionAllowed(from, to ForeshadowStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case ForeshadowPlanned:
		return to == ForeshadowPlanted || to == ForeshadowAbandoned
	case ForeshadowPlanted:
		return to == ForeshadowReinforced || to == ForeshadowRevealed || to == ForeshadowAbandoned
	case ForeshadowReinforced:
		return to == ForeshadowReinforced || to == ForeshadowRevealed || to == ForeshadowAbandoned
	case ForeshadowRevealed, ForeshadowAbandoned:
		return false
	default:
		return false
	}
}

func secretTransitionAllowed(from, to SecretStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case SecretHidden:
		return to == SecretHinted || to == SecretRevealed || to == SecretRetired
	case SecretHinted:
		return to == SecretRevealed || to == SecretRetired
	case SecretRevealed:
		return to == SecretRetired
	case SecretRetired:
		return false
	default:
		return false
	}
}
