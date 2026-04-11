// Package cli implements the NitroCLI command tree.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Build information variables injected via ldflags.
var (
	Version   string
	BuildTime string
	GitCommit string
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the version details",
		Long:  "This command shows the version details.",
		Run: func(_ *cobra.Command, _ []string) {
			v := Version
			if v == "" {
				v = "dev"
			}
			bt := BuildTime
			if bt == "" {
				bt = "unknown"
			}
			gc := GitCommit
			if gc == "" {
				gc = "unknown"
			}
			fmt.Printf("version:    %s\n", v)
			fmt.Printf("build_time: %s\n", bt)
			fmt.Printf("git_commit: %s\n", gc)
		},
	}
}

func init() {
	rootCmd.AddCommand(newVersionCmd())
}
