package pipelines

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Load reads a CUE pipeline file, validates it via CUE, parses the config,
// and runs logical validation on the resulting structure.
func Load(ctx context.Context, path string) (*Config, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, &PipelineError{Phase: "resolve", Details: []string{err.Error()}}
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, &PipelineError{Phase: "load", Details: []string{fmt.Sprintf("file not found: %s", absPath)}}
	}

	raw, err := cueExport(ctx, absPath)
	if err != nil {
		return nil, err
	}

	cfg, err := parseOutput(raw)
	if err != nil {
		return nil, &PipelineError{Phase: "parse", Details: []string{err.Error()}}
	}

	if errs := validate(cfg); len(errs) > 0 {
		return nil, &PipelineError{Phase: "validate", Details: errs}
	}

	return cfg, nil
}

// PipelineError is a structured error with phase and detail lines.
type PipelineError struct {
	Phase   string
	Details []string
}

func (e *PipelineError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Phase, strings.Join(e.Details, "; "))
}

// FormatError renders a PipelineError with styled output.
func FormatError(err error) string {
	var pe *PipelineError
	if !errors.As(err, &pe) {
		return styleRed.Render("  error: ") + styleMuted.Render(err.Error())
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styleRed.Render("  ✗ Pipeline failed") + styleDim.Render(fmt.Sprintf(" [%s]", pe.Phase)) + "\n")
	b.WriteString(styleDim.Render("  ──────────────────────────────────────────") + "\n")
	for _, d := range pe.Details {
		for _, line := range strings.Split(strings.TrimSpace(d), "\n") {
			if line == "" {
				continue
			}
			b.WriteString("  " + styleMuted.Render(line) + "\n")
		}
	}
	b.WriteString(styleDim.Render("  ──────────────────────────────────────────") + "\n")
	return b.String()
}

func cueExport(ctx context.Context, absPath string) ([]byte, error) {
	dir := filepath.Dir(absPath)
	filename := filepath.Base(absPath)

	cmd := exec.CommandContext(ctx, "cue", "export", "--out", "json", filename)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			lines := parseErrorLines(string(exitErr.Stderr))
			return nil, &PipelineError{Phase: "schema", Details: lines}
		}
		return nil, &PipelineError{Phase: "schema", Details: []string{err.Error()}}
	}

	return out, nil
}

func parseOutput(data []byte) (*Config, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("cannot parse CUE output: %w", err)
	}

	payload := data
	if configData, ok := raw["config"]; ok {
		payload = configData
	}

	var cfg Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse pipeline config: %w", err)
	}

	return &cfg, nil
}

func validate(cfg *Config) []string {
	var errs []string

	if len(cfg.Artifacts) == 0 {
		errs = append(errs, "no artifacts defined")
	}

	if len(cfg.Environments) == 0 {
		errs = append(errs, "no environments defined")
	}

	for name, env := range cfg.Environments {
		if env.PromotesFrom != "" {
			if _, ok := cfg.Environments[env.PromotesFrom]; !ok {
				errs = append(errs, fmt.Sprintf(
					"environment %q promotes from %q which does not exist",
					name, env.PromotesFrom,
				))
			}
		}

		for _, artName := range env.Artifacts {
			if _, ok := cfg.Artifacts[artName]; !ok {
				errs = append(errs, fmt.Sprintf(
					"environment %q references artifact %q which does not exist",
					name, artName,
				))
			}
		}
	}

	return errs
}

func parseErrorLines(stderr string) []string {
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}
