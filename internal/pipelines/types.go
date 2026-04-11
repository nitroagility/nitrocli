// Package pipelines implements pipeline loading, validation, and execution.
package pipelines

// Config is the top-level pipeline configuration.
type Config struct {
	Providers    map[string]*Provider    `json:"providers,omitempty"`
	Artifacts    map[string]*Artifact    `json:"artifacts"`
	Environments map[string]*Environment `json:"environments"`
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
	Type      string     `json:"type"`
	Priority  int        `json:"priority"`
	URL       string     `json:"url"`
	Envs      []string   `json:"envs"`
	Variables []Variable `json:"variables"`
}

// Variable is a single secret reference within a provider.
type Variable struct {
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Secret  *bool   `json:"secret,omitempty"`
	Default *string `json:"default,omitempty"`
}

// IsSecret returns true if this variable is a secret (default: true).
func (v *Variable) IsSecret() bool {
	if v.Secret == nil {
		return true
	}
	return *v.Secret
}

// Artifact represents a buildable unit (docker image, binary, package).
type Artifact struct {
	Type       string            `json:"type"`
	Workdir    string            `json:"workdir,omitempty"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Platforms  []string          `json:"platforms,omitempty"`
	BuildArgs  map[string]string `json:"buildArgs,omitempty"`
	Language   string            `json:"language,omitempty"`
	Build      []BuildStep       `json:"build,omitempty"`
	Repository Repository        `json:"repository"`
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
}

// Repository defines where an artifact is stored or published.
type Repository struct {
	Type   string      `json:"type"`
	URL    string      `json:"url,omitempty"`
	User   string      `json:"user,omitempty"`
	Image  string      `json:"image,omitempty"`
	Path   string      `json:"path,omitempty"`
	Kind   string      `json:"kind,omitempty"`
	PreRun []BuildStep `json:"preRun,omitempty"`
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
	Strategy     string   `json:"strategy"`
	PromotesFrom string   `json:"promotesFrom,omitempty"`
	Artifacts    []string `json:"artifacts,omitempty"`
	Deploy       *Deploy  `json:"deploy,omitempty"`
}

// IsBuild returns true if this environment builds from source.
func (e *Environment) IsBuild() bool { return e.Strategy == "build" }

// IsPromote returns true if this environment promotes from another.
func (e *Environment) IsPromote() bool { return e.Strategy == "promote" }

// Deploy holds deployment configuration.
type Deploy struct {
	Type       string         `json:"type"`
	Chart      string         `json:"chart,omitempty"`
	Repo       string         `json:"repo,omitempty"`
	Namespace  string         `json:"namespace"`
	Parameters string         `json:"parameters,omitempty"`
	Values     map[string]any `json:"values,omitempty"`
}
