package qualitygate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type TruthContinuityService struct {
	Truth truthstore.Repository
}

func (s TruthContinuityService) Check(ctx context.Context, request ContinuityRequest) (ContinuityResult, error) {
	if s.Truth == nil {
		return ContinuityResult{}, fmt.Errorf("truth repository is required")
	}
	result := ContinuityResult{Status: ContinuityPass, Issues: []ContinuityIssue{}}
	for _, change := range request.Proposal.AllChanges() {
		subjectType, subjectID := splitSubject(change.Subject)
		page, err := s.Truth.State(ctx, truthstore.StateQuery{
			Chapter:     request.Chapter,
			SubjectType: subjectType,
			SubjectID:   subjectID,
			Predicate:   change.Predicate,
			Limit:       10,
		})
		if err != nil {
			return ContinuityResult{}, err
		}
		for _, fact := range page.Facts {
			if bytes.Equal(bytes.TrimSpace(fact.Value), bytes.TrimSpace(change.Object)) {
				continue
			}
			severity := SeverityWarning
			status := ContinuityWarn
			if isBlockingPredicate(change.Predicate) {
				severity = SeverityBlocking
				status = ContinuityFail
				result.Blocking = true
			}
			if status == ContinuityFail || result.Status == ContinuityPass {
				result.Status = status
			}
			result.Issues = append(result.Issues, ContinuityIssue{
				IssueCode:       continuityCode(change.Predicate),
				Severity:        severity,
				Entity:          change.Subject,
				Predicate:       change.Predicate,
				Expected:        append(json.RawMessage(nil), fact.Value...),
				Actual:          append(json.RawMessage(nil), change.Object...),
				Evidence:        fmt.Sprintf("authoritative Chapter-%d truth event %s", request.Chapter, fact.ID),
				SourceChapter:   fact.Source.Chapter,
				SourceVersion:   fact.Source.Version,
				SuggestedAction: "rewrite the draft or explicitly supersede the authoritative fact after acceptance",
			})
		}
	}
	return result, result.Validate()
}

func splitSubject(subject string) (string, string) {
	parts := strings.SplitN(subject, ":", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "entity", strings.TrimSpace(subject)
}

func isBlockingPredicate(predicate string) bool {
	switch strings.ToLower(strings.TrimSpace(predicate)) {
	case "alive", "dead", "location", "current_location", "inventory", "knowledge", "knowledge_holder", "timeline", "world_rule", "relationship", "injury", "cultivation", "plot_sequence":
		return true
	default:
		return false
	}
}

func continuityCode(predicate string) string {
	name := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.TrimSpace(predicate)))
	if name == "" {
		name = "FACT"
	}
	return "CONTINUITY_" + name + "_CONFLICT"
}
