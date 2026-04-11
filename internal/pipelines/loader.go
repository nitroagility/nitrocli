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

// PipelineError is a structured error with phase, summary, and detail lines.
type PipelineError struct {
	Phase   string
	Summary string
	Details []string
	Hint    string
}

func (e *PipelineError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Phase, e.Summary)
}

// FormatError renders any error with styled output.
func FormatError(err error) string {
	var pe *PipelineError
	if !errors.As(err, &pe) {
		return "\n" +
			styleRed.Render("  ✗ Error") + "\n" +
			styleDim.Render("  ──────────────────────────────────────────────") + "\n" +
			"  " + styleMuted.Render(err.Error()) + "\n" +
			styleDim.Render("  ──────────────────────────────────────────────") + "\n"
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styleRed.Render("  ✗ "+pe.Summary) + styleDim.Render(fmt.Sprintf("  [%s]", pe.Phase)) + "\n")
	b.WriteString(styleDim.Render("  ──────────────────────────────────────────────") + "\n")

	for _, d := range pe.Details {
		for _, line := range strings.Split(strings.TrimSpace(d), "\n") {
			if line == "" {
				continue
			}
			b.WriteString("  " + styleMuted.Render("  "+line) + "\n")
		}
	}

	b.WriteString(styleDim.Render("  ──────────────────────────────────────────────") + "\n")

	if pe.Hint != "" {
		b.WriteString("  " + styleYellow.Render("hint: ") + styleMuted.Render(pe.Hint) + "\n")
	}

	return b.String()
}

// Load reads a CUE pipeline file, validates it via CUE, parses the config,
// and runs logical validation on the resulting structure.
func Load(ctx context.Context, path string) (*Config, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, &PipelineError{
			Phase:   "resolve",
			Summary: "Cannot resolve pipeline path",
			Details: []string{err.Error()},
		}
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, &PipelineError{
			Phase:   "load",
			Summary: "Pipeline file not found",
			Details: []string{absPath},
			Hint:    "check the --pipeline flag or ensure the file exists in the current directory",
		}
	}

	raw, err := cueExport(ctx, absPath)
	if err != nil {
		return nil, err
	}

	cfg, err := parseOutput(raw)
	if err != nil {
		return nil, &PipelineError{
			Phase:   "parse",
			Summary: "Failed to parse pipeline output",
			Details: []string{err.Error()},
			Hint:    "ensure the pipeline file has a top-level 'config' field matching #PipelineFile",
		}
	}

	if errs := validate(cfg); len(errs) > 0 {
		return nil, &PipelineError{
			Phase:   "validate",
			Summary: "Pipeline structure is invalid",
			Details: errs,
			Hint:    "fix the issues above and re-run",
		}
	}

	return cfg, nil
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
			return nil, &PipelineError{
				Phase:   "schema",
				Summary: "CUE schema validation failed",
				Details: lines,
				Hint:    "check field names and types against the published schema",
			}
		}

		if errors.Is(err, exec.ErrNotFound) {
			return nil, &PipelineError{
				Phase:   "schema",
				Summary: "CUE CLI not found",
				Details: []string{"the 'cue' command is not installed or not in PATH"},
				Hint:    "install CUE: https://cuelang.org/docs/install/",
			}
		}

		return nil, &PipelineError{
			Phase:   "schema",
			Summary: "Failed to run CUE export",
			Details: []string{err.Error()},
		}
	}

	return out, nil
}

func parseOutput(data []byte) (*Config, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	payload := data
	if configData, ok := raw["config"]; ok {
		payload = configData
	}

	var cfg Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, fmt.Errorf("cannot decode config: %w", err)
	}

	return &cfg, nil
}

func validate(cfg *Config) []string {
	var errs []string

	if len(cfg.Artifacts) == 0 {
		errs = append(errs, "no artifacts defined — at least one artifact is required")
	}

	if len(cfg.Environments) == 0 {
		errs = append(errs, "no environments defined — at least one environment is required")
	}

	for name, env := range cfg.Environments {
		if env.PromotesFrom != "" {
			if _, ok := cfg.Environments[env.PromotesFrom]; !ok {
				errs = append(errs, fmt.Sprintf(
					"environment %q: promotesFrom references %q which does not exist",
					name, env.PromotesFrom,
				))
			}
		}

		for _, artName := range env.Artifacts {
			if _, ok := cfg.Artifacts[artName]; !ok {
				errs = append(errs, fmt.Sprintf(
					"environment %q: references artifact %q which does not exist",
					name, artName,
				))
			}
		}

		if env.IsPromote() && env.PromotesFrom == "" {
			errs = append(errs, fmt.Sprintf(
				"environment %q: strategy is 'promote' but promotesFrom is not set",
				name,
			))
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
