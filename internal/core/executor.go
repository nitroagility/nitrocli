package core

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
	DryRun       bool
	Log          *Logger
	Masker       *Masker
	ConnResolver *ConnectionResolver
	envVars      map[string]string
	template     *TemplateEngine
}

// SetEnv sets the shared environment variables and initializes the template engine.
func (e *Executor) SetEnv(vars map[string]string) {
	e.envVars = vars
	e.template = NewTemplateEngine(vars)
}

// EvalEnvValues evaluates templates in all resolved variable values.
// This handles defaults containing {{ .Env.XXX }} references.
func (e *Executor) EvalEnvValues() {
	if e.template == nil {
		return
	}
	for k, v := range e.envVars {
		resolved, err := e.template.Eval(v)
		if err == nil && resolved != v {
			e.envVars[k] = resolved
		}
	}
	// Rebuild template engine with resolved values.
	e.template = NewTemplateEngine(e.envVars)
}

// EvalString evaluates a single string through the template engine.
func (e *Executor) EvalString(s string) string {
	if e.template == nil {
		return s
	}
	resolved, err := e.template.Eval(s)
	if err != nil {
		return s
	}
	return resolved
}

// Run evaluates templates in args, then executes the command.
// In dry-run mode it only prints the resolved command.
// All stdout/stderr output is masked before being written to the terminal.
func (e *Executor) Run(ctx context.Context, args []string, workdir string) error {
	return e.RunWithConnection(ctx, args, workdir, "")
}

// RunWithConnection is like Run but injects a specific connection's credentials
// into the command environment. When connName is empty, the standard env is used.
func (e *Executor) RunWithConnection(ctx context.Context, args []string, workdir, connName string) error {
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
	cmd.Env = e.buildEnvWithConnection(connName)

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

// buildEnvWithConnection returns the command env, overlaying a specific
// connection's credentials on top when connName is non-empty. This scopes
// the override to a single command execution (e.g., helm deploy with an
// EKS-specific role) without mutating the shared env vars.
func (e *Executor) buildEnvWithConnection(connName string) []string {
	base := e.buildEnv()
	if connName == "" || e.ConnResolver == nil {
		return base
	}
	connVars := e.ConnResolver.ConnectionVars(connName)
	if len(connVars) == 0 {
		return base
	}

	overridden := make(map[string]bool, len(connVars))
	for k := range connVars {
		overridden[k] = true
	}

	result := make([]string, 0, len(base)+len(connVars))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if !overridden[key] {
			result = append(result, entry)
		}
	}
	for k, v := range connVars {
		result = append(result, k+"="+v)
	}
	return result
}
