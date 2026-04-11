package pipelines

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Runner orchestrates pipeline execution by coordinating loader, commands,
// executor, providers, and output.
type Runner struct {
	Config   *Config
	DryRun   bool
	Workdir  string
	Commands *CommandBuilder
	Executor *Executor
	Provider *ProviderResolver
	Log      *Logger
}

// NewRunner creates a fully wired Runner for the given config and mode.
// A single Masker instance is shared across all layers to prevent secret leaks.
// The workdir is used as the base directory for all artifact commands.
func NewRunner(cfg *Config, dryRun bool, workdir string) *Runner {
	masker := &Masker{}
	log := &Logger{Masker: masker}
	return &Runner{
		Config:   cfg,
		DryRun:   dryRun,
		Workdir:  workdir,
		Commands: &CommandBuilder{},
		Executor: &Executor{DryRun: dryRun, Log: log, Masker: masker},
		Provider: &ProviderResolver{Log: log, Masker: masker},
		Log:      log,
	}
}

// Run executes the pipeline for the specified environment.
func (r *Runner) Run(ctx context.Context, envName string) error {
	env, ok := r.Config.Environments[envName]
	if !ok {
		return fmt.Errorf(
			"environment %q not found, available: %s",
			envName,
			strings.Join(r.Config.EnvironmentNames(), ", "),
		)
	}

	start := time.Now()

	r.printHeader(envName, env)
	r.resolveProviders(envName)

	if err := r.processArtifacts(ctx, env); err != nil {
		r.printFailure(start, err)
		return err
	}

	if err := r.processDeploy(ctx, envName, env); err != nil {
		r.printFailure(start, err)
		return err
	}

	r.printFooter(start)
	return nil
}

// resolveWorkdir joins the base workdir with the artifact's workdir.
func (r *Runner) resolveWorkdir(art *Artifact) string {
	return filepath.Join(r.Workdir, art.EffectiveWorkdir())
}

func (r *Runner) printHeader(envName string, env *Environment) {
	mode := "LIVE"
	if r.DryRun {
		mode = "DRY-RUN"
	}

	r.Log.Separator()
	r.Log.Header(fmt.Sprintf("Pipeline Run [%s]", mode))
	r.Log.Info(fmt.Sprintf("environment: %s | strategy: %s | workdir: %s", envName, env.Strategy, r.Workdir))
	if env.PromotesFrom != "" {
		r.Log.Info(fmt.Sprintf("promotes from: %s", env.PromotesFrom))
	}
	r.Log.Separator()
}

func (r *Runner) resolveProviders(envName string) {
	if len(r.Config.Providers) == 0 {
		return
	}

	r.Log.Step("Resolving providers")
	vars := r.Provider.Resolve(r.Config.Providers, envName)
	r.Executor.SetEnv(vars)
	r.Log.Separator()
}

func (r *Runner) processArtifacts(ctx context.Context, env *Environment) error {
	names := env.Artifacts
	if len(names) == 0 {
		names = r.Config.ArtifactNames()
	}

	for _, name := range names {
		art, ok := r.Config.Artifacts[name]
		if !ok {
			r.Log.Fail(fmt.Sprintf("artifact %q not found, skipping", name))
			continue
		}
		if err := r.runArtifact(ctx, name, art, env); err != nil {
			return fmt.Errorf("artifact %q failed: %w", name, err)
		}
	}

	return nil
}

func (r *Runner) runArtifact(ctx context.Context, name string, art *Artifact, env *Environment) error {
	r.Log.Step(fmt.Sprintf("%s (%s)", name, art.Type))

	var err error
	switch {
	case art.IsDocker():
		err = r.runDocker(ctx, art, env)
	case art.IsBinary():
		err = r.runBuild(ctx, art)
	case art.IsPackage():
		err = r.runPackage(ctx, art)
	default:
		r.Log.Info(fmt.Sprintf("unknown artifact type: %s", art.Type))
	}

	if err != nil {
		return err
	}

	r.Log.Separator()
	return nil
}

func (r *Runner) runDocker(ctx context.Context, art *Artifact, env *Environment) error {
	if env.IsPromote() {
		r.Log.Promote(fmt.Sprintf("promoting %s from %s", art.Repository.FullImage(), env.PromotesFrom))
		return nil
	}

	workdir := r.resolveWorkdir(art)
	r.Log.Info(fmt.Sprintf("workdir: %s", workdir))

	args := r.Commands.DockerBuild(art)
	return r.Executor.Run(ctx, args, workdir)
}

func (r *Runner) runBuild(ctx context.Context, art *Artifact) error {
	workdir := r.resolveWorkdir(art)
	r.Log.Info(fmt.Sprintf("workdir: %s", workdir))

	for i, s := range art.Build {
		r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(art.Build), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
		args := r.Commands.BuildStepCommand(&s)
		if err := r.Executor.Run(ctx, args, workdir); err != nil {
			return err
		}
	}

	if art.Repository.Type == "filesystem" {
		r.Log.Info(fmt.Sprintf("output: %s", art.Repository.Path))
	}

	return nil
}

func (r *Runner) runPackage(ctx context.Context, art *Artifact) error {
	workdir := r.resolveWorkdir(art)
	r.Log.Info(fmt.Sprintf("workdir: %s | language: %s", workdir, art.Language))

	for i, s := range art.Build {
		r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(art.Build), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
		args := r.Commands.BuildStepCommand(&s)
		if err := r.Executor.Run(ctx, args, workdir); err != nil {
			return err
		}
	}

	r.Log.Promote(fmt.Sprintf("publish to %s (%s)", art.Repository.URL, art.Repository.Kind))
	return nil
}

func (r *Runner) processDeploy(ctx context.Context, envName string, env *Environment) error {
	if env.Deploy == nil {
		return nil
	}

	r.Log.Step(fmt.Sprintf("deploy --> %s", envName))

	var err error
	switch env.Deploy.Type {
	case "helm":
		args := r.Commands.HelmDeploy(envName, env.Deploy)
		err = r.Executor.Run(ctx, args, r.Workdir)
	default:
		r.Log.Info(fmt.Sprintf("deploy type: %s", env.Deploy.Type))
	}

	if err != nil {
		return err
	}

	r.Log.Separator()
	return nil
}

func (r *Runner) printFooter(start time.Time) {
	elapsed := time.Since(start).Round(time.Millisecond)
	r.Log.Success(fmt.Sprintf("Pipeline completed in %s", elapsed))
	r.Log.Separator()
}

func (r *Runner) printFailure(start time.Time, err error) {
	elapsed := time.Since(start).Round(time.Millisecond)
	r.Log.Separator()
	r.Log.Fail(fmt.Sprintf("Pipeline failed in %s: %s", elapsed, err))
	r.Log.Separator()
}
