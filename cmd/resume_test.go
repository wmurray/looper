package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/willmurray/looper/internal/guards"
	looperstate "github.com/willmurray/looper/internal/state"
)

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
	loopCalled := false
	err := resumeCore("IMP-EXPLICIT",
		func(ticket string) (looperstate.State, error) {
			gotTicket = ticket
			return looperstate.State{
				Ticket:         ticket,
				PlanFile:       ticket + "_PLAN.md",
				CyclesTotal:    3,
				CycleCompleted: 1,
			}, nil
		},
		func(startCycle int, g guards.State) error {
			loopCalled = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTicket != "IMP-EXPLICIT" {
		t.Errorf("ticket = %q, want %q", gotTicket, "IMP-EXPLICIT")
	}
	if !loopCalled {
		t.Error("loop was not called")
	}
}

func TestResumeAlreadyComplete(t *testing.T) {
	t.Parallel()
	loopCalled := false
	err := resumeCore("IMP-99",
		func(ticket string) (looperstate.State, error) {
			return looperstate.State{
				Ticket:         "IMP-99",
				CyclesTotal:    3,
				CycleCompleted: 3,
			}, nil
		},
		func(startCycle int, g guards.State) error {
			loopCalled = true
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected error when loop already completed, got nil")
	}
	if loopCalled {
		t.Error("loop should not be called when all cycles already completed")
	}
}

func TestResumeAlreadyComplete(t *testing.T) {
	t.Parallel()
	loopCalled := false
	err := resumeCore("IMP-99",
		func(ticket string) (looperstate.State, error) {
			return looperstate.State{
				Ticket:         "IMP-99",
				CyclesTotal:    3,
				CycleCompleted: 3,
			}, nil
		},
		func(startCycle int, g guards.State) error {
			loopCalled = true
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected error when loop already completed, got nil")
	}
	if loopCalled {
		t.Error("loop should not be called when all cycles already completed")
	}
}
