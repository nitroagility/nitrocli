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
// If cliVersion is non-empty (i.e. not "dev"), it checks that the schema
// version used by the pipeline file matches the CLI version.
func Load(ctx context.Context, path string, cliVersion string) (*Config, error) {
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

	// Check schema version compatibility before doing anything else.
	if err := checkSchemaVersion(absPath, cliVersion); err != nil {
		return nil, err
	}

	raw, err := cueExport(ctx, absPath)
	if err != nil {
		return nil, err
	}

	strict := cliVersion != "" && cliVersion != "dev"
	cfg, err := parseOutput(raw, strict)
	if err != nil {
		return nil, err
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

const schemaModule = "github.com/nitroagility/nitrocli@v0"

func checkSchemaVersion(absPath string, cliVersion string) error {
	// Skip check in dev mode (no version injected).
	if cliVersion == "" || cliVersion == "dev" {
		return nil
	}

	dir := filepath.Dir(absPath)
	modulePath := filepath.Join(dir, "cue.mod", "module.cue")

	data, err := os.ReadFile(modulePath)
	if err != nil {
		// No cue.mod — skip check, cue export will catch it.
		return nil
	}

	schemaVersion := extractSchemaVersion(string(data), schemaModule)
	if schemaVersion == "" {
		// Schema dep not found — skip, could be using a different module.
		return nil
	}

	// Normalize: strip leading "v" for comparison.
	sv := strings.TrimPrefix(schemaVersion, "v")
	cv := strings.TrimPrefix(cliVersion, "v")

	if sv != cv {
		return &PipelineError{
			Phase:   "compat",
			Summary: "Schema version mismatch",
			Details: []string{
				fmt.Sprintf("CLI version:    v%s", cv),
				fmt.Sprintf("schema version: v%s", sv),
			},
			Hint: fmt.Sprintf(
				"update the schema: cd %s && cue mod get %s@v%s",
				dir, strings.TrimSuffix(schemaModule, "@v0"), cv,
			),
		}
	}

	return nil
}

func extractSchemaVersion(content string, module string) string {
	// Parse the module.cue to find: "module@v0": { v: "v0.0.X" }
	// Simple string scanning — no CUE parser needed.
	idx := strings.Index(content, module)
	if idx < 0 {
		return ""
	}

	rest := content[idx:]
	vIdx := strings.Index(rest, "v: \"")
	if vIdx < 0 {
		return ""
	}

	start := vIdx + 4 // len(`v: "`)
	end := strings.Index(rest[start:], "\"")
	if end < 0 {
		return ""
	}

	return rest[start : start+end]
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

func parseOutput(data []byte, strict bool) (*Config, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &PipelineError{
			Phase:   "parse",
			Summary: "Failed to parse pipeline output",
			Details: []string{err.Error()},
			Hint:    "ensure the pipeline file has a top-level 'config' field matching #PipelineFile",
		}
	}

	payload := data
	if configData, ok := raw["config"]; ok {
		payload = configData
	}

	if strict {
		// Use strict decoding to detect unknown fields from a newer schema version.
		var cfg Config
		dec := json.NewDecoder(strings.NewReader(string(payload)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			return nil, &PipelineError{
				Phase:   "compat",
				Summary: "Schema version is not compatible with this CLI",
				Details: []string{err.Error()},
				Hint:    "upgrade NitroCLI to a version that supports this schema, or downgrade the schema version",
			}
		}
		return &cfg, nil
	}

	// In dev mode, use lenient decoding (ignore unknown fields).
	var cfg Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, &PipelineError{
			Phase:   "parse",
			Summary: "Failed to parse pipeline output",
			Details: []string{err.Error()},
			Hint:    "ensure the pipeline file has a top-level 'config' field matching #PipelineFile",
		}
	}

	return &cfg, nil
}

func validate(cfg *Config) []string {
	var errs []string

	// Structure checks.
	if len(cfg.Artifacts) == 0 {
		errs = append(errs, "no artifacts defined — at least one artifact is required")
	}

	if len(cfg.Environments) == 0 {
		errs = append(errs, "no environments defined — at least one environment is required")
	}

	// Artifact type checks.
	validArtifactTypes := map[string]bool{"docker": true, "binary": true, "package": true}
	validRepoTypes := map[string]bool{"registry": true, "filesystem": true, "package": true}

	for name, art := range cfg.Artifacts {
		if !validArtifactTypes[art.Type] {
			errs = append(errs, fmt.Sprintf(
				"artifact %q: unknown type %q (supported: docker, binary, package)",
				name, art.Type,
			))
		}

		if !validRepoTypes[art.Repository.Type] {
			errs = append(errs, fmt.Sprintf(
				"artifact %q: unknown repository type %q (supported: registry, filesystem, package)",
				name, art.Repository.Type,
			))
		}

		if art.IsDocker() && len(art.Platforms) == 0 {
			errs = append(errs, fmt.Sprintf(
				"artifact %q: docker artifact requires at least one platform",
				name,
			))
		}

		if (art.IsBinary() || art.IsPackage()) && len(art.Build) == 0 {
			errs = append(errs, fmt.Sprintf(
				"artifact %q: %s artifact requires at least one build step",
				name, art.Type,
			))
		}
	}

	// Environment checks.
	validStrategies := map[string]bool{"build": true, "promote": true}

	for name, env := range cfg.Environments {
		if !validStrategies[env.Strategy] {
			errs = append(errs, fmt.Sprintf(
				"environment %q: unknown strategy %q (supported: build, promote)",
				name, env.Strategy,
			))
		}

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

		if env.Deploy != nil {
			validDeployTypes := map[string]bool{"helm": true}
			if !validDeployTypes[env.Deploy.Type] {
				errs = append(errs, fmt.Sprintf(
					"environment %q: unknown deploy type %q (supported: helm)",
					name, env.Deploy.Type,
				))
			}
		}
	}

	// Provider checks.
	validProviderTypes := map[string]bool{"bitwarden": true, "env": true, "transformer": true, "aws-secretsmanager": true}

	// Track variable name → environments across all providers to detect duplicates.
	// Key: variable name, Value: list of (provider, envs) pairs.
	type varScope struct {
		provider string
		envs     []string // empty = all environments
	}
	varEnvMap := make(map[string][]varScope)

	for name, p := range cfg.Providers {
		if !validProviderTypes[p.Type] {
			errs = append(errs, fmt.Sprintf(
				"provider %q: unknown type %q (supported: env, aws-secretsmanager, bitwarden, transformer)",
				name, p.Type,
			))
		}

		for _, v := range p.Variables {
			if strings.HasPrefix(v.Name, "NITRO_") {
				errs = append(errs, fmt.Sprintf(
					"provider %q: variable %q uses reserved prefix NITRO_ — this prefix is reserved for NitroCLI internal use",
					name, v.Name,
				))
			}

			if v.IsSecret() && v.Default != nil {
				errs = append(errs, fmt.Sprintf(
					"provider %q: variable %q is a secret and must not have a default value — use 'nitro config set %s <value>' to store it securely",
					name, v.Name, v.Name,
				))
			}

			// Effective envs = intersection of provider envs and variable envs.
			effectiveEnvs := effectiveVarEnvs(p.Envs, v.Envs)
			varEnvMap[v.Name] = append(varEnvMap[v.Name], varScope{provider: name, envs: effectiveEnvs})
		}

		validTransformerTypes := map[string]bool{"envfile": true, "template": true, "": true}
		for _, t := range p.Transformers {
			if strings.HasPrefix(t.Name, "NITRO_") {
				errs = append(errs, fmt.Sprintf(
					"provider %q: transformer %q uses reserved prefix NITRO_ — this prefix is reserved for NitroCLI internal use",
					name, t.Name,
				))
			}

			if !validTransformerTypes[t.Type] {
				errs = append(errs, fmt.Sprintf(
					"provider %q: transformer %q has unknown type %q (supported: envfile, template)",
					name, t.Name, t.Type,
				))
			}

			if t.EffectiveType() == "template" && t.Format == "" {
				errs = append(errs, fmt.Sprintf(
					"provider %q: transformer %q has type \"template\" but no format field",
					name, t.Name,
				))
			}

			if t.IsSecret() && t.Default != nil {
				errs = append(errs, fmt.Sprintf(
					"provider %q: transformer %q is a secret and must not have a default value",
					name, t.Name,
				))
			}

			effectiveEnvs := effectiveVarEnvs(p.Envs, t.Envs)
			varEnvMap[t.Name] = append(varEnvMap[t.Name], varScope{provider: name, envs: effectiveEnvs})
		}
	}

	// Check for duplicate variable name + overlapping environments.
	for varName, scopes := range varEnvMap {
		if len(scopes) < 2 {
			continue
		}
		for i := 0; i < len(scopes); i++ {
			for j := i + 1; j < len(scopes); j++ {
				if overlap := envsOverlap(scopes[i].envs, scopes[j].envs); overlap != "" {
					errs = append(errs, fmt.Sprintf(
						"variable %q is declared in providers %q and %q with overlapping environment %q — each variable name must be unique per environment",
						varName, scopes[i].provider, scopes[j].provider, overlap,
					))
				}
			}
		}
	}

	return errs
}

// effectiveVarEnvs returns the effective environment list for a variable.
// If the variable has its own envs, intersect with the provider envs.
// If either is empty (meaning "all"), use the other.
func effectiveVarEnvs(providerEnvs, varEnvs []string) []string {
	if len(varEnvs) == 0 {
		return providerEnvs // variable inherits provider scope
	}
	if len(providerEnvs) == 0 {
		return varEnvs // provider applies to all, use variable scope
	}
	// Intersection.
	pSet := make(map[string]bool, len(providerEnvs))
	for _, e := range providerEnvs {
		pSet[e] = true
	}
	var result []string
	for _, e := range varEnvs {
		if pSet[e] {
			result = append(result, e)
		}
	}
	return result
}

// envsOverlap returns the first overlapping environment between two scopes.
// An empty slice means "all environments", which overlaps with everything.
func envsOverlap(a, b []string) string {
	if len(a) == 0 || len(b) == 0 {
		// One side applies to all environments → always overlaps.
		if len(a) == 0 && len(b) == 0 {
			return "*"
		}
		if len(a) == 0 {
			return b[0]
		}
		return a[0]
	}
	set := make(map[string]bool, len(a))
	for _, e := range a {
		set[e] = true
	}
	for _, e := range b {
		if set[e] {
			return e
		}
	}
	return ""
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
