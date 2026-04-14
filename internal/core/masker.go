package core

import (
	"strings"
	"sync"
)

// Masker replaces secret values with asterisks in any string.
// It is safe for concurrent use.
type Masker struct {
	mu      sync.RWMutex
	secrets []string
}

// Add registers a secret value for masking.
// Empty strings are ignored.
func (m *Masker) Add(value string) {
	if value == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Avoid duplicates.
	for _, s := range m.secrets {
		if s == value {
			return
		}
	}

	m.secrets = append(m.secrets, value)
}

// Mask replaces all occurrences of registered secret values with asterisks.
// Each character of the secret is replaced with '*'.
func (m *Masker) Mask(input string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := input
	for _, s := range m.secrets {
		masked := strings.Repeat("*", len(s))
		result = strings.ReplaceAll(result, s, masked)
	}

	return result
}

// HasSecrets returns true if any secrets are registered.
func (m *Masker) HasSecrets() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.secrets) > 0
}
