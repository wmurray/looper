package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/runlog"
)

var (
	flagReportLast   int
	flagReportTicket string
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show a summary of recent looper runs",
	RunE:  runReport,
}

func init() {
	reportCmd.Flags().IntVar(&flagReportLast, "last", 20, "Number of most recent runs to show")
	reportCmd.Flags().StringVar(&flagReportTicket, "ticket", "", "Filter by ticket ID")
}

func runReport(_ *cobra.Command, _ []string) error {
	entries, err := runlog.ReadAll()
	if err != nil {
		return fmt.Errorf("reading run log: %w", err)
	}
	fmt.Print(formatReport(entries, flagReportTicket, flagReportLast))
	return nil
}

// formatReport builds the report string from entries. Exported for testing.
func formatReport(entries []runlog.RunEntry, ticket string, last int) string {
	// Filter by ticket
	filtered := entries[:0:0]
	for _, e := range entries {
		if ticket != "" && e.Ticket != ticket {
			continue
		}
		filtered = append(filtered, e)
	}

	// Take the last N
	if len(filtered) > last {
		filtered = filtered[len(filtered)-last:]
	}

	if len(filtered) == 0 {
		return "No runs found.\n"
	}

	var sb strings.Builder

	// Header
	fmt.Fprintf(&sb, "%-12s  %-24s  %-22s  %-6s  %-5s  %s\n",
		"TICKET", "STARTED", "OUTCOME", "CYCLES", "GUARD", "LAST REVIEW")
	fmt.Fprintf(&sb, "%s\n", strings.Repeat("-", 100))

	var totalCycles int
	var successCount int
	var totalGuard int

	for _, e := range filtered {
		started := e.StartedAt
		if len(started) > 19 {
			started = started[:19]
		}
		cyclesRatio := fmt.Sprintf("%d/%d", e.CyclesUsed, e.CyclesMax)
		guardCount := len(e.GuardEvents)
		totalGuard += guardCount
		totalCycles += e.CyclesUsed
		if e.Outcome == "complete" {
			successCount++
		}
		reviewSnippet := e.LastReviewerMsg
		if len(reviewSnippet) > 30 {
			reviewSnippet = reviewSnippet[:27] + "..."
		}
		fmt.Fprintf(&sb, "%-12s  %-24s  %-22s  %-6s  %-5d  %s\n",
			e.Ticket, started, e.Outcome, cyclesRatio, guardCount, reviewSnippet)
	}

	// Footer
	n := len(filtered)
	avgCycles := float64(totalCycles) / float64(n)
	successRate := float64(successCount) / float64(n) * 100
	fmt.Fprintf(&sb, "%s\n", strings.Repeat("-", 100))
	fmt.Fprintf(&sb, "Runs: %d  |  Avg cycles: %.1f  |  Success rate: %.0f%%  |  Total guard events: %d\n",
		n, avgCycles, successRate, totalGuard)

	return sb.String()
}
