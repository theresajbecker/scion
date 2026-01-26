package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/harness"
	"github.com/ptone/scion-agent/pkg/util"
	"github.com/spf13/cobra"
)

var globalInit bool

// groveCmd represents the grove command
var groveCmd = &cobra.Command{
	Use:     "grove",
	Aliases: []string{"group"},
	Short:   "Manage scion groves (agent groups)",
	Long:    `A grove is the grouping construct for a set of agents. The .scion folder represents a grove.`,
}

// groveInitCmd represents the init subcommand for grove
var groveInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new grove",
	Long: `Initialize a new grove by creating the .scion directory structure
and seeding the default template. 

By default, it initializes in:
- The root of the current git repo if run inside a repo
- The current directory

With --global, it initializes in the user's home folder.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		harnesses := harness.All()

		if globalInit {
			fmt.Println("Initializing global scion directory...")
			if err := config.InitGlobal(harnesses); err != nil {
				return fmt.Errorf("failed to initialize global config: %w", err)
			}
			fmt.Println("scion grove successfully initialized.")
			return nil
		}

		// Check for nested grove - error if already inside a scion project
		if _, rootDir, found := config.GetEnclosingGrovePath(); found {
			wd, _ := os.Getwd()
			if filepath.Clean(wd) == filepath.Clean(rootDir) {
				return fmt.Errorf("already inside a scion project at %s. skipping re-initialization", rootDir)
			}
			return fmt.Errorf("already inside a scion project at %s. Nested groves are not supported", rootDir)
		}

		// Determine target directory
		targetDir, err := config.GetTargetProjectDir()
		if err != nil {
			return fmt.Errorf("failed to determine project directory: %w", err)
		}

		// Check if we're in a subdirectory of a git repo
		wd, _ := os.Getwd()
		if util.IsGitRepo() {
			repoRoot, err := util.RepoRoot()
			if err == nil && repoRoot != "" {
				expectedTarget := filepath.Join(repoRoot, config.DotScion)
				if targetDir == expectedTarget && wd != repoRoot {
					fmt.Printf("Note: Creating .scion at repository root (%s)\n", repoRoot)
				}
			}
		}

		fmt.Println("Initializing scion project grove...")
		if err := config.InitProject("", harnesses); err != nil {
			return fmt.Errorf("failed to initialize project grove: %w", err)
		}

		// Generate and save grove_id
		groveID := config.GenerateGroveIDForDir(filepath.Dir(targetDir))
		if err := config.UpdateSetting(targetDir, "grove_id", groveID, false); err != nil {
			fmt.Printf("Warning: failed to save grove_id: %v\n", err)
		}

		fmt.Println("scion grove successfully initialized.")
		fmt.Printf("Grove ID: %s\n", groveID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(groveCmd)
	groveCmd.AddCommand(groveInitCmd)

	groveInitCmd.Flags().BoolVar(&globalInit, "global", false, "Initialize the global grove in the home directory")
}
