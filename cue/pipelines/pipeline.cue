package pipelines

// ============================================================
// SHARED CONSTRAINTS
// ============================================================

_#SafeVarName: string & =~"^[A-Z][A-Z0-9_]+$" & !~"^NITRO_" & !~"^(PATH|HOME|SHELL|USER|LOGNAME|LD_PRELOAD|LD_LIBRARY_PATH|DYLD_INSERT_LIBRARIES|DYLD_LIBRARY_PATH|DYLD_FRAMEWORK_PATH|IFS|ENV|BASH_ENV|CDPATH)$"

// ============================================================
// PROVIDERS
// ============================================================

// Secret variable: no default allowed (zero-trust)
_#SecretVariable: {
	name:   _#SafeVarName
	path:   string & =~".+"
	key?:   string
	secret: *true | true
	envs?:  [...string]
}

// Config variable: default is allowed
_#ConfigVariable: {
	name:     _#SafeVarName
	path:     string & =~".+"
	key?:     string
	secret:   false
	default?: string
	envs?:    [...string]
}

#ProviderVariable: _#SecretVariable | _#ConfigVariable

#EnvProvider: {
	type:      "env"
	priority:  int & >=1
	envs:      [...string]
	variables: [...#ProviderVariable]
}

// Optional per-env AWS credentials: names of ALREADY-RESOLVED variables to use
// as the static credentials provider for GetSecretValue calls. Use this when the
// SDK default chain (env → ~/.aws/credentials → IMDS) would fail — typically in
// local runs where no profile is configured and the machine is not on EC2.
//
// Each referenced variable must be resolved BEFORE this provider runs — i.e. it
// must come from globals or a provider with a higher `priority` number than this
// provider.
//
// Credentials are keyed by environment name: if the provider runs for env "X"
// and `credentialsFromVars` has a "X" entry, those variables are used; if no
// entry exists for "X" (or credentialsFromVars is unset), the SDK default chain
// is used. This lets different envs authenticate with different credentials
// without duplicating provider definitions.
_#AWSCredentialsRef: {
	accessKeyID:     string & =~".+"
	secretAccessKey: string & =~".+"
	sessionToken?:   string
}

#AWSSecretsManagerProvider: {
	type:                 "aws-secretsmanager"
	priority:             int & >=1
	region:               string & =~".+"
	credentialsFromVars?: [envName=string]: _#AWSCredentialsRef
	envs:                 [...string]
	variables:            [...#ProviderVariable]
}

#BitwardenProvider: {
	type:      "bitwarden"
	priority:  int & >=1
	url?:      string
	envs:      [...string]
	variables: [...#ProviderVariable]
}

// Envfile transformer: joins vars as KEY=VALUE (default)
_#EnvfileTransformer: {
	type?:    *"envfile" | "envfile"
	name:     _#SafeVarName
	vars:     [string, ...string]
	secret?:  *true | bool
	default?: string
	base64?:  *false | bool
	envs?:    [...string]
}

// Template transformer: format is required
_#TemplateTransformer: {
	type:     "template"
	name:     _#SafeVarName
	vars:     [string, ...string]
	secret?:  *true | bool
	default?: string
	base64?:  *false | bool
	format:   string & =~".+"
	envs?:    [...string]
}

#TransformerVariable: _#EnvfileTransformer | _#TemplateTransformer

#TransformerProvider: {
	type:         "transformer"
	priority:     int & >=1
	envs:         [...string]
	transformers: [...#TransformerVariable]
}

#Provider: #EnvProvider | #AWSSecretsManagerProvider | #BitwardenProvider | #TransformerProvider

// ============================================================
// REPOSITORIES
// ============================================================

#DockerRegistry: {
	type:  "registry"
	url:   string & =~".+"
	user?: string
	image: string & =~".+"
}

#FilesystemRepo: {
	type: "filesystem"
	path: string & =~".+"
}

#PackageRepo: {
	type: "package"
	kind: "npm" | "maven" | "pypi" | "go"
	url:  string & =~".+"
}

#Repository: #DockerRegistry | #FilesystemRepo | #PackageRepo

// ============================================================
// BUILD COMMANDS
// ============================================================

#Platform: "linux/amd64" | "linux/arm64" | "linux/arm/v7"

#BuildCommand: {
	command:  string & =~".+"
	args:     [...string]
	env?:     [string]: string
	workdir?: string
	envs?:    [...string]
}

// ============================================================
// ARTIFACTS
// ============================================================

#ArtifactLifecycle: {
	preRun?:     [...#BuildCommand]
	preBuild?:   [...#BuildCommand]
	postBuild?:  [...#BuildCommand]
	preDeploy?:  [...#BuildCommand]
	postDeploy?: [...#BuildCommand]
	postRun?:    [...#BuildCommand]
}

#DockerArtifact: {
	#ArtifactLifecycle
	type:       "docker"
	workdir?:   string | *"."
	dockerfile: string | *"./Dockerfile"
	platforms:  [#Platform, ...#Platform]
	buildArgs?: [string]: string
	deploy?:    #DeployPhase
	promote?:   #DeployPhase
	repository: #DockerRegistry
}

#BinaryArtifact: {
	#ArtifactLifecycle
	type:      "binary"
	workdir?:  string | *"."
	platforms: [#Platform, ...#Platform]
	build:     [#BuildCommand, ...#BuildCommand]
	deploy?:   #DeployPhase
	promote?:  #DeployPhase
	repository: #FilesystemRepo
}

#PackageArtifact: {
	#ArtifactLifecycle
	type:      "package"
	workdir?:  string | *"."
	language:  "go" | "java" | "python" | "node"
	build:     [#BuildCommand, ...#BuildCommand]
	deploy?:   #DeployPhase
	promote?:  #DeployPhase
	repository: #PackageRepo
}

#Artifact: #DockerArtifact | #BinaryArtifact | #PackageArtifact

// ============================================================
// DEPLOY
// ============================================================

#HelmDeploy: {
	type:         "helm"
	chart:        string & =~".+"
	repo?:        string
	releaseName?: string & =~".+" // defaults to the env name when unset
	namespace:    string & =~".+"
	parameters?:  string
	values?:      [string]: _
}

#ScriptDeploy: {
	type:  "script"
	steps: [#BuildCommand, ...#BuildCommand]
}

#FilesystemDeploy: {
	type:        "filesystem"
	source:      string & =~".+"
	destination: string & =~".+"
}

#Deploy: #HelmDeploy | #ScriptDeploy | #FilesystemDeploy

#DeployPhase: {
	#Deploy
	preRun?:  [...#BuildCommand]
	postRun?: [...#BuildCommand]
}

// ============================================================
// ENVIRONMENTS
// ============================================================

#BuildPhase: {
	preRun?:  [...#BuildCommand]
	postRun?: [...#BuildCommand]
}

// Build from source
_#BuildEnvironment: {
	strategy:      "build"
	promotesFrom?: string
	artifacts?:    [...string]
	build?:        #BuildPhase
	deploy?:       #DeployPhase
}

// Promote from another environment (promotesFrom required)
_#PromoteEnvironment: {
	strategy:     "promote"
	promotesFrom: string & =~".+"
	artifacts?:   [...string]
	build?:       #BuildPhase
	deploy?:      #DeployPhase
}

#Environment: _#BuildEnvironment | _#PromoteEnvironment

// ============================================================
// PIPELINE FILE
// ============================================================

#PipelineFile: {
	globals?:     [...string]
	providers?:   [id=string]:   #Provider
	preRun?:      [...#BuildCommand]
	artifacts:    [name=string]: #Artifact
	environments: [name=string]: #Environment
	postRun?:     [...#BuildCommand]
}
