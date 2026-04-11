package pipelines

import (
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ProviderResolver resolves secrets from configured providers for a given environment.
type ProviderResolver struct {
	Log    *Logger
	Masker *Masker
}

// Resolve collects all variables from providers that apply to the given environment.
// Providers are loaded in descending priority order (highest first).
// Secret values are automatically registered in the Masker.
// Returns a map of variable name → resolved value.
func (r *ProviderResolver) Resolve(providers map[string]*Provider, envName string) map[string]string {
	applicable := r.applicableProviders(providers, envName)

	sort.Slice(applicable, func(i, j int) bool {
		return applicable[i].provider.Priority > applicable[j].provider.Priority
	})

	vars := make(map[string]string)
	secretCount := 0

	for _, entry := range applicable {
		p := entry.provider

		switch p.Type {
		case "composite":
			r.Log.Info(fmt.Sprintf("provider %q (priority: %d, type: %s) → %d composites",
				entry.name, p.Priority, p.Type, len(p.Composites)))
			r.resolveComposites(p, vars, &secretCount)
		default:
			r.Log.Info(fmt.Sprintf("provider %q (priority: %d, type: %s) → %d variables",
				entry.name, p.Priority, p.Type, len(p.Variables)))
			r.resolveStandardVars(p, vars, &secretCount)
		}
	}

	r.Log.Info(fmt.Sprintf("session: %d variables loaded (%d secrets masked)", len(vars), secretCount))

	return vars
}

func (r *ProviderResolver) resolveStandardVars(p *Provider, vars map[string]string, secretCount *int) {
	for i := range p.Variables {
		v := &p.Variables[i]
		if _, exists := vars[v.Name]; exists {
			continue
		}

		value := r.resolveVariable(p, v)
		vars[v.Name] = value

		if v.IsSecret() {
			r.Masker.Add(value)
			*secretCount++
		}
	}
}

func (r *ProviderResolver) resolveComposites(p *Provider, vars map[string]string, secretCount *int) {
	for i := range p.Composites {
		cv := &p.Composites[i]
		if _, exists := vars[cv.Name]; exists {
			continue
		}

		// Build the concatenated value from referenced variables.
		var parts []string
		for _, varName := range cv.Vars {
			val, ok := vars[varName]
			if !ok {
				r.Log.Fail(fmt.Sprintf("  %s: references %q which is not resolved yet", cv.Name, varName))
				val = ""
			}
			parts = append(parts, varName+"="+val)
		}

		value := strings.Join(parts, "\n")

		if cv.IsBase64() {
			value = base64.StdEncoding.EncodeToString([]byte(value))
		}

		vars[cv.Name] = value

		if cv.IsSecret() {
			r.Masker.Add(value)
			*secretCount++
		}

		label := cv.Name
		if cv.IsBase64() {
			label += " (base64)"
		}
		r.Log.Info(fmt.Sprintf("  %s: composed from %d variables", label, len(cv.Vars)))
	}
}

func (r *ProviderResolver) resolveVariable(p *Provider, v *Variable) string {
	switch p.Type {
	case "env":
		return r.resolveEnvVariable(v)
	case "bitwarden":
		// TODO: implement bitwarden resolution via CLI or API.
		return ""
	default:
		return ""
	}
}

func (r *ProviderResolver) resolveEnvVariable(v *Variable) string {
	value, ok := os.LookupEnv(v.Path)
	if ok {
		return value
	}

	if v.Default != nil {
		r.Log.Info(fmt.Sprintf("  %s: using default (env $%s not set)", v.Name, v.Path))
		return *v.Default
	}

	r.Log.Fail(fmt.Sprintf("  %s: not set (expected in $%s, no default)", v.Name, v.Path))
	return ""
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
