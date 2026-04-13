package playground

import "github.com/nitroagility/nitrocli/pipelines@v0"

config: pipelines.#PipelineFile & {

	globals: [
		"GITHUB_TOKEN",
		"DOCKER_PASSWORD",
		"BWS_ACCESS_TOKEN",
		// AWS_DEPLOY_ACCESS_KEY_ID / AWS_DEPLOY_SECRET_ACCESS_KEY would go here
		// to feed aws-secrets.credentialsFromVars (see provider below). Pending
		// a nitrocli module tag that exposes that schema field.
	]

	preRun: [
		{command: "echo", args: ["[global] Pipeline started for {{ .Env.NITRO_ENV }} environment"]},
	]

	postRun: [
		{command: "echo", args: ["[global] Pipeline finished for {{ .Env.NITRO_ENV }} environment"]},
	]

	providers: {
		"local-env": {
			type:     "env"
			priority: 3
			envs:     ["dev", "uat", "prod"]
			variables: [
				{name: "GITHUB_TOKEN", path: "GITHUB_TOKEN", secret: true},
				{name: "BUILD_VERSION", path: "BUILD_VERSION", secret: false, default: "0.0.0-dev"},
				{name: "ECR_URL", path: "ECR_URL", secret: false, default: "851725253520.dkr.ecr.eu-central-1.amazonaws.com"},
				{name: "DOCKER_USER", path: "DOCKER_USER", secret: false, default: "myorg"},
				{name: "DOCKER_PASSWORD", path: "DOCKER_PASSWORD", secret: true},
				{name: "AWS_ACCOUNT_ID", path: "AWS_ACCOUNT_ID_DEV", secret: false, envs: ["dev"]},
				{name: "AWS_ACCOUNT_ID", path: "AWS_ACCOUNT_ID_UAT", secret: false, envs: ["uat"]},
				{name: "AWS_ACCOUNT_ID", path: "AWS_ACCOUNT_ID_PROD", secret: false, envs: ["prod"]},
			]
		}

		"aws-secrets": {
			type:     "aws-secretsmanager"
			priority: 2
			region:   "eu-central-1"
			envs:     ["prod"]
			// NOTE: requires nitrocli schema > v0.0.17 (this playground's cue.mod
			// dep). Once a new tag is published, bump cue.mod/module.cue and
			// uncomment below. Until then, the SDK default chain is used.
			//
			// Per-env static credentials for GetSecretValue. Each entry points at
			// already-resolved variables (globals or higher-priority provider).
			// When no entry matches the current env, the SDK default chain is used
			// (env → ~/.aws/credentials → IMDS) — handy for CodeBuild with IAM role.
			//
			// credentialsFromVars: {
			// 	prod: {
			// 		accessKeyID:     "AWS_DEPLOY_ACCESS_KEY_ID"
			// 		secretAccessKey: "AWS_DEPLOY_SECRET_ACCESS_KEY"
			// 	}
			// }
			variables: [
				{name: "DB_CONNECTION_STRING", path: "prod/database/connection", secret: true},
				{name: "DB_USERNAME", path: "prod/database/credentials", key: "username", secret: true},
				{name: "DB_PASSWORD", path: "prod/database/credentials", key: "password", secret: true},
			]
		}

		"bitwarden-secrets": {
			type:     "bitwarden"
			priority: 2
			envs:     ["dev", "uat"]
			variables: [
				{name: "DB_CONNECTION_STRING", path: "bf14e956-baed-11ed-afa1-0242ac120002", secret: true},
			]
		}

		"docker-auth": {
			type:     "transformer"
			priority: 1
			envs:     ["dev", "uat", "prod"]
			transformers: [
				{type: "envfile", name: "DOCKER_AUTH_B64", vars: ["DOCKER_USER", "DOCKER_PASSWORD"], secret: true, base64: true},
				{type: "template", name: "DOCKER_AUTH_BASIC", vars: ["DOCKER_USER", "DOCKER_PASSWORD"], secret: true, format: "{{ .DOCKER_USER }}:{{ .DOCKER_PASSWORD }}"},
			]
		}
	}

	artifacts: {
		"api-gateway": {
			type:       "docker"
			workdir:    "./services/api-gateway"
			dockerfile: "./Dockerfile"
			platforms:  ["linux/amd64"]
			buildArgs: {
				BUILD_VERSION: "{{ .Env.BUILD_VERSION }}"
			}
			postBuild: [
				{command: "echo", args: ["[artifact] api-gateway:{{ .Env.NITRO_BUILD_NUMBER }} built"]},
			]
			promote: {
				type: "script"
				steps: [
					{command: "echo", args: ["[promote] re-tagging api-gateway from {{ .Env.NITRO_ENV }}"]},
				]
			}
			repository: {
				type:  "registry"
				url:   "{{ .Env.ECR_URL }}"
				image: "api-gateway"
			}
		}

		"payment-service": {
			type:       "docker"
			workdir:    "./services/payment"
			dockerfile: "./Dockerfile"
			platforms:  ["linux/amd64"]
			buildArgs: {
				BUILD_VERSION: "{{ .Env.BUILD_VERSION }}"
			}
			promote: {
				type: "script"
				steps: [
					{command: "echo", args: ["[promote] re-tagging payment-service from {{ .Env.NITRO_ENV }}"]},
				]
			}
			repository: {
				type:  "registry"
				url:   "{{ .Env.ECR_URL }}"
				image: "payment-service"
			}
		}

		"mycli": {
			type:    "binary"
			workdir: "./tools/mycli"
			platforms: ["linux/amd64"]
			build: [
				{command: "go", args: ["build", "-ldflags", "-X main.version={{ .Env.BUILD_VERSION }}", "-o", "bin/mycli", "."], env: {CGO_ENABLED: "0"}},
			]
			deploy: {
				type:        "filesystem"
				source:      "bin/mycli"
				destination: "/tmp/nitro-artifacts/bin/"
			}
			repository: {
				type: "filesystem"
				path: "/tmp/nitro-artifacts/bin"
			}
		}

		"shared-utils": {
			type:     "package"
			workdir:  "./libs/shared-utils"
			language: "node"
			build: [
				{command: "node", args: ["test.js"]},
			]
			deploy: {
				type:        "filesystem"
				source:      "."
				destination: "/tmp/nitro-artifacts/packages/shared-utils/"
			}
			repository: {
				type: "package"
				kind: "npm"
				url:  "https://registry.npmjs.org"
			}
		}
	}

	environments: {
		"build": {
			strategy: "build"
		}

		"dev": {
			strategy:     "build"
			promotesFrom: "build"
			build: {
				preRun: [
					{command: "echo", args: ["[env] Starting {{ .Env.NITRO_ENV }} build phase"]},
				]
			}
			deploy: {
				type:      "helm"
				chart:     "myorg/api-gateway"
				repo:      "https://charts.myorg.io"
				namespace: "dev"
				values: {
					replicaCount: 1
					ingress: enabled: false
					resources: limits: {cpu: "200m", memory: "256Mi"}
				}
			}
		}

		"uat": {
			strategy:     "promote"
			promotesFrom: "dev"
			deploy: {
				type:      "helm"
				chart:     "myorg/api-gateway"
				repo:      "https://charts.myorg.io"
				namespace: "uat"
				values: {
					replicaCount: 2
					ingress: enabled: true
					resources: limits: {cpu: "500m", memory: "512Mi"}
				}
			}
		}

		"prod": {
			strategy:     "promote"
			promotesFrom: "uat"
			artifacts: ["api-gateway", "payment-service"]
			deploy: {
				type:      "helm"
				chart:     "myorg/api-gateway"
				repo:      "https://charts.myorg.io"
				namespace: "production"
				preRun: [
					{command: "echo", args: ["[deploy] WARNING: Deploying to {{ .Env.NITRO_ENV }}!"]},
				]
				values: {
					replicaCount: 5
					ingress: {enabled: true, host: "api.myorg.io"}
					resources: limits: {cpu: "1000m", memory: "1Gi"}
				}
			}
		}
	}
}
