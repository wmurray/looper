package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/discover"
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
  polish_agent        Path to polish agent file (falls back to reviewer_agent if unset)
  polish_cmds         Comma-separated lint/format commands run before the polish agent
  notify              Send desktop notification on loop completion or abort (true/false)
  notify_webhook      Slack webhook URL to POST notification to`,
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

var discoverApply bool
var discoverAI bool
var discoverYes bool

var settingsDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Scan ~/.claude/ for installed skills and agents",
	Long: `Scan canonical ~/.claude/ locations for installed skill and agent files.

Prints ready-to-run "looper settings set" commands for each file found.

With --apply, automatically sets any key that has exactly one candidate.
When multiple candidates exist for a key, the command is printed but skipped.

With --ai, reads discovered file contents, detects the project stack, and asks
the configured backend to recommend which paths to assign to skill_path and
reviewer_agent. A diff is printed and the user is prompted before any changes
are written. Use --yes to skip the confirmation prompt.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %w", err)
		}

		if discoverAI {
			return runAIDiscover(home, discoverYes)
		}

		found, err := discover.Scan(home)
		if err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}

		if len(found) == 0 {
			fmt.Println("No skills or agents found under ~/.claude/")
			return nil
		}

		var skills, agents []discover.Found
		for _, f := range found {
			if f.Kind == discover.KindSkill {
				skills = append(skills, f)
			} else {
				agents = append(agents, f)
			}
		}

		printGroup := func(header, key string, items []discover.Found) {
			if len(items) == 0 {
				return
			}
			fmt.Printf("%s:\n", header)
			for _, item := range items {
				fmt.Printf("  looper settings set %s %s\n", key, item.Path)
			}
			fmt.Println()
		}

		printGroup("Skills (skill_path)", "skill_path", skills)
		printGroup("Agents (reviewer_agent)", "reviewer_agent", agents)

		if !discoverApply {
			return nil
		}

		applyOne := func(key string, items []discover.Found) error {
			if len(items) == 0 {
				return nil
			}
			if len(items) > 1 {
				fmt.Printf("Skipping %s — %d candidates found (ambiguous)\n", key, len(items))
				return nil
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cfg, err = config.Set(cfg, key, items[0].Path)
			if err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("Set %s = %s\n", key, items[0].Path)
			return nil
		}

		if err := applyOne("skill_path", skills); err != nil {
			return err
		}
		return applyOne("reviewer_agent", agents)
	},
}

func init() {
	settingsDiscoverCmd.Flags().BoolVar(&discoverApply, "apply", false, "Auto-set keys with exactly one candidate")
	settingsDiscoverCmd.Flags().BoolVar(&discoverAI, "ai", false, "Use the configured backend to recommend skill_path and reviewer_agent")
	settingsDiscoverCmd.Flags().BoolVarP(&discoverYes, "yes", "y", false, "Skip confirmation prompt (only meaningful with --ai)")
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
	settingsCmd.AddCommand(settingsResetCmd)
	settingsCmd.AddCommand(settingsDiscoverCmd)
}
