package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "looper",
	Short: "Automated agent loop CLI for plan-driven development",
	Long: `looper runs an automated implement/review cycle against a plan file.

Commands:
  start       Fetch a Linear ticket, branch, plan, and implement
  implement   Run the agent loop against a plan file
  plan        Create a plan file with the correct naming scheme
  settings    View or set default configuration`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(implementCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(settingsCmd)
	rootCmd.AddCommand(cleanCmd)
}
