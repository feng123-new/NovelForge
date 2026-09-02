package truthstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	maxIdentifierRunes     = 200
	maxIdempotencyKeyRunes = 128
	maxPredicateRunes      = 128
	maxValueBytes          = 64 << 10
	maxExcerptRunes        = 2000
	maxPageSize            = 500
)

var (
	predicatePattern      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,127}$`)
	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

type Authority string

// Authority follows the product contract from weakest creative suggestion to
// strongest accepted human final. The ordering is deterministic Go logic; an
// LLM cannot promote its own proposal.
const (
	AuthorityLLMSuggestion  Authority = "llm_suggestion"
	AuthorityStoryCompass   Authority = "story_compass"
	AuthorityVolumePlan     Authority = "volume_plan"
	AuthorityArcPlan        Authority = "arc_plan"
	AuthorityChapterPlan    Authority = "chapter_plan"
	AuthorityGeneratedFinal Authority = "generated_final"
	AuthorityHumanFinal     Authority = "human_final"
)

func (a Authority) rank() (int, bool) {
	switch a {
	case AuthorityLLMSuggestion:
		return 10, true
	case AuthorityStoryCompass:
		return 20, true
	case AuthorityVolumePlan:
		return 30, true
	case AuthorityArcPlan:
		return 40, true
	case AuthorityChapterPlan:
		return 50, true
	case AuthorityGeneratedFinal:
		return 60, true
	case AuthorityHumanFinal:
		return 70, true
	default:
		return 0, false
	}
}

type EventKind string

const (
	EventAssert    EventKind = "assert"
	EventSupersede EventKind = "supersede"
	EventRetract   EventKind = "retract"
)

type Source struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Chapter     int    `json:"chapter"`
	Version     string `json:"version"`
	Extractor   string `json:"extractor,omitempty"`
	ConfirmedBy string `json:"confirmed_by,omitempty"`
	Excerpt     string `json:"excerpt,omitempty"`
}

type AppendInput struct {
	IdempotencyKey    string          `json:"-"`
	Kind              EventKind       `json:"kind"`
	SubjectType       string          `json:"subject_type"`
	SubjectID         string          `json:"subject_id"`
	Predicate         string          `json:"predicate"`
	Value             json.RawMessage `json:"value"`
	ValidFromChapter  int             `json:"valid_from_chapter"`
	ValidToChapter    *int            `json:"valid_to_chapter,omitempty"`
	KnownFromChapter  int             `json:"known_from_chapter"`
	KnownToChapter    *int            `json:"known_to_chapter,omitempty"`
	Authority         Authority       `json:"authority"`
	Confidence        float64         `json:"confidence"`
	Source            Source          `json:"source"`
	SupersedesEventID string          `json:"supersedes_event_id,omitempty"`
}

type Event struct {
	Sequence          int64           `json:"sequence"`
	ID                string          `json:"id"`
	IdempotencyKey    string          `json:"-"`
	RequestHash       string          `json:"-"`
	Kind              EventKind       `json:"kind"`
	SubjectType       string          `json:"subject_type"`
	SubjectID         string          `json:"subject_id"`
	Predicate         string          `json:"predicate"`
	Value             json.RawMessage `json:"value"`
	ValidFromChapter  int             `json:"valid_from_chapter"`
	ValidToChapter    *int            `json:"valid_to_chapter,omitempty"`
	KnownFromChapter  int             `json:"known_from_chapter"`
	KnownToChapter    *int            `json:"known_to_chapter,omitempty"`
	Authority         Authority       `json:"authority"`
	Confidence        float64         `json:"confidence"`
	Source            Source          `json:"source"`
	SupersedesEventID string          `json:"supersedes_event_id,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	Checksum          string          `json:"checksum"`
}

type AppendResult struct {
	Event    Event `json:"event"`
	Replayed bool  `json:"replayed"`
}

type StateQuery struct {
	Chapter     int    `json:"chapter"`
	SubjectType string `json:"subject_type,omitempty"`
	SubjectID   string `json:"subject_id,omitempty"`
	Predicate   string `json:"predicate,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

type Fact struct {
	Event
	EffectiveFromChapter int    `json:"effective_from_chapter"`
	EffectiveToChapter   *int   `json:"effective_to_chapter,omitempty"`
	SupersededByEventID  string `json:"superseded_by_event_id,omitempty"`
	Conflicted           bool   `json:"conflicted"`
}

type StatePage struct {
	Facts      []Fact `json:"facts"`
	Conflicts  int    `json:"conflicts"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	NextOffset *int   `json:"next_offset,omitempty"`
}

type EventQuery struct {
	AfterSequence  int64
	ThroughChapter *int
	SubjectType    string
	SubjectID      string
	Predicate      string
	Limit          int
}

type EventPage struct {
	Events        []Event `json:"events"`
	AfterSequence int64   `json:"after_sequence"`
	Limit         int     `json:"limit"`
	NextSequence  *int64  `json:"next_sequence,omitempty"`
}

type ConflictStatus string

const (
	ConflictUnresolved ConflictStatus = "unresolved"
	ConflictResolved   ConflictStatus = "resolved"
)

type ConflictQuery struct {
	Chapter     *int
	SubjectType string
	SubjectID   string
	Predicate   string
	Status      ConflictStatus
	Limit       int
	Offset      int
}

type Conflict struct {
	ID           string         `json:"id"`
	SubjectType  string         `json:"subject_type"`
	SubjectID    string         `json:"subject_id"`
	Predicate    string         `json:"predicate"`
	LeftEventID  string         `json:"left_event_id"`
	RightEventID string         `json:"right_event_id"`
	FromChapter  int            `json:"from_chapter"`
	ToChapter    *int           `json:"to_chapter,omitempty"`
	Status       ConflictStatus `json:"status"`
	Reason       string         `json:"reason"`
}

type ConflictPage struct {
	Conflicts  []Conflict `json:"conflicts"`
	Total      int        `json:"total"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	NextOffset *int       `json:"next_offset,omitempty"`
}

type RebuildResult struct {
	FromChapter        int    `json:"from_chapter"`
	EventsReplayed     int    `json:"events_replayed"`
	FactsProjected     int    `json:"facts_projected"`
	ConflictsProjected int    `json:"conflicts_projected"`
	ProjectionDigest   string `json:"projection_digest"`
}

type VerifyResult struct {
	Events           int      `json:"events"`
	Facts            int      `json:"facts"`
	Conflicts        int      `json:"conflicts"`
	ProjectionDigest string   `json:"projection_digest"`
	Valid            bool     `json:"valid"`
	Violations       []string `json:"violations"`
}

type Code string

const (
	CodeValidation          Code = "TRUTH_VALIDATION_FAILED"
	CodeNotFound            Code = "TRUTH_EVENT_NOT_FOUND"
	CodeConflict            Code = "TRUTH_CONFLICT"
	CodeAuthority           Code = "TRUTH_AUTHORITY_VIOLATION"
	CodeIdempotencyConflict Code = "TRUTH_IDEMPOTENCY_CONFLICT"
	CodeCorrupt             Code = "TRUTH_PROJECTION_CORRUPT"
	CodeBusy                Code = "TRUTH_STORE_BUSY"
	CodeStorage             Code = "TRUTH_STORAGE_ERROR"
)

type Error struct {
	Code      Code
	Message   string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func newError(code Code, message string, retryable bool, cause error) error {
	return &Error{Code: code, Message: message, Retryable: retryable, Cause: cause}
}

func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

type normalizedInput struct {
	AppendInput
	ValueHash   string
	RequestHash string
}

func normalizeAppendInput(input AppendInput) (normalizedInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.SubjectType = strings.TrimSpace(input.SubjectType)
	input.SubjectID = strings.TrimSpace(input.SubjectID)
	input.Predicate = strings.TrimSpace(input.Predicate)
	input.SupersedesEventID = strings.TrimSpace(input.SupersedesEventID)
	input.Source.Type = strings.TrimSpace(input.Source.Type)
	input.Source.ID = strings.TrimSpace(input.Source.ID)
	input.Source.Version = strings.TrimSpace(input.Source.Version)
	input.Source.Extractor = strings.TrimSpace(input.Source.Extractor)
	input.Source.ConfirmedBy = strings.TrimSpace(input.Source.ConfirmedBy)
	input.Source.Excerpt = strings.TrimSpace(input.Source.Excerpt)
	if input.Kind == "" {
		input.Kind = EventAssert
	}
	if !idempotencyKeyPattern.MatchString(input.IdempotencyKey) || len([]rune(input.IdempotencyKey)) > maxIdempotencyKeyRunes {
		return normalizedInput{}, newError(CodeValidation, "Idempotency-Key must be 1-128 safe ASCII characters", false, nil)
	}
	if input.Kind != EventAssert && input.Kind != EventSupersede && input.Kind != EventRetract {
		return normalizedInput{}, newError(CodeValidation, "kind must be assert, supersede, or retract", false, nil)
	}
	if input.Kind == EventAssert && input.SupersedesEventID != "" {
		return normalizedInput{}, newError(CodeValidation, "assert events cannot name supersedes_event_id", false, nil)
	}
	if (input.Kind == EventSupersede || input.Kind == EventRetract) && input.SupersedesEventID == "" {
		return normalizedInput{}, newError(CodeValidation, "supersede and retract events require supersedes_event_id", false, nil)
	}
	if !predicatePattern.MatchString(input.SubjectType) {
		return normalizedInput{}, newError(CodeValidation, "subject_type has an invalid format", false, nil)
	}
	if input.SubjectID == "" || len([]rune(input.SubjectID)) > maxIdentifierRunes {
		return normalizedInput{}, newError(CodeValidation, "subject_id is required and must be at most 200 characters", false, nil)
	}
	if !predicatePattern.MatchString(input.Predicate) || len([]rune(input.Predicate)) > maxPredicateRunes {
		return normalizedInput{}, newError(CodeValidation, "predicate has an invalid format", false, nil)
	}
	if input.ValidFromChapter < 0 || input.KnownFromChapter < 0 {
		return normalizedInput{}, newError(CodeValidation, "chapter boundaries must not be negative", false, nil)
	}
	if input.ValidToChapter != nil && *input.ValidToChapter < input.ValidFromChapter {
		return normalizedInput{}, newError(CodeValidation, "valid_to_chapter must not precede valid_from_chapter", false, nil)
	}
	if input.KnownToChapter != nil && *input.KnownToChapter < input.KnownFromChapter {
		return normalizedInput{}, newError(CodeValidation, "known_to_chapter must not precede known_from_chapter", false, nil)
	}
	effectiveFrom, effectiveTo := effectiveBounds(input.ValidFromChapter, input.ValidToChapter, input.KnownFromChapter, input.KnownToChapter)
	if effectiveTo != nil && *effectiveTo < effectiveFrom {
		return normalizedInput{}, newError(CodeValidation, "valid and knowledge chapter ranges must overlap", false, nil)
	}
	if _, ok := input.Authority.rank(); !ok {
		return normalizedInput{}, newError(CodeValidation, "authority is invalid", false, nil)
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return normalizedInput{}, newError(CodeValidation, "confidence must be between 0 and 1", false, nil)
	}
	if input.Source.Type == "" || input.Source.ID == "" || input.Source.Version == "" {
		return normalizedInput{}, newError(CodeValidation, "source.type, source.id, and source.version are required", false, nil)
	}
	if input.Source.Chapter < 0 {
		return normalizedInput{}, newError(CodeValidation, "source.chapter must not be negative", false, nil)
	}
	for _, value := range []string{input.Source.Type, input.Source.ID, input.Source.Version, input.Source.Extractor, input.Source.ConfirmedBy} {
		if len([]rune(value)) > maxIdentifierRunes {
			return normalizedInput{}, newError(CodeValidation, "source metadata must be at most 200 characters per field", false, nil)
		}
		if likelySecretText(value) {
			return normalizedInput{}, newError(CodeValidation, "credentials must not be stored in Truth provenance", false, nil)
		}
	}
	if len([]rune(input.Source.Excerpt)) > maxExcerptRunes {
		return normalizedInput{}, newError(CodeValidation, "source excerpt must be at most 2000 characters", false, nil)
	}
	if likelySecretText(input.Source.Excerpt) {
		return normalizedInput{}, newError(CodeValidation, "credentials must not be stored in Truth provenance", false, nil)
	}
	canonical, valueHash, err := canonicalizeValue(input.Kind, input.Value)
	if err != nil {
		return normalizedInput{}, err
	}
	input.Value = canonical
	requestHash, err := hashJSON(struct {
		Kind              EventKind       `json:"kind"`
		SubjectType       string          `json:"subject_type"`
		SubjectID         string          `json:"subject_id"`
		Predicate         string          `json:"predicate"`
		Value             json.RawMessage `json:"value"`
		ValidFromChapter  int             `json:"valid_from_chapter"`
		ValidToChapter    *int            `json:"valid_to_chapter,omitempty"`
		KnownFromChapter  int             `json:"known_from_chapter"`
		KnownToChapter    *int            `json:"known_to_chapter,omitempty"`
		Authority         Authority       `json:"authority"`
		Confidence        float64         `json:"confidence"`
		Source            Source          `json:"source"`
		SupersedesEventID string          `json:"supersedes_event_id,omitempty"`
	}{input.Kind, input.SubjectType, input.SubjectID, input.Predicate, input.Value,
		input.ValidFromChapter, input.ValidToChapter, input.KnownFromChapter,
		input.KnownToChapter, input.Authority, input.Confidence, input.Source,
		input.SupersedesEventID})
	if err != nil {
		return normalizedInput{}, newError(CodeValidation, "truth request cannot be canonicalized", false, err)
	}
	return normalizedInput{AppendInput: input, ValueHash: valueHash, RequestHash: requestHash}, nil
}

func canonicalizeValue(kind EventKind, raw json.RawMessage) (json.RawMessage, string, error) {
	if kind == EventRetract && len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("null")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, "", newError(CodeValidation, "value is required", false, nil)
	}
	if len(raw) > maxValueBytes {
		return nil, "", newError(CodeValidation, "value exceeds 64 KiB", false, nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, "", newError(CodeValidation, "value must be valid JSON", false, err)
	}
	if containsCredential(value) {
		return nil, "", newError(CodeValidation, "credentials must not be stored in Truth values", false, nil)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, "", newError(CodeValidation, "value must contain one JSON value", false, err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, "", newError(CodeValidation, "value cannot be canonicalized", false, err)
	}
	sum := sha256.Sum256(canonical)
	return json.RawMessage(canonical), hex.EncodeToString(sum[:]), nil
}

func containsCredential(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			for _, fragment := range []string{"api_key", "apikey", "access_token", "refresh_token", "client_secret", "provider_secret", "password", "authorization"} {
				if strings.Contains(lowerKey, fragment) {
					return true
				}
			}
			if containsCredential(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsCredential(child) {
				return true
			}
		}
	case string:
		return likelySecretText(typed)
	}
	return false
}

func likelySecretText(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "sk-") ||
		strings.HasPrefix(lower, "ghp_") ||
		strings.HasPrefix(lower, "github_pat_") ||
		strings.Contains(lower, "api_key=") ||
		strings.Contains(lower, `"api_key"`) ||
		strings.Contains(lower, "authorization: bearer") ||
		strings.Contains(lower, `"authorization"`) ||
		strings.HasPrefix(lower, "bearer ") ||
		strings.Contains(lower, " bearer ")
}

func hashJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func eventChecksum(event Event) string {
	value := struct {
		ID                string          `json:"id"`
		IdempotencyKey    string          `json:"idempotency_key"`
		RequestHash       string          `json:"request_hash"`
		Kind              EventKind       `json:"kind"`
		SubjectType       string          `json:"subject_type"`
		SubjectID         string          `json:"subject_id"`
		Predicate         string          `json:"predicate"`
		Value             json.RawMessage `json:"value"`
		ValidFromChapter  int             `json:"valid_from_chapter"`
		ValidToChapter    *int            `json:"valid_to_chapter,omitempty"`
		KnownFromChapter  int             `json:"known_from_chapter"`
		KnownToChapter    *int            `json:"known_to_chapter,omitempty"`
		Authority         Authority       `json:"authority"`
		Confidence        float64         `json:"confidence"`
		Source            Source          `json:"source"`
		SupersedesEventID string          `json:"supersedes_event_id,omitempty"`
		CreatedAt         string          `json:"created_at"`
	}{event.ID, event.IdempotencyKey, event.RequestHash, event.Kind,
		event.SubjectType, event.SubjectID, event.Predicate, event.Value,
		event.ValidFromChapter, event.ValidToChapter, event.KnownFromChapter,
		event.KnownToChapter, event.Authority, event.Confidence, event.Source,
		event.SupersedesEventID, event.CreatedAt.UTC().Format(time.RFC3339Nano)}
	hash, err := hashJSON(value)
	if err != nil {
		panic(fmt.Sprintf("truth event checksum: %v", err))
	}
	return hash
}

func effectiveBounds(validFrom int, validTo *int, knownFrom int, knownTo *int) (int, *int) {
	from := validFrom
	if knownFrom > from {
		from = knownFrom
	}
	var to *int
	if validTo != nil {
		value := *validTo
		to = &value
	}
	if knownTo != nil && (to == nil || *knownTo < *to) {
		value := *knownTo
		to = &value
	}
	return from, to
}

func normalizePage(limit, offset int) (int, int, error) {
	if offset < 0 {
		return 0, 0, newError(CodeValidation, "offset must not be negative", false, nil)
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	return limit, offset, nil
}

func sameKey(left, right Event) bool {
	return left.SubjectType == right.SubjectType && left.SubjectID == right.SubjectID && left.Predicate == right.Predicate
}

func trimFilter(value string, pattern bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len([]rune(value)) > maxIdentifierRunes {
		return "", newError(CodeValidation, "truth filter is too long", false, nil)
	}
	if pattern && !predicatePattern.MatchString(value) {
		return "", newError(CodeValidation, "truth filter has an invalid format", false, nil)
	}
	return value, nil
}
