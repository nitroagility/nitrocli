// Package config manages the NitroCLI local configuration file (~/.nitro/config.json).
// Entries are stored as JSON with restricted file permissions (0600).
// Each entry tracks whether it is a secret (masked in output) or a plain config value.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	dirName  = ".nitro"
	fileName = "config.json"
	dirPerm  = 0o700
	filePerm = 0o600
)

// Entry is a single configuration value with metadata.
type Entry struct {
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// configPath returns the absolute path to ~/.nitro/config.json.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, dirName, fileName), nil
}

// ensureDir creates ~/.nitro with restricted permissions if it doesn't exist.
func ensureDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, dirName)
	return os.MkdirAll(dir, dirPerm)
}

// readAll reads the config file and returns all entries.
func readAll() (map[string]Entry, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]Entry), nil
		}
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	entries := make(map[string]Entry)
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("corrupt config file: %w", err)
	}

	return entries, nil
}

// writeAll writes all entries to the config file with secure permissions.
func writeAll(entries map[string]Entry) error {
	if err := ensureDir(); err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return os.WriteFile(path, append(data, '\n'), filePerm)
}

// Set stores a key-value pair in the config file.
// If secret is true, the value will be masked when listed.
func Set(key, value string, secret bool) error {
	entries, err := readAll()
	if err != nil {
		return err
	}
	entries[key] = Entry{Value: value, Secret: secret}
	return writeAll(entries)
}

// Get returns the entry for a key, or (Entry{}, false) if not found.
func Get(key string) (Entry, bool, error) {
	entries, err := readAll()
	if err != nil {
		return Entry{}, false, err
	}
	e, ok := entries[key]
	return e, ok, nil
}

// Delete removes a key from the config file.
func Delete(key string) error {
	entries, err := readAll()
	if err != nil {
		return err
	}
	delete(entries, key)
	return writeAll(entries)
}

// List returns all entries sorted by key.
func List() ([]ListEntry, error) {
	entries, err := readAll()
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]ListEntry, 0, len(entries))
	for _, k := range keys {
		e := entries[k]
		result = append(result, ListEntry{Key: k, Entry: e})
	}
	return result, nil
}

// ListEntry is a key-entry pair for ordered iteration.
type ListEntry struct {
	Key string
	Entry
}

// Lookup returns the value for a key if it exists in the config file.
// This is intended for use by providers as a fallback resolution.
func Lookup(key string) (string, bool) {
	e, ok, err := Get(key)
	if err != nil || !ok {
		return "", false
	}
	return e.Value, true
}
