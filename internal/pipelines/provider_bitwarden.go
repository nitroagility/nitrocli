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
// The access token is passed via environment variable (not CLI args) to avoid
// exposing it in process listings (ps).
func (b *bitwardenResolver) resolve(ctx context.Context, v *Variable) (string, error) {
	args := []string{"secret", "get", v.Path, "--output", "json"}

	cmd := exec.CommandContext(ctx, "bws", args...)

	// Pass token and server URL via environment — never on the command line.
	cmd.Env = append(os.Environ(), bwsTokenEnvVar+"="+b.token)
	if b.url != "" {
		cmd.Env = append(cmd.Env, "BWS_SERVER_URL="+b.url)
	}

	out, err := cmd.Output()
	if err != nil {
		if isNotFound(err) {
			return "", errors.New("bws CLI not found — install it: https://bitwarden.com/help/secrets-manager-cli/")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("bws secret get %q failed (exit %d)", v.Path, exitErr.ExitCode())
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
		return extractJSONKey(raw, v.Path, v.Key)
	}

	return raw, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}
