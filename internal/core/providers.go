package core

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
//
// awsResolvers is keyed by (region + access key) so envs using different credential
// sets each get their own SDK client + secret-value cache. When a provider uses the
// SDK default chain (no credentialsFromVars), the key has an empty access key part.
type ProviderResolver struct {
	Log          *Logger
	Masker       *Masker
	Globals      map[string]bool // allowlist: only these variables can be resolved from ~/.nitro/config.json
	connResolver *ConnectionResolver
	awsResolvers map[string]*awsResolver
	bwResolver   *bitwardenResolver
}

// Resolve collects all variables from providers that apply to the given environment.
//
// Resolution order (lowest to highest priority):
//  1. Globals from ~/.nitro/config.json (bootstrap baseline)
//  2. Providers sorted by priority descending (high number = low priority, loaded first)
//  3. Priority 1 = maximum priority (loaded last, overwrites everything)
//
// Each layer overwrites previous values. Secret values are registered in the Masker.
//
// Hard fail: if any provider returns an error while fetching a variable, or any
// transformer fails to resolve its inputs, Resolve aborts and returns the error.
// We don't run partially-resolved pipelines — a missing secret means the rest of
// the pipeline can't be trusted.
func (r *ProviderResolver) Resolve(ctx context.Context, providers map[string]*Provider, connections map[string]*Connection, cr *ConnectionResolver, envName string) (map[string]string, error) {
	vars := make(map[string]string)
	secretCount := 0

	// Phase 1: bootstrap globals from ~/.nitro/config.json.
	r.loadGlobals(vars, &secretCount)

	// Phase 2: resolve connections (AFTER globals, BEFORE providers).
	r.connResolver = cr
	if cr != nil && len(connections) > 0 {
		r.Log.Step("Resolving connections")
		if err := cr.Resolve(ctx, connections, envName, vars); err != nil {
			return nil, fmt.Errorf("connections: %w", err)
		}
	}

	// Phase 3: resolve providers (high priority number first → low priority number last wins).
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
			if err := r.resolveTransformers(p, vars, &secretCount, envName); err != nil {
				return nil, fmt.Errorf("provider %q: %w", entry.name, err)
			}
		default:
			r.Log.Info(fmt.Sprintf("provider %q (priority: %d, type: %s) → %d variables",
				entry.name, p.Priority, p.Type, len(p.Variables)))
			if err := r.resolveStandardVars(ctx, p, vars, &secretCount, envName); err != nil {
				return nil, fmt.Errorf("provider %q: %w", entry.name, err)
			}
		}
	}

	r.Log.Info(fmt.Sprintf("session: %d variables loaded (%d secrets masked)", len(vars), secretCount))

	return vars, nil
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

func (r *ProviderResolver) resolveStandardVars(ctx context.Context, p *Provider, vars map[string]string, secretCount *int, envName string) error {
	for i := range p.Variables {
		v := &p.Variables[i]

		if !v.AppliesToEnv(envName) {
			continue
		}

		value, err := r.resolveVariable(ctx, p, v, vars, envName)
		if err != nil {
			return fmt.Errorf("variable %q: %w", v.Name, err)
		}
		if value == "" {
			continue // no value resolved and no default — preserve any prior layer's value
		}

		vars[v.Name] = value

		if v.IsSecret() {
			r.Masker.Add(value)
			*secretCount++
		}
	}
	return nil
}

func (r *ProviderResolver) resolveTransformers(p *Provider, vars map[string]string, secretCount *int, envName string) error {
	for i := range p.Transformers {
		t := &p.Transformers[i]

		if !t.AppliesToEnv(envName) {
			continue
		}

		// Collect referenced variable values. Missing inputs are a hard fail —
		// continuing with empty values would silently produce wrong output (e.g. an
		// envfile with KEY= entries that the consumer can't distinguish from intended ones).
		refVars := make(map[string]string, len(t.Vars))
		for _, varName := range t.Vars {
			val, ok := vars[varName]
			if !ok {
				r.Log.Fail(fmt.Sprintf("  %s: references %q which is not resolved yet", t.Name, varName))
				return fmt.Errorf("transformer %q: references variable %q which is not resolved (check provider order/priority)", t.Name, varName)
			}
			refVars[varName] = val
		}

		var value string
		switch t.EffectiveType() {
		case "template":
			rendered, err := r.evalFormat(t.Name, t.Format, refVars)
			if err != nil {
				r.Log.Fail(fmt.Sprintf("  %s: format error: %s", t.Name, err))
				return fmt.Errorf("transformer %q: %w", t.Name, err)
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
	return nil
}

// evalFormat renders a Go template using the referenced variable values as context.
// Template syntax: {{ .VAR_NAME }}
func (r *ProviderResolver) evalFormat(name, format string, vars map[string]string) (string, error) {
	tmpl, err := template.New(name).Funcs(safeFuncs).Parse(format)
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
//
// `vars` is the in-progress resolution map — some providers (e.g. aws-secretsmanager
// with credentialsFromVars) need to read already-resolved values from it.
//
// Behavior:
//   - Provider error (e.g. AWS GetSecretValue fails): hard fail. We don't fall back to
//     the default, because an error means the source is broken — not that the value is
//     simply absent — and continuing with a default would mask the real problem.
//   - Provider returns no value but no error (e.g. env var unset): fall back to default
//     if one is configured; otherwise return "" so the caller preserves any prior layer.
func (r *ProviderResolver) resolveVariable(ctx context.Context, p *Provider, v *Variable, vars map[string]string, envName string) (string, error) {
	value, err := r.fetchFromProvider(ctx, p, v, vars, envName)
	if err != nil {
		r.Log.Fail(fmt.Sprintf("  %s: %s", v.Name, err))
		return "", err
	}
	if value != "" {
		return value, nil
	}

	// Empty value, no error → fall back to default (validated to exist only on non-secrets).
	if v.Default != nil {
		r.Log.Info(fmt.Sprintf("  %s: using default", v.Name))
		return *v.Default, nil
	}

	return "", nil
}

// fetchFromProvider calls the provider-specific resolver.
// `vars` provides access to already-resolved values (needed by aws-secretsmanager
// to read static credentials). `envName` selects per-env options, e.g. which
// credentialsFromVars entry applies.
func (r *ProviderResolver) fetchFromProvider(ctx context.Context, p *Provider, v *Variable, vars map[string]string, envName string) (string, error) {
	switch p.Type {
	case "env":
		value, ok := os.LookupEnv(v.Path)
		if ok {
			return value, nil
		}
		return "", nil

	case "aws-secretsmanager":
		resolver, err := r.awsResolverFor(ctx, p, vars, envName)
		if err != nil {
			return "", err
		}
		return resolver.resolve(ctx, v)

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

// awsResolverFor returns (creating if needed) the AWS resolver for this provider
// in the current env. Resolvers are cached by (region + access key) so different
// credential sets each get their own SDK client and secret-value cache.
func (r *ProviderResolver) awsResolverFor(ctx context.Context, p *Provider, vars map[string]string, envName string) (*awsResolver, error) {
	if r.awsResolvers == nil {
		r.awsResolvers = make(map[string]*awsResolver)
	}

	// Prefer a named connection's SDK config when the provider declares one.
	if p.Connection != "" && r.connResolver != nil {
		cacheKey := "conn:" + p.Connection
		if existing, ok := r.awsResolvers[cacheKey]; ok {
			return existing, nil
		}
		resolved := r.connResolver.AWSConfig(p.Connection)
		if resolved == nil {
			return nil, fmt.Errorf("connection %q not resolved (check connection name and envs)", p.Connection)
		}
		resolver := newAWSResolverFromConfig(resolved.cfg)
		r.awsResolvers[cacheKey] = resolver
		return resolver, nil
	}

	// Fallback: credentialsFromVars or SDK default chain (backward compatible).
	creds, err := pickAWSCreds(p.CredentialsFromVars, vars, envName)
	if err != nil {
		return nil, err
	}

	cacheKey := p.Region + "|"
	if creds != nil {
		cacheKey += creds.AccessKeyID
	} else {
		cacheKey += "<sdk-default-chain>"
	}

	if existing, ok := r.awsResolvers[cacheKey]; ok {
		return existing, nil
	}

	resolver, err := newAWSResolver(ctx, p.Region, creds)
	if err != nil {
		return nil, err
	}
	r.awsResolvers[cacheKey] = resolver
	return resolver, nil
}

// pickAWSCreds selects the credentialsFromVars entry for envName and resolves
// the referenced variable values from `vars`. Returns nil (use SDK default chain)
// when credentialsFromVars is unset or has no entry for this env. Errors when
// an entry exists but its referenced variables are not in `vars` — that's a
// config mistake (wrong provider order/priority), not a runtime miss.
func pickAWSCreds(refs map[string]*AWSCredentialsRef, vars map[string]string, envName string) (*awsStaticCreds, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	ref, ok := refs[envName]
	if !ok || ref == nil {
		return nil, nil // no entry for this env → fall back to SDK default chain
	}

	ak, okAK := vars[ref.AccessKeyID]
	sk, okSK := vars[ref.SecretAccessKey]
	if !okAK || ak == "" {
		return nil, fmt.Errorf("credentialsFromVars[%q]: access key variable %q is not resolved (must come from globals or a higher-priority provider)", envName, ref.AccessKeyID)
	}
	if !okSK || sk == "" {
		return nil, fmt.Errorf("credentialsFromVars[%q]: secret key variable %q is not resolved (must come from globals or a higher-priority provider)", envName, ref.SecretAccessKey)
	}
	creds := &awsStaticCreds{AccessKeyID: ak, SecretAccessKey: sk}
	if ref.SessionToken != "" {
		if tok, ok := vars[ref.SessionToken]; ok {
			creds.SessionToken = tok
		}
	}
	return creds, nil
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
