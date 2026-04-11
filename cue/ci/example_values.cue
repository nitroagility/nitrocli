package ci

nitrocli: #CIBuild & {
	project:   "nitrocli"
	goVersion: "1.26"
	binary:    "nitro"
	platforms: [
		{os: "linux", arch: "amd64"},
		{os: "linux", arch: "arm64"},
		{os: "darwin", arch: "amd64"},
		{os: "darwin", arch: "arm64"},
		{os: "windows", arch: "amd64"},
		{os: "windows", arch: "arm64"},
	]
	lint: {
		enabled: true
		config:  ".golangci.yaml"
	}
	test: {
		enabled: true
		race:    true
	}
	release: {
		goreleaser: {
			config: ".goreleaser-pro.yaml"
			pro:    true
		}
		homebrew: {
			tap:  "nitroagility/homebrew-tap"
			name: "nitro"
		}
	}
}
