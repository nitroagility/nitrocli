// Package pipelines defines the CUE schema for nitrocli pipeline files.
// Shared types (connections, providers, deploy, commands) are imported from core.
package pipelines

import "github.com/nitroagility/nitrocli/core@v0"

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
// ARTIFACTS
// ============================================================

#ArtifactLifecycle: {
	preRun?:     [...core.#BuildCommand]
	preBuild?:   [...core.#BuildCommand]
	postBuild?:  [...core.#BuildCommand]
	preDeploy?:  [...core.#BuildCommand]
	postDeploy?: [...core.#BuildCommand]
	postRun?:    [...core.#BuildCommand]
}

#DockerArtifact: {
	#ArtifactLifecycle
	type:       "docker"
	workdir?:   string | *"."
	dockerfile: string | *"./Dockerfile"
	platforms:  [core.#Platform, ...core.#Platform]
	buildArgs?: [string]: string
	deploy?:    core.#DeployPhase
	promote?:   core.#DeployPhase
	undeploy?:  core.#UndeployPhase
	repository: #DockerRegistry
}

#BinaryArtifact: {
	#ArtifactLifecycle
	type:      "binary"
	workdir?:  string | *"."
	platforms: [core.#Platform, ...core.#Platform]
	build:     [core.#BuildCommand, ...core.#BuildCommand]
	deploy?:   core.#DeployPhase
	promote?:  core.#DeployPhase
	undeploy?: core.#UndeployPhase
	repository: #FilesystemRepo
}

#PackageArtifact: {
	#ArtifactLifecycle
	type:      "package"
	workdir?:  string | *"."
	language:  "go" | "java" | "python" | "node"
	build:     [core.#BuildCommand, ...core.#BuildCommand]
	deploy?:   core.#DeployPhase
	promote?:  core.#DeployPhase
	undeploy?: core.#UndeployPhase
	repository: #PackageRepo
}

#Artifact: #DockerArtifact | #BinaryArtifact | #PackageArtifact

// ============================================================
// ENVIRONMENTS
// ============================================================

// Build from source
_#BuildEnvironment: {
	strategy:      "build"
	promotesFrom?: string
	artifacts?:    [...string]
	build?:        core.#BuildPhase
	deploy?:       core.#DeployPhase
	undeploy?:     core.#UndeployPhase
}

// Promote from another environment (promotesFrom required)
_#PromoteEnvironment: {
	strategy:     "promote"
	promotesFrom: string & =~".+"
	artifacts?:   [...string]
	build?:       core.#BuildPhase
	deploy?:      core.#DeployPhase
	undeploy?:    core.#UndeployPhase
}

#Environment: _#BuildEnvironment | _#PromoteEnvironment

// ============================================================
// PIPELINE FILE
// ============================================================

#PipelineFile: {
	globals?:      [...string]
	connections?:  [name=string]: core.#Connection
	providers?:    [id=string]:   core.#Provider
	preRun?:       [...core.#BuildCommand]
	artifacts:     [name=string]: #Artifact
	environments:  [name=string]: #Environment
	postRun?:      [...core.#BuildCommand]
}
