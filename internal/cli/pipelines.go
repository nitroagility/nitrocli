// Package cli implements the NitroCLI command tree.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

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
		Short:         "Run a pipeline for the specified environment",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pipelineFile, _ := cmd.Flags().GetString("pipeline")
			envName, _ := cmd.Flags().GetString("env")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			doBuild, _ := cmd.Flags().GetBool("build")
			doDeploy, _ := cmd.Flags().GetBool("deploy")
			buildNumber, _ := cmd.Flags().GetString("build-number")

			if envName == "" {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "Missing required flag --env",
					Details: []string{"the --env / -e flag is required to specify the target environment"},
					Hint:    "usage: nitro pipelines run --env <environment>",
				})
			}

			if !doBuild && !doDeploy {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "No phase selected",
					Details: []string{"at least one of --build or --deploy must be specified"},
					Hint:    "usage: nitro pipelines run --env <environment> --build --deploy",
				})
			}

			if doBuild && buildNumber == "" {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "Missing required flag --build-number",
					Details: []string{"--build-number is required when --build is specified"},
					Hint:    "usage: nitro pipelines run --env <environment> --build --build-number <number>",
				})
			}

			workdir, err := resolveWorkdir(cmd)
			if err != nil {
				return pipelineError(err)
			}

			// Resolve pipeline file relative to workdir.
			if !filepath.IsAbs(pipelineFile) {
				pipelineFile = filepath.Join(workdir, pipelineFile)
			}

			cfg, err := pipelines.Load(cmd.Context(), pipelineFile, Version)
			if err != nil {
				return pipelineError(err)
			}

			opts := pipelines.RunOptions{
				Build:       doBuild,
				Deploy:      doDeploy,
				BuildNumber: buildNumber,
			}
			runner := pipelines.NewRunner(cfg, dryRun, workdir)
			if err := runner.Run(cmd.Context(), envName, opts); err != nil {
				return pipelineError(err)
			}

			return nil
		},
	}

	runCmd.Flags().StringP("pipeline", "p", "nitro-pipeline.cue", "path to the pipeline CUE file")
	runCmd.Flags().StringP("env", "e", "", "target environment")
	runCmd.Flags().BoolP("dry-run", "n", false, "print commands without executing")
	runCmd.Flags().Bool("build", false, "run the build phase")
	runCmd.Flags().Bool("deploy", false, "run the deploy phase")
	runCmd.Flags().String("build-number", "", "build number used for tagging artifacts")

	cmd.AddCommand(runCmd)
	return cmd
}

func init() {
	rootCmd.AddCommand(newPipelinesCmd())
}
