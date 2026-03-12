package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "View or set configuration defaults",
	Long: `View or set configuration defaults stored in ~/.config/looper/config.json.

Usage:
  looper settings                        Print all settings as JSON
  looper settings get <key>              Get a single value
  looper settings set <key> <value>      Set a value
  looper settings reset                  Reset all settings to defaults

Valid keys:
  backend             cursor or claude
  defaults.cycles     Default number of loop cycles
  defaults.timeout    Default timeout in seconds per iteration
  skill_path          Path to skill/workflow file
  reviewer_agent      Path to reviewer agent file
  ticket_pattern      Regex for inferring ticket ID from branch name (default: [A-Z]+-[0-9]+)
  linear_api_key      Linear personal API key (used by looper start)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, repoConfigPath, repoKeys, err := config.LoadWithRepo()
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		path, _ := config.ConfigPath()
		fmt.Fprintf(os.Stderr, "\n(config file: %s)\n", path)
		if repoConfigPath != "" {
			fmt.Fprintf(os.Stderr, "(repo config: %s)\n", repoConfigPath)
			if len(repoKeys) > 0 {
				fmt.Fprintf(os.Stderr, "\nRepo overrides:\n")
				for _, k := range repoKeys {
					fmt.Fprintf(os.Stderr, "  %s  [repo]\n", k)
				}
			}
		}
		return nil
	},
}

var settingsGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _, repoKeys, err := config.LoadWithRepo()
		if err != nil {
			return err
		}
		val, err := config.Get(cfg, args[0])
		if err != nil {
			return err
		}
		if slices.Contains(repoKeys, args[0]) {
			fmt.Printf("%s  [repo]\n", val)
		} else {
			fmt.Println(val)
		}
		return nil
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Intentionally uses Load (global only), not LoadWithRepo: settings set
		// writes to the global config file and must not be influenced by a
		// read-only repo config.
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg, err = config.Set(cfg, args[0], args[1])
		if err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Set %s = %s\n", args[0], args[1])
		return nil
	},
}

var settingsResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset all settings to defaults",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Reset(); err != nil {
			return err
		}
		fmt.Println("Settings reset to defaults.")
		return nil
	},
}

func init() {
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
	settingsCmd.AddCommand(settingsResetCmd)
}
