package playground

import "github.com/nitroagility/nitrocli/pipelines@v0"

config: pipelines.#PipelineFile & {

	// ============================================================
	// GLOBALS — allowlist for ~/.nitro/config.json imports
	// Only these variables can be resolved from the local config.
	// ============================================================

	globals: [
		"GITHUB_TOKEN",
		"DOCKER_PASSWORD",
		"BWS_ACCESS_TOKEN",
	]

	// ============================================================
	// GLOBAL HOOKS
	// ============================================================

	preRun: [
		{command: "echo", args: ["[global] Pipeline started for {{ .Env.NITRO_ENV }} environment"]},
	]

	postRun: [
		{command: "echo", args: ["[global] Pipeline finished for {{ .Env.NITRO_ENV }} environment"]},
	]

	// ============================================================
	// PROVIDERS
	// ============================================================

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

				// Per-environment variable mapping: same logical name, different source per env.
				{name: "AWS_ACCOUNT_ID", path: "AWS_ACCOUNT_ID_DEV", secret: false, envs: ["dev"]},
				{name: "AWS_ACCOUNT_ID", path: "AWS_ACCOUNT_ID_UAT", secret: false, envs: ["uat"]},
				{name: "AWS_ACCOUNT_ID", path: "AWS_ACCOUNT_ID_PROD", secret: false, envs: ["prod"]},
			]
		}

		// AWS Secrets Manager: reads secrets from AWS SM.
		// path = secret name/ARN, key = optional JSON key extraction.
		// Credentials via standard AWS SDK chain (env vars, shared config, IAM role).
		"aws-secrets": {
			type:     "aws-secretsmanager"
			priority: 2
			region:   "eu-central-1"
			envs:     ["prod"]
			variables: [
				// Reads the entire secret as a single string.
				{name: "DB_CONNECTION_STRING", path: "prod/database/connection", secret: true},

				// Reads only the "username" key from a JSON secret.
				{name: "DB_USERNAME", path: "prod/database/credentials", key: "username", secret: true},
				{name: "DB_PASSWORD", path: "prod/database/credentials", key: "password", secret: true},
			]
		}

		// Bitwarden Secrets Manager: reads secrets via bws CLI.
		// path = Bitwarden secret UUID.
		// Token via BWS_ACCESS_TOKEN env var or ~/.nitro/config.json.
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
				// envfile: produces DOCKER_USER=...\nDOCKER_PASSWORD=...
				{type: "envfile", name: "DOCKER_AUTH_B64", vars: ["DOCKER_USER", "DOCKER_PASSWORD"], secret: true, base64: true},

				// template: produces user:password using Go template
				{type: "template", name: "DOCKER_AUTH_BASIC", vars: ["DOCKER_USER", "DOCKER_PASSWORD"], secret: true, format: "{{ .DOCKER_USER }}:{{ .DOCKER_PASSWORD }}"},
			]
		}
	}

	// ============================================================
	// ARTIFACTS
	// ============================================================

	artifacts: {
		"api-gateway": {
			type:       "docker"
			workdir:    "./services/api-gateway"
			dockerfile: "./Dockerfile"
			platforms:  ["linux/amd64", "linux/arm64"]
			buildArgs: {
				GO_VERSION:    "1.22"
				BUILD_VERSION: "{{ .Env.BUILD_VERSION }}"
				GITHUB_TOKEN:  "{{ .Env.GITHUB_TOKEN }}"
			}
			preRun: [
				{command: "echo", args: ["[artifact] ECR login for api-gateway on {{ .Env.ECR_URL }}"]},
				{command: "echo", args: ["[artifact] Ensuring ECR repo api-gateway exists"]},
			]
			postRun: [
				{command: "echo", args: ["[artifact] api-gateway:{{ .Env.NITRO_BUILD_NUMBER }} pushed to {{ .Env.ECR_URL }}"]},
			]
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
			preRun: [
				{command: "echo", args: ["[artifact] Ensuring ECR repo payment-service exists"]},
			]
			postRun: [
				{command: "echo", args: ["[artifact] payment-service:{{ .Env.NITRO_BUILD_NUMBER }} pushed to {{ .Env.ECR_URL }}"]},
			]
			repository: {
				type:  "registry"
				url:   "{{ .Env.ECR_URL }}"
				image: "payment-service"
			}
		}

		"mycli": {
			type:    "binary"
			workdir: "./tools/mycli"
			platforms: ["linux/amd64", "linux/arm64"]
			preRun: [
				{command: "echo", args: ["[artifact] Setting up Go build for mycli"]},
			]
			build: [
				{command: "echo", args: ["[build] go generate ./..."]},
				{command: "echo", args: ["[build] go test ./..."]},
				{command: "echo", args: ["[build] CGO_ENABLED=0 go build -o bin/mycli ./cmd/mycli"]},
			]
			postRun: [
				{command: "echo", args: ["[artifact] mycli binary ready at /artifacts/bin"]},
			]
			repository: {
				type: "filesystem"
				path: "/artifacts/bin"
			}
		}

		"shared-utils": {
			type:     "package"
			workdir:  "./libs/shared-utils"
			language: "go"
			preRun: [
				{command: "echo", args: ["[artifact] Preparing shared-utils package"]},
			]
			build: [
				{command: "echo", args: ["[build] go test ./..."]},
				{command: "echo", args: ["[build] go build ./..."]},
			]
			postRun: [
				{command: "echo", args: ["[artifact] shared-utils published to Go proxy"]},
			]
			repository: {
				type: "package"
				kind: "go"
				url:  "https://proxy.golang.org"
			}
		}
	}

	// ============================================================
	// ENVIRONMENTS
	// ============================================================

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
				postRun: [
					{command: "echo", args: ["[env] {{ .Env.NITRO_ENV }} build phase completed"]},
				]
			}
			deploy: {
				type:      "helm"
				chart:     "myorg/api-gateway"
				repo:      "https://charts.myorg.io"
				namespace: "dev"
				preRun: [
					{command: "echo", args: ["[deploy] Preparing {{ .Env.NITRO_ENV }} deployment"]},
				]
				postRun: [
					{command: "echo", args: ["[deploy] {{ .Env.NITRO_ENV }} deployment completed"]},
				]
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
				preRun: [
					{command: "echo", args: ["[deploy] Preparing {{ .Env.NITRO_ENV }} deployment"]},
				]
				postRun: [
					{command: "echo", args: ["[deploy] {{ .Env.NITRO_ENV }} deployment completed"]},
				]
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
			artifacts:    ["api-gateway", "payment-service"]
			deploy: {
				type:      "helm"
				chart:     "myorg/api-gateway"
				repo:      "https://charts.myorg.io"
				namespace: "production"
				preRun: [
					{command: "echo", args: ["[deploy] WARNING: Deploying to {{ .Env.NITRO_ENV }}!"]},
				]
				postRun: [
					{command: "echo", args: ["[deploy] {{ .Env.NITRO_ENV }} deployment completed"]},
					{command: "echo", args: ["[deploy] Running post-deploy health checks for {{ .Env.NITRO_ENV }}"]},
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
