package pipelines

import "strings"

// CommandBuilder translates pipeline model objects into shell commands.
type CommandBuilder struct{}

// DockerBuild returns the docker buildx command for a docker artifact.
// If buildNumber is provided, tags both :latest and :<buildNumber>.
func (b *CommandBuilder) DockerBuild(art *Artifact, buildNumber string) []string {
	args := []string{"docker", "buildx", "build", "--progress=plain"}
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
	args = append(args, "--push", art.EffectiveWorkdir())
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
func (b *CommandBuilder) HelmDeploy(envName string, d *Deploy) []string {
	args := []string{"helm", "upgrade", "--install", envName, d.Chart, "--namespace", d.Namespace}
	if d.Repo != "" {
		args = append(args, "--repo", d.Repo)
	}
	if d.Parameters != "" {
		args = append(args, d.Parameters)
	}
	return args
}

// FormatCommand joins command parts into a single string.
func (b *CommandBuilder) FormatCommand(parts []string) string {
	return strings.Join(parts, " ")
}
