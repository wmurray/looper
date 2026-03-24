package state_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/willmurray/looper/internal/state"
)

// inTempDir runs f with the working directory set to a fresh temp directory.
// Gotcha: os.Chdir is process-wide; tests using this must not run in parallel.
func inTempDir(t *testing.T, f func()) {
	t.Helper()
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir %s: %v", tmp, err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	f()
}

func TestPath(t *testing.T) {
	t.Parallel()
	got := state.Path("IMP-6")
	want := "IMP-6_STATE.json"
	if got != want {
		t.Errorf("Path(%q) = %q, want %q", "IMP-6", got, want)
	}
}

func TestNewPath(t *testing.T) {
	t.Parallel()
	got := state.NewPath("IMP-34")
	want := filepath.Join(".looper", "IMP-34", "IMP-34_STATE.json")
	if got != want {
		t.Errorf("NewPath(%q) = %q, want %q", "IMP-34", got, want)
	}
}

func TestWriteCreatesDirectory(t *testing.T) {
	inTempDir(t, func() {
		ticket := "WDIR-1"
		s := state.State{Ticket: ticket, CyclesTotal: 2, CycleCompleted: 1}
		if err := state.Write(s); err != nil {
			t.Fatalf("Write: %v", err)
		}
		want := state.NewPath(ticket)
		if _, err := os.Stat(want); err != nil {
			t.Errorf("Write did not create file at %s: %v", want, err)
		}
		// Legacy path must not be written.
		if _, err := os.Stat(state.Path(ticket)); err == nil {
			t.Errorf("Write must not write to legacy path %s", state.Path(ticket))
		}
	})
}

func TestReadFallback(t *testing.T) {
	inTempDir(t, func() {
		ticket := "FALL-1"
		s := state.State{Ticket: ticket, CyclesTotal: 3, CycleCompleted: 1}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// Write only to legacy path (no .looper/ dir).
		if err := os.WriteFile(state.Path(ticket), data, 0o644); err != nil {
			t.Fatalf("WriteFile legacy: %v", err)
		}

		got, err := state.Read(ticket)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if got.Ticket != ticket || got.CycleCompleted != 1 {
			t.Errorf("fallback read mismatch: %+v", got)
		}
	})
}

func TestReadPrefersNewPath(t *testing.T) {
	inTempDir(t, func() {
		ticket := "PREF-1"

		// Legacy path has cycle 1, new path has cycle 2.
		legacy := state.State{Ticket: ticket, CyclesTotal: 3, CycleCompleted: 1}
		legacyData, _ := json.Marshal(legacy)
		if err := os.WriteFile(state.Path(ticket), legacyData, 0o644); err != nil {
			t.Fatalf("write legacy: %v", err)
		}

		newS := state.State{Ticket: ticket, CyclesTotal: 3, CycleCompleted: 2}
		newData, _ := json.Marshal(newS)
		if err := os.MkdirAll(filepath.Dir(state.NewPath(ticket)), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(state.NewPath(ticket), newData, 0o644); err != nil {
			t.Fatalf("write new: %v", err)
		}

		got, err := state.Read(ticket)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if got.CycleCompleted != 2 {
			t.Errorf("Read should prefer NewPath: got CycleCompleted=%d, want 2", got.CycleCompleted)
		}
	})
}

func TestWriteRead_RoundTrip(t *testing.T) {
	inTempDir(t, func() {
		ticket := "RT-1"
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
			ReviewerApprovals: map[string]bool{
				"/abs/reviewers/general.md": true,
				"/abs/reviewers/spec.md":    false,
			},
		}

		if err := state.Write(in); err != nil {
			t.Fatalf("Write: %v", err)
		}

		got, err := state.Read(ticket)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}

		if got.Ticket != in.Ticket || got.PlanFile != in.PlanFile ||
			got.CyclesTotal != in.CyclesTotal || got.CycleCompleted != in.CycleCompleted ||
			got.ThrashCount != in.ThrashCount || got.StuckCount != in.StuckCount ||
			got.PrevIssues != in.PrevIssues ||
			!got.StartedAt.Equal(in.StartedAt) || !got.UpdatedAt.Equal(in.UpdatedAt) {
			t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, in)
		}
		if len(got.ReviewerApprovals) != 2 ||
			!got.ReviewerApprovals["/abs/reviewers/general.md"] ||
			got.ReviewerApprovals["/abs/reviewers/spec.md"] {
			t.Errorf("ReviewerApprovals round-trip mismatch: got %v", got.ReviewerApprovals)
		}
	})
}

func TestRead_MissingFile(t *testing.T) {
	inTempDir(t, func() {
		_, err := state.Read("NOTEXIST-99")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected os.ErrNotExist, got: %v", err)
		}
	})
}

func TestDelete_BothPaths(t *testing.T) {
	inTempDir(t, func() {
		ticket := "BOTH-1"
		s := state.State{Ticket: ticket, CyclesTotal: 3, CycleCompleted: 1}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(state.Path(ticket), data, 0o644); err != nil {
			t.Fatalf("write legacy: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(state.NewPath(ticket)), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(state.NewPath(ticket), data, 0o644); err != nil {
			t.Fatalf("write new: %v", err)
		}

		if err := state.Delete(ticket); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := os.Stat(state.Path(ticket)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("legacy path still exists after Delete")
		}
		if _, err := os.Stat(state.NewPath(ticket)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("new path still exists after Delete")
		}
	})
}

func TestDelete_MissingFile(t *testing.T) {
	inTempDir(t, func() {
		if err := state.Delete("NOTEXIST-99"); err != nil {
			t.Errorf("Delete on missing file: want nil, got %v", err)
		}
	})
}

func TestDelete_ErrorIncludesPath(t *testing.T) {
	inTempDir(t, func() {
		ticket := "ERRPATH-1"
		// Create new-path as a non-empty directory so os.Remove fails with ENOTEMPTY.
		stateDir := state.NewPath(ticket)
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		sentinel := filepath.Join(stateDir, "sentinel")
		if err := os.WriteFile(sentinel, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile sentinel: %v", err)
		}
		err := state.Delete(ticket)
		if err == nil {
			t.Fatal("expected error when state path is a non-empty directory, got nil")
		}
		// Invariant: error message must include the path that failed, not just a generic os error.
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			if pathErr.Path != state.NewPath(ticket) {
				t.Errorf("PathError.Path = %q, want %q", pathErr.Path, state.NewPath(ticket))
			}
		} else {
			t.Errorf("expected wrapped *os.PathError, got: %T %v", err, err)
		}
	})
}

func TestWriteRead_ReviewerApprovals(t *testing.T) {
	inTempDir(t, func() {
		ticket := "RA-1"
		in := state.State{
			Ticket:         ticket,
			CyclesTotal:    3,
			CycleCompleted: 1,
			ReviewerApprovals: map[string]bool{
				"/abs/path/general.md": true,
				"/abs/path/spec.md":    false,
			},
		}
		if err := state.Write(in); err != nil {
			t.Fatalf("Write: %v", err)
		}
		got, err := state.Read(ticket)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(got.ReviewerApprovals) != 2 {
			t.Fatalf("ReviewerApprovals len = %d, want 2", len(got.ReviewerApprovals))
		}
		if !got.ReviewerApprovals["/abs/path/general.md"] {
			t.Errorf("general.md should be approved")
		}
		if got.ReviewerApprovals["/abs/path/spec.md"] {
			t.Errorf("spec.md should not be approved")
		}
	})
}

func TestWrite_Idempotent(t *testing.T) {
	inTempDir(t, func() {
		ticket := "IDEM-1"
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
	})
}
