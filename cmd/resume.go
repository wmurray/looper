package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/git"
	"github.com/willmurray/looper/internal/guards"
	"github.com/willmurray/looper/internal/signals"
	looperstate "github.com/willmurray/looper/internal/state"
	"github.com/willmurray/looper/internal/ui"
)

// resumeCore is the injectable core of the resume command. readState and
// loopFn are injected so that unit tests need no filesystem or git repo.
func resumeCore(
	ticket string,
	readState func(string) (looperstate.State, error),
	loopFn func(startCycle int, g guards.State) error,
) error {
	s, err := readState(ticket)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no state file found for %s — nothing to resume", ticket)
	}
	if err != nil {
		return fmt.Errorf("reading state for %s: %w", ticket, err)
	}

	ui.Header("Resuming %s from cycle %d/%d", ticket, s.CycleCompleted+1, s.CyclesTotal)
	fmt.Println()

	g := guards.State{
		ThrashCount: s.ThrashCount,
		StuckCount:  s.StuckCount,
		PrevIssueHash: s.PrevIssues,
	}
	return loopFn(s.CycleCompleted+1, g)
}

var resumeCmd = &cobra.Command{
	Use:   "resume [TICKET]",
	Short: "Resume a loop from its last completed cycle",
	Long: `Resume the implement/review agent loop from the last completed cycle.

looper resume reads {TICKET}_STATE.json written after each completed cycle and
restarts the loop from cycle_completed+1, restoring guard counters so thrash
and stuck detection continue from where they left off.

If TICKET is omitted, it is inferred from the current branch name.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runResume,
}

func runResume(cmd *cobra.Command, args []string) error {
	cfg, _, _, err := config.LoadWithRepo()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Resolve ticket: explicit arg > branch inference.
	ticket := ""
	if len(args) == 1 {
		ticket = args[0]
	} else {
		ticketRe, err := regexp.Compile(cfg.TicketPattern)
		if err != nil {
			return fmt.Errorf("invalid ticket_pattern %q: %w", cfg.TicketPattern, err)
		}
		ticket = git.InferTicketFromBranch(ticketRe)
	}
	if ticket == "" {
		return fmt.Errorf("could not determine ticket — pass TICKET as an argument")
	}

	// Warn if skill files are missing — same as implement.
	skillPath := config.ExpandPath(cfg.SkillPath)
	reviewerAgent := config.ExpandPath(cfg.ReviewerAgent)
	if _, err := os.Stat(skillPath); err != nil {
		ui.Warn("skill_path not found: %s", skillPath)
	}
	if _, err := os.Stat(reviewerAgent); err != nil {
		ui.Warn("reviewer_agent not found: %s", reviewerAgent)
	}

	ctx, cancel := signals.WithInterrupt(context.Background())
	defer cancel()

	// capturedState is populated by the readState call inside resumeCore so
	// the loopFn closure can access planFile, cycles, and startedAt.
	var capturedState looperstate.State

	return resumeCore(
		ticket,
		func(t string) (looperstate.State, error) {
			s, err := looperstate.Read(t)
			capturedState = s
			return s, err
		},
		func(startCycle int, g guards.State) error {
			timeout := cfg.Defaults.Timeout
			retries := 0
			if cfg.Retries != nil {
				retries = *cfg.Retries
			}
			reviewEvery := 1
			if cfg.ReviewEvery != nil {
				reviewEvery = *cfg.ReviewEvery
			}
			gp := g
			return implementLoopFrom(ctx, cfg, capturedState.Ticket, capturedState.PlanFile,
				capturedState.CyclesTotal, timeout, retries, reviewEvery, false, startCycle, &gp, capturedState.StartedAt)
		},
	)
}
