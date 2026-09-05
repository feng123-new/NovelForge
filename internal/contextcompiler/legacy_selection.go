package contextcompiler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SelectLegacyMap projects the compiler's selected records back into the
// existing tool shape. The original map is not mutated and dropped records are
// not returned to the model through an unbudgeted compatibility copy.
func SelectLegacyMap(ctx context.Context, raw map[string]any, request Request, counter TokenCounter) (map[string]any, Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, Result{}, err
	}
	// The old classifier skips individual marshal failures. Do not let a
	// non-serializable mandatory value disappear before validation.
	if _, err := json.Marshal(raw); err != nil {
		return nil, Result{}, fmt.Errorf("encode context input: %w", err)
	}
	classified := classifyLegacy(raw, request.Chapter)
	var all []Item
	for _, layer := range layerOrder {
		for _, item := range classified[layer] {
			// Preserve the old non-trimmable contract (progress, plans, user
			// rules, state and bounded statistics). Explicit requirements are
			// mandatory even when their containing field is normally optional.
			if !legacyOptional(item.Kind) {
				item.Mandatory = true
			}
			all = append(all, item)
		}
	}
	compiler := New(Providers{Truth: ProviderFunc(func(context.Context, Request) ([]Item, error) {
		return all, nil
	})}, counter)
	compiled, err := compiler.Compile(ctx, request)
	if err != nil {
		return nil, Result{}, err
	}
	selected := make(map[string]bool, len(compiled.Items))
	for _, item := range compiled.Items {
		selected[item.ID] = true
	}
	out := make(map[string]any, len(raw))
	trimmed := []string{}
	for key, value := range raw {
		if strings.HasPrefix(key, "_") {
			if key == "_warnings" {
				out[key] = value
			}
			continue
		}
		if section, ok := value.(map[string]any); ok && isLegacyContainer(key) {
			next := make(map[string]any, len(section))
			for child, content := range section {
				path := key + "." + child
				if strings.HasPrefix(child, "_") {
					// Usage notes are not an alternative story-content channel.
					if child == "_usage" {
						next[child] = content
					}
				} else if selected["legacy:"+path] {
					next[child] = content
				} else {
					trimmed = append(trimmed, path)
				}
			}
			out[key] = next
		} else if selected["legacy:"+key] {
			out[key] = value
		} else {
			trimmed = append(trimmed, key)
		}
	}
	if len(trimmed) > 0 {
		sort.Strings(trimmed)
		out["_trimmed"] = trimmed
	}
	return out, compiled, nil
}

func legacyOptional(kind string) bool {
	switch kind {
	case "references", "voice_samples", "style_anchors", "style_rules", "previous_tail", "timeline", "recent_state_changes", "foreshadow_ledger", "relationship_state":
		return true
	default:
		return false
	}
}
