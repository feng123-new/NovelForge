package contextcompiler

import (
	"fmt"
	"strings"
)

// SelectLegacyMap preserves selected fields' Go types and legacy field shape.
// No unselected payload is returned alongside compiler diagnostics.
func SelectLegacyMap(raw map[string]any, compiled Result) (map[string]any, error) {
	selected := make(map[string]any)
	for _, item := range compiled.Items {
		if !strings.HasPrefix(item.ID, "legacy:") {
			return nil, fmt.Errorf("unexpected legacy context item %q", item.ID)
		}
		path := strings.TrimPrefix(item.ID, "legacy:")
		if value, ok := raw[path]; ok {
			selected[path] = value
			continue
		}
		parts := strings.SplitN(path, ".", 2)
		if len(parts) != 2 || !isLegacyContainer(parts[0]) {
			return nil, fmt.Errorf("unknown legacy context path %q", path)
		}
		source, ok := raw[parts[0]].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("missing legacy container %q", parts[0])
		}
		value, ok := source[parts[1]]
		if !ok {
			return nil, fmt.Errorf("missing legacy context field %q", path)
		}
		target, ok := selected[parts[0]].(map[string]any)
		if !ok {
			target = make(map[string]any)
			selected[parts[0]] = target
		}
		target[parts[1]] = value
		// Preserve the old safety note only when this container has selected data.
		if usage, ok := source["_usage"]; ok {
			target["_usage"] = usage
		}
	}
	if warnings, ok := raw["_warnings"]; ok {
		selected["_warnings"] = warnings
	}
	return selected, nil
}
