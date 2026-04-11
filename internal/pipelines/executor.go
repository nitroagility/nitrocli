package pipelines

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Executor runs shell commands with a shared environment session.
// All command args are evaluated as Go templates before execution.
// All output is masked to prevent secret leaks.
type Executor struct {
	DryRun   bool
	Log      *Logger
	Masker   *Masker
	envVars  map[string]string
	template *TemplateEngine
}

// SetEnv sets the shared environment variables and initializes the template engine.
func (e *Executor) SetEnv(vars map[string]string) {
	e.envVars = vars
	e.template = NewTemplateEngine(vars)
}

// Run evaluates templates in args, then executes the command.
// In dry-run mode it only prints the resolved command.
// All stdout/stderr output is masked before being written to the terminal.
func (e *Executor) Run(ctx context.Context, args []string, workdir string) error {
	resolved, err := e.resolveArgs(args)
	if err != nil {
		return err
	}

	cmdStr := strings.Join(resolved, " ")

	e.Log.Command(cmdStr, e.DryRun)

	if e.DryRun {
		return nil
	}

	if len(resolved) == 0 {
		return nil
	}

	cmd := exec.CommandContext(ctx, resolved[0], resolved[1:]...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Env = e.buildEnv()

	cmd.Stdout = newMaskedWriter(os.Stdout, e.Masker)
	cmd.Stderr = newMaskedWriter(os.Stderr, e.Masker)

	execErr := cmd.Run()

	if execErr != nil {
		return fmt.Errorf("command failed: %s: %w", e.maskOutput(cmdStr), execErr)
	}

	return nil
}

func (e *Executor) resolveArgs(args []string) ([]string, error) {
	if e.template == nil {
		return args, nil
	}
	return e.template.EvalArgs(args)
}

func (e *Executor) maskOutput(s string) string {
	if e.Masker != nil {
		return e.Masker.Mask(s)
	}
	return s
}

func (e *Executor) buildEnv() []string {
	base := os.Environ()

	if len(e.envVars) == 0 {
		return base
	}

	env := make([]string, 0, len(base)+len(e.envVars))

	overridden := make(map[string]bool, len(e.envVars))
	for k := range e.envVars {
		overridden[k] = true
	}

	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if !overridden[key] {
			env = append(env, entry)
		}
	}

	for k, v := range e.envVars {
		env = append(env, k+"="+v)
	}

	return env
}
