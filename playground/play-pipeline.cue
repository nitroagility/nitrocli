package pipelines

config: #PipelineFile & {

	// ============================================================
	// PROVIDERS
	// ============================================================

	providers: {
		"nitro-bitwarden": {
			type:     "bitwarden"
			priority: 1
			url:      "https://vault.bitwarden.com"
			envs:     ["dev", "uat", "prod"]
			variables: [
				{name: "DOCKER_PASSWORD", path: "collection/docker-credentials/password"},
				{name: "JFROG_PASSWORD",  path: "collection/jfrog-credentials/password"},
				{name: "GITHUB_TOKEN",    path: "collection/github-token/password"},
			]
		}
	}

	// ============================================================
	// ARTIFACTS
	// ============================================================

	artifacts: {

		// Docker image — multi-arch, pushed to GHCR
		"api-gateway": {
			type:       "docker"
			workdir:    "./services/api-gateway"
			dockerfile: "./Dockerfile"
			platforms:  ["linux/amd64", "linux/arm64"]
			buildArgs: {
				GO_VERSION:    "1.22"
				BUILD_VERSION: "{{ .BUILD_VERSION }}"
				GITHUB_TOKEN:  "{{ .GITHUB_TOKEN }}"
			}
			repository: {
				type:  "registry"
				url:   "ghcr.io"
				user:  "myorg"
				image: "myorg/api-gateway"
			}
		}

		// Docker image — single arch internal service
		"payment-service": {
			type:       "docker"
			workdir:    "./services/payment"
			dockerfile: "./Dockerfile"
			platforms:  ["linux/amd64"]
			buildArgs: {
				BUILD_VERSION: "{{ .BUILD_VERSION }}"
			}
			repository: {
				type:  "registry"
				url:   "ghcr.io"
				user:  "myorg"
				image: "myorg/payment-service"
			}
		}

		// Binary — CLI tool, multi-step build
		"mycli": {
			type:    "binary"
			workdir: "./tools/mycli"
			platforms: ["linux/amd64", "linux/arm64"]
			build: [
				{
					command: "go"
					args:    ["generate", "./..."]
				},
				{
					command: "go"
					args:    ["test", "./..."]
				},
				{
					command: "go"
					args:    ["build", "-ldflags", "-s -w", "-o", "bin/mycli", "./cmd/mycli"]
					env: {
						CGO_ENABLED: "0"
						GOOS:        "linux"
					}
				},
			]
			repository: {
				type: "filesystem"
				path: "/artifacts/bin"
			}
		}

		// Go library — published to Go proxy
		"shared-utils": {
			type:     "package"
			workdir:  "./libs/shared-utils"
			language: "go"
			build: [
				{
					command: "go"
					args:    ["test", "./..."]
				},
				{
					command: "go"
					args:    ["build", "./..."]
				},
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

		// First environment — no predecessor, always build from source
		"build": {
			strategy: "build"
		}

		// Dev — rebuild from source, deploy with minimal resources
		"dev": {
			strategy:     "build"
			promotesFrom: "build"
			deploy: {
				type:       "helm"
				chart:      "myorg/api-gateway"
				repo:       "https://charts.myorg.io"
				namespace:  "dev"
				parameters: "--set image.tag={{ .BUILD_VERSION }}"
				values: {
					replicaCount: 1
					ingress: enabled: false
					resources: limits: {
						cpu:    "200m"
						memory: "256Mi"
					}
				}
			}
		}

		// UAT — promote all artifacts from dev
		"uat": {
			strategy:     "promote"
			promotesFrom: "dev"
			// artifacts absent = promotes everything
			deploy: {
				type:       "helm"
				chart:      "myorg/api-gateway"
				repo:       "https://charts.myorg.io"
				namespace:  "uat"
				parameters: "--set image.tag={{ .BUILD_VERSION }}"
				values: {
					replicaCount: 2
					ingress: enabled: true
					resources: limits: {
						cpu:    "500m"
						memory: "512Mi"
					}
				}
			}
		}

		// Prod — promote only docker images from uat, full resources
		"prod": {
			strategy:     "promote"
			promotesFrom: "uat"
			artifacts:    ["api-gateway", "payment-service"] // selective promote
			deploy: {
				type:       "helm"
				chart:      "myorg/api-gateway"
				repo:       "https://charts.myorg.io"
				namespace:  "production"
				parameters: "--set image.tag={{ .BUILD_VERSION }}"
				values: {
					replicaCount: 5
					ingress: {
						enabled: true
						host:    "api.myorg.io"
					}
					resources: limits: {
						cpu:    "1000m"
						memory: "1Gi"
					}
				}
			}
		}
	}
}
