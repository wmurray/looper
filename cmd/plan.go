package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/git"
	"github.com/willmurray/looper/internal/runner"
	"github.com/willmurray/looper/internal/signals"
	"github.com/willmurray/looper/internal/ui"
)

const planFileMode = 0644

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
		planOpenFlag, err := cmd.Flags().GetBool("open")
		if err != nil {
			return fmt.Errorf("reading --open flag: %w", err)
		}
		planPromptFlag, err := cmd.Flags().GetString("prompt")
		if err != nil {
			return fmt.Errorf("reading --prompt flag: %w", err)
		}

		// Lazy config loader — loads at most once, only when needed.
		var loadedCfg *config.Config
		getCfg := func() (*config.Config, error) {
			if loadedCfg != nil {
				return loadedCfg, nil
			}
			c, err := config.Load()
			if err != nil {
				return nil, fmt.Errorf("failed to load config: %w", err)
			}
			loadedCfg = &c
			return loadedCfg, nil
		}

		var ticket string
		if len(args) > 0 {
			ticket = strings.ToUpper(args[0])
		} else {
			cfg, err := getCfg()
			if err != nil {
				return err
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
			if planPromptFlag != "" {
				fmt.Fprintf(os.Stderr, "warning: plan file already exists, --prompt ignored: %s\n", filename)
			}
			fmt.Printf("%s\n", filename)
			if planOpenFlag {
				return openInEditor(filename)
			}
			return nil
		}

		if planPromptFlag == "" {
			if err := writePlanTemplate(filename, ticket); err != nil {
				return fmt.Errorf("could not create plan file: %w", err)
			}
		} else {
			cfg, err := getCfg()
			if err != nil {
				return err
			}

			ctx, cancel := signals.WithInterrupt(context.Background())
			defer cancel()

			spinner := ui.NewSpinner(fmt.Sprintf("Generating %s via %s...", filename, cfg.Backend))
			spinner.Start()
			aborted := true
			defer func() {
				if aborted {
					spinner.Abort()
				}
			}()

			resultCh := runner.RunAsync(ctx, buildPlanPrompt(ticket, planPromptFlag), cfg.Defaults.Timeout, cfg.Backend)
			result := <-resultCh

			if result.Cancelled {
				return errors.New("interrupted")
			}
			if result.TimedOut {
				return fmt.Errorf("agent timed out after %ds — plan not created", cfg.Defaults.Timeout)
			}
			if result.ExitCode != 0 {
				if result.Err != nil {
					return fmt.Errorf("agent could not be started — plan not created: %w", result.Err)
				}
				if result.Stderr != "" {
					return fmt.Errorf("agent exited with code %d — plan not created\n%s", result.ExitCode, result.Stderr)
				}
				return fmt.Errorf("agent exited with code %d — plan not created", result.ExitCode)
			}
			if strings.TrimSpace(result.Output) == "" {
				return errors.New("agent returned empty output — plan not created")
			}
			spinner.Stop()
			aborted = false
			if err := os.WriteFile(filename, []byte(strings.TrimSpace(result.Output)+"\n"), planFileMode); err != nil {
				return fmt.Errorf("could not write plan file: %w", err)
			}
		}

		abs, err := filepath.Abs(filename)
		if err != nil {
			abs = filename
		}
		fmt.Printf("Created: %s\n", abs)

		if planOpenFlag {
			return openInEditor(filename)
		}
		return nil
	},
}

func init() {
	planCmd.Flags().Bool("open", false, "Open the plan file in $EDITOR after creation")
	planCmd.Flags().String("prompt", "", "Generate plan content via AI using this prompt (ignored if plan already exists)")
}

func planTemplateBytes(ticket string) []byte {
	return []byte(fmt.Sprintf(`# Ticket: %s

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
`, ticket))
}

func writePlanTemplate(filename, ticket string) error {
	return os.WriteFile(filename, planTemplateBytes(ticket), planFileMode)
}

const planPromptTemplate = `You are a senior software engineer writing a technical implementation plan.

Produce a complete markdown plan document for ticket: {TICKET}

User's request: {PROMPT}

The document must follow this exact structure, with all sections filled in.
Do not add, rename, or reorder sections. Output only the markdown — no preamble or commentary.

# Ticket: {TICKET}

## Objective
<!-- clear statement of what needs to be built -->

## Context
<!-- background, relevant code areas, related tickets if mentioned -->

## Implementation Steps
1. ...

## Acceptance Criteria
- [ ] ...

## Out of Scope
- ...`

// buildPlanPrompt substitutes {TICKET} and {PROMPT} into the plan template.
// strings.NewReplacer does not re-scan its own output, so literal "{TICKET}" or
// "{PROMPT}" text inside userPrompt will not be expanded a second time.
func buildPlanPrompt(ticket, userPrompt string) string {
	r := strings.NewReplacer("{TICKET}", ticket, "{PROMPT}", userPrompt)
	return r.Replace(planPromptTemplate)
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
