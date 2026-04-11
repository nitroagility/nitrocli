package pipelines

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Executor runs shell commands with a shared environment session.
// All output is masked to prevent secret leaks.
type Executor struct {
	DryRun  bool
	Log     *Logger
	Masker  *Masker
	envVars map[string]string
}

// SetEnv sets the shared environment variables for all subsequent commands.
func (e *Executor) SetEnv(vars map[string]string) {
	e.envVars = vars
}

// Run executes a command with the shared environment.
// In dry-run mode it only prints the command.
// All stdout/stderr output is masked before being written to the terminal.
func (e *Executor) Run(ctx context.Context, args []string, workdir string) error {
	cmdStr := strings.Join(args, " ")

	e.Log.Command(cmdStr, e.DryRun)

	if e.DryRun {
		return nil
	}

	if len(args) == 0 {
		return nil
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Env = e.buildEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Mask and write stdout.
	if stdout.Len() > 0 {
		masked := e.maskOutput(stdout.String())
		fmt.Fprint(os.Stdout, masked)
	}

	// Mask and write stderr.
	if stderr.Len() > 0 {
		masked := e.maskOutput(stderr.String())
		fmt.Fprint(os.Stderr, masked)
	}

	if err != nil {
		return fmt.Errorf("command failed: %s: %w", e.maskOutput(cmdStr), err)
	}

	return nil
}

func (e *Executor) maskOutput(s string) string {
	if e.Masker != nil {
		return e.Masker.Mask(s)
	}
	return s
}

// buildEnv merges the current OS environment with the shared session variables.
// Session variables take precedence over OS env.
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
