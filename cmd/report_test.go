package cmd

import (
	"strings"
	"testing"

	"github.com/willmurray/looper/internal/runlog"
)

func TestFormatReport_EmptyEntries(t *testing.T) {
	out := formatReport(nil, "", 20)
	if !strings.Contains(out, "No runs") {
		t.Errorf("expected 'No runs' in output, got: %q", out)
	}
}

func TestFormatReport_SingleEntry(t *testing.T) {
	entries := []runlog.RunEntry{
		{
			Ticket:      "IMP-1",
			StartedAt:   "2026-01-01T00:00:00Z",
			FinishedAt:  "2026-01-01T00:05:00Z",
			Outcome:     "complete",
			CyclesUsed:  2,
			CyclesMax:   5,
			GuardEvents: nil,
		},
	}
	out := formatReport(entries, "", 20)
	if !strings.Contains(out, "IMP-1") {
		t.Errorf("expected ticket in output, got: %q", out)
	}
	if !strings.Contains(out, "complete") {
		t.Errorf("expected outcome in output, got: %q", out)
	}
	if !strings.Contains(out, "2/5") {
		t.Errorf("expected cycles ratio in output, got: %q", out)
	}
}

func TestFormatReport_FilterByTicket(t *testing.T) {
	entries := []runlog.RunEntry{
		{Ticket: "IMP-1", Outcome: "complete", CyclesUsed: 1, CyclesMax: 5},
		{Ticket: "IMP-2", Outcome: "max-cycles", CyclesUsed: 5, CyclesMax: 5},
	}
	out := formatReport(entries, "IMP-1", 20)
	if !strings.Contains(out, "IMP-1") {
		t.Errorf("expected IMP-1 in output")
	}
	if strings.Contains(out, "IMP-2") {
		t.Errorf("IMP-2 should be filtered out")
	}
}

func TestFormatReport_LastLimit(t *testing.T) {
	var entries []runlog.RunEntry
	for i := 0; i < 5; i++ {
		entries = append(entries, runlog.RunEntry{Ticket: "IMP-1", Outcome: "complete", CyclesUsed: 1, CyclesMax: 5})
	}
	out := formatReport(entries, "", 3)
	// Count rows — each entry appears as a line with the ticket name
	count := strings.Count(out, "IMP-1")
	if count != 3 {
		t.Errorf("expected 3 rows, got %d rows in:\n%s", count, out)
	}
}

func TestFormatReport_Footer(t *testing.T) {
	entries := []runlog.RunEntry{
		{Ticket: "IMP-1", Outcome: "complete", CyclesUsed: 2, CyclesMax: 5, GuardEvents: []string{"g1"}},
		{Ticket: "IMP-2", Outcome: "max-cycles", CyclesUsed: 5, CyclesMax: 5, GuardEvents: nil},
	}
	out := formatReport(entries, "", 20)
	// avg cycles: (2+5)/2 = 3.5
	if !strings.Contains(out, "3.5") {
		t.Errorf("expected avg cycles 3.5 in footer, got:\n%s", out)
	}
	// success rate: 1/2 = 50%
	if !strings.Contains(out, "50") {
		t.Errorf("expected 50%% success rate in footer, got:\n%s", out)
	}
	// total guard events: 1
	if !strings.Contains(out, "1") {
		t.Errorf("expected guard event count in footer, got:\n%s", out)
	}
}
