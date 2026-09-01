package project

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var sensitiveConfigFragments = []string{
	"api_key",
	"apikey",
	"authorization",
	"credential",
	"password",
	"private_key",
	"secret",
	"token",
}

func sanitizeProjectConfig(sourceRoot, destinationRoot string) error {
	sourcePath := filepath.Join(sourceRoot, projectConfigRelative)
	data, err := os.ReadFile(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return writeJSONAtomic(
			filepath.Join(destinationRoot, projectConfigRelative),
			defaultProjectConfig(),
			0o600,
		)
	}
	if err != nil {
		return newError(
			"PROJECT_DUPLICATE_FAILED",
			"source project configuration could not be read",
			err,
		)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		// Never copy an opaque configuration blob because it could contain a
		// provider secret. A safe default is deterministic and reversible.
		value = defaultProjectConfig()
	} else {
		value = scrubSensitiveValue(value)
	}
	if object, ok := value.(map[string]any); ok {
		if _, exists := object["version"]; !exists {
			object["version"] = 1
		}
	}
	if err := writeJSONAtomic(
		filepath.Join(destinationRoot, projectConfigRelative),
		value,
		0o600,
	); err != nil {
		return newError(
			"PROJECT_DUPLICATE_FAILED",
			"duplicate project configuration could not be written",
			err,
		)
	}
	return nil
}

func scrubSensitiveValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		cleaned := make(map[string]any, len(typed))
		for _, key := range keys {
			if isSensitiveConfigKey(key) {
				continue
			}
			cleaned[key] = scrubSensitiveValue(typed[key])
		}
		return cleaned
	case []any:
		cleaned := make([]any, len(typed))
		for index, item := range typed {
			cleaned[index] = scrubSensitiveValue(item)
		}
		return cleaned
	default:
		return value
	}
}

func isSensitiveConfigKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	for _, fragment := range sensitiveConfigFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func stableProjectID(relative string) string {
	digest := sha256.Sum256([]byte(filepath.ToSlash(relative)))
	return hex.EncodeToString(digest[:8])
}
