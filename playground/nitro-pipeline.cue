package playground

import "github.com/nitroagility/nitrocli/pipelines@v0"

config: pipelines.#PipelineFile & {

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
			priority: 2
			envs:     ["dev", "uat", "prod"]
			variables: [
				{name: "GITHUB_TOKEN", path: "GITHUB_TOKEN", secret: true},
				{name: "BUILD_VERSION", path: "BUILD_VERSION", secret: false, default: "0.0.0-dev"},
				{name: "ECR_URL", path: "ECR_URL", secret: false, default: "851725253520.dkr.ecr.eu-central-1.amazonaws.com"},
				{name: "AWS_PROFILE", path: "AWS_PROFILE", secret: false, default: "default"},
				{name: "DOCKER_USER", path: "DOCKER_USER", secret: false, default: "myorg"},
				{name: "DOCKER_PASSWORD", path: "DOCKER_PASSWORD", secret: true},
			]
		}
		"docker-auth": {
			type:     "composite"
			priority: 1
			envs:     ["dev", "uat", "prod"]
			composites: [
				{name: "DOCKER_AUTH_B64", vars: ["DOCKER_USER", "DOCKER_PASSWORD"], secret: true, base64: true},
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
