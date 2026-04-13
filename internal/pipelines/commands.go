package pipelines

import "strings"

// CommandBuilder translates pipeline model objects into shell commands.
type CommandBuilder struct{}

// DockerBuild returns the docker buildx command for a docker artifact.
// If buildNumber is provided, tags both :latest and :<buildNumber>.
func (b *CommandBuilder) DockerBuild(art *Artifact, buildNumber string) []string {
	args := []string{"docker", "buildx", "build", "--progress=plain", "--provenance=false"}
	for _, p := range art.Platforms {
		args = append(args, "--platform", p)
	}
	for k, v := range art.BuildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}
	if art.Dockerfile != "" {
		args = append(args, "-f", art.Dockerfile)
	}
	image := art.Repository.FullImage()
	args = append(args, "-t", image+":latest")
	if buildNumber != "" {
		args = append(args, "-t", image+":"+buildNumber)
	}
	args = append(args, "--push", ".")
	return args
}

// BuildStepCommand returns the shell command for a build step.
func (b *CommandBuilder) BuildStepCommand(s *BuildStep) []string {
	var parts []string
	for k, v := range s.Env {
		parts = append(parts, k+"="+v)
	}
	parts = append(parts, s.Command)
	parts = append(parts, s.Args...)
	return parts
}

// HelmDeploy returns the helm upgrade/install command for a deploy.
//
// Parameters is a free-form shell-command tail (e.g. `--set foo=bar --set baz="with spaces"`).
// It is shell-split so each flag / value lands as its own argv entry, otherwise helm
// would see the whole blob as a single argument and reject it with "unknown flag: ...".
func (b *CommandBuilder) HelmDeploy(envName string, d *Deploy) []string {
	releaseName := d.ReleaseName
	if releaseName == "" {
		releaseName = envName
	}
	args := []string{"helm", "upgrade", "--install", releaseName, d.Chart, "--namespace", d.Namespace}
	if d.Repo != "" {
		args = append(args, "--repo", d.Repo)
	}
	if d.Parameters != "" {
		args = append(args, splitShellArgs(d.Parameters)...)
	}
	return args
}

// splitShellArgs performs a minimal POSIX-style split of s into argv tokens,
// preserving quoted regions. It supports single quotes (literal), double quotes
// (literal except \"), and \ escapes outside quotes. It does NOT do env expansion
// or globbing — templates were already expanded before we got here.
func splitShellArgs(s string) []string {
	var (
		tokens  []string
		cur     strings.Builder
		inSingle bool
		inDouble bool
		escaped  bool
	)

	flush := func() {
		tokens = append(tokens, cur.String())
		cur.Reset()
	}

	hasToken := false
	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			cur.WriteByte(c)
			escaped = false
			hasToken = true
			continue
		}

		switch {
		case !inSingle && !inDouble && c == '\\':
			escaped = true
		case !inSingle && c == '"':
			inDouble = !inDouble
			hasToken = true
		case !inDouble && c == '\'':
			inSingle = !inSingle
			hasToken = true
		case !inSingle && !inDouble && (c == ' ' || c == '\t' || c == '\n'):
			if hasToken {
				flush()
				hasToken = false
			}
		default:
			cur.WriteByte(c)
			hasToken = true
		}
	}
	if hasToken {
		flush()
	}
	return tokens
}

// FormatCommand joins command parts into a single string.
func (b *CommandBuilder) FormatCommand(parts []string) string {
	return strings.Join(parts, " ")
}
