package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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
  reviewer_agent      Path to reviewer agent file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
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
		return nil
	},
}

var settingsGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		val, err := config.Get(cfg, args[0])
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
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
