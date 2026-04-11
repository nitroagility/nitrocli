// Package cli implements the NitroCLI command tree.
package cli

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Build information variables injected via ldflags.
var (
	Version   string
	BuildTime string
	GitCommit string
)

var (
	vLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#546E7A"))
	vValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#00E5FF")).Bold(true)
	vDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#546E7A"))
)

func printVersion() {
	v := Version
	if v == "" {
		v = "0.0.0-dev"
	}
	bt := BuildTime
	if bt == "" {
		bt = "unknown"
	}
	gc := GitCommit
	if gc == "" {
		gc = "unknown"
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", vLabel.Render("version"), vValue.Render(v))
	fmt.Printf("  %s  %s\n", vLabel.Render("commit "), vDim.Render(gc))
	fmt.Printf("  %s   %s\n", vLabel.Render("built  "), vDim.Render(bt))
	fmt.Println()
}

func init() {
	rootCmd.Flags().BoolP("version", "v", false, "show version")

	defaultRun := rootCmd.RunE
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		showVersion, _ := cmd.Flags().GetBool("version")
		if showVersion {
			printVersion()
			return nil
		}
		if defaultRun != nil {
			return defaultRun(cmd, args)
		}
		return cmd.Help()
	}
}
