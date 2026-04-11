// Package cli implements the NitroCLI command tree.
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/nitroagility/nitrocli/internal/pipelines"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00E5FF")).
			Bold(true)

	cliStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00E5FF")).
			Bold(true)

	taglineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B0BEC5"))

	copyrightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#546E7A"))
)

const (
	logoArt  = `   ███╗   ██╗ ██╗ ████████╗ ██████╗   ██████╗ `
	cliArt0  = `  ██████╗ ██╗      ██╗`
	logoArt1 = `   ████╗  ██║ ██║ ╚══██╔══╝ ██╔══██╗ ██╔═══██╗`
	cliArt1  = ` ██╔════╝ ██║      ██║`
	logoArt2 = `   ██╔██╗ ██║ ██║    ██║    ██████╔╝ ██║   ██║`
	cliArt2  = ` ██║      ██║      ██║`
	logoArt3 = `   ██║╚██╗██║ ██║    ██║    ██╔══██╗ ██║   ██║`
	cliArt3  = ` ██║      ██║      ██║`
	logoArt4 = `   ██║ ╚████║ ██║    ██║    ██║  ██║ ╚██████╔╝`
	cliArt4  = ` ╚██████╗ ███████╗ ██║`
	logoArt5 = `   ╚═╝  ╚═══╝ ╚═╝    ╚═╝    ╚═╝  ╚═╝  ╚═════╝ `
	cliArt5  = ` ╚═════╝ ╚══════╝ ╚═╝`
)

func buildBanner() string {
	lines := []struct{ logo, cli string }{
		{logoArt, cliArt0},
		{logoArt1, cliArt1},
		{logoArt2, cliArt2},
		{logoArt3, cliArt3},
		{logoArt4, cliArt4},
		{logoArt5, cliArt5},
	}

	var b strings.Builder
	b.WriteString("\n")
	for _, l := range lines {
		b.WriteString(logoStyle.Render(l.logo))
		b.WriteString(cliStyle.Render(l.cli))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(taglineStyle.Render("   Fast. Minimal. Powerful."))
	b.WriteString("\n")
	b.WriteString(descStyle.Render("   The official CLI for NitroAgility"))
	b.WriteString("\n")
	b.WriteString(copyrightStyle.Render("   Copyright (c) NitroAgility - nitroagility.com"))
	b.WriteString("\n")
	return b.String()
}

var rootCmd = &cobra.Command{
	Use:           "nitro",
	Short:         "NitroCLI - The official CLI for NitroAgility",
	Long:          buildBanner(),
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringP("workdir", "w", "", "working directory (default: current directory)")
}

func initConfig() {
	viper.SetEnvPrefix("NITRO")
	viper.AutomaticEnv()
}

// alreadyPrinted wraps an error that was already printed to stderr.
type alreadyPrinted struct{ err error }

func (a *alreadyPrinted) Error() string { return a.err.Error() }
func (a *alreadyPrinted) Unwrap() error { return a.err }

// Execute runs the root command.
// Any error from cobra (unknown flags, missing args, etc.) is printed styled.
func Execute() error {
	err := rootCmd.Execute()
	if err != nil {
		var ap *alreadyPrinted
		if !errors.As(err, &ap) {
			fmt.Fprint(os.Stderr, pipelines.FormatError(err))
		}
	}
	return err
}
