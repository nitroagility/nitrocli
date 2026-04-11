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
	return err
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

			if envName == "" {
				return pipelineError(&pipelines.PipelineError{
					Phase:   "args",
					Summary: "Missing required flag --env",
					Details: []string{"the --env / -e flag is required to specify the target environment"},
					Hint:    "usage: nitro pipelines run --env <environment>",
				})
			}

			workdir, err := resolveWorkdir(cmd)
			if err != nil {
				return pipelineError(err)
			}

			cfg, err := pipelines.Load(cmd.Context(), pipelineFile)
			if err != nil {
				return pipelineError(err)
			}

			runner := pipelines.NewRunner(cfg, dryRun, workdir)
			if err := runner.Run(cmd.Context(), envName); err != nil {
				return pipelineError(err)
			}

			return nil
		},
	}

	runCmd.Flags().StringP("pipeline", "p", "nitro-pipeline.cue", "path to the pipeline CUE file")
	runCmd.Flags().StringP("env", "e", "", "target environment")
	runCmd.Flags().BoolP("dry-run", "n", false, "print commands without executing")

	cmd.AddCommand(runCmd)
	return cmd
}

func init() {
	rootCmd.AddCommand(newPipelinesCmd())
}
