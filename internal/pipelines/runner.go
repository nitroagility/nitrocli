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
	Config      *Config
	DryRun      bool
	Unsafe      bool
	Workdir     string
	envName     string
	Commands    *CommandBuilder
	Executor    *Executor
	Provider    *ProviderResolver
	Connections *ConnectionResolver
	Log         *Logger

	// preResolved caches provider output per env, populated by RunAll's preflight
	// so we fail fast before running any build/deploy phase.
	preResolved map[string]map[string]string
}

// RunOptions controls which phases of the pipeline to execute.
type RunOptions struct {
	Build       bool
	Deploy      bool
	Undeploy    bool // mutually exclusive with Build
	BuildNumber string
	Unsafe      bool // disables path traversal protection for workdir
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

	connResolver := &ConnectionResolver{Log: log, Masker: masker}

	return &Runner{
		Config:      cfg,
		DryRun:      dryRun,
		Workdir:     workdir,
		Commands:    &CommandBuilder{},
		Executor:    &Executor{DryRun: dryRun, Log: log, Masker: masker, ConnResolver: connResolver},
		Provider:    &ProviderResolver{Log: log, Masker: masker, Globals: globals},
		Connections: connResolver,
		Log:         log,
	}
}

// RunAll executes the pipeline for multiple environments in dependency order.
// Environments are topologically sorted based on promotesFrom relationships
// between the requested environments. Disconnected environments run in any order.
//
// Flow:
//  1. Preflight: resolve providers for every requested env upfront. If any secret
//     fetch fails (AWS/Bitwarden error, missing required value, transformer input
//     missing) the whole run aborts here — we don't start docker builds or helm
//     deploys on a half-resolved config.
//  2. Execute: run each env's phases using its pre-resolved variables.
//
// Resolve() is idempotent and the AWS resolver caches by secret path, so resolving
// during preflight and again inside Run() does not duplicate API calls. We cache
// the result map directly to avoid even the map-building overhead.
func (r *Runner) RunAll(ctx context.Context, envNames []string, opts RunOptions) error {
	sorted, err := r.topoSortEnvs(envNames)
	if err != nil {
		return err
	}

	// ── Phase 1: preflight ─────────────────────────────────────────
	r.preResolved = make(map[string]map[string]string, len(sorted))
	if len(r.Config.Providers) > 0 {
		r.Log.Separator()
		r.Log.Header("Preflight: resolving providers for all environments")
		r.Log.Info(fmt.Sprintf("environments: %s", strings.Join(sorted, ", ")))
		r.Log.Separator()

		for _, envName := range sorted {
			r.Log.Step(fmt.Sprintf("Resolving %q", envName))
			vars, resErr := r.Provider.Resolve(ctx, r.Config.Providers, r.Config.Connections, r.Connections, envName)
			if resErr != nil {
				return fmt.Errorf("preflight: env %q: %w", envName, resErr)
			}
			r.preResolved[envName] = vars
		}
		r.Log.Separator()
	}

	// ── Phase 2: execute ───────────────────────────────────────────
	for _, envName := range sorted {
		if err := r.Run(ctx, envName, opts); err != nil {
			return err
		}
	}
	return nil
}

// topoSortEnvs returns the environments in dependency order (promotesFrom first).
// Only considers edges between the requested environments.
func (r *Runner) topoSortEnvs(envNames []string) ([]string, error) {
	if len(envNames) <= 1 {
		return envNames, nil
	}

	// Validate all environments exist.
	requested := make(map[string]bool, len(envNames))
	for _, name := range envNames {
		if _, ok := r.Config.Environments[name]; !ok {
			return nil, &PipelineError{
				Phase:   "run",
				Summary: fmt.Sprintf("Environment %q not found", name),
				Details: []string{fmt.Sprintf("available environments: %s", strings.Join(r.Config.EnvironmentNames(), ", "))},
				Hint:    "check the --env flag value",
			}
		}
		requested[name] = true
	}

	// Build in-degree map and adjacency list (only edges within requested set).
	inDegree := make(map[string]int, len(envNames))
	dependents := make(map[string][]string)
	for _, name := range envNames {
		inDegree[name] = 0
	}
	for _, name := range envNames {
		env := r.Config.Environments[name]
		if env.PromotesFrom != "" && requested[env.PromotesFrom] {
			inDegree[name]++
			dependents[env.PromotesFrom] = append(dependents[env.PromotesFrom], name)
		}
	}

	// Kahn's algorithm.
	var queue []string
	for _, name := range envNames {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)
		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(sorted) != len(envNames) {
		return nil, &PipelineError{
			Phase:   "run",
			Summary: "Circular dependency between requested environments",
			Details: []string{fmt.Sprintf("requested: %s", strings.Join(envNames, ", "))},
			Hint:    "check the promotesFrom chain for cycles",
		}
	}

	return sorted, nil
}

// Run executes the pipeline for a single environment.
func (r *Runner) Run(ctx context.Context, envName string, opts RunOptions) error {
	r.Unsafe = opts.Unsafe

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
	if err := r.resolveProviders(ctx, envName, opts); err != nil {
		r.printFailure(start, err)
		return err
	}

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

	if opts.Undeploy {
		if err := r.runUndeployPhase(ctx, envName, env); err != nil {
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
// The artifact workdir is template-evaluated to support {{ .Env.XXX }} references.
// Returns an error if the resolved path escapes the base workdir (path traversal),
// unless --unsafe is set.
func (r *Runner) resolveWorkdir(art *Artifact) (string, error) {
	artWorkdir := r.Executor.EvalString(art.EffectiveWorkdir())
	resolved := filepath.Join(r.Workdir, artWorkdir)

	if r.Unsafe {
		return resolved, nil
	}

	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot resolve workdir: %w", err)
	}
	absBase, err := filepath.Abs(r.Workdir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve base workdir: %w", err)
	}

	if !strings.HasPrefix(absResolved+string(filepath.Separator), absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("workdir %q escapes base directory %q — use --unsafe to allow this", artWorkdir, r.Workdir)
	}

	return resolved, nil
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
	if opts.Undeploy {
		phases = append(phases, "undeploy")
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

func (r *Runner) resolveProviders(ctx context.Context, envName string, opts RunOptions) error {
	r.Log.Step("Resolving providers")

	var (
		vars map[string]string
		err  error
	)
	// Use the preflight cache if RunAll populated it; falls back to live resolution
	// when Run is invoked directly (e.g. single-env CLI call).
	if cached, ok := r.preResolved[envName]; ok {
		vars = make(map[string]string, len(cached))
		for k, v := range cached {
			vars[k] = v
		}
		r.Log.Info(fmt.Sprintf("using preflight-resolved variables (%d)", len(vars)))
	} else if len(r.Config.Providers) > 0 {
		vars, err = r.Provider.Resolve(ctx, r.Config.Providers, r.Config.Connections, r.Connections, envName)
		if err != nil {
			return err
		}
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

	// Evaluate templates in resolved values (handles defaults with {{ .Env.XXX }}).
	r.Executor.EvalEnvValues()

	r.Log.Separator()
	return nil
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

// runArtifact executes the artifact lifecycle.
//
// Build strategy (full lifecycle):
//
//	preRun → preBuild → build → postBuild → preDeploy → deploy → postDeploy → postRun
//
// Promote strategy (skip build, run deploy):
//
//	preRun → preDeploy → deploy → postDeploy → postRun
func (r *Runner) runArtifact(ctx context.Context, name string, art *Artifact, env *Environment, opts RunOptions) error {
	strategy := "build"
	if env.IsPromote() {
		strategy = "promote"
	}
	r.Log.Step(fmt.Sprintf("%s (%s) [%s]", name, art.Type, strategy))

	workdir, err := r.resolveWorkdir(art)
	if err != nil {
		return err
	}
	r.Log.Info(fmt.Sprintf("workdir: %s", workdir))

	// 1. preRun
	if err := r.runHooks(ctx, r.envName, name+" pre-run", art.PreRun); err != nil {
		return err
	}

	// 2–4. Build phase (skipped for promote).
	if env.IsBuild() {
		if err := r.runHooks(ctx, r.envName, name+" pre-build", art.PreBuild); err != nil {
			return err
		}
		if err := r.runBuild(ctx, art, opts, workdir); err != nil {
			return err
		}
		if err := r.runHooks(ctx, r.envName, name+" post-build", art.PostBuild); err != nil {
			return err
		}
	} else {
		r.Log.Promote(fmt.Sprintf("promoting %s from %s (build skipped)", name, env.PromotesFrom))
	}

	// 5. preDeploy
	if err := r.runHooks(ctx, r.envName, name+" pre-deploy", art.PreDeploy); err != nil {
		return err
	}

	// 6. deploy (build) or promote (promote) — each has its own preRun/postRun
	if env.IsBuild() && art.Deploy != nil {
		if err := r.runDeploy(ctx, name, art.Deploy, workdir); err != nil {
			return err
		}
	}
	if env.IsPromote() && art.Promote != nil {
		if err := r.runDeploy(ctx, name, art.Promote, workdir); err != nil {
			return err
		}
	}

	// 7. postDeploy
	if err := r.runHooks(ctx, r.envName, name+" post-deploy", art.PostDeploy); err != nil {
		return err
	}

	// 8. postRun
	if err := r.runHooks(ctx, r.envName, name+" post-run", art.PostRun); err != nil {
		return err
	}

	r.Log.Separator()
	return nil
}

func (r *Runner) runBuild(ctx context.Context, art *Artifact, opts RunOptions, workdir string) error {
	switch {
	case art.IsDocker():
		args := r.Commands.DockerBuild(art, opts.BuildNumber)
		return r.Executor.Run(ctx, args, workdir)
	case art.IsBinary(), art.IsPackage():
		return r.runBuildSteps(ctx, art, workdir)
	default:
		r.Log.Info(fmt.Sprintf("unknown artifact type: %s", art.Type))
		return nil
	}
}

func (r *Runner) runBuildSteps(ctx context.Context, art *Artifact, workdir string) error {
	steps := filterSteps(art.Build, r.envName)
	for i, s := range steps {
		r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(steps), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
		if err := r.Executor.RunWithConnection(ctx, r.Commands.BuildStepCommand(&s), workdir, s.Connection); err != nil {
			return err
		}
	}
	return nil
}

// ── Deploy ──────────────────────────────────────────

// runDeploy executes a deploy (helm, script). Used by both artifacts and environments.
func (r *Runner) runDeploy(ctx context.Context, label string, d *Deploy, workdir string) error {
	if err := r.runHooks(ctx, r.envName, label+" deploy pre-run", d.PreRun); err != nil {
		return err
	}

	r.Log.Step(fmt.Sprintf("deploy --> %s (%s)", label, d.Type))

	var err error
	switch d.Type {
	case "helm":
		args := r.Commands.HelmDeploy(label, d)
		err = r.Executor.RunWithConnection(ctx, args, workdir, d.Connection)
	case "script":
		steps := filterSteps(d.Steps, r.envName)
		for i, s := range steps {
			r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(steps), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
			conn := s.Connection
			if conn == "" {
				conn = d.Connection // inherit from deploy block
			}
			if err := r.Executor.RunWithConnection(ctx, r.Commands.BuildStepCommand(&s), workdir, conn); err != nil {
				return err
			}
		}
	case "filesystem":
		src := r.Executor.EvalString(d.Source)
		dst := r.Executor.EvalString(d.Destination)
		r.Log.Info(fmt.Sprintf("  copy %s → %s", src, dst))
		copyArgs := []string{"cp", "-r", src, dst}
		err = r.Executor.Run(ctx, copyArgs, workdir)
	default:
		r.Log.Info(fmt.Sprintf("unknown deploy type: %s", d.Type))
	}

	if err != nil {
		return err
	}

	r.Log.Separator()

	if err := r.runHooks(ctx, r.envName, label+" deploy post-run", d.PostRun); err != nil {
		return err
	}

	return nil
}

// runDeployPhase runs the environment-level deploy.
func (r *Runner) runDeployPhase(ctx context.Context, envName string, env *Environment) error {
	if env.Deploy == nil {
		return nil
	}
	return r.runDeploy(ctx, envName, env.Deploy, r.Workdir)
}

// ── Undeploy ────────────────────────────────────────

// runUndeploy is the mirror of runDeploy. For helm it runs "uninstall" instead
// of "upgrade --install". For script/filesystem it runs the user-defined steps.
func (r *Runner) runUndeploy(ctx context.Context, label string, d *Deploy, workdir string) error {
	if err := r.runHooks(ctx, r.envName, label+" undeploy pre-run", d.PreRun); err != nil {
		return err
	}

	r.Log.Step(fmt.Sprintf("undeploy --> %s (%s)", label, d.Type))

	var err error
	switch d.Type {
	case "helm":
		args := r.Commands.HelmUninstall(label, d)
		err = r.Executor.RunWithConnection(ctx, args, workdir, d.Connection)
	case "script":
		steps := filterSteps(d.Steps, r.envName)
		for i, s := range steps {
			r.Log.Info(fmt.Sprintf("[%d/%d] %s", i+1, len(steps), r.Commands.FormatCommand(r.Commands.BuildStepCommand(&s))))
			conn := s.Connection
			if conn == "" {
				conn = d.Connection
			}
			if err := r.Executor.RunWithConnection(ctx, r.Commands.BuildStepCommand(&s), workdir, conn); err != nil {
				return err
			}
		}
	case "filesystem":
		dst := r.Executor.EvalString(d.Destination)
		r.Log.Info(fmt.Sprintf("  rm -rf %s", dst))
		rmArgs := []string{"rm", "-rf", dst}
		err = r.Executor.Run(ctx, rmArgs, workdir)
	default:
		r.Log.Info(fmt.Sprintf("unknown undeploy type: %s", d.Type))
	}

	if err != nil {
		return err
	}

	r.Log.Separator()

	if err := r.runHooks(ctx, r.envName, label+" undeploy post-run", d.PostRun); err != nil {
		return err
	}

	return nil
}

// runUndeployPhase runs undeploy for artifacts first (cleanup), then environment-level.
// This is the inverse order of deploy: artifacts are torn down before the environment.
func (r *Runner) runUndeployPhase(ctx context.Context, envName string, env *Environment) error {
	// 1. Artifact-level undeploy (cleanup ECR repos, etc.)
	for name, art := range r.Config.Artifacts {
		if art.Undeploy == nil {
			continue
		}
		if len(env.Artifacts) > 0 && !contains(env.Artifacts, name) {
			continue
		}
		workdir, err := r.resolveWorkdir(art)
		if err != nil {
			return err
		}
		if err := r.runUndeploy(ctx, name, art.Undeploy, workdir); err != nil {
			return err
		}
	}

	// 2. Environment-level undeploy (helm uninstall, etc.)
	if env.Undeploy != nil {
		if err := r.runUndeploy(ctx, envName, env.Undeploy, r.Workdir); err != nil {
			return err
		}
	}

	return nil
}

func contains(items []string, target string) bool {
	for _, s := range items {
		if s == target {
			return true
		}
	}
	return false
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
		if err := r.Executor.RunWithConnection(ctx, args, r.Workdir, s.Connection); err != nil {
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
