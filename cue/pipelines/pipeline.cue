package pipelines

// ============================================================
// PROVIDERS
// ============================================================

#ProviderVariable: {
	name:    string & =~"^[A-Z][A-Z0-9_]+$"
	path:    string
	secret?: *true | bool
}

#BitwardenProvider: {
	type:      "bitwarden"
	priority:  int & >=1
	url:       string | *"https://vault.bitwarden.com"
	envs:      [...string]
	variables: [...#ProviderVariable]
}

#EnvProvider: {
	type:      "env"
	priority:  int & >=1
	envs:      [...string]
	variables: [...#ProviderVariable]
}

#Provider: #BitwardenProvider | #EnvProvider

// ============================================================
// REPOSITORIES
// ============================================================

#DockerRegistry: {
	type:    "registry"
	url:     string
	user?:   string
	image:   string
	preRun?: [...#BuildCommand]
}

#FilesystemRepo: {
	type:    "filesystem"
	path:    string
	preRun?: [...#BuildCommand]
}

#PackageRepo: {
	type:    "package"
	kind:    "npm" | "maven" | "pypi" | "go"
	url:     string
	preRun?: [...#BuildCommand]
}

// Extend here: | #S3Repo | #ArtifactoryRepo etc.
#Repository: #DockerRegistry | #FilesystemRepo | #PackageRepo

// ============================================================
// ARTIFACTS
// ============================================================

#Platform: "linux/amd64" | "linux/arm64" | "linux/arm/v7"

// A single build command with optional per-command workdir override
#BuildCommand: {
	command: string
	args:    [...string]
	env?:    [string]: string
	workdir?: string // overrides artifact-level workdir for this step
}

#DockerArtifact: {
	type:       "docker"
	workdir?:   string | *"."
	dockerfile: string | *"./Dockerfile"
	platforms:  [#Platform, ...#Platform]
	buildArgs?: [string]: string
	repository: #DockerRegistry
}

#BinaryArtifact: {
	type:      "binary"
	workdir?:  string | *"."
	platforms: [#Platform, ...#Platform]
	build:     [#BuildCommand, ...#BuildCommand] // at least one required
	repository: #FilesystemRepo
}

#PackageArtifact: {
	type:      "package"
	workdir?:  string | *"."
	language:  "go" | "java" | "python" | "node"
	build:     [#BuildCommand, ...#BuildCommand]
	repository: #PackageRepo
}

// Extend here: | #LambdaArtifact | #HelmChartArtifact etc.
#Artifact: #DockerArtifact | #BinaryArtifact | #PackageArtifact

// ============================================================
// DEPLOY
// ============================================================

#HelmDeploy: {
	type:        "helm"
	chart:       string
	repo?:       string
	namespace:   string
	parameters?: string
	values?:     [string]: _
}

// Extend here: | #KubectlDeploy | #LambdaDeploy etc.
#Deploy: #HelmDeploy

// ============================================================
// ENVIRONMENTS
// ============================================================

#Environment: {
	strategy:      "build" | "promote"
	promotesFrom?: string          // absent only for the first environment
	artifacts?:    [...string]     // only for promote — if absent, promotes all
	deploy?:       #Deploy         // optional — not all environments deploy
}

// ============================================================
// PIPELINE FILE
// ============================================================

#PipelineFile: {
	providers?:   [id=string]:   #Provider
	artifacts:    [name=string]: #Artifact
	environments: [name=string]: #Environment
}
