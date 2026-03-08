package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/git"
)

var planOpenFlag bool

var planCmd = &cobra.Command{
	Use:   "plan [TICKET]",
	Short: "Create or show a plan file with the correct naming scheme",
	Long: `Create or show a plan file named {TICKET}_PLAN.md.

If TICKET is omitted, it is inferred from the git branch name.
If the file already exists, its path is printed.
If it does not exist, it is created from a template.

Use --open to open the file in $EDITOR after creation.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var ticket string

		if len(args) > 0 {
			ticket = strings.ToUpper(args[0])
		} else {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			ticketRe, err := regexp.Compile(cfg.TicketPattern)
			if err != nil {
				return fmt.Errorf("invalid ticket_pattern %q: %w", cfg.TicketPattern, err)
			}
			ticket = git.InferTicketFromBranch(ticketRe)
			if ticket == "" {
				return fmt.Errorf("could not infer ticket from branch name\nPass a ticket ID explicitly: looper plan DX-123")
			}
		}

		filename := ticket + "_PLAN.md"

		if _, err := os.Stat(filename); err == nil {
			fmt.Printf("Plan file already exists: %s\n", filename)
			if planOpenFlag {
				return openInEditor(filename)
			}
			return nil
		}

		if err := writePlanTemplate(filename, ticket); err != nil {
			return fmt.Errorf("could not create plan file: %w", err)
		}

		abs, _ := filepath.Abs(filename)
		fmt.Printf("Created: %s\n", abs)

		if planOpenFlag {
			return openInEditor(filename)
		}
		return nil
	},
}

func init() {
	planCmd.Flags().BoolVar(&planOpenFlag, "open", false, "Open the plan file in $EDITOR after creation")
}

func writePlanTemplate(filename, ticket string) error {
	template := fmt.Sprintf(`# Ticket: %s

## Objective
<!-- What needs to be done -->

## Context
<!-- Background, links to ticket, related files -->

## Implementation Steps
1.
2.
3.

## Acceptance Criteria
- [ ]
- [ ]

## Out of Scope
-
`, ticket)

	return os.WriteFile(filename, []byte(template), 0644)
}

func openInEditor(filename string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		fmt.Printf("$EDITOR not set — open manually: %s\n", filename)
		return nil
	}

	cmd := exec.Command(editor, filename)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
