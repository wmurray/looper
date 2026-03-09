package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/git"
	"github.com/willmurray/looper/internal/guards"
	"github.com/willmurray/looper/internal/progress"
	"github.com/willmurray/looper/internal/runner"
	"github.com/willmurray/looper/internal/signals"
	"github.com/willmurray/looper/internal/ui"
)

var (
	flagCycles   int
	flagPlan     string
	flagTimeout  int
	flagDryRun   bool
	flagYes      bool
	flagReviewer string
)

// Safety guarantees:
//   - Never pushes code to a remote
//   - Never changes branches
//   - Never rebases, force-pushes, or rewrites history
//   - Never cherry-picks or resets HEAD
//   - All commits are local and preserved for audit
//   - Only git operations used: add, commit, diff, status, log, rev-parse
//   - git add respects .gitignore — ignored files (e.g. .env) are never staged
//   - Progress and plan files are written only in the working directory

var implementCmd = &cobra.Command{
	Use:   "implement",
	Short: "Run the agent implementation loop against a plan file",
	Long: `Run the automated implement/review agent loop.

Each cycle runs two agent phases:
  1. Execution: the agent implements the plan
  2. Review: a reviewer agent checks the work

The loop exits early if the reviewer signals success ("Job's done!"),
or if safety guards detect thrashing or repeated failures.

Safety: never pushes, never changes branches, never rewrites history.
All git operations are local: add, commit, diff, status, log only.`,
	RunE: runImplement,
}

func init() {
	implementCmd.Flags().IntVar(&flagCycles, "cycles", 0, "Number of cycles (default from config)")
	implementCmd.Flags().StringVar(&flagPlan, "plan", "", "Plan file to use (default: inferred from ticket)")
	implementCmd.Flags().IntVar(&flagTimeout, "timeout", 0, "Timeout per iteration in seconds (default from config)")
	implementCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Resolve config and print, but don't run agents")
	implementCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "Skip git staging confirmation prompt")
}

func runImplement(cmd *cobra.Command, args []string) error {
	cfg, _, _, err := config.LoadWithRepo()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply flag overrides
	cycles := cfg.Defaults.Cycles
	if flagCycles > 0 {
		cycles = flagCycles
	}
	timeout := cfg.Defaults.Timeout
	if flagTimeout > 0 {
		timeout = flagTimeout
	}
	planFile := flagPlan

	// Git validation
	if err := git.AssertRepo(); err != nil {
		return err
	}
	if err := git.AssertClean(); err != nil {
		return err
	}

	// Compile ticket pattern
	ticketRe, err := regexp.Compile(cfg.TicketPattern)
	if err != nil {
		return fmt.Errorf("invalid ticket_pattern %q: %w", cfg.TicketPattern, err)
	}

	// Ticket inference
	ticket := git.InferTicketFromBranch(ticketRe)
	if ticket == "" && planFile != "" {
		ticket = git.InferTicketFromPlan(planFile, ticketRe)
	}
	if ticket == "" && planFile != "" {
		ticket = ticketRe.FindString(filepath.Base(planFile))
	}
	if ticket == "" {
		ticket = "UNKNOWN"
	}

	// Plan file resolution
	if planFile == "" {
		candidates := []string{
			ticket + "_PLAN.md",
			ticket + "_plan.md",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				planFile = c
				break
			}
		}
	}
	if planFile == "" {
		return fmt.Errorf("plan file not found (tried %s_PLAN.md)\nPass --plan <file> to specify explicitly", ticket)
	}
	if _, err := os.Stat(planFile); err != nil {
		return fmt.Errorf("plan file not found: %s", planFile)
	}

	skillPath := config.ExpandPath(cfg.SkillPath)
	reviewerAgent := config.ExpandPath(cfg.ReviewerAgent)

	// Warn if skill files are missing — the loop will run but agent quality will be degraded.
	missingFiles := false
	if _, err := os.Stat(skillPath); err != nil {
		ui.Warn("skill_path not found: %s", skillPath)
		ui.Warn("Set it with: looper settings set skill_path <path>")
		missingFiles = true
	}
	if _, err := os.Stat(reviewerAgent); err != nil {
		ui.Warn("reviewer_agent not found: %s", reviewerAgent)
		ui.Warn("Set it with: looper settings set reviewer_agent <path>")
		missingFiles = true
	}
	if missingFiles && !flagYes {
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

	// Git staging confirmation
	if !flagDryRun && !flagYes {
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

	// Dry run
	if flagDryRun {
		fmt.Printf("looper implement — dry run\n\n")
		fmt.Printf("  Ticket:         %s\n", ticket)
		fmt.Printf("  Plan file:      %s\n", planFile)
		fmt.Printf("  Cycles:         %d\n", cycles)
		fmt.Printf("  Timeout:        %ds\n", timeout)
		fmt.Printf("  Backend:        %s\n", cfg.Backend)
		fmt.Printf("  Skill path:     %s\n", skillPath)
		fmt.Printf("  Reviewer agent: %s\n", reviewerAgent)
		return nil
	}

	ctx, cancel := signals.WithInterrupt(context.Background())
	defer cancel()

	return implementLoop(ctx, cfg, ticket, planFile, cycles, timeout)
}

// implementLoop runs the implement/review agent cycle. It is called by both
// runImplement and runStart after all preflight checks have passed.
func implementLoop(ctx context.Context, cfg config.Config, ticket, planFile string, cycles, timeout int) error {
	skillPath := config.ExpandPath(cfg.SkillPath)
	reviewerAgent := config.ExpandPath(cfg.ReviewerAgent)

	planContent, err := os.ReadFile(planFile)
	if err != nil {
		return fmt.Errorf("could not read plan file: %w", err)
	}

	progressFile := ticket + "_PROGRESS.md"
	pw := progress.New(progressFile, ticket, planFile, cycles, timeout)
	if err := pw.WriteHeader(); err != nil {
		return fmt.Errorf("could not create progress file: %w", err)
	}

	ui.Header("Starting loop: %s", ticket)
	ui.Header("Max cycles: %d | Timeout per iteration: %ds | Backend: %s", cycles, timeout, cfg.Backend)
	fmt.Println()

	guardState := &guards.State{}
	totalIterations := 0

	jobsDoneRe := regexp.MustCompile(`(?i)job.*s\s+done`)

	for i := 1; i <= cycles; i++ {
		iterStart := time.Now()
		totalIterations = i

		_ = pw.BeginRun(i)
		ui.Iteration("=== Iteration %d of %d ===", i, cycles)

		// --- PHASE 1: EXECUTION ---
		// Read progress file so exec agent has full history of previous iterations.
		execProgressBytes, err := os.ReadFile(progressFile)
		if err != nil {
			return fmt.Errorf("could not read progress file before iteration %d: %w", i, err)
		}
		execSpinner := ui.NewSpinner(fmt.Sprintf("[%s] Executing plan...", time.Now().Format("15:04:05")))
		execSpinner.Start()
		execResultCh := runner.RunAsync(ctx, buildExecPrompt(string(planContent), string(execProgressBytes), skillPath), timeout, cfg.Backend)
		execResult := <-execResultCh

		if execResult.Cancelled {
			execSpinner.Abort()
			fmt.Println()
			ui.Alert("Interrupted — committing partial work")
			git.CommitWIP(i, "execution")
			_ = pw.WriteSummary("interrupted", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
			return fmt.Errorf("interrupted")
		}
		execSpinner.Stop()

		if execResult.TimedOut {
			_ = pw.WriteGuardTriggered(fmt.Sprintf("Execution timeout after %ds", timeout))
			ui.Alert("Execution agent timeout")
			git.CommitWIP(i, "execution")
			return fmt.Errorf("execution timed out at iteration %d", i)
		}
		if execResult.ExitCode != 0 {
			_ = pw.WriteGuardTriggered(fmt.Sprintf("Execution failed (exit code %d)", execResult.ExitCode))
			ui.Error("Execution failed (code %d)", execResult.ExitCode)
			return fmt.Errorf("execution agent failed at iteration %d", i)
		}

		gitDiff := git.Diff()
		_ = pw.WriteExecution(execResult.Output)

		// --- GUARD 1: No changes ---
		g1 := guardState.CheckNoChanges(gitDiff)
		if g1.Warning {
			_ = pw.WriteGuardAlert(g1.Message)
			ui.Warn("%s", g1.Message)
		}
		if g1.Triggered {
			_ = pw.WriteGuardTriggered(g1.Message)
			ui.Alert("%s", g1.Message)
			ui.Alert("Aborting.")
			_ = pw.WriteSummary("aborted — no changes", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
			return fmt.Errorf("guard triggered: %s", g1.Message)
		}

		// --- PHASE 2: REVIEW ---
		// Re-read progress file — now includes this iteration's execution output.
		reviewProgressBytes, err := os.ReadFile(progressFile)
		if err != nil {
			return fmt.Errorf("could not read progress file before review at iteration %d: %w", i, err)
		}
		reviewSpinner := ui.NewSpinner(fmt.Sprintf("[%s] Reviewing...", time.Now().Format("15:04:05")))
		reviewSpinner.Start()
		reviewResultCh := runner.RunAsync(ctx, buildReviewPrompt(string(planContent), string(reviewProgressBytes), reviewerAgent), timeout, cfg.Backend)
		reviewResult := <-reviewResultCh

		if reviewResult.Cancelled {
			reviewSpinner.Abort()
			fmt.Println()
			ui.Alert("Interrupted — committing partial work")
			git.CommitWIP(i, "review")
			_ = pw.WriteSummary("interrupted", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
			return fmt.Errorf("interrupted")
		}
		reviewSpinner.Stop()

		if reviewResult.TimedOut {
			_ = pw.WriteGuardTriggered(fmt.Sprintf("Review timeout after %ds", timeout))
			ui.Alert("Review agent timeout")
			git.CommitWIP(i, "review")
			return fmt.Errorf("review timed out at iteration %d", i)
		}
		if reviewResult.ExitCode != 0 {
			_ = pw.WriteGuardTriggered(fmt.Sprintf("Review failed (exit code %d)", reviewResult.ExitCode))
			ui.Error("Review failed (code %d)", reviewResult.ExitCode)
			return fmt.Errorf("review agent failed at iteration %d", i)
		}

		_ = pw.WriteReview(reviewResult.Output)

		// --- GUARD 2: Repeated issues ---
		g2 := guardState.CheckRepeatedIssues(reviewResult.Output)
		if g2.Warning {
			_ = pw.WriteGuardAlert(g2.Message)
			ui.Warn("%s", g2.Message)
		}
		if g2.Triggered {
			_ = pw.WriteGuardTriggered(g2.Message)
			ui.Alert("%s", g2.Message)
			ui.Alert("Aborting.")
			_ = pw.WriteSummary("aborted — repeated issues", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
			return fmt.Errorf("guard triggered: %s", g2.Message)
		}

		// --- GUARD 3: Iteration duration (log only) ---
		elapsed := int64(time.Since(iterStart).Seconds())
		_ = pw.WriteIterationTime(elapsed)

		// --- COMMIT ---
		if err := git.CommitIteration(i, execResult.Output); err != nil {
			ui.Alert("Commit failed: %v", err)
		} else {
			ui.Phase("[%s] Committed iteration %d", time.Now().Format("15:04:05"), i)
		}

		// --- CHECK FOR SUCCESS ---
		if jobsDoneRe.MatchString(reviewResult.Output) {
			_ = pw.WriteSuccess(i)
			_ = pw.WriteSummary("complete", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
			fmt.Println()
			ui.Success("👷 Job's done - completed in %d of %d iterations", i, cycles)
			return nil
		}

		fmt.Println()
	}

	// Max cycles reached
	ui.Alert("Max cycles (%d) reached without approval", cycles)
	_ = pw.WriteSummary("max cycles reached", totalIterations, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(totalIterations))

	return fmt.Errorf("max cycles (%d) reached without completion", cycles)
}

// confirmGitStaging warns the user that git add -A will be run and prompts for
// confirmation. Returns (trusted=true) if the user chose "always".
func confirmGitStaging(cwd string) (trusted bool, err error) {
	fmt.Println()
	ui.Warn("looper will run \"git add -A\" after each iteration.")
	ui.Warn("This stages all untracked and modified files in:")
	fmt.Printf(ui.Bold+ui.Yellow+"    %s"+ui.Reset+"\n", cwd)
	fmt.Println()
	ui.Warn("Files listed in .gitignore will NOT be staged, but you should")
	ui.Warn("verify your .gitignore is correctly configured before proceeding.")
	fmt.Printf("\nContinue?\n")
	fmt.Printf("  [y]es\n")
	fmt.Printf("  [a]lways (trust this repository)\n")
	fmt.Printf("  [N]o\n")
	fmt.Printf("\n> ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false, fmt.Errorf("aborted")
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))

	switch answer {
	case "a", "always":
		fmt.Printf("\nRepository trusted. You won't be asked again for:\n  %s\n\n", cwd)
		return true, nil
	case "y", "yes":
		fmt.Println()
		return false, nil
	default:
		return false, fmt.Errorf("aborted by user")
	}
}

func buildExecPrompt(planContent, progressContent, skillPath string) string {
	var historySection string
	if strings.TrimSpace(progressContent) == "" {
		historySection = "(First iteration — no history yet)"
	} else {
		historySection = "Loop history — do not regress on issues addressed in previous iterations:\n\n" + progressContent
	}

	return strings.TrimSpace(fmt.Sprintf(`Follow %s

Execute this plan:
`+"```"+`
%s
`+"```"+`

%s

Complete the plan using Test-Driven Development:
1. Write failing tests
2. Write implementation to pass tests
3. Refactor as needed
4. All tests must pass before you complete

Provide a concise summary of what you accomplished.`, skillPath, planContent, historySection))
}

func buildReviewPrompt(planContent, progressContent, reviewerAgent string) string {
	return strings.TrimSpace(fmt.Sprintf(`Using the code reviewer subagent (%s)

Review the implementation against this plan:
`+"```"+`
%s
`+"```"+`

The loop history below contains all iterations so far. The most recent ### Execution section is the current implementation to review.

%s

Provide your assessment. If the implementation looks good and meets the plan, start your response with: 👷‍♂️ Job's done!

If there are issues to address, start with: 🔧 Needs work and list the issues.`, reviewerAgent, planContent, progressContent))
}
