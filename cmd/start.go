package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/git"
	"github.com/willmurray/looper/internal/linear"
	"github.com/willmurray/looper/internal/notify"
	"github.com/willmurray/looper/internal/runner"
	"github.com/willmurray/looper/internal/signals"
	"github.com/willmurray/looper/internal/ui"
)

var (
	startFlagCycles  int
	startFlagTimeout int
	startFlagYes     bool
	startFlagDryRun  bool
	startFlagNotify  bool
	startFlagStream  bool
	startFlagRetries int
)

type resumeState int

const (
	resumeNoPlan       resumeState = iota // branch exists, no plan, no iterations
	resumePlanExists                      // plan written, loop not yet started
	resumeHasIterations                   // implement loop ran at least once
)

// Why: injected predicates make this testable without a real git repo or filesystem.
func resolveResumeState(hasWork func() bool, statPlan func() error) resumeState {
	if hasWork() {
		return resumeHasIterations
	}
	if statPlan() == nil {
		return resumePlanExists
	}
	return resumeNoPlan
}

var startCmd = &cobra.Command{
	Use:   "start <TICKET-ID>",
	Short: "Start a Linear ticket: fetch, branch, plan, and implement",
	Long: `Fetch a Linear ticket, create a branch, generate a plan, and run the implement loop.

Steps:
  1. Fetch the ticket from Linear (title, description, suggested branch name)
  2. git checkout -b <branch>
  3. Set the ticket state to In Progress
  4. Resolve the plan: decode from a looper-plan attachment, or generate via AI
  5. Attach the plan to the Linear ticket, then run the implement loop

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
	startCmd.Flags().BoolVar(&startFlagNotify, "notify", false, "Send desktop notification when loop completes or aborts")
	startCmd.Flags().BoolVar(&startFlagStream, "stream", false, "Stream agent output to the terminal (suppresses spinner)")
	startCmd.Flags().IntVar(&startFlagRetries, "retries", -1, "max retries per phase on transient errors (0 = no retries; default from config)")
}

func runStart(cmd *cobra.Command, args []string) error {
	ticketID := strings.ToUpper(args[0])

	cfg, _, _, err := config.LoadWithRepo()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.LinearAPIKey == "" {
		return fmt.Errorf("linear_api_key not set\nRun: looper settings set linear_api_key <your-key>")
	}

	// Why: validate pattern before hitting the network to avoid a wasted API round-trip.
	ticketRe, err := regexp.Compile(cfg.TicketPattern)
	if err != nil {
		return fmt.Errorf("invalid ticket_pattern %q: %w", cfg.TicketPattern, err)
	}

	if err := git.AssertRepo(); err != nil {
		return err
	}
	if err := git.AssertClean(); err != nil {
		return err
	}

	// Why: covers Linear API calls, plan generation, and the implement loop.
	ctx, cancel := signals.WithInterrupt(context.Background())
	defer cancel()

	client := linear.New(cfg.LinearAPIKey)

	fetchSpinner := ui.NewSpinner(fmt.Sprintf("Fetching %s from Linear...", ticketID))
	fetchSpinner.Start()
	issue, err := client.GetIssue(ctx, ticketID)
	if err != nil {
		fetchSpinner.Abort()
		return fmt.Errorf("fetch issue: %w", err)
	}
	fetchSpinner.Stop()

	// Why: guard against an unexpected identifier being used as a filename.
	if !ticketRe.MatchString(issue.Identifier) {
		return fmt.Errorf("Linear returned unexpected identifier %q (does not match ticket_pattern %q)", issue.Identifier, cfg.TicketPattern)
	}

	ui.Header("  %s: %s", issue.Identifier, issue.Title)
	fmt.Println()

	branchName := issue.BranchName
	if branchName == "" {
		branchName = linear.SlugifyBranch(issue.Identifier, issue.Title)
	}

	cycles := cfg.Defaults.Cycles
	if startFlagCycles > 0 {
		cycles = startFlagCycles
	}
	timeout := cfg.Defaults.Timeout
	if startFlagTimeout > 0 {
		timeout = startFlagTimeout
	}
	retries := cfg.Retries
	if startFlagRetries >= 0 {
		retries = startFlagRetries
	}

	if startFlagDryRun {
		fmt.Printf("looper start — dry run\n\n")
		fmt.Printf("  Ticket:   %s — %s\n", issue.Identifier, issue.Title)
		fmt.Printf("  Branch:   %s\n", branchName)
		fmt.Printf("  Plan:     %s_PLAN.md\n", issue.Identifier)
		fmt.Printf("  Cycles:   %d\n", cycles)
		fmt.Printf("  Timeout:  %ds\n", timeout)
		fmt.Printf("  Backend:  %s\n", cfg.Backend)
		return nil
	}

	resumed := false
	if git.BranchExists(branchName) {
		ui.Phase("Branch %s already exists — resuming", branchName)
		if err := git.Checkout(branchName); err != nil {
			return fmt.Errorf("switch to branch: %w", err)
		}
		resumed = true
	} else {
		ui.Phase("Creating branch: %s", branchName)
		if err := git.CheckoutNewBranch(branchName); err != nil {
			return fmt.Errorf("create branch: %w", err)
		}
	}

	progressSpinner := ui.NewSpinner("Setting ticket In Progress...")
	progressSpinner.Start()
	if err := client.SetInProgress(ctx, issue.ID, issue.Team.ID); err != nil {
		progressSpinner.Abort()
		// Why: a stale ticket state should not block implementation.
		ui.Warn("Could not set In Progress: %v", err)
	} else {
		progressSpinner.Stop()
	}

	planFile := issue.Identifier + "_PLAN.md"
	skipPlanGeneration := false
	if resumed {
		switch resolveResumeState(git.HasIterationWork, func() error {
			_, err := os.Stat(planFile)
			return err
		}) {
		case resumeHasIterations:
			if _, err := os.Stat(planFile); err != nil {
				return fmt.Errorf("resuming %s: plan file %s not found (was it deleted?)", issue.Identifier, planFile)
			}
			ui.Phase("Resuming implement loop for %s", issue.Identifier)
			fmt.Println()
			loopErr := implementLoop(ctx, cfg, issue.Identifier, planFile, cycles, timeout, retries, startFlagStream)
			doNotify := cfg.Notify || startFlagNotify
			notifyTitle := "Looper — " + issue.Identifier
			if loopErr != nil {
				notify.Send(doNotify, cfg.NotifyWebhook, notifyTitle, "Loop aborted: "+loopErr.Error())
			} else {
				notify.Send(doNotify, cfg.NotifyWebhook, notifyTitle, "Loop finished successfully")
			}
			return loopErr
		case resumePlanExists:
			ui.Phase("Plan already exists: %s", planFile)
			skipPlanGeneration = true
		case resumeNoPlan:
		}
	}

	needsPlan := !skipPlanGeneration
	if needsPlan {
		if plan, ok := linear.PlanFromAttachment(issue.Attachments); ok {
			ui.Phase("Using plan from looper-plan attachment")
			if err := os.WriteFile(planFile, []byte(strings.TrimSpace(plan)+"\n"), 0644); err != nil {
				return fmt.Errorf("write plan file: %w", err)
			}
		} else {
			var planContent []byte
			if issue.Description == "" {
				ui.Warn("Ticket has no description — generating minimal plan template")
				planContent = planTemplateBytes(issue.Identifier)
				if err := os.WriteFile(planFile, planContent, 0644); err != nil {
					return fmt.Errorf("write plan template: %w", err)
				}
			} else {
				genSpinner := ui.NewSpinner(fmt.Sprintf("Generating %s via %s...", planFile, cfg.Backend))
				genSpinner.Start()

				prompt := buildPlanPrompt(issue.Identifier, issue.Description)
				result := runner.RunWithRetry(ctx, runner.RunAsyncFn(), prompt, cfg.Defaults.Timeout, cfg.Backend, retries, "plan-gen", nil, func(format string, args ...any) {
					ui.Warn(format, args...)
				})

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

				planContent = []byte(strings.TrimSpace(result.Output) + "\n")
				if err := os.WriteFile(planFile, planContent, 0644); err != nil {
					return fmt.Errorf("write plan file: %w", err)
				}
			}

			// Why: plan travels with the issue in Linear; non-fatal so the loop is never blocked.
			attachSpinner := ui.NewSpinner(fmt.Sprintf("Attaching plan to %s...", issue.Identifier))
			attachSpinner.Start()
			if err := client.AttachPlan(ctx, issue.ID, string(planContent)); err != nil {
				attachSpinner.Abort()
				if ctx.Err() == nil {
					ui.Warn("Could not attach plan to Linear: %v", err)
				}
			} else {
				attachSpinner.Stop()
			}
		}

		ui.Phase("Plan written: %s", planFile)
	}

	skillPath := config.ExpandPath(cfg.SkillPath)
	reviewerAgent := config.ExpandPath(cfg.ReviewerAgent)
	missingFiles := warnIfPathMissing("skill_path", skillPath) || warnIfPathMissing("reviewer_agent", reviewerAgent)

	if cwd, err := os.Getwd(); err == nil {
		warnOnStackMismatch(cwd, filepath.Base(reviewerAgent))
	}

	if missingFiles && !startFlagYes {
		// Gotcha: branch and plan already exist; aborting here leaves them for manual cleanup.
		fmt.Printf("\nSkill files are missing. Continue anyway? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("aborted")
		}
		if answer := strings.TrimSpace(strings.ToLower(scanner.Text())); answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted")
		}
		fmt.Println()
	}

	if !startFlagYes {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine working directory: %w", err)
		}
		if !config.IsTrusted(cfg, cwd) {
			trusted, err := confirmGitStaging(cwd)
			if err != nil {
				return err
			}
			if trusted {
				cfg, err = config.TrustDir(cfg, cwd)
				if err != nil {
					ui.Warn("Could not save trusted directory: %v", err)
				}
			}
		}
	}

	fmt.Println()

	loopErr := implementLoop(ctx, cfg, issue.Identifier, planFile, cycles, timeout, retries, startFlagStream)
	doNotify := cfg.Notify || startFlagNotify
	notifyTitle := "Looper — " + issue.Identifier
	if loopErr != nil {
		notify.Send(doNotify, cfg.NotifyWebhook, notifyTitle, "Loop aborted: "+loopErr.Error())
	} else {
		notify.Send(doNotify, cfg.NotifyWebhook, notifyTitle, "Loop finished successfully")
	}
	return loopErr
}
