package ci

// CIBuild defines the schema for a continuous integration build pipeline.
#CIBuild: {
	// project is the project name.
	project: string

	// goVersion is the Go version to use.
	goVersion: string

	// binary is the output binary name.
	binary: string

	// entrypoint is the path to the main package.
	entrypoint: string | *"./cmd/\(binary)"

	// platforms is the list of target os/arch pairs.
	platforms: [...#Platform]

	// lint holds the linter configuration.
	lint: {
		enabled: *true | bool
		config:  *".golangci.yaml" | string
	}

	// test holds the test configuration.
	test: {
		enabled: *true | bool
		race:    *true | bool
	}

	// release holds the release configuration.
	release: {
		goreleaser: {
			config: *".goreleaser.yaml" | string
			pro:    *false | bool
		}
		homebrew?: {
			tap:  string
			name: string
		}
	}
}

// Platform defines a build target.
#Platform: {
	os:   "linux" | "darwin" | "windows"
	arch: "amd64" | "arm64"
}
