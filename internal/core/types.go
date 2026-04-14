// Package core provides shared infrastructure for nitrocli features:
// connections (authentication), providers (secrets/config), executor (command running),
// logging, masking, and template evaluation. Imported by pipelines, operations, etc.
package core

// Connection represents a named authentication connection.
// Connections resolve BEFORE providers and produce either env vars
// (exportEnv: true) or cached SDK clients for provider consumption.
type Connection struct {
	Type      string                     `json:"type"`
	Region    string                     `json:"region,omitempty"`
	ExportEnv *bool                      `json:"exportEnv,omitempty"`
	Envs      []string                   `json:"envs,omitempty"`
	Auth      map[string]*ConnectionAuth `json:"auth,omitempty"`
}

// IsExportEnv returns true if this connection exports credentials to env vars.
func (c *Connection) IsExportEnv() bool {
	return c.ExportEnv != nil && *c.ExportEnv
}

// ConnectionAuth holds the authentication method for a specific environment.
type ConnectionAuth struct {
	Method             string `json:"method"`                       // "assume-role" | "static"
	RoleArn            string `json:"roleArn,omitempty"`            // assume-role
	RoleSessionName    string `json:"roleSessionName,omitempty"`    // assume-role
	Duration           int    `json:"duration,omitempty"`           // assume-role (seconds)
	AccessKeyIDVar     string `json:"accessKeyIDVar,omitempty"`     // static
	SecretAccessKeyVar string `json:"secretAccessKeyVar,omitempty"` // static
	SessionTokenVar    string `json:"sessionTokenVar,omitempty"`    // static
}

// Provider represents an external secrets/config provider.
type Provider struct {
	Type                string                        `json:"type"`
	Priority            int                           `json:"priority"`
	URL                 string                        `json:"url,omitempty"`
	Region              string                        `json:"region,omitempty"`
	Connection          string                        `json:"connection,omitempty"`
	CredentialsFromVars map[string]*AWSCredentialsRef `json:"credentialsFromVars,omitempty"`
	Envs                []string                      `json:"envs"`
	Variables           []Variable                    `json:"variables,omitempty"`
	Transformers        []Transformer                 `json:"transformers,omitempty"`
}

// AWSCredentialsRef names already-resolved variables for static AWS credentials.
type AWSCredentialsRef struct {
	AccessKeyID     string `json:"accessKeyID"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken,omitempty"`
}

// Transformer derives a new variable by combining multiple resolved variables.
type Transformer struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Vars    []string `json:"vars"`
	Secret  *bool    `json:"secret,omitempty"`
	Default *string  `json:"default,omitempty"`
	Base64  *bool    `json:"base64,omitempty"`
	Format  string   `json:"format,omitempty"`
	Envs    []string `json:"envs,omitempty"`
}

// EffectiveType returns the transformer type, defaulting to "envfile".
func (t *Transformer) EffectiveType() string {
	if t.Type != "" {
		return t.Type
	}
	return "envfile"
}

// IsSecret returns true if this transformer is a secret (default: true).
func (t *Transformer) IsSecret() bool {
	if t.Secret == nil {
		return true
	}
	return *t.Secret
}

// IsBase64 returns true if the value should be base64 encoded (default: false).
func (t *Transformer) IsBase64() bool {
	if t.Base64 == nil {
		return false
	}
	return *t.Base64
}

// AppliesToEnv returns true if this transformer applies to the given environment.
func (t *Transformer) AppliesToEnv(envName string) bool {
	if len(t.Envs) == 0 {
		return true
	}
	for _, e := range t.Envs {
		if e == envName {
			return true
		}
	}
	return false
}

// Variable is a single secret reference within a provider.
type Variable struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Key     string   `json:"key,omitempty"`
	Secret  *bool    `json:"secret,omitempty"`
	Default *string  `json:"default,omitempty"`
	Envs    []string `json:"envs,omitempty"`
}

// IsSecret returns true if this variable is a secret (default: true).
func (v *Variable) IsSecret() bool {
	if v.Secret == nil {
		return true
	}
	return *v.Secret
}

// AppliesToEnv returns true if this variable applies to the given environment.
func (v *Variable) AppliesToEnv(envName string) bool {
	if len(v.Envs) == 0 {
		return true
	}
	for _, e := range v.Envs {
		if e == envName {
			return true
		}
	}
	return false
}

// BuildStep is a single command to execute.
type BuildStep struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	Env        map[string]string `json:"env,omitempty"`
	Workdir    string            `json:"workdir,omitempty"`
	Connection string            `json:"connection,omitempty"`
	Envs       []string          `json:"envs,omitempty"`
}

// AppliesToEnv returns true if this step should run for the given environment.
func (s *BuildStep) AppliesToEnv(envName string) bool {
	if len(s.Envs) == 0 {
		return true
	}
	for _, e := range s.Envs {
		if e == envName {
			return true
		}
	}
	return false
}

// Deploy holds deployment configuration with pre/post hooks.
// Supported types: "helm", "script", "filesystem".
type Deploy struct {
	Type        string         `json:"type"`
	Chart       string         `json:"chart,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	Connection  string         `json:"connection,omitempty"`
	ReleaseName string         `json:"releaseName,omitempty"`
	Namespace   string         `json:"namespace,omitempty"`
	Parameters  string         `json:"parameters,omitempty"`
	Values      map[string]any `json:"values,omitempty"`
	Steps       []BuildStep    `json:"steps,omitempty"`
	Source      string         `json:"source,omitempty"`
	Destination string         `json:"destination,omitempty"`
	PreRun      []BuildStep    `json:"preRun,omitempty"`
	PostRun     []BuildStep    `json:"postRun,omitempty"`
}

// BuildPhase holds pre/post hooks for the build phase.
type BuildPhase struct {
	PreRun  []BuildStep `json:"preRun,omitempty"`
	PostRun []BuildStep `json:"postRun,omitempty"`
}
