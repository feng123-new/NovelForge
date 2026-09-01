package project

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const foundationRequestRelative = ".novelforge/foundation-request.json"

// AutomationSettings records the requested operating mode without claiming that
// the durable Phase 9 worker is already available.
type AutomationSettings struct {
	Mode            string `json:"mode"`
	ReviewPolicy    string `json:"review_policy"`
	ReviewEveryN    int    `json:"review_every_n,omitempty"`
	MaxRewrites     int    `json:"max_rewrites"`
	WorkerAvailable bool   `json:"worker_available"`
}

// FoundationRequestInput is the non-secret wizard payload.
type FoundationRequestInput struct {
	Idea         string             `json:"idea"`
	Style        string             `json:"style,omitempty"`
	ModelProfile map[string]string  `json:"model_profile,omitempty"`
	Automation   AutomationSettings `json:"automation"`
}

// FoundationRequest is an immutable request snapshot persisted in the project.
type FoundationRequest struct {
	ID           string             `json:"id"`
	ProjectID    string             `json:"project_id"`
	Status       string             `json:"status"`
	Idea         string             `json:"idea"`
	Style        string             `json:"style,omitempty"`
	ModelProfile map[string]string  `json:"model_profile,omitempty"`
	Automation   AutomationSettings `json:"automation"`
	CreatedAt    time.Time          `json:"created_at"`
}

// SaveFoundationRequest validates and persists the real wizard request. It does
// not execute Autopilot and marks worker availability explicitly.
func (r *Repository) SaveFoundationRequest(
	_ context.Context,
	projectID string,
	input FoundationRequestInput,
) (FoundationRequest, error) {
	candidate, err := r.find(projectID)
	if err != nil {
		return FoundationRequest{}, err
	}
	input.Idea = strings.TrimSpace(input.Idea)
	input.Style = strings.TrimSpace(input.Style)
	if err := validateFoundationRequest(input); err != nil {
		return FoundationRequest{}, err
	}
	id, err := r.newFoundationID()
	if err != nil {
		return FoundationRequest{}, newError(
			"FOUNDATION_REQUEST_FAILED",
			"foundation request identifier could not be created",
			err,
		)
	}
	input.Automation.WorkerAvailable = false
	if input.Automation.MaxRewrites == 0 {
		input.Automation.MaxRewrites = 2
	}
	request := FoundationRequest{
		ID:           id,
		ProjectID:    projectID,
		Status:       "requested",
		Idea:         input.Idea,
		Style:        input.Style,
		ModelProfile: cloneStringMap(input.ModelProfile),
		Automation:   input.Automation,
		CreatedAt:    r.now().UTC(),
	}
	if err := writeJSONAtomic(
		filepath.Join(candidate.Root, foundationRequestRelative),
		request,
		0o600,
	); err != nil {
		return FoundationRequest{}, newError(
			"FOUNDATION_REQUEST_FAILED",
			"foundation request could not be stored",
			err,
		)
	}
	return request, nil
}

// GetFoundationRequest returns the current persisted request, when present.
func (r *Repository) GetFoundationRequest(
	_ context.Context,
	projectID string,
) (FoundationRequest, error) {
	candidate, err := r.find(projectID)
	if err != nil {
		return FoundationRequest{}, err
	}
	path := filepath.Join(candidate.Root, foundationRequestRelative)
	var request FoundationRequest
	if err := readJSONFile(path, &request); errors.Is(err, os.ErrNotExist) {
		return FoundationRequest{}, newError(
			"FOUNDATION_REQUEST_NOT_FOUND",
			"foundation request not found",
			ErrNotFound,
		)
	} else if err != nil {
		return FoundationRequest{}, newError(
			"FOUNDATION_REQUEST_FAILED",
			"foundation request could not be read",
			err,
		)
	}
	return request, nil
}

func validateFoundationRequest(input FoundationRequestInput) error {
	if input.Idea == "" || utf8.RuneCountInString(input.Idea) > 10_000 {
		return newError(
			"FOUNDATION_REQUEST_INVALID",
			"idea must contain between 1 and 10000 characters",
			ErrValidation,
		)
	}
	if likelySecretText(input.Idea) || likelySecretText(input.Style) {
		return newError(
			"FOUNDATION_SECRET_REJECTED",
			"credentials must not be stored in a foundation request",
			ErrValidation,
		)
	}
	if utf8.RuneCountInString(input.Style) > 4_000 {
		return newError(
			"FOUNDATION_REQUEST_INVALID",
			"style must not exceed 4000 characters",
			ErrValidation,
		)
	}
	if len(input.ModelProfile) > 16 {
		return newError(
			"FOUNDATION_REQUEST_INVALID",
			"model profile contains too many roles",
			ErrValidation,
		)
	}
	for key, value := range input.ModelProfile {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || len(key) > 64 || value == "" || len(value) > 256 {
			return newError(
				"FOUNDATION_REQUEST_INVALID",
				"model profile entries are invalid",
				ErrValidation,
			)
		}
		if likelyCredential(key, value) {
			return newError(
				"FOUNDATION_SECRET_REJECTED",
				"credentials must not be stored in a foundation request",
				ErrValidation,
			)
		}
	}
	switch input.Automation.Mode {
	case "copilot", "autopilot":
	default:
		return newError(
			"FOUNDATION_REQUEST_INVALID",
			"automation mode must be copilot or autopilot",
			ErrValidation,
		)
	}
	switch input.Automation.ReviewPolicy {
	case "every_chapter", "full_automatic":
		if input.Automation.ReviewEveryN != 0 {
			return newError(
				"FOUNDATION_REQUEST_INVALID",
				"review_every_n is only valid for every_n review policy",
				ErrValidation,
			)
		}
	case "every_n":
		if input.Automation.ReviewEveryN < 1 || input.Automation.ReviewEveryN > 100 {
			return newError(
				"FOUNDATION_REQUEST_INVALID",
				"review_every_n must be between 1 and 100",
				ErrValidation,
			)
		}
	default:
		return newError(
			"FOUNDATION_REQUEST_INVALID",
			"review policy is invalid",
			ErrValidation,
		)
	}
	if input.Automation.MaxRewrites != 0 &&
		(input.Automation.MaxRewrites < 1 || input.Automation.MaxRewrites > 5) {
		return newError(
			"FOUNDATION_REQUEST_INVALID",
			"max_rewrites must be between 1 and 5",
			ErrValidation,
		)
	}
	return nil
}

func likelyCredential(key, value string) bool {
	lowerKey := strings.ToLower(key)
	for _, fragment := range []string{"api_key", "apikey", "secret", "token", "password", "authorization"} {
		if strings.Contains(lowerKey, fragment) {
			return true
		}
	}
	return likelySecretText(value)
}

func likelySecretText(value string) bool {
	lowerValue := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lowerValue, "sk-") ||
		strings.Contains(lowerValue, " bearer ") ||
		strings.HasPrefix(lowerValue, "bearer ") ||
		strings.Contains(lowerValue, "api_key=") ||
		strings.Contains(lowerValue, `"api_key"`) ||
		strings.Contains(lowerValue, `"authorization"`)
}

func (r *Repository) newFoundationID() (string, error) {
	buffer := make([]byte, 12)
	if _, err := io.ReadFull(r.random, buffer); err != nil {
		return "", err
	}
	return "fr_" + hex.EncodeToString(buffer), nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty JSON file")
	}
	return json.Unmarshal(data, target)
}
