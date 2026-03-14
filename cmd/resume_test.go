package cmd

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/willmurray/looper/internal/guards"
	looperstate "github.com/willmurray/looper/internal/state"
)

// resolveResumeState takes injected predicates so tests need no git repo or filesystem.

func TestResolveResumeState_HasIterations(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return true }, // hasWork: loop already ran
		func() error { return nil }, // plan exists (irrelevant when work found)
	)
	if state != resumeHasIterations {
		t.Errorf("got %v, want resumeHasIterations", state)
	}
}

func TestResolveResumeState_PlanExists(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return false }, // no iteration work
		func() error { return nil },  // plan file exists on disk
	)
	if state != resumePlanExists {
		t.Errorf("got %v, want resumePlanExists", state)
	}
}

func TestResolveResumeState_NoPlan(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return false },    // no iteration work
		func() error { return errors.New("not found") }, // no plan file
	)
	if state != resumeNoPlan {
		t.Errorf("got %v, want resumeNoPlan", state)
	}
}

// HasIterationWork takes priority over plan-file presence.
func TestResolveResumeState_IterationsTakesPriority(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return true },                // work found
		func() error { return errors.New("gone") }, // plan missing — shouldn't matter
	)
	if state != resumeHasIterations {
		t.Errorf("got %v, want resumeHasIterations even when plan missing", state)
	}
}

func TestResumeNoStateFile(t *testing.T) {
	t.Parallel()
	loopCalled := false
	err := resumeCore("IMP-99",
		func(ticket string) (looperstate.State, error) {
			return looperstate.State{}, fmt.Errorf("missing: %w", os.ErrNotExist)
		},
		func(startCycle int, g guards.State) error {
			loopCalled = true
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected error for missing state file, got nil")
	}
	if loopCalled {
		t.Error("loop should not be called when state file is missing")
	}
}

func TestResumeRestoresGuardState(t *testing.T) {
	t.Parallel()
	var gotGuards guards.State
	err := resumeCore("IMP-7",
		func(ticket string) (looperstate.State, error) {
			return looperstate.State{
				Ticket:         "IMP-7",
				PlanFile:       "IMP-7_PLAN.md",
				CyclesTotal:    5,
				CycleCompleted: 1,
				ThrashCount:    1,
				StuckCount:     1,
				PrevIssues:     "bug,error",
			}, nil
		},
		func(startCycle int, g guards.State) error {
			gotGuards = g
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := guards.State{ThrashCount: 1, StuckCount: 1, PrevIssueHash: "bug,error"}
	if gotGuards != want {
		t.Errorf("guards mismatch:\n got  %+v\n want %+v", gotGuards, want)
	}
}

func TestResumeStartCycle(t *testing.T) {
	t.Parallel()
	var gotStartCycle int
	_ = resumeCore("IMP-8",
		func(ticket string) (looperstate.State, error) {
			return looperstate.State{
				Ticket:         "IMP-8",
				CyclesTotal:    5,
				CycleCompleted: 2,
			}, nil
		},
		func(startCycle int, g guards.State) error {
			gotStartCycle = startCycle
			return nil
		},
	)
	if gotStartCycle != 3 {
		t.Errorf("startCycle = %d, want 3", gotStartCycle)
	}
}

func TestResumeTicketFromArg(t *testing.T) {
	t.Parallel()
	var gotTicket string
	_ = resumeCore("IMP-EXPLICIT",
		func(ticket string) (looperstate.State, error) {
			gotTicket = ticket
			return looperstate.State{Ticket: ticket}, nil
		},
		func(startCycle int, g guards.State) error { return nil },
	)
	if gotTicket != "IMP-EXPLICIT" {
		t.Errorf("ticket = %q, want %q", gotTicket, "IMP-EXPLICIT")
	}
}
