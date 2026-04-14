// Package cli implements the NitroCLI command tree.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nitroagility/nitrocli/internal/pipelines"
	"github.com/spf13/cobra"
)

func resolveWorkdir(cmd *cobra.Command) (string, error) {
	workdir, _ := cmd.Flags().GetString("workdir")
	if workdir == "" {
		return os.Getwd()
	}
	return filepath.Abs(workdir)
}

func pipelineError(err error) error {
	fmt.Fprint(os.Stderr, pipelines.FormatError(err))
	return &alreadyPrinted{err: err}
}

func newPipelinesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipelines",
		Short: "Manage and run pipelines",
	}

	runCmd := &cobra.Command{
		Use:           "run",
		Short:         "Run a pipeline for the specified environment(s)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pipelineFile, _ := cmd.Flags().GetString("pipeline")
			envNames, _ := cmd.Flags().GetStringArray("env")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			doBuild, _ := cmd.Flags().GetBool("build")
			doDeploy, _ := cmd.Flags().GetBool("deploy")
			doUndeploy, _ := cmd.Flags().GetBool("undeploy")
			buildNumber, _ := cmd.Flags().GetString("build-number")
			unsafe, _ := cmd.Flags().GetBool("unsafe")
			globalFlag, _ := cmd.Flags().GetString("global")

			if len(envNames) == 0 {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "Missing required flag --env",
					Details: []string{"at least one --env / -e flag is required"},
					Hint:    "usage: nitro pipelines run -e dev -e uat --build --build-number 1",
				})
			}

			if doUndeploy && (doBuild || doDeploy) {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "Conflicting phases",
					Details: []string{"--undeploy cannot be combined with --build or --deploy"},
					Hint:    "usage: nitro pipelines run -e prod --undeploy",
				})
			}

			if !doBuild && !doDeploy && !doUndeploy {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "No phase selected",
					Details: []string{"at least one of --build, --deploy, or --undeploy must be specified"},
					Hint:    "usage: nitro pipelines run -e dev --build --deploy",
				})
			}

			if doBuild && buildNumber == "" {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "Missing required flag --build-number",
					Details: []string{"--build-number is required when --build is specified"},
					Hint:    "usage: nitro pipelines run -e dev --build --build-number 1",
				})
			}

			workdir, err := resolveWorkdir(cmd)
			if err != nil {
				return pipelineError(err)
			}

			if !filepath.IsAbs(pipelineFile) {
				pipelineFile = filepath.Join(workdir, pipelineFile)
			}

			cfg, err := pipelines.Load(cmd.Context(), pipelineFile, Version)
			if err != nil {
				return pipelineError(err)
			}

			if globalFlag != "" {
				for _, g := range strings.Split(globalFlag, ",") {
					g = strings.TrimSpace(g)
					if g != "" {
						cfg.Globals = append(cfg.Globals, g)
					}
				}
			}

			opts := pipelines.RunOptions{
				Build:       doBuild,
				Deploy:      doDeploy,
				Undeploy:    doUndeploy,
				BuildNumber: buildNumber,
				Unsafe:      unsafe,
			}
			runner := pipelines.NewRunner(cfg, dryRun, workdir)
			if err := runner.RunAll(cmd.Context(), envNames, opts); err != nil {
				return pipelineError(err)
			}

			return nil
		},
	}

	runCmd.Flags().StringP("pipeline", "p", "nitro-pipeline.cue", "path to the pipeline CUE file")
	runCmd.Flags().StringArrayP("env", "e", nil, "target environment (repeatable, executed in dependency order)")
	runCmd.Flags().BoolP("dry-run", "n", false, "print commands without executing")
	runCmd.Flags().Bool("build", false, "run the build phase")
	runCmd.Flags().Bool("deploy", false, "run the deploy phase")
	runCmd.Flags().Bool("undeploy", false, "run the undeploy phase (tears down deploy: helm uninstall, cleanup scripts)")
	runCmd.Flags().String("build-number", "", "build number used for tagging artifacts")
	runCmd.Flags().Bool("unsafe", false, "allow workdir paths outside the base directory (disables path traversal protection)")
	runCmd.Flags().String("global", "", "comma-separated list of variable names to import from ~/.nitro/config.json (additive to pipeline globals)")

	cmd.AddCommand(runCmd)
	return cmd
}

func init() {
	rootCmd.AddCommand(newPipelinesCmd())
}
