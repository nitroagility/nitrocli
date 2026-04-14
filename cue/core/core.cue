// Package core provides shared CUE schemas for connections, providers,
// build commands, and deploy phases. Imported by pipelines, operations, etc.
package core

// ============================================================
// SHARED CONSTRAINTS
// ============================================================

#SafeVarName: string & =~"^[A-Z][A-Z0-9_]+$" & !~"^NITRO_" & !~"^(PATH|HOME|SHELL|USER|LOGNAME|LD_PRELOAD|LD_LIBRARY_PATH|DYLD_INSERT_LIBRARIES|DYLD_LIBRARY_PATH|DYLD_FRAMEWORK_PATH|IFS|ENV|BASH_ENV|CDPATH)$"

// ============================================================
// CONNECTIONS
// ============================================================

_#AssumeRoleAuth: {
	method:           "assume-role"
	roleArn:          string & =~"^arn:aws:iam::"
	roleSessionName?: string & =~".+"
	duration?:        int & >=900 & <=43200
}

_#StaticAuth: {
	method:             "static"
	accessKeyIDVar:     string & =~".+"
	secretAccessKeyVar: string & =~".+"
	sessionTokenVar?:   string
}

#AWSAuth: _#AssumeRoleAuth | _#StaticAuth

#AWSConnection: {
	type:       "aws"
	region:     string & =~".+"
	exportEnv?: bool
	envs?:      [...string]
	auth?: [envName=string]: #AWSAuth
}

#Connection: #AWSConnection

// ============================================================
// PROVIDERS
// ============================================================

_#SecretVariable: {
	name:   #SafeVarName
	path:   string & =~".+"
	key?:   string
	secret: *true | true
	envs?:  [...string]
}

_#ConfigVariable: {
	name:     #SafeVarName
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

_#AWSCredentialsRef: {
	accessKeyID:     string & =~".+"
	secretAccessKey: string & =~".+"
	sessionToken?:   string
}

#AWSSecretsManagerProvider: {
	type:                 "aws-secretsmanager"
	priority:             int & >=1
	region:               string & =~".+"
	connection?:          string & =~".+"
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

_#EnvfileTransformer: {
	type?:    *"envfile" | "envfile"
	name:     #SafeVarName
	vars:     [string, ...string]
	secret?:  *true | bool
	default?: string
	base64?:  *false | bool
	envs?:    [...string]
}

_#TemplateTransformer: {
	type:     "template"
	name:     #SafeVarName
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
// BUILD COMMANDS
// ============================================================

#Platform: "linux/amd64" | "linux/arm64" | "linux/arm/v7"

#BuildCommand: {
	command:     string & =~".+"
	args:        [...string]
	env?:        [string]: string
	workdir?:    string
	connection?: string & =~".+"
	envs?:       [...string]
}

// ============================================================
// DEPLOY
// ============================================================

#HelmDeploy: {
	type:         "helm"
	chart:        string & =~".+"
	repo?:        string
	connection?:  string & =~".+"
	releaseName?: string & =~".+"
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

// Undeploy: helm only needs releaseName + namespace (no chart/parameters).
// Script and filesystem are identical to deploy (user defines cleanup steps).
#HelmUndeploy: {
	type:         "helm"
	connection?:  string & =~".+"
	releaseName?: string & =~".+"
	namespace:    string & =~".+"
}

#FilesystemUndeploy: {
	type:        "filesystem"
	destination: string & =~".+"
}

#Undeploy: #HelmUndeploy | #ScriptDeploy | #FilesystemUndeploy

#UndeployPhase: {
	#Undeploy
	preRun?:  [...#BuildCommand]
	postRun?: [...#BuildCommand]
}

#BuildPhase: {
	preRun?:  [...#BuildCommand]
	postRun?: [...#BuildCommand]
}
