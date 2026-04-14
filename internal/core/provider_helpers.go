package core

import (
	"encoding/json"
	"fmt"
)

// extractJSONKey parses raw as JSON and returns the value of the given key.
func extractJSONKey(raw, secretPath, key string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", fmt.Errorf("secret %q is not valid JSON (needed to extract key %q): %w", secretPath, key, err)
	}

	val, ok := m[key]
	if !ok {
		return "", fmt.Errorf("secret %q does not contain key %q", secretPath, key)
	}

	switch tv := val.(type) {
	case string:
		return tv, nil
	default:
		b, _ := json.Marshal(tv)
		return string(b), nil
	}
}
