package pipelines

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"

	"github.com/nitroagility/nitrocli/internal/config"
)

// ProviderResolver resolves secrets from configured providers for a given environment.
type ProviderResolver struct {
	Log         *Logger
	Masker      *Masker
	Globals     map[string]bool // allowlist: only these variables can be resolved from ~/.nitro/config.json
	awsResolver *awsResolver
	bwResolver  *bitwardenResolver
}

// Resolve collects all variables from providers that apply to the given environment.
//
// Resolution order (lowest to highest priority):
//  1. Globals from ~/.nitro/config.json (bootstrap baseline)
//  2. Providers sorted by priority descending (high number = low priority, loaded first)
//  3. Priority 1 = maximum priority (loaded last, overwrites everything)
//
// Each layer overwrites previous values. Secret values are registered in the Masker.
func (r *ProviderResolver) Resolve(ctx context.Context, providers map[string]*Provider, envName string) map[string]string {
	vars := make(map[string]string)
	secretCount := 0

	// Phase 1: bootstrap globals from ~/.nitro/config.json.
	r.loadGlobals(vars, &secretCount)

	// Phase 2: resolve providers (high priority number first → low priority number last wins).
	applicable := r.applicableProviders(providers, envName)
	sort.Slice(applicable, func(i, j int) bool {
		return applicable[i].provider.Priority > applicable[j].provider.Priority
	})

	for _, entry := range applicable {
		p := entry.provider

		switch p.Type {
		case "transformer":
			r.Log.Info(fmt.Sprintf("provider %q (priority: %d, type: %s) → %d transformers",
				entry.name, p.Priority, p.Type, len(p.Transformers)))
			r.resolveTransformers(p, vars, &secretCount, envName)
		default:
			r.Log.Info(fmt.Sprintf("provider %q (priority: %d, type: %s) → %d variables",
				entry.name, p.Priority, p.Type, len(p.Variables)))
			r.resolveStandardVars(ctx, p, vars, &secretCount, envName)
		}
	}

	r.Log.Info(fmt.Sprintf("session: %d variables loaded (%d secrets masked)", len(vars), secretCount))

	return vars
}

// loadGlobals pre-loads variables from ~/.nitro/config.json that are in the globals allowlist.
func (r *ProviderResolver) loadGlobals(vars map[string]string, secretCount *int) {
	if len(r.Globals) == 0 {
		return
	}

	r.Log.Step("Loading globals from ~/.nitro/config.json")
	loaded := 0

	for name := range r.Globals {
		value, ok := config.Lookup(name)
		if !ok {
			continue
		}
		vars[name] = value
		loaded++

		// Check if it's stored as a secret in the config.
		entry, entryOk, _ := config.Get(name)
		if entryOk && entry.Secret {
			r.Masker.Add(value)
			*secretCount++
		}
	}

	r.Log.Info(fmt.Sprintf("  %d/%d globals loaded", loaded, len(r.Globals)))
}

func (r *ProviderResolver) resolveStandardVars(ctx context.Context, p *Provider, vars map[string]string, secretCount *int, envName string) {
	for i := range p.Variables {
		v := &p.Variables[i]

		if !v.AppliesToEnv(envName) {
			continue
		}

		value := r.resolveVariable(ctx, p, v)
		if value == "" {
			continue // no value resolved, keep previous (global or lower-priority provider)
		}

		vars[v.Name] = value

		if v.IsSecret() {
			r.Masker.Add(value)
			*secretCount++
		}
	}
}

func (r *ProviderResolver) resolveTransformers(p *Provider, vars map[string]string, secretCount *int, envName string) {
	for i := range p.Transformers {
		t := &p.Transformers[i]

		if !t.AppliesToEnv(envName) {
			continue
		}

		// Collect referenced variable values.
		refVars := make(map[string]string, len(t.Vars))
		for _, varName := range t.Vars {
			val, ok := vars[varName]
			if !ok {
				r.Log.Fail(fmt.Sprintf("  %s: references %q which is not resolved yet", t.Name, varName))
				val = ""
			}
			refVars[varName] = val
		}

		var value string
		switch t.EffectiveType() {
		case "template":
			rendered, err := r.evalFormat(t.Name, t.Format, refVars)
			if err != nil {
				r.Log.Fail(fmt.Sprintf("  %s: format error: %s", t.Name, err))
				continue
			}
			value = rendered
		default: // "envfile"
			var parts []string
			for _, varName := range t.Vars {
				parts = append(parts, varName+"="+refVars[varName])
			}
			value = strings.Join(parts, "\n")
		}

		if t.IsBase64() {
			value = base64.StdEncoding.EncodeToString([]byte(value))
		}

		vars[t.Name] = value

		if t.IsSecret() {
			r.Masker.Add(value)
			*secretCount++
		}

		label := fmt.Sprintf("%s (%s)", t.Name, t.EffectiveType())
		if t.IsBase64() {
			label += " (base64)"
		}
		r.Log.Info(fmt.Sprintf("  %s: transformed from %d variables", label, len(t.Vars)))
	}
}

// evalFormat renders a Go template using the referenced variable values as context.
// Template syntax: {{ .VAR_NAME }}
func (r *ProviderResolver) evalFormat(name, format string, vars map[string]string) (string, error) {
	tmpl, err := template.New(name).Parse(format)
	if err != nil {
		return "", fmt.Errorf("invalid template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// resolveVariable resolves a variable from its provider.
// Fallback: default (only for non-secret variables, validated at load time).
// Returns "" if nothing resolved — the caller preserves any existing value.
func (r *ProviderResolver) resolveVariable(ctx context.Context, p *Provider, v *Variable) string {
	value, err := r.fetchFromProvider(ctx, p, v)
	if err == nil && value != "" {
		return value
	}
	if err != nil {
		r.Log.Fail(fmt.Sprintf("  %s: %s", v.Name, err))
	}

	// Fallback: default (only allowed for non-secret variables; validated at load time).
	if v.Default != nil {
		r.Log.Info(fmt.Sprintf("  %s: using default", v.Name))
		return *v.Default
	}

	return ""
}

// fetchFromProvider calls the provider-specific resolver.
func (r *ProviderResolver) fetchFromProvider(ctx context.Context, p *Provider, v *Variable) (string, error) {
	switch p.Type {
	case "env":
		value, ok := os.LookupEnv(v.Path)
		if ok {
			return value, nil
		}
		return "", nil

	case "aws-secretsmanager":
		if r.awsResolver == nil {
			resolver, err := newAWSResolver(ctx, p.Region)
			if err != nil {
				return "", err
			}
			r.awsResolver = resolver
		}
		return r.awsResolver.resolve(ctx, v)

	case "bitwarden":
		if r.bwResolver == nil {
			resolver, err := newBitwardenResolver(p.URL)
			if err != nil {
				return "", err
			}
			r.bwResolver = resolver
		}
		return r.bwResolver.resolve(ctx, v)

	default:
		return "", nil
	}
}

type providerEntry struct {
	name     string
	provider *Provider
}

func (r *ProviderResolver) applicableProviders(providers map[string]*Provider, envName string) []providerEntry {
	var result []providerEntry
	for name, p := range providers {
		if appliesToEnv(p, envName) {
			result = append(result, providerEntry{name: name, provider: p})
		}
	}
	return result
}

func appliesToEnv(p *Provider, envName string) bool {
	if len(p.Envs) == 0 {
		return true
	}
	for _, e := range p.Envs {
		if e == envName {
			return true
		}
	}
	return false
}
