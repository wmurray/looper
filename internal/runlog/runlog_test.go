package runlog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDir_DefaultsToLocalShare(t *testing.T) {
	os.Unsetenv("XDG_DATA_HOME")
	dir, err := dataDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "share", "looper")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestDataDir_UsesXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/testxdg")
	dir, err := dataDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/tmp/testxdg/looper"
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
}

func TestAppendAndReadAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	e1 := RunEntry{
		Ticket:          "IMP-1",
		StartedAt:       "2026-01-01T00:00:00Z",
		FinishedAt:      "2026-01-01T00:05:00Z",
		Outcome:         "complete",
		CyclesUsed:      2,
		CyclesMax:       5,
		GuardEvents:     []string{"no-changes warned"},
		LastReviewerMsg: "Job's done",
	}
	if err := Append(e1); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries, err := ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Ticket != "IMP-1" {
		t.Errorf("Ticket: got %q", entries[0].Ticket)
	}
	if entries[0].Outcome != "complete" {
		t.Errorf("Outcome: got %q", entries[0].Outcome)
	}
	if len(entries[0].GuardEvents) != 1 {
		t.Errorf("GuardEvents: got %v", entries[0].GuardEvents)
	}
}

func TestReadAll_MissingFileReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	entries, err := ReadAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("want empty slice, got %d entries", len(entries))
	}
}
