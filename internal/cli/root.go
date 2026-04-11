// Package cli implements the NitroCLI command tree.
package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	Use:   "nitro",
	Short: "NitroCLI - The official CLI for NitroAgility",
	Long:  buildBanner(),
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	viper.SetEnvPrefix("NITRO")
	viper.AutomaticEnv()
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
