package pipelines

import (
	"fmt"
	"os"
	"sort"
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
		r.Log.Info(fmt.Sprintf("provider %q (priority: %d, type: %s) → %d variables",
			entry.name, entry.provider.Priority, entry.provider.Type, len(entry.provider.Variables)))

		for i := range entry.provider.Variables {
			v := &entry.provider.Variables[i]
			// Lower priority providers do not overwrite higher priority values.
			if _, exists := vars[v.Name]; exists {
				continue
			}

			value := r.resolveVariable(entry.provider, v)
			vars[v.Name] = value

			if v.IsSecret() {
				r.Masker.Add(value)
				secretCount++
			}
		}
	}

	r.Log.Info(fmt.Sprintf("session: %d variables loaded (%d secrets masked)", len(vars), secretCount))

	return vars
}

func (r *ProviderResolver) resolveVariable(p *Provider, v *Variable) string {
	switch p.Type {
	case "env":
		return r.resolveEnvVariable(v)
	case "bitwarden":
		// TODO: implement bitwarden resolution via CLI or API.
		return ""
	default:
		return fmt.Sprintf("{{ .%s }}", v.Name)
	}
}

func (r *ProviderResolver) resolveEnvVariable(v *Variable) string {
	// Path is the OS environment variable name to read from.
	value, ok := os.LookupEnv(v.Path)
	if !ok {
		r.Log.Fail(fmt.Sprintf("  env variable %q not set (expected in $%s)", v.Name, v.Path))
		return ""
	}
	return value
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
