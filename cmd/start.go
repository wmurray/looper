package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/git"
	"github.com/willmurray/looper/internal/linear"
	"github.com/willmurray/looper/internal/runner"
	"github.com/willmurray/looper/internal/signals"
	"github.com/willmurray/looper/internal/ui"
)

var (
	startFlagCycles  int
	startFlagTimeout int
	startFlagYes     bool
	startFlagDryRun  bool
)

var startCmd = &cobra.Command{
	Use:   "start <TICKET-ID>",
	Short: "Start a Linear ticket: fetch, branch, plan, and implement",
	Long: `Fetch a Linear ticket, create a branch, generate a plan, and run the implement loop.

Steps:
  1. Fetch the ticket from Linear (title, description, suggested branch name)
  2. git checkout -b <branch>
  3. Set the ticket state to In Progress
  4. Resolve the plan: decode from a looper-plan attachment, or generate via AI
  5. Commit the plan file, then hand off to looper implement

Requires linear_api_key to be set:
  looper settings set linear_api_key <your-key>`,
	Args: cobra.ExactArgs(1),
	RunE: runStart,
}

func init() {
	startCmd.Flags().IntVar(&startFlagCycles, "cycles", 0, "Number of cycles (default from config)")
	startCmd.Flags().IntVar(&startFlagTimeout, "timeout", 0, "Timeout per iteration in seconds (default from config)")
	startCmd.Flags().BoolVarP(&startFlagYes, "yes", "y", false, "Skip git staging confirmation prompt")
	startCmd.Flags().BoolVar(&startFlagDryRun, "dry-run", false, "Fetch and plan but don't run agents")
}

func runStart(cmd *cobra.Command, args []string) error {
	ticketID := strings.ToUpper(args[0])

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.LinearAPIKey == "" {
		return fmt.Errorf("linear_api_key not set\nRun: looper settings set linear_api_key <your-key>")
	}

	// Require a clean working tree before creating a new branch.
	if err := git.AssertRepo(); err != nil {
		return err
	}
	if err := git.AssertClean(); err != nil {
		return err
	}

	// --- FETCH ISSUE ---
	client := linear.New(cfg.LinearAPIKey)

	fetchSpinner := ui.NewSpinner(fmt.Sprintf("Fetching %s from Linear...", ticketID))
	fetchSpinner.Start()
	issue, err := client.GetIssue(ticketID)
	if err != nil {
		fetchSpinner.Abort()
		return fmt.Errorf("fetch issue: %w", err)
	}
	fetchSpinner.Stop()

	// Validate the API-returned identifier before using it as a filename.
	ticketRe, err := regexp.Compile(cfg.TicketPattern)
	if err != nil {
		return fmt.Errorf("invalid ticket_pattern %q: %w", cfg.TicketPattern, err)
	}
	if !ticketRe.MatchString(issue.Identifier) {
		return fmt.Errorf("Linear returned unexpected identifier %q (does not match ticket_pattern %q)", issue.Identifier, cfg.TicketPattern)
	}

	ui.Header("  %s: %s", issue.Identifier, issue.Title)
	fmt.Println()

	// --- BRANCH ---
	branchName := issue.BranchName
	if branchName == "" {
		branchName = linear.SlugifyBranch(issue.Identifier, issue.Title)
	}

	if startFlagDryRun {
		fmt.Printf("looper start — dry run\n\n")
		fmt.Printf("  Ticket:   %s — %s\n", issue.Identifier, issue.Title)
		fmt.Printf("  Branch:   %s\n", branchName)
		fmt.Printf("  Plan:     %s_PLAN.md\n", issue.Identifier)
		fmt.Printf("  Cycles:   %d\n", cfg.Defaults.Cycles)
		fmt.Printf("  Timeout:  %ds\n", cfg.Defaults.Timeout)
		fmt.Printf("  Backend:  %s\n", cfg.Backend)
		return nil
	}

	ui.Phase("Creating branch: %s", branchName)
	if err := git.CheckoutNewBranch(branchName); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}

	// --- SET IN PROGRESS ---
	progressSpinner := ui.NewSpinner("Setting ticket In Progress...")
	progressSpinner.Start()
	if err := client.SetInProgress(issue.ID, issue.Team.ID); err != nil {
		progressSpinner.Abort()
		// Non-fatal: warn and continue. Failing to update the ticket state
		// should not block the implementation work.
		ui.Warn("Could not set In Progress: %v", err)
	} else {
		progressSpinner.Stop()
	}

	// --- RESOLVE PLAN ---
	planFile := issue.Identifier + "_PLAN.md"

	if plan, ok := linear.PlanFromAttachment(issue.Attachments); ok {
		// Attachment contains a pre-written plan — use it directly.
		ui.Phase("Using plan from looper-plan attachment")
		if err := os.WriteFile(planFile, []byte(strings.TrimSpace(plan)+"\n"), 0644); err != nil {
			return fmt.Errorf("write plan file: %w", err)
		}
	} else {
		// Generate plan from the ticket description via AI.
		if issue.Description == "" {
			ui.Warn("Ticket has no description — generating minimal plan template")
			if err := writePlanTemplate(planFile, issue.Identifier); err != nil {
				return fmt.Errorf("write plan template: %w", err)
			}
		} else {
			ctx, cancel := signals.WithInterrupt(context.Background())
			defer cancel()

			genSpinner := ui.NewSpinner(fmt.Sprintf("Generating %s via %s...", planFile, cfg.Backend))
			genSpinner.Start()

			prompt := buildPlanPrompt(issue.Identifier, issue.Description)
			result := <-runner.RunAsync(ctx, prompt, cfg.Defaults.Timeout, cfg.Backend)

			if result.Cancelled {
				genSpinner.Abort()
				return errors.New("interrupted")
			}
			if result.TimedOut {
				genSpinner.Abort()
				return fmt.Errorf("plan generation timed out after %ds", cfg.Defaults.Timeout)
			}
			if result.ExitCode != 0 {
				genSpinner.Abort()
				if result.Err != nil {
					return fmt.Errorf("agent could not start: %w", result.Err)
				}
				return fmt.Errorf("plan generation failed (exit %d)", result.ExitCode)
			}
			if strings.TrimSpace(result.Output) == "" {
				genSpinner.Abort()
				return errors.New("agent returned empty plan — aborting")
			}

			genSpinner.Stop()

			if err := os.WriteFile(planFile, []byte(strings.TrimSpace(result.Output)+"\n"), 0644); err != nil {
				return fmt.Errorf("write plan file: %w", err)
			}
		}
	}

	ui.Phase("Plan written: %s", planFile)

	// Commit the plan file so implement's AssertClean() passes.
	if err := git.CommitPlan(planFile, issue.Identifier); err != nil {
		return fmt.Errorf("commit plan: %w", err)
	}

	fmt.Println()

	// --- HAND OFF TO IMPLEMENT ---
	// Set the package-level flag vars that runImplement reads.
	flagPlan = planFile
	flagYes = startFlagYes
	if startFlagCycles > 0 {
		flagCycles = startFlagCycles
	}
	if startFlagTimeout > 0 {
		flagTimeout = startFlagTimeout
	}
	flagDryRun = false // already handled dry-run above

	return runImplement(implementCmd, []string{})
}
