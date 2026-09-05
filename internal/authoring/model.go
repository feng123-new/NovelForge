// Package authoring manages writing instructions and references, never story Truth.
package authoring

import (
 "crypto/sha256"
 "embed"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "regexp"
 "strings"
 "unicode/utf8"
)

var ErrValidation = errors.New("invalid authoring input")
var ErrConflict = errors.New("authoring revision or idempotency conflict")
var ErrNotFound = errors.New("authoring entry not found")

//go:embed skills/*.md
var skillFiles embed.FS

const MaxEntries = 500
const MaxMarkdownBytes = 16 * 1024
var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,95}$`)

// Entry is user-managed Markdown. Knowledge is reference material, not a fact.
type Entry struct {
 ID string `json:"id"`
 Kind string `json:"kind"`
 Role string `json:"role"`
 Title string `json:"title"`
 Markdown string `json:"markdown"`
 Source string `json:"source"`
 Enabled bool `json:"enabled"`
 Pinned bool `json:"pinned"`
 Priority int `json:"priority"`
 FromChapter int `json:"from_chapter"`
 POV string `json:"pov"`
}
func ValidKind(s string) bool { return s=="skill" || s=="style" || s=="knowledge" }
func ValidRole(s string) bool { return s=="writing" || s=="review" || s=="polish" || s=="planning" }
func (e Entry) Validate() error {
 if !identifier.MatchString(e.ID) || !ValidKind(e.Kind) || strings.TrimSpace(e.Title)=="" || len(e.Title)>240 || strings.TrimSpace(e.Markdown)=="" || len(e.Markdown)>MaxMarkdownBytes || len(e.Source)>512 || len(e.POV)>200 || e.Priority<0 || e.Priority>100 || e.FromChapter<0 || e.FromChapter>1000 { return ErrValidation }
 if !utf8.ValidString(e.Markdown) || strings.ContainsRune(e.Markdown,0) { return ErrValidation }
 if e.Kind=="skill" { if !ValidRole(e.Role) { return ErrValidation } } else if e.Role!="" { return ErrValidation }
 return nil
}

type Rules struct {
 Enabled bool `json:"enabled"`
 Phrases []string `json:"phrases"`
 MaxPhraseOccurrences int `json:"max_phrase_occurrences"`
 MaxSentenceRepeats int `json:"max_sentence_repeats"`
 MinSentenceRunes int `json:"min_sentence_runes"`
 PreviousChapters int `json:"previous_chapters"`
}
func DefaultRules() Rules { return Rules{Enabled:true,Phrases:[]string{"不由得","嘴角勾起一抹","命运的齿轮"},MaxPhraseOccurrences:1,MaxSentenceRepeats:1,MinSentenceRunes:12,PreviousChapters:3} }
func (r Rules) Validate() error {
 if len(r.Phrases)>32 || r.MaxPhraseOccurrences<0 || r.MaxPhraseOccurrences>100 || r.MaxSentenceRepeats<1 || r.MaxSentenceRepeats>20 || r.MinSentenceRunes<4 || r.MinSentenceRunes>200 || r.PreviousChapters<0 || r.PreviousChapters>3 { return ErrValidation }
 seen:=map[string]bool{}
 for _,p:=range r.Phrases { p=strings.TrimSpace(p); if p=="" || len(p)>160 || !utf8.ValidString(p) || seen[p] || strings.ContainsAny(p,"\x00\r\n") { return ErrValidation }; seen[p]=true }
 return nil
}

type Mutation struct {
 ExpectedRevision int64 `json:"expected_revision"`
 Entry *Entry `json:"entry,omitempty"`
 DeleteID string `json:"delete_id,omitempty"`
 Rules *Rules `json:"rules,omitempty"`
}
type State struct { Revision int64 `json:"revision"`; Rules Rules `json:"rules"`; Entries []Entry `json:"entries"`; Builtins []Entry `json:"builtins"`; Total int `json:"total"`; Limit int `json:"limit"`; Offset int `json:"offset"` }
type Change struct { Revision int64 `json:"revision"`; EntryID string `json:"entry_id,omitempty"`; Replayed bool `json:"replayed"` }
type Selection struct {
 Revision int64 `json:"revision"`
 Role string `json:"role"`
 Entries []Entry `json:"entries"`
 Rules Rules `json:"rules"`
}
func Digest(v any) string { b,_:=json.Marshal(v); h:=sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Builtins() []Entry {
 out:=[]Entry{}
 for _,role:=range []string{"writing","review","polish","planning"} { b,err:=skillFiles.ReadFile("skills/"+role+".md"); if err!=nil { panic(err) }; out=append(out,Entry{ID:"builtin:"+role,Kind:"skill",Role:role,Title:role,Markdown:string(b),Enabled:true,Pinned:true,Priority:100}) }
 return out
}
func (s Selection) Instructions() string { b,_:=json.Marshal(s.Rules); return fmt.Sprintf("Advisory writing rules (not an AI detector; never override facts or continuity): %s",b) }
