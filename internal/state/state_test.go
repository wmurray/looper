package state_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/willmurray/looper/internal/state"
)

func TestPath(t *testing.T) {
	t.Parallel()
	got := state.Path("IMP-6")
	want := "IMP-6_STATE.json"
	if got != want {
		t.Errorf("Path(%q) = %q, want %q", "IMP-6", got, want)
	}
}

func TestWriteRead_RoundTrip(t *testing.T) {
	t.Parallel()
	ticket := "RT-1"
	t.Cleanup(func() { os.Remove(state.Path(ticket)) })

	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	in := state.State{
		Ticket:         ticket,
		PlanFile:       "RT-1_PLAN.md",
		CyclesTotal:    5,
		CycleCompleted: 2,
		ThrashCount:    1,
		StuckCount:     1,
		PrevIssues:     "bug,error",
		StartedAt:      now,
		UpdatedAt:      now.Add(time.Minute),
	}

	if err := state.Write(in); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := state.Read(ticket)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got != in {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, in)
	}
}

func TestRead_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := state.Read("NOTEXIST-99")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got: %v", err)
	}
}

func TestDelete_MissingFile(t *testing.T) {
	t.Parallel()
	if err := state.Delete("NOTEXIST-99"); err != nil {
		t.Errorf("Delete on missing file: want nil, got %v", err)
	}
}

func TestWrite_Idempotent(t *testing.T) {
	t.Parallel()
	ticket := "IDEM-1"
	t.Cleanup(func() { os.Remove(state.Path(ticket)) })

	s := state.State{Ticket: ticket, CyclesTotal: 3, CycleCompleted: 1}
	if err := state.Write(s); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	s.CycleCompleted = 2
	if err := state.Write(s); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	got, err := state.Read(ticket)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.CycleCompleted != 2 {
		t.Errorf("after second write: CycleCompleted = %d, want 2", got.CycleCompleted)
	}
}
