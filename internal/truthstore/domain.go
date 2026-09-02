package truthstore

import (
	"encoding/json"
	"time"
)

type EntityType string

const (
	EntityCharacter    EntityType = "character"
	EntityLocation     EntityType = "location"
	EntityOrganization EntityType = "organization"
	EntityItem         EntityType = "item"
	EntityAbility      EntityType = "ability"
	EntitySpecies      EntityType = "species"
	EntityConcept      EntityType = "concept"
	EntityEvent        EntityType = "event"
)

type Novel struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Genre           string `json:"genre"`
	Language        string `json:"language"`
	TargetWords     int    `json:"target_words"`
	TargetChapters  int    `json:"target_chapters"`
	WordsPerChapter int    `json:"words_per_chapter"`
	Status          string `json:"status"`
}

type Entity struct {
	ID            string          `json:"id"`
	Type          EntityType      `json:"type"`
	CanonicalName string          `json:"canonical_name"`
	Aliases       []string        `json:"aliases"`
	Metadata      json.RawMessage `json:"metadata"`
	FirstChapter  int             `json:"first_chapter"`
	LastChapter   *int            `json:"last_chapter,omitempty"`
}

type Character struct {
	EntityID    string `json:"entity_id"`
	Description string `json:"description"`
	Personality string `json:"personality"`
	Motivation  string `json:"motivation"`
	Goal        string `json:"goal"`
	Gender      string `json:"gender"`
	Age         string `json:"age"`
}

type CharacterState struct {
	ID               string          `json:"id"`
	CharacterID      string          `json:"character_id"`
	StateType        string          `json:"state_type"`
	Value            json.RawMessage `json:"value"`
	ValidFromChapter int             `json:"valid_from_chapter"`
	ValidToChapter   *int            `json:"valid_to_chapter,omitempty"`
	SourceChapter    int             `json:"source_chapter"`
	SourceVersion    string          `json:"source_version"`
	Authority        Authority       `json:"authority"`
}

type Relation struct {
	ID               string    `json:"id"`
	SubjectID        string    `json:"subject_id"`
	Predicate        string    `json:"predicate"`
	ObjectID         string    `json:"object_id"`
	ValidFromChapter int       `json:"valid_from_chapter"`
	ValidToChapter   *int      `json:"valid_to_chapter,omitempty"`
	Confidence       float64   `json:"confidence"`
	SourceChapter    int       `json:"source_chapter"`
	SourceVersion    string    `json:"source_version"`
	Authority        Authority `json:"authority"`
}

type KnowledgeFact struct {
	ID              string `json:"id"`
	Fact            string `json:"fact"`
	CreatedChapter  int    `json:"created_chapter"`
	RevealedChapter *int   `json:"revealed_chapter,omitempty"`
	PublicStatus    string `json:"public_status"`
	SourceVersion   string `json:"source_version"`
}

type KnowledgeHolder struct {
	KnowledgeFactID  string `json:"knowledge_fact_id"`
	EntityID         string `json:"entity_id"`
	ValidFromChapter int    `json:"valid_from_chapter"`
	ValidToChapter   *int   `json:"valid_to_chapter,omitempty"`
	Source           string `json:"source"`
	SourceVersion    string `json:"source_version"`
}

type InventoryEvent struct {
	ID            string `json:"id"`
	OwnerID       string `json:"owner_id"`
	ItemID        string `json:"item_id"`
	EventType     string `json:"event_type"`
	Chapter       int    `json:"chapter"`
	Quantity      int    `json:"quantity"`
	SourceVersion string `json:"source_version"`
}

type TimelineEvent struct {
	ID            string          `json:"id"`
	Chapter       int             `json:"chapter"`
	InternalTime  string          `json:"internal_time"`
	EventType     string          `json:"event_type"`
	Characters    []string        `json:"characters"`
	Location      string          `json:"location"`
	Payload       json.RawMessage `json:"payload"`
	SourceVersion string          `json:"source_version"`
}

type WorldRule struct {
	ID               string    `json:"id"`
	Category         string    `json:"category"`
	Rule             string    `json:"rule"`
	Boundary         string    `json:"boundary"`
	ValidFromChapter int       `json:"valid_from_chapter"`
	ValidToChapter   *int      `json:"valid_to_chapter,omitempty"`
	Authority        Authority `json:"authority"`
	SourceVersion    string    `json:"source_version"`
}

type Provenance struct {
	ID            string    `json:"id"`
	SourceChapter int       `json:"source_chapter"`
	SourceVersion string    `json:"source_version"`
	Extractor     string    `json:"extractor"`
	ConfirmedBy   string    `json:"confirmed_by"`
	CreatedAt     time.Time `json:"created_at"`
}

type StateEvent struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Chapter       int             `json:"chapter"`
	Payload       json.RawMessage `json:"payload"`
	SourceVersion string          `json:"source_version"`
	Authority     Authority       `json:"authority"`
	CreatedAt     time.Time       `json:"created_at"`
}
