package pulse

import (
	"fmt"
	"strings"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

// parseKeyValuePairs parses a comma-separated list of key=value pairs.
// Example: "key1=val1, key2=val2" → [{Key:"key1", Value:"val1"}, {Key:"key2", Value:"val2"}]
func parseKeyValuePairs(input string) ([]api.KeyValue, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}

	parts := strings.Split(input, ",")
	var result []api.KeyValue

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			return nil, fmt.Errorf("invalid key=value pair: %q (missing '=')", part)
		}

		key := strings.TrimSpace(part[:eqIdx])
		value := strings.TrimSpace(part[eqIdx+1:])

		if key == "" {
			return nil, fmt.Errorf("invalid key=value pair: %q (empty key)", part)
		}
		if value == "" {
			return nil, fmt.Errorf("invalid key=value pair: %q (empty value)", part)
		}

		// Normalize key: lowercase, spaces → underscores
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, " ", "_")

		result = append(result, api.KeyValue{Key: key, Value: value})
	}

	return result, nil
}

// parseCommaSeparated splits a comma-separated string into trimmed, non-empty parts.
func parseCommaSeparated(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}

	parts := strings.Split(input, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
