// Package pipelines implements pipeline loading, validation, and execution.
package pipelines

// Config is the top-level pipeline configuration.
type Config struct {
	Globals      []string                `json:"globals,omitempty"`
	Providers    map[string]*Provider    `json:"providers,omitempty"`
	PreRun       []BuildStep             `json:"preRun,omitempty"`
	Artifacts    map[string]*Artifact    `json:"artifacts"`
	Environments map[string]*Environment `json:"environments"`
	PostRun      []BuildStep             `json:"postRun,omitempty"`
}

// EnvironmentNames returns the list of available environment names.
func (c *Config) EnvironmentNames() []string {
	names := make([]string, 0, len(c.Environments))
	for k := range c.Environments {
		names = append(names, k)
	}
	return names
}

// ArtifactNames returns the list of available artifact names.
func (c *Config) ArtifactNames() []string {
	names := make([]string, 0, len(c.Artifacts))
	for k := range c.Artifacts {
		names = append(names, k)
	}
	return names
}

// Provider represents an external secrets/config provider.
type Provider struct {
	Type         string        `json:"type"`
	Priority     int           `json:"priority"`
	URL          string        `json:"url,omitempty"`
	Region       string        `json:"region,omitempty"`
	Envs         []string      `json:"envs"`
	Variables    []Variable    `json:"variables,omitempty"`
	Transformers []Transformer `json:"transformers,omitempty"`
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
// If Envs is empty, the transformer applies to all environments.
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
// If Envs is empty, the variable applies to all environments.
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

// Artifact represents a buildable and deployable unit (docker image, binary, package).
//
// Build strategy lifecycle:
//
//	preRun → preBuild → build → postBuild → preDeploy → deploy → postDeploy → postRun
//
// Promote strategy lifecycle:
//
//	preRun → preDeploy → promote → postDeploy → postRun
type Artifact struct {
	Type       string            `json:"type"`
	Workdir    string            `json:"workdir,omitempty"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Platforms  []string          `json:"platforms,omitempty"`
	BuildArgs  map[string]string `json:"buildArgs,omitempty"`
	Language   string            `json:"language,omitempty"`
	Build      []BuildStep       `json:"build,omitempty"`
	Deploy     *Deploy           `json:"deploy,omitempty"`
	Promote    *Deploy           `json:"promote,omitempty"`
	Repository Repository        `json:"repository"`
	PreRun     []BuildStep       `json:"preRun,omitempty"`
	PreBuild   []BuildStep       `json:"preBuild,omitempty"`
	PostBuild  []BuildStep       `json:"postBuild,omitempty"`
	PreDeploy  []BuildStep       `json:"preDeploy,omitempty"`
	PostDeploy []BuildStep       `json:"postDeploy,omitempty"`
	PostRun    []BuildStep       `json:"postRun,omitempty"`
}

// EffectiveWorkdir returns the working directory, defaulting to ".".
func (a *Artifact) EffectiveWorkdir() string {
	if a.Workdir != "" {
		return a.Workdir
	}
	return "."
}

// IsDocker returns true if this is a docker artifact.
func (a *Artifact) IsDocker() bool { return a.Type == "docker" }

// IsBinary returns true if this is a binary artifact.
func (a *Artifact) IsBinary() bool { return a.Type == "binary" }

// IsPackage returns true if this is a package artifact.
func (a *Artifact) IsPackage() bool { return a.Type == "package" }

// BuildStep is a single command in a multi-step build.
type BuildStep struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	Workdir string            `json:"workdir,omitempty"`
	Envs    []string          `json:"envs,omitempty"`
}

// AppliesToEnv returns true if this step should run for the given environment.
// If Envs is empty, the step runs for all environments.
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

// Repository defines where an artifact is stored or published.
type Repository struct {
	Type  string `json:"type"`
	URL   string `json:"url,omitempty"`
	User  string `json:"user,omitempty"`
	Image string `json:"image,omitempty"`
	Path  string `json:"path,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// FullImage returns "url/image" for registry repos.
func (r *Repository) FullImage() string {
	if r.URL != "" && r.Image != "" {
		return r.URL + "/" + r.Image
	}
	return r.Image
}

// Environment represents a deployment target (build, dev, uat, prod...).
type Environment struct {
	Strategy     string      `json:"strategy"`
	PromotesFrom string      `json:"promotesFrom,omitempty"`
	Artifacts    []string    `json:"artifacts,omitempty"`
	Build        *BuildPhase `json:"build,omitempty"`
	Deploy       *Deploy     `json:"deploy,omitempty"`
}

// IsBuild returns true if this environment builds from source.
func (e *Environment) IsBuild() bool { return e.Strategy == "build" }

// IsPromote returns true if this environment promotes from another.
func (e *Environment) IsPromote() bool { return e.Strategy == "promote" }

// BuildPhase holds pre/post hooks for the build phase.
type BuildPhase struct {
	PreRun  []BuildStep `json:"preRun,omitempty"`
	PostRun []BuildStep `json:"postRun,omitempty"`
}

// Deploy holds deployment configuration with pre/post hooks.
// Supported types: "helm", "script", "filesystem".
// Used by both environments (infra deploy) and artifacts (publish/push).
type Deploy struct {
	Type        string         `json:"type"`
	Chart       string         `json:"chart,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	Namespace   string         `json:"namespace,omitempty"`
	Parameters  string         `json:"parameters,omitempty"`
	Values      map[string]any `json:"values,omitempty"`
	Steps       []BuildStep    `json:"steps,omitempty"`
	Source      string         `json:"source,omitempty"`
	Destination string         `json:"destination,omitempty"`
	PreRun      []BuildStep    `json:"preRun,omitempty"`
	PostRun     []BuildStep    `json:"postRun,omitempty"`
}
