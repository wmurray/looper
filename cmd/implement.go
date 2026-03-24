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
	"github.com/willmurray/looper/internal/agent"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/detect"
	"github.com/willmurray/looper/internal/git"
	"github.com/willmurray/looper/internal/guards"
	"github.com/willmurray/looper/internal/notify"
	"github.com/willmurray/looper/internal/plan"
	"github.com/willmurray/looper/internal/progress"
	"github.com/willmurray/looper/internal/runlog"
	"github.com/willmurray/looper/internal/runner"
	"github.com/willmurray/looper/internal/selector"
	"github.com/willmurray/looper/internal/signals"
	looperstate "github.com/willmurray/looper/internal/state"
	"github.com/willmurray/looper/internal/ui"
)

var (
	flagCycles      int
	flagPlan        string
	flagTimeout     int
	flagDryRun      bool
	flagYes         bool
	flagReviewer    string
	flagStream      bool
	flagNotify      bool
	flagRetries     int
	flagReviewEvery int
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
	implementCmd.Flags().BoolVar(&flagStream, "stream", false, "Stream agent output to the terminal (suppresses spinner)")
	implementCmd.Flags().BoolVar(&flagNotify, "notify", false, "Send desktop notification when loop completes or aborts")
	implementCmd.Flags().IntVar(&flagRetries, "retries", -1, "max retries per phase on transient errors (0 = no retries; default from config)")
	implementCmd.Flags().IntVar(&flagReviewEvery, "review-every", -1, "run reviewer every N cycles (1 = every cycle; default from config)")
}

func runImplement(cmd *cobra.Command, args []string) error {
	cfg, _, _, err := config.LoadWithRepo()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cycles := cfg.Defaults.Cycles
	if flagCycles > 0 {
		cycles = flagCycles
	}
	timeout := cfg.Defaults.Timeout
	if flagTimeout > 0 {
		timeout = flagTimeout
	}
	retries := 0
	if cfg.Retries != nil {
		retries = *cfg.Retries
	}
	if flagRetries >= 0 {
		retries = flagRetries
	}
	reviewEvery := 1
	if cfg.ReviewEvery != nil {
		reviewEvery = *cfg.ReviewEvery
	}
	if flagReviewEvery >= 1 {
		reviewEvery = flagReviewEvery
	}
	planFile := flagPlan

	if err := git.AssertRepo(); err != nil {
		return err
	}
	if err := git.AssertClean(); err != nil {
		return err
	}

	ticketRe, err := regexp.Compile(cfg.TicketPattern)
	if err != nil {
		return fmt.Errorf("invalid ticket_pattern %q: %w", cfg.TicketPattern, err)
	}

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

	planBytes, err := os.ReadFile(planFile)
	if err != nil {
		return fmt.Errorf("could not read plan file: %w", err)
	}
	planErrs := plan.Validate(string(planBytes))
	var fatalMsgs []string
	for _, ve := range planErrs {
		if ve.Fatal {
			fatalMsgs = append(fatalMsgs, ve.Message)
		} else {
			ui.Warn("%s", ve.Message)
		}
	}
	if len(fatalMsgs) > 0 {
		msg := fmt.Sprintf("plan file is not ready to implement (%s):\n", planFile)
		for _, m := range fatalMsgs {
			msg += "  • " + m + "\n"
		}
		return fmt.Errorf("%s", strings.TrimRight(msg, "\n"))
	}

	skillPath := config.ExpandPath(cfg.SkillPath)
	reviewerAgent := config.ExpandPath(cfg.ReviewerAgent)

	// Why: warnings are non-fatal; the loop runs regardless, but missing files degrade agent quality.
	missingFiles := warnIfPathMissing("skill_path", skillPath) || warnIfPathMissing("reviewer_agent", reviewerAgent)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine working directory: %w", err)
	}
	warnOnStackMismatch(cwd, filepath.Base(reviewerAgent))

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

	if !flagDryRun && !flagYes {
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

	if flagDryRun {
		fmt.Printf("looper implement — dry run\n\n")
		fmt.Printf("  Ticket:         %s\n", ticket)
		fmt.Printf("  Plan file:      %s\n", planFile)
		fmt.Printf("  Cycles:         %d\n", cycles)
		fmt.Printf("  Timeout:        %ds\n", timeout)
		fmt.Printf("  Backend:        %s\n", cfg.Backend)
		fmt.Printf("  Skill path:     %s\n", skillPath)
		fmt.Printf("  Reviewer agent: %s\n", reviewerAgent)
		fmt.Printf("  Review every:   %d\n", reviewEvery)
		return nil
	}

	ctx, cancel := signals.WithInterrupt(context.Background())
	defer cancel()

	doNotify := cfg.Notify || flagNotify
	notifyTitle := "Looper — " + ticket
	err = implementLoop(ctx, cfg, ticket, planFile, cycles, timeout, retries, reviewEvery, flagStream)
	if err != nil {
		notify.Send(doNotify, cfg.NotifyWebhook, notifyTitle, "Loop aborted: "+err.Error())
	} else {
		notify.Send(doNotify, cfg.NotifyWebhook, notifyTitle, "Loop finished successfully")
	}
	return err
}

// shouldReview returns true when the review phase should run for iteration i.
// The final cycle always triggers a review so the loop never ends without feedback.
func shouldReview(i, cycles, reviewEvery int) bool {
	if reviewEvery <= 1 {
		return true
	}
	return i == cycles || i%reviewEvery == 0
}

// implementLoop runs the implement/review agent cycle. It is called by both
// runImplement and runStart after all preflight checks have passed.
func implementLoop(ctx context.Context, cfg config.Config, ticket, planFile string, cycles, timeout, retries, reviewEvery int, stream bool) error {
	// Invariant: stale state file from a prior interrupted run must not bleed into a fresh run.
	// Check both new and legacy paths via a Read probe.
	if _, statErr := looperstate.Read(ticket); statErr == nil {
		_ = looperstate.Delete(ticket)
		ui.Warn("Deleted stale state file — starting fresh")
	}
	return implementLoopFrom(ctx, cfg, ticket, planFile, cycles, timeout, retries, reviewEvery, stream, 1, &guards.State{}, time.Now().UTC())
}

// appendRunLog writes a RunEntry to the append-only run log best-effort (errors are discarded).
func appendRunLog(ticket, outcome string, cyclesUsed, cyclesMax int, guardEvents []string, lastReviewerMsg string, startedAt time.Time) {
	_ = runlog.Append(runlog.RunEntry{
		Ticket:          ticket,
		StartedAt:       startedAt.Format(time.RFC3339),
		FinishedAt:      time.Now().UTC().Format(time.RFC3339),
		Outcome:         outcome,
		CyclesUsed:      cyclesUsed,
		CyclesMax:       cyclesMax,
		GuardEvents:     guardEvents,
		LastReviewerMsg: lastReviewerMsg,
	})
}

// implementLoopFrom is the shared loop body used by implementLoop and resumeCmd.
// startCycle lets resume skip already-completed cycles; g carries restored guard
// counters so thrash/stuck detection is continuous across resumptions.
// buildMetadataMap loads agent metadata for all reviewers in r, keyed by path.
func buildMetadataMap(r *config.Reviewers) map[string]agent.Metadata {
	m := map[string]agent.Metadata{}
	if r == nil {
		return m
	}
	paths := []string{r.General}
	paths = append(paths, r.Specialized...)
	for _, p := range paths {
		if p == "" {
			continue
		}
		expanded := config.ExpandPath(p)
		md, err := agent.ParseMetadata(expanded)
		if err != nil {
			ui.Warn("could not parse agent metadata %s: %v", expanded, err)
			continue
		}
		md.Path = expanded
		// Invariant: keys are always unexpanded paths; callers must not expand before passing to SelectReviewers.
		m[p] = md
	}
	return m
}

func implementLoopFrom(ctx context.Context, cfg config.Config, ticket, planFile string, cycles, timeout, retries, reviewEvery int, stream bool, startCycle int, guardState *guards.State, startedAt time.Time) error {
	skillPath := config.ExpandPath(cfg.SkillPath)
	reviewerAgent := config.ExpandPath(cfg.ReviewerAgent)

	planContent, err := os.ReadFile(planFile)
	if err != nil {
		return fmt.Errorf("could not read plan file: %w", err)
	}

	config.MigrateReviewerAgent(&cfg)
	metadataMap := buildMetadataMap(cfg.Reviewers)

	progressFile := ensureProgressPath(ticket)
	pw := progress.New(progressFile, ticket, planFile, cycles, timeout)
	if startCycle == 1 {
		if err := pw.WriteHeader(); err != nil {
			return fmt.Errorf("could not create progress file: %w", err)
		}
	}

	ui.Header("Starting loop: %s", ticket)
	ui.Header("Max cycles: %d | Timeout per iteration: %ds | Backend: %s", cycles, timeout, cfg.Backend)
	fmt.Println()

	totalIterations := startCycle - 1

	var guardEvents []string
	var lastReviewerOutput string

	for i := startCycle; i <= cycles; i++ {
		iterStart := time.Now()
		totalIterations = i

		_ = pw.BeginRun(i)
		ui.Iteration("=== Iteration %d of %d ===", i, cycles)

		// Why: exec agent needs the full iteration history to avoid regressions.
		execProgressBytes, err := os.ReadFile(progressFile)
		if err != nil {
			return fmt.Errorf("could not read progress file before iteration %d: %w", i, err)
		}
		headBefore := git.Head()
		phaseMsg := fmt.Sprintf("[%s] Executing plan...", time.Now().Format("15:04:05"))
		var execSpinner *ui.Spinner
		execPrompt := buildExecPrompt(string(planContent), lastNRuns(string(execProgressBytes), 2), skillPath)
		var execResult runner.Result
		if stream {
			fmt.Fprintln(os.Stderr, phaseMsg)
			execResult = runner.RunWithRetry(ctx, runner.RunStreamAsyncFn(os.Stdout), execPrompt, timeout, cfg.Backend, retries, "execution", pw, ui.Warn)
		} else {
			execSpinner = ui.NewSpinner(phaseMsg)
			execSpinner.Start()
			execResult = runner.RunWithRetry(ctx, runner.RunAsyncFn(), execPrompt, timeout, cfg.Backend, retries, "execution", pw, ui.Warn)
		}

		if execResult.Cancelled {
			if execSpinner != nil {
				execSpinner.Abort()
			}
			fmt.Println()
			ui.Alert("Interrupted — committing partial work")
			git.CommitWIP(i, "execution")
			_ = pw.WriteSummary("interrupted", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
			appendRunLog(ticket, "interrupted", i, cycles, guardEvents, lastReviewerOutput, startedAt)
			return fmt.Errorf("interrupted")
		}
		if execSpinner != nil {
			execSpinner.Stop()
		}

		if execResult.TimedOut {
			_ = pw.WriteGuardTriggered(fmt.Sprintf("Execution timeout after %ds", timeout))
			ui.Alert("Execution agent timeout")
			git.CommitWIP(i, "execution")
			appendRunLog(ticket, "exec-timeout", i, cycles, guardEvents, lastReviewerOutput, startedAt)
			return fmt.Errorf("execution timed out at iteration %d", i)
		}
		if execResult.ExitCode != 0 {
			_ = pw.WriteGuardTriggered(fmt.Sprintf("Execution failed (exit code %d)", execResult.ExitCode))
			ui.Error("Execution failed (code %d)", execResult.ExitCode)
			appendRunLog(ticket, "exec-failed", i, cycles, guardEvents, lastReviewerOutput, startedAt)
			return fmt.Errorf("execution agent failed at iteration %d", i)
		}

		gitDiff := git.Diff()
		_ = pw.WriteExecution(execResult.Output)

		g1 := guardState.CheckNoChanges(gitDiff, git.Head() != headBefore)
		if g1.Warning {
			_ = pw.WriteGuardAlert(g1.Message)
			ui.Warn("%s", g1.Message)
			guardEvents = append(guardEvents, g1.Message)
		}
		if g1.Triggered {
			_ = pw.WriteGuardTriggered(g1.Message)
			ui.Alert("%s", g1.Message)
			ui.Alert("Aborting.")
			_ = pw.WriteSummary("aborted — no changes", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
			guardEvents = append(guardEvents, g1.Message)
			appendRunLog(ticket, "aborted-no-changes", i, cycles, guardEvents, lastReviewerOutput, startedAt)
			return fmt.Errorf("guard triggered: %s", g1.Message)
		}

		elapsed := int64(time.Since(iterStart).Seconds())

		if shouldReview(i, cycles, reviewEvery) {
			// Why: re-read after execution so the reviewer sees the latest output.
			reviewProgressBytes, err := os.ReadFile(progressFile)
			if err != nil {
				return fmt.Errorf("could not read progress file before review at iteration %d: %w", i, err)
			}
			reviewProgressContent := lastNRuns(string(reviewProgressBytes), 2)

			detected := detect.FromGitDiff(gitDiff)
			reviewerPaths := selector.SelectReviewers(
				config.EffectiveReviewers(cfg),
				config.EffectiveReviewStrategy(cfg),
				metadataMap,
				detected,
				i, cycles,
			)

			// Fall back to legacy reviewer_agent if no reviewers configured.
			if len(reviewerPaths) == 0 && reviewerAgent != "" {
				reviewerPaths = []string{reviewerAgent}
			}

			approvals := 0
			var allReviewOutputs []string
			reviewerApprovals := map[string]bool{}

			for _, reviewerPath := range reviewerPaths {
				reviewerPath = config.ExpandPath(reviewerPath)
				output, approved, runErr := runReviewer(ctx, cfg, reviewerPath, string(planContent), reviewProgressContent, progressFile, timeout, retries, stream, pw, i)
				if runErr != nil {
					appendRunLog(ticket, "review-failed", i, cycles, guardEvents, lastReviewerOutput, startedAt)
					return runErr
				}
				allReviewOutputs = append(allReviewOutputs, output)
				_ = pw.WriteReviewerResult(reviewerPath, output)
				reviewerApprovals[reviewerPath] = approved
				if approved {
					approvals++
				}
			}

			lastReviewerOutput = strings.Join(allReviewOutputs, "\n\n---\n\n")

			g2 := guardState.CheckRepeatedIssues(lastReviewerOutput)
			if g2.Warning {
				_ = pw.WriteGuardAlert(g2.Message)
				ui.Warn("%s", g2.Message)
				guardEvents = append(guardEvents, g2.Message)
			}
			if g2.Triggered {
				_ = pw.WriteGuardTriggered(g2.Message)
				ui.Alert("%s", g2.Message)
				ui.Alert("Aborting.")
				_ = pw.WriteSummary("aborted — repeated issues", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
				guardEvents = append(guardEvents, g2.Message)
				appendRunLog(ticket, "aborted-repeated-issues", i, cycles, guardEvents, lastReviewerOutput, startedAt)
				return fmt.Errorf("guard triggered: %s", g2.Message)
			}

			elapsed = int64(time.Since(iterStart).Seconds())
			_ = pw.WriteIterationTime(elapsed)

			if err := git.CommitIteration(i, execResult.Output); err != nil {
				ui.Alert("Commit failed: %v", err)
			} else {
				ui.Phase("[%s] Committed iteration %d", time.Now().Format("15:04:05"), i)
			}

			// --- PERSIST STATE (best-effort) ---
			_ = looperstate.Write(looperstate.State{
				Ticket:            ticket,
				PlanFile:          planFile,
				CyclesTotal:       cycles,
				CycleCompleted:    i,
				ThrashCount:       guardState.ThrashCount,
				StuckCount:        guardState.StuckCount,
				PrevIssues:        guardState.PrevIssueHash,
				StartedAt:         startedAt,
				UpdatedAt:         time.Now().UTC(),
				ReviewerApprovals: reviewerApprovals,
			})

			threshold := config.EffectiveReviewStrategy(cfg).MajorityThreshold
			approved := selector.MajorityApproved(approvals, len(reviewerPaths), threshold)
			if approved {
				_ = pw.WriteSuccess(i)
				_ = pw.WriteSummary("complete", i, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(i))
				_ = looperstate.Delete(ticket)
				appendRunLog(ticket, "complete", i, cycles, guardEvents, lastReviewerOutput, startedAt)
				fmt.Println()
				ui.Success("👷 Job's done - completed in %d of %d iterations", i, cycles)
				return nil
			}
		} else {
			_ = pw.WriteIterationTime(elapsed)

			if err := git.CommitIteration(i, execResult.Output); err != nil {
				ui.Alert("Commit failed: %v", err)
			} else {
				ui.Phase("[%s] Committed iteration %d", time.Now().Format("15:04:05"), i)
			}

			// --- PERSIST STATE (best-effort) ---
			_ = looperstate.Write(looperstate.State{
				Ticket:         ticket,
				PlanFile:       planFile,
				CyclesTotal:    cycles,
				CycleCompleted: i,
				ThrashCount:    guardState.ThrashCount,
				StuckCount:     guardState.StuckCount,
				PrevIssues:     guardState.PrevIssueHash,
				StartedAt:      startedAt,
				UpdatedAt:      time.Now().UTC(),
			})
		}

		fmt.Println()
	}

	ui.Alert("Max cycles (%d) reached without approval", cycles)
	_ = pw.WriteSummary("max cycles reached", totalIterations, guardState.ThrashCount, guardState.StuckCount, git.RecentCommits(totalIterations))
	_ = looperstate.Delete(ticket)
	appendRunLog(ticket, "max-cycles", totalIterations, cycles, guardEvents, lastReviewerOutput, startedAt)

	return fmt.Errorf("max cycles (%d) reached without completion", cycles)
}

// runReviewer runs a single reviewer agent and returns (output, approved, err).
// err is non-nil only for hard failures (cancelled, timeout, exit code != 0).
func runReviewer(ctx context.Context, cfg config.Config, reviewerPath, planContent, progressContent, progressFile string, timeout, retries int, stream bool, pw *progress.Writer, iteration int) (string, bool, error) {
	reviewMsg := fmt.Sprintf("[%s] Reviewing (%s)...", time.Now().Format("15:04:05"), filepath.Base(reviewerPath))
	reviewPrompt := buildReviewPrompt(planContent, progressContent, reviewerPath)
	var reviewSpinner *ui.Spinner
	var reviewResult runner.Result
	if stream {
		fmt.Fprintln(os.Stderr, reviewMsg)
		reviewResult = runner.RunWithRetry(ctx, runner.RunStreamAsyncFn(os.Stdout), reviewPrompt, timeout, cfg.Backend, retries, "review", pw, ui.Warn)
	} else {
		reviewSpinner = ui.NewSpinner(reviewMsg)
		reviewSpinner.Start()
		reviewResult = runner.RunWithRetry(ctx, runner.RunAsyncFn(), reviewPrompt, timeout, cfg.Backend, retries, "review", pw, ui.Warn)
	}

	if reviewResult.Cancelled {
		if reviewSpinner != nil {
			reviewSpinner.Abort()
		}
		fmt.Println()
		ui.Alert("Interrupted — committing partial work")
		git.CommitWIP(iteration, "review")
		return "", false, fmt.Errorf("interrupted")
	}
	if reviewSpinner != nil {
		reviewSpinner.Stop()
	}
	if reviewResult.TimedOut {
		_ = pw.WriteGuardTriggered(fmt.Sprintf("Review timeout after %ds", timeout))
		ui.Alert("Review agent timeout")
		git.CommitWIP(iteration, "review")
		return "", false, fmt.Errorf("review timed out at iteration %d", iteration)
	}
	if reviewResult.ExitCode != 0 {
		_ = pw.WriteGuardTriggered(fmt.Sprintf("Review failed (exit code %d)", reviewResult.ExitCode))
		ui.Error("Review failed (code %d)", reviewResult.ExitCode)
		return "", false, fmt.Errorf("review agent failed at iteration %d", iteration)
	}

	jobsDoneRe := regexp.MustCompile(`(?i)job.*s\s+done`)
	approved := jobsDoneRe.MatchString(reviewResult.Output)
	return reviewResult.Output, approved, nil
}

// ensureProgressPath returns the progress file path under .looper/{ticket}/.
// Gotcha: MkdirAll is best-effort; WriteHeader catches real failures.
func ensureProgressPath(ticket string) string {
	dir := filepath.Join(".looper", ticket)
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, ticket+"_PROGRESS.md")
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

// lastNRuns returns a windowed view of content, keeping only the last n run
// sections. Sections are delimited by progress.RunSeparator (as written by
// progress.Writer.BeginRun). If n <= 0, n >= the number of runs, or there are
// no runs, the full content is returned unchanged. The header block (everything
// before the first separator) is always preserved.
func lastNRuns(content string, n int) string {
	sep := progress.RunSeparator
	parts := strings.Split(content, sep)
	// parts[0] is the header; parts[1:] are run bodies (number + content).
	runs := len(parts) - 1
	// Return full content when there is nothing to trim.
	if n <= 0 || runs == 0 || n >= runs {
		return content
	}
	return parts[0] + sep + strings.Join(parts[len(parts)-n:], sep)
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

Comment style (mandatory):
- Only add a comment when it provides non-obvious value
- Allowed prefixes: Why:, Invariant:, Gotcha:, Perf:, Ref: — followed by one specific sentence
- Remove any comment that restates what the code does, narrates control flow, or explains a name that could be improved by renaming
- If no comment is needed, write none

## OUTPUT FORMAT
When done, output ONLY a commit message summarising your changes — nothing else.
Format:
  <subject line>

  <optional bullet body>
Example:
  Add state persistence after each completed cycle

  - Write {TICKET}_STATE.json after every successful iteration
  - Delete state file on clean finish or max-cycles
Subject line: imperative mood, ≤72 chars, starts with Add/Fix/Refactor/Remove/Update
Do NOT commit. Do NOT output test results, status summaries, or narration.`, skillPath, planContent, historySection))
}

func buildReviewPrompt(planContent, progressContent, reviewerAgent string) string {
	return strings.TrimSpace(fmt.Sprintf(`Using the code reviewer subagent (%s)

Review the implementation against this plan:
`+"```"+`
%s
`+"```"+`

The loop history below contains the most recent iterations. The most recent ### Execution section is the current implementation to review.

%s

Provide your assessment. If the implementation looks good and meets the plan, start your response with: 👷‍♂️ Job's done!

If there are issues to address, start with: 🔧 Needs work and list the issues.`, reviewerAgent, planContent, progressContent))
}
