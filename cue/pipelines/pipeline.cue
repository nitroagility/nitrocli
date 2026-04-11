package pipelines

// ============================================================
// PROVIDERS
// ============================================================

#ProviderVariable: {
	name:     string & =~"^[A-Z][A-Z0-9_]+$"
	path:     string
	secret?:  *true | bool
	default?: string
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

#CompositeVariable: {
	name:     string & =~"^[A-Z][A-Z0-9_]+$"
	vars:     [...string]
	secret?:  *true | bool
	base64?:  *false | bool
}

#CompositeProvider: {
	type:       "composite"
	priority:   int & >=1
	envs:       [...string]
	composites: [...#CompositeVariable]
}

#Provider: #BitwardenProvider | #EnvProvider | #CompositeProvider

// ============================================================
// REPOSITORIES
// ============================================================

#DockerRegistry: {
	type:  "registry"
	url:   string
	user?: string
	image: string
}

#FilesystemRepo: {
	type: "filesystem"
	path: string
}

#PackageRepo: {
	type: "package"
	kind: "npm" | "maven" | "pypi" | "go"
	url:  string
}

// Extend here: | #S3Repo | #ArtifactoryRepo etc.
#Repository: #DockerRegistry | #FilesystemRepo | #PackageRepo

// ============================================================
// ARTIFACTS
// ============================================================

#Platform: "linux/amd64" | "linux/arm64" | "linux/arm/v7"

// A single build command with optional per-command workdir override
#BuildCommand: {
	command:  string
	args:     [...string]
	env?:     [string]: string
	workdir?: string
	envs?:    [...string]
}

#DockerArtifact: {
	type:       "docker"
	workdir?:   string | *"."
	dockerfile: string | *"./Dockerfile"
	platforms:  [#Platform, ...#Platform]
	buildArgs?: [string]: string
	preRun?:    [...#BuildCommand]
	postRun?:   [...#BuildCommand]
	repository: #DockerRegistry
}

#BinaryArtifact: {
	type:      "binary"
	workdir?:  string | *"."
	platforms: [#Platform, ...#Platform]
	build:     [#BuildCommand, ...#BuildCommand]
	preRun?:   [...#BuildCommand]
	postRun?:  [...#BuildCommand]
	repository: #FilesystemRepo
}

#PackageArtifact: {
	type:      "package"
	workdir?:  string | *"."
	language:  "go" | "java" | "python" | "node"
	build:     [#BuildCommand, ...#BuildCommand]
	preRun?:   [...#BuildCommand]
	postRun?:  [...#BuildCommand]
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

#BuildPhase: {
	preRun?:  [...#BuildCommand]
	postRun?: [...#BuildCommand]
}

#DeployPhase: {
	#Deploy
	preRun?:  [...#BuildCommand]
	postRun?: [...#BuildCommand]
}

#Environment: {
	strategy:      "build" | "promote"
	promotesFrom?: string
	artifacts?:    [...string]
	build?:        #BuildPhase
	deploy?:       #DeployPhase
}

// ============================================================
// PIPELINE FILE
// ============================================================

#PipelineFile: {
	providers?:   [id=string]:   #Provider
	preRun?:      [...#BuildCommand]
	artifacts:    [name=string]: #Artifact
	environments: [name=string]: #Environment
	postRun?:     [...#BuildCommand]
}
