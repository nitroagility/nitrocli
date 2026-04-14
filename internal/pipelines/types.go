// Package pipelines implements pipeline loading, validation, and execution.
package pipelines

import "github.com/nitroagility/nitrocli/internal/core"

//nolint:revive // Type aliases re-exported from core — documented in core/types.go.
type (
	Connection         = core.Connection
	ConnectionAuth     = core.ConnectionAuth
	Provider           = core.Provider
	AWSCredentialsRef  = core.AWSCredentialsRef
	Variable           = core.Variable
	Transformer        = core.Transformer
	BuildStep          = core.BuildStep
	Deploy             = core.Deploy
	BuildPhase         = core.BuildPhase
	Masker             = core.Masker
	Logger             = core.Logger
	Executor           = core.Executor
	ProviderResolver   = core.ProviderResolver
	ConnectionResolver = core.ConnectionResolver
)

// Config is the top-level pipeline configuration.
type Config struct {
	Globals      []string                `json:"globals,omitempty"`
	Connections  map[string]*Connection  `json:"connections,omitempty"`
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

// Artifact represents a buildable and deployable unit (docker image, binary, package).
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
	Undeploy   *Deploy           `json:"undeploy,omitempty"`
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
	Undeploy     *Deploy     `json:"undeploy,omitempty"`
}

// IsBuild returns true if this environment builds from source.
func (e *Environment) IsBuild() bool { return e.Strategy == "build" }

// IsPromote returns true if this environment promotes from another.
func (e *Environment) IsPromote() bool { return e.Strategy == "promote" }
