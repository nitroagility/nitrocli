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
	envName  string
	Commands *CommandBuilder
	Executor *Executor
	Provider *ProviderResolver
	Log      *Logger
}

// RunOptions controls which phases of the pipeline to execute.
type RunOptions struct {
	Build       bool
	Deploy      bool
	BuildNumber string
}

// NewRunner creates a fully wired Runner for the given config and mode.
// A single Masker instance is shared across all layers to prevent secret leaks.
// The workdir is used as the base directory for all artifact commands.
func NewRunner(cfg *Config, dryRun bool, workdir string) *Runner {
	masker := &Masker{}
	log := &Logger{Masker: masker}

	// Build globals allowlist for config.json lookups.
	globals := make(map[string]bool, len(cfg.Globals))
	for _, g := range cfg.Globals {
		globals[g] = true
	}

	return &Runner{
		Config:   cfg,
		DryRun:   dryRun,
		Workdir:  workdir,
		Commands: &CommandBuilder{},
		Executor: &Executor{DryRun: dryRun, Log: log, Masker: masker},
		Provider: &ProviderResolver{Log: log, Masker: masker, Globals: globals},
		Log:      log,
	}
}

// Run executes the pipeline for the specified environment.
func (r *Runner) Run(ctx context.Context, envName string, opts RunOptions) error {
	env, ok := r.Config.Environments[envName]
	if !ok {
		return &PipelineError{
			Phase:   "run",
			Summary: fmt.Sprintf("Environment %q not found", envName),
			Details: []string{fmt.Sprintf("available environments: %s", strings.Join(r.Config.EnvironmentNames(), ", "))},
			Hint:    "check the --env flag value",
		}
	}

	r.envName = envName
	start := time.Now()

	r.printHeader(envName, env, opts)
	r.resolveProviders(ctx, envName, opts)

	// Global preRun.
	if err := r.runHooks(ctx, r.envName, "Global pre-run", r.Config.PreRun); err != nil {
		r.printFailure(start, err)
		return err
	}

	if opts.Build {
		if err := r.runBuildPhase(ctx, env, opts); err != nil {
			r.printFailure(start, err)
			return err
		}
	}

	if opts.Deploy {
		if err := r.runDeployPhase(ctx, envName, env); err != nil {
			r.printFailure(start, err)
			return err
		}
	}

	// Global postRun.
	if err := r.runHooks(ctx, r.envName, "Global post-run", r.Config.PostRun); err != nil {
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

func (r *Runner) printHeader(envName string, env *Environment, opts RunOptions) {
	mode := "LIVE"
	if r.DryRun {
		mode = "DRY-RUN"
	}

	var phases []string
	if opts.Build {
		phases = append(phases, "build")
	}
	if opts.Deploy {
		phases = append(phases, "deploy")
	}

	r.Log.Separator()
	r.Log.Header(fmt.Sprintf("Pipeline Run [%s]", mode))
	r.Log.Info(fmt.Sprintf("environment: %s | strategy: %s | phases: %s", envName, env.Strategy, strings.Join(phases, ", ")))
	r.Log.Info(fmt.Sprintf("workdir: %s", r.Workdir))
	if env.PromotesFrom != "" {
		r.Log.Info(fmt.Sprintf("promotes from: %s", env.PromotesFrom))
	}
	r.Log.Separator()
}

func (r *Runner) resolveProviders(ctx context.Context, envName string, opts RunOptions) {
	r.Log.Step("Resolving providers")

	var vars map[string]string
	if len(r.Config.Providers) > 0 {
		vars = r.Provider.Resolve(ctx, r.Config.Providers, envName)
	} else {
		vars = make(map[string]string)
	}

	// Inject NITRO_ reserved variables.
	vars["NITRO_ENV"] = envName
	r.Log.Info(fmt.Sprintf("NITRO_ENV = %s", envName))

	if opts.BuildNumber != "" {
		vars["NITRO_BUILD_NUMBER"] = opts.BuildNumber
		r.Log.Info(fmt.Sprintf("NITRO_BUILD_NUMBER = %s", opts.BuildNumber))
	}

	r.Executor.SetEnv(vars)
	r.Log.Separator()
}

// ── Build Phase ──────────────────────────────────────

func (r *Runner) runBuildPhase(ctx context.Context, env *Environment, opts RunOptions) error {
	// Build preRun.
	if env.Build != nil {
		if err := r.runHooks(ctx, r.envName, "Build pre-run", env.Build.PreRun); err != nil {
			return err
		}
	}

	// Build artifacts.
	if err := r.processArtifacts(ctx, env, opts); err != nil {
		return err
	}

	// Build postRun.
	if env.Build != nil {
		if err := r.runHooks(ctx, r.envName, "Build post-run", env.Build.PostRun); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) processArtifacts(ctx context.Context, env *Environment, opts RunOptions) error {
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
		if err := r.runArtifact(ctx, name, art, env, opts); err != nil {
			return fmt.Errorf("artifact %q failed: %w", name, err)
		}
	}

	return nil
}

func (r *Runner) runArtifact(ctx context.Context, name string, art *Artifact, env *Environment, opts RunOptions) error {
	r.Log.Step(fmt.Sprintf("%s (%s)", name, art.Type))

	workdir := r.resolveWorkdir(art)

	// Artifact preRun.
	if err := r.runHooks(ctx, r.envName, fmt.Sprintf("%s pre-run", name), art.PreRun); err != nil {
		return err
	}

	var err error
	switch {
	case art.IsDocker():
		err = r.runDocker(ctx, art, env, opts, workdir)
	case art.IsBinary():
		err = r.runBinaryBuild(ctx, art, workdir)
	case art.IsPackage():
		err = r.runPackage(ctx, art, workdir)
	default:
		r.Log.Info(fmt.Sprintf("unknown artifact type: %s", art.Type))
	}

	if err != nil {
		return err
	}

	// Artifact postRun.
	if err := r.runHooks(ctx, r.envName, fmt.Sprintf("%s post-run", name), art.PostRun); err != nil {
		return err
	}

	r.Log.Separator()
	return nil
}

func (r *Runner) runDocker(ctx context.Context, art *Artifact, env *Environment, opts RunOptions, workdir string) error {
	if env.IsPromote() {
		r.Log.Promote(fmt.Sprintf("promoting %s from %s", art.Repository.FullImage(), env.PromotesFrom))
		return nil
	}

	r.Log.Info(fmt.Sprintf("workdir: %s", workdir))

	args := r.Commands.DockerBuild(art, opts.BuildNumber)
	return r.Executor.Run(ctx, args, workdir)
}

func (r *Runner) runBinaryBuild(ctx context.Context, art *Artifact, workdir string) error {
	r.Log.Info(fmt.Sprintf("workdir: %s", workdir))

	steps := filterSteps(art.Build, r.envName)
	for i, s := range steps {
		r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(steps), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
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

func (r *Runner) runPackage(ctx context.Context, art *Artifact, workdir string) error {
	r.Log.Info(fmt.Sprintf("workdir: %s | language: %s", workdir, art.Language))

	steps := filterSteps(art.Build, r.envName)
	for i, s := range steps {
		r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(steps), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
		args := r.Commands.BuildStepCommand(&s)
		if err := r.Executor.Run(ctx, args, workdir); err != nil {
			return err
		}
	}

	r.Log.Promote(fmt.Sprintf("publish to %s (%s)", art.Repository.URL, art.Repository.Kind))
	return nil
}

// ── Deploy Phase ─────────────────────────────────────

func (r *Runner) runDeployPhase(ctx context.Context, envName string, env *Environment) error {
	if env.Deploy == nil {
		return nil
	}

	// Deploy preRun.
	if err := r.runHooks(ctx, r.envName, "Deploy pre-run", env.Deploy.PreRun); err != nil {
		return err
	}

	// Deploy.
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

	// Deploy postRun.
	if err := r.runHooks(ctx, r.envName, "Deploy post-run", env.Deploy.PostRun); err != nil {
		return err
	}

	return nil
}

// ── Hooks ────────────────────────────────────────────

func (r *Runner) runHooks(ctx context.Context, envName string, label string, steps []BuildStep) error {
	if len(steps) == 0 {
		return nil
	}

	applicable := filterSteps(steps, envName)
	if len(applicable) == 0 {
		return nil
	}

	r.Log.Step(label)

	for i, s := range applicable {
		r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(applicable), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
		args := r.Commands.BuildStepCommand(&s)
		if err := r.Executor.Run(ctx, args, r.Workdir); err != nil {
			return fmt.Errorf("%s step %d failed: %w", label, i+1, err)
		}
	}

	r.Log.Separator()
	return nil
}

func filterSteps(steps []BuildStep, envName string) []BuildStep {
	var result []BuildStep
	for _, s := range steps {
		if s.AppliesToEnv(envName) {
			result = append(result, s)
		}
	}
	return result
}

// ── Footer ───────────────────────────────────────────

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
