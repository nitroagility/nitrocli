package playground

import "github.com/nitroagility/nitrocli/pipelines@v0"

config: pipelines.#PipelineFile & {

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
				{command: "bash", args: ["-c", "aws ecr get-login-password --region eu-central-1 --profile {{ .Env.AWS_PROFILE }} | docker login --username AWS --password-stdin {{ .Env.ECR_URL }}"]},
				{command: "bash", args: ["-c", "aws ecr describe-repositories --repository-names api-gateway --region eu-central-1 --profile {{ .Env.AWS_PROFILE }} 2>/dev/null || aws ecr create-repository --repository-name api-gateway --region eu-central-1 --profile {{ .Env.AWS_PROFILE }}"]},
			]
			postRun: [
				{command: "echo", args: ["api-gateway image pushed successfully"]},
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
				{command: "bash", args: ["-c", "aws ecr describe-repositories --repository-names payment-service --region eu-central-1 --profile {{ .Env.AWS_PROFILE }} 2>/dev/null || aws ecr create-repository --repository-name payment-service --region eu-central-1 --profile {{ .Env.AWS_PROFILE }}"]},
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
			build: [
				{command: "go", args: ["generate", "./..."]},
				{command: "go", args: ["test", "./..."]},
				{
					command: "go"
					args: ["build", "-ldflags", "-s -w", "-o", "bin/mycli", "./cmd/mycli"]
					env: {CGO_ENABLED: "0", GOOS: "linux"}
				},
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
			build: [
				{command: "go", args: ["test", "./..."]},
				{command: "go", args: ["build", "./..."]},
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
					{command: "echo", args: ["Starting dev build phase..."]},
				]
				postRun: [
					{command: "echo", args: ["Dev build phase completed"]},
				]
			}
			deploy: {
				type:       "helm"
				chart:      "myorg/api-gateway"
				repo:       "https://charts.myorg.io"
				namespace:  "dev"
				parameters: "--set image.tag={{ .Env.NITRO_BUILD_NUMBER }}"
				preRun: [
					{command: "echo", args: ["Preparing dev deployment..."]},
				]
				postRun: [
					{command: "echo", args: ["Dev deployment completed"]},
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
				type:       "helm"
				chart:      "myorg/api-gateway"
				repo:       "https://charts.myorg.io"
				namespace:  "uat"
				parameters: "--set image.tag={{ .Env.NITRO_BUILD_NUMBER }}"
				preRun: [
					{command: "echo", args: ["Preparing UAT deployment..."]},
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
				type:       "helm"
				chart:      "myorg/api-gateway"
				repo:       "https://charts.myorg.io"
				namespace:  "production"
				parameters: "--set image.tag={{ .Env.NITRO_BUILD_NUMBER }}"
				preRun: [
					{command: "echo", args: ["WARNING: Deploying to production!"]},
				]
				postRun: [
					{command: "echo", args: ["Production deployment completed"]},
					{command: "echo", args: ["Running post-deploy health checks..."]},
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
