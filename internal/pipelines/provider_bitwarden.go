package pipelines

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/nitroagility/nitrocli/internal/config"
)

// bwsTokenEnvVar is the environment variable name for the Bitwarden access token.
const bwsTokenEnvVar = "BWS_ACCESS_TOKEN"

// bitwardenResolver resolves variables from Bitwarden Secrets Manager
// using the bws CLI. The access token is resolved from:
//  1. OS env var BWS_ACCESS_TOKEN
//  2. ~/.nitro/config.json key BWS_ACCESS_TOKEN
type bitwardenResolver struct {
	token string
	url   string // server URL override (empty = default cloud)
}

// newBitwardenResolver creates a Bitwarden resolver, looking up the access token.
func newBitwardenResolver(url string) (*bitwardenResolver, error) {
	// 1. OS environment variable.
	token := os.Getenv(bwsTokenEnvVar)

	// 2. Nitro local config.
	if token == "" {
		token, _ = config.Lookup(bwsTokenEnvVar)
	}

	if token == "" {
		return nil, fmt.Errorf(
			"%s not set — provide it via environment variable or 'nitro config set %s <token> --secret'",
			bwsTokenEnvVar, bwsTokenEnvVar,
		)
	}

	return &bitwardenResolver{token: token, url: url}, nil
}

// bwsSecret is the JSON structure returned by `bws secret get`.
type bwsSecret struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Note  string `json:"note"`
}

// resolve fetches a secret from Bitwarden Secrets Manager.
// v.Path is the secret UUID in Bitwarden.
// If v.Key is set and the secret value is JSON, extracts that specific key.
func (b *bitwardenResolver) resolve(ctx context.Context, v *Variable) (string, error) {
	args := []string{"secret", "get", v.Path, "--access-token", b.token, "--output", "json"}

	cmd := exec.CommandContext(ctx, "bws", args...)

	// Set server URL if configured (for self-hosted Bitwarden).
	if b.url != "" {
		cmd.Env = append(os.Environ(), "BWS_SERVER_URL="+b.url)
	}

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			return "", fmt.Errorf("bws secret get %q failed: %s", v.Path, string(exitErr.Stderr))
		}
		if isNotFound(err) {
			return "", errors.New("bws CLI not found — install it: https://bitwarden.com/help/secrets-manager-cli/")
		}
		return "", fmt.Errorf("bws secret get %q: %w", v.Path, err)
	}

	var secret bwsSecret
	if err := json.Unmarshal(out, &secret); err != nil {
		return "", fmt.Errorf("failed to parse bws output for %q: %w", v.Path, err)
	}

	raw := secret.Value

	// If key is specified, extract from JSON value.
	if v.Key != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return "", fmt.Errorf("secret %q value is not valid JSON (needed to extract key %q): %w", v.Path, v.Key, err)
		}
		val, ok := m[v.Key]
		if !ok {
			return "", fmt.Errorf("secret %q does not contain key %q", v.Path, v.Key)
		}
		switch tv := val.(type) {
		case string:
			return tv, nil
		default:
			b, _ := json.Marshal(tv)
			return string(b), nil
		}
	}

	return raw, nil
}

func isExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}

func isNotFound(err error) bool {
	return err == exec.ErrNotFound
}
