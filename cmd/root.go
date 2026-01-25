/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/ptone/scion-agent/pkg/util"
	"github.com/spf13/cobra"
)

var (
	grovePath    string
	globalMode   bool
	profile      string
	outputFormat string
	hubEndpoint  string // Hub API endpoint override
	noHub        bool   // Disable Hub integration for this invocation
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "scion",
	Short: "A container-based orchestration tool for managing concurrent LLM agents",
	Long: `Scion is a container-based orchestration tool for managing 
concurrent LLM agents. It enables parallel execution of specialized 
sub-agents with isolated identities, credentials, and workspaces.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if globalMode && grovePath == "" {
			grovePath = "global"
		}

		if util.IsGitRepo() {
			if err := util.CheckGitVersion(); err != nil {
				return fmt.Errorf("git check failed: %w", err)
			}
		}

		if outputFormat != "" {
			if outputFormat != "json" && outputFormat != "plain" {
				return fmt.Errorf("invalid format: %s (allowed: json, plain)", outputFormat)
			}
			if cmd != listCmd {
				// TODO: support format for other commands
				return fmt.Errorf("format flag is not yet supported for command %s", cmd.Name())
			}
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s%sError: %v%s\n\n", util.BgRed, util.Black, err, util.Reset)
		if cmd != nil {
			cmd.Usage()
		}
		os.Exit(1)
	}
}

func init() {
	rootCmd.Long = util.GetBanner() + "\n" + rootCmd.Long
	rootCmd.PersistentFlags().StringVarP(&grovePath, "grove", "g", "", "Path to a .scion grove directory")
	rootCmd.PersistentFlags().BoolVar(&globalMode, "global", false, "Use the global grove (equivalent to --grove global)")
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "", "Configuration profile to use")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "format", "", "Output format (e.g., json)")

	// Hub integration flags
	rootCmd.PersistentFlags().StringVar(&hubEndpoint, "hub", "", "Hub API endpoint URL (overrides SCION_HUB_ENDPOINT)")
	rootCmd.PersistentFlags().BoolVar(&noHub, "no-hub", false, "Disable Hub integration for this invocation (local-only mode)")
}

// GetHubEndpoint returns the effective Hub endpoint based on flags and settings.
// Returns empty string if Hub is disabled or not configured.
func GetHubEndpoint(settings interface{ GetHubEndpoint() string }) string {
	if noHub {
		return ""
	}
	if hubEndpoint != "" {
		return hubEndpoint
	}
	if settings != nil {
		return settings.GetHubEndpoint()
	}
	return ""
}

// IsHubEnabled returns true if Hub integration is enabled for this invocation.
func IsHubEnabled() bool {
	return !noHub
}
