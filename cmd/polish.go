package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/git"
	"github.com/willmurray/looper/internal/runner"
	"github.com/willmurray/looper/internal/signals"
	"github.com/willmurray/looper/internal/ui"
)

var polishCmd = newPolishCmd()

func newPolishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "polish",
		Short: "Run a post-implementation polish pass (lint + agent tidy) on the current branch",
		Long: `Run a post-implementation polish pass on the current branch.

Steps:
  1. Lint phase: run each command in polish_cmds (e.g. go fmt, go vet) and commit if changes.
  2. Agent phase: invoke a polish agent to tighten comments, remove debug artifacts, and fix style.

The command is idempotent: if neither phase produces changes, it exits 0 with "nothing to change".

Safety: never pushes, never changes branches, never rewrites history.`,
	}
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().Bool("dry-run", false, "Print resolved config without executing any external process")
	cmd.Flags().Int("timeout", 0, "Agent timeout in seconds (default from config)")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runPolish(c)
	}
	return cmd
}

func runPolish(cmd *cobra.Command) error {
	if err := git.AssertRepo(); err != nil {
		return err
	}
	if err := git.AssertClean(); err != nil {
		return err
	}

	cfg, _, _, err := config.LoadWithRepo()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	timeoutSecs := cfg.Defaults.Timeout
	if t, _ := cmd.Flags().GetInt("timeout"); t > 0 {
		timeoutSecs = t
	}

	agentPath := config.ExpandPath(resolvePolishAgent(cfg))

	// Warn (non-fatal) if agent path does not exist.
	if _, err := os.Stat(agentPath); err != nil {
		ui.Warn("polish agent not found: %s", agentPath)
		ui.Warn("Set it with: looper settings set polish_agent <path>")
	}

	// Infer ticket from branch name (non-fatal if absent).
	var ticket string
	if cfg.TicketPattern != "" {
		if re, err := regexp.Compile(cfg.TicketPattern); err == nil {
			ticket = git.InferTicketFromBranch(re)
		}
	}
	if ticket == "" {
		ui.Warn("No ticket found in branch name — polish will run without ticket context.")
	}

	// Warn if branch has no iteration work.
	if !git.HasIterationWork() {
		ui.Warn("No looper iteration commits found on this branch — polish may be running prematurely.")
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		fmt.Printf("looper polish — dry run\n\n")
		fmt.Printf("  Ticket:         %s\n", ticket)
		fmt.Printf("  Agent:          %s\n", agentPath)
		fmt.Printf("  Lint commands:  %s\n", strings.Join(cfg.PolishCmds, ", "))
		fmt.Printf("  Timeout:        %ds\n", timeoutSecs)
		fmt.Printf("  Backend:        %s\n", cfg.Backend)
		return nil
	}

	ctx, cancel := signals.WithInterrupt(context.Background())
	defer cancel()

	commitsMade := 0

	// --- LINT PHASE ---
	if len(cfg.PolishCmds) > 0 {
		ui.Phase("Lint phase — running formatters/linters")
		for _, lintCmd := range cfg.PolishCmds {
			c := exec.CommandContext(ctx, "sh", "-c", lintCmd)
			root, err := git.RepoRoot()
			if err == nil {
				c.Dir = root
			}
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				ui.Alert("Lint command failed: %s", lintCmd)
				return fmt.Errorf("lint command %q failed: %w", lintCmd, err)
			}
		}

		diff := git.Diff()
		status := git.StatusShort()
		if strings.TrimSpace(diff) != "" || strings.TrimSpace(status) != "" {
			if err := git.CommitPolish("Refactor: apply linters", ""); err != nil {
				return fmt.Errorf("lint commit failed: %w", err)
			}
			ui.Success("Lint phase: committed formatter changes.")
			commitsMade++
		} else {
			fmt.Println("Lint phase: nothing to format.")
		}
	}

	// --- AGENT POLISH PHASE ---
	ui.Iteration("Polish pass — agent review")

	headBefore := git.Head()
	prompt := buildPolishPrompt(agentPath)

	spinner := ui.NewSpinner(fmt.Sprintf("[%s] Polishing...", time.Now().Format("15:04:05")))
	spinner.Start()

	resultCh := runner.RunAsync(ctx, prompt, timeoutSecs, cfg.Backend)
	result := <-resultCh

	if result.Cancelled {
		spinner.Abort()
		fmt.Println()
		ui.Warn("Interrupted — no commit created.")
		return nil
	}

	if result.TimedOut {
		spinner.Abort()
		ui.Alert("Polish agent timed out after %ds", timeoutSecs)
		git.CommitWIP(0, "polish")
		return fmt.Errorf("polish agent timed out")
	}

	if result.Err != nil {
		spinner.Abort()
		return result.Err
	}

	if result.ExitCode != 0 {
		spinner.Abort()
		ui.Alert("Polish agent failed (exit code %d)", result.ExitCode)
		if result.Stderr != "" {
			fmt.Fprintln(os.Stderr, result.Stderr)
		}
		return fmt.Errorf("polish agent exited with code %d", result.ExitCode)
	}

	spinner.Stop()

	// Check for changes (diff or new HEAD commit from agent self-commit).
	diffAfter := git.Diff()
	headAfter := git.Head()
	statusAfter := git.StatusShort()
	if strings.TrimSpace(diffAfter) == "" && strings.TrimSpace(statusAfter) == "" && headAfter == headBefore {
		fmt.Println("Agent polish: nothing to change.")
	} else {
		subject := firstOutputLine(result.Output)
		body := remainingOutputLines(result.Output)
		if err := git.CommitPolish(subject, body); err != nil {
			return fmt.Errorf("agent polish commit failed: %w", err)
		}
		ui.Success("Agent polish: committed — %s", subject)
		commitsMade++
	}

	if commitsMade > 0 {
		ui.Success("Polish complete — %d commit(s) made.", commitsMade)
	} else {
		fmt.Println("Polish complete — nothing to change.")
	}
	return nil
}

// resolvePolishAgent returns the effective agent path for the polish pass.
// Falls back to cfg.ReviewerAgent when PolishAgent is empty.
func resolvePolishAgent(cfg config.Config) string {
	if cfg.PolishAgent != "" {
		return cfg.PolishAgent
	}
	return cfg.ReviewerAgent
}

// buildPolishPrompt constructs the inline prompt sent to the polish agent.
func buildPolishPrompt(agentPath string) string {
	return strings.TrimSpace(fmt.Sprintf(`You are performing a post-implementation polish pass on this branch.

Your reviewer agent is: %s

Your task:
- Remove debug artifacts (fmt.Println, console.log, TODO comments left by the implementer, commented-out dead code)
- Tighten comments: ensure exported identifiers have godoc, remove redundant inline comments
- Fix minor style inconsistencies not caught by the linter
- Ensure consistency across files touched in this branch

IMPORTANT CONSTRAINTS:
- Do NOT add new features or change behaviour
- Do NOT refactor anything that is not already inconsistent
- Do NOT change public APIs or function signatures
- Run `+"`go build ./...`"+` and `+"`go test ./...`"+` and confirm they pass before finishing
- If there is nothing meaningful to change, do nothing — the command is idempotent

Describe your changes in an imperative commit message (first line ≤ 72 chars, body optional).`, agentPath))
}

// firstOutputLine returns the first non-empty line of the agent output.
func firstOutputLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// remainingOutputLines returns everything after the first non-empty line.
func remainingOutputLines(output string) string {
	lines := strings.Split(output, "\n")
	first := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			first = i
			break
		}
	}
	if first < 0 || first+1 >= len(lines) {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[first+1:], "\n"))
}
