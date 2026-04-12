package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/nitroagility/nitrocli/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgKey   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00E5FF")).Bold(true)
	cfgVal   = lipgloss.NewStyle().Foreground(lipgloss.Color("#B0BEC5"))
	cfgOk    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00E676")).Bold(true)
	cfgWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC107"))
	cfgMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#546E7A"))
	cfgTag   = lipgloss.NewStyle().Foreground(lipgloss.Color("#546E7A"))
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage local configuration (~/.nitro/config.json)",
	}

	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigDeleteCmd())
	cmd.AddCommand(newConfigListCmd())

	return cmd
}

func newConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "set <key> <value>",
		Short:         "Set a configuration value",
		Args:          cobra.ExactArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			secret, _ := cmd.Flags().GetBool("secret")
			if err := config.Set(key, value, secret); err != nil {
				return fmt.Errorf("failed to set %s: %w", key, err)
			}
			tag := "config"
			if secret {
				tag = "secret"
			}
			fmt.Fprintf(os.Stdout, "  %s %s %s\n", cfgOk.Render("set"), cfgKey.Render(key), cfgTag.Render("("+tag+")"))
			return nil
		},
	}

	cmd.Flags().Bool("secret", false, "mark the value as a secret (masked in list output)")

	return cmd
}

func newConfigGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "get <key>",
		Short:         "Get a configuration value (secrets are masked unless --plain is used)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			plain, _ := cmd.Flags().GetBool("plain")
			entry, ok, err := config.Get(key)
			if err != nil {
				return fmt.Errorf("failed to get %s: %w", key, err)
			}
			if !ok {
				fmt.Fprintf(os.Stderr, "  %s %s\n", cfgWarn.Render("not found:"), cfgKey.Render(key))
				os.Exit(1)
			}
			if entry.Secret && !plain {
				fmt.Fprintln(os.Stdout, maskValue(entry.Value))
			} else {
				fmt.Fprintln(os.Stdout, entry.Value)
			}
			return nil
		},
	}

	cmd.Flags().Bool("plain", false, "show the actual value even for secrets")

	return cmd
}

func newConfigDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "delete <key>",
		Short:         "Delete a configuration value",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			key := args[0]
			if err := config.Delete(key); err != nil {
				return fmt.Errorf("failed to delete %s: %w", key, err)
			}
			fmt.Fprintf(os.Stdout, "  %s %s\n", cfgOk.Render("deleted"), cfgKey.Render(key))
			return nil
		},
	}
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "List all configuration values",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			entries, err := config.List()
			if err != nil {
				return fmt.Errorf("failed to list config: %w", err)
			}
			if len(entries) == 0 {
				fmt.Fprintf(os.Stdout, "  %s\n", cfgMuted.Render("no configuration values set"))
				return nil
			}

			for _, e := range entries {
				display := e.Value
				if e.Secret {
					display = maskValue(e.Value)
				}
				tag := "config"
				if e.Secret {
					tag = "secret"
				}
				fmt.Fprintf(os.Stdout, "  %s = %s %s\n",
					cfgKey.Render(e.Key),
					cfgVal.Render(display),
					cfgTag.Render("("+tag+")"),
				)
			}
			return nil
		},
	}
}

func maskValue(_ string) string {
	return "********"
}

func init() {
	rootCmd.AddCommand(newConfigCmd())
}
