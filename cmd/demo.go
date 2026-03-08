package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/ui"
)

var demoCmd = &cobra.Command{
	Use:    "demo",
	Short:  "Preview terminal output styles",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println()

		// --- Startup ---
		ui.Header("Starting loop: DX-123")
		ui.Header("Max cycles: 5 | Timeout per iteration: 420s | Backend: cursor")
		fmt.Println()

		// --- Iteration header ---
		ui.Iteration("=== Iteration 1 of 5 ===")
		fmt.Println()

		// --- Spinner: completes normally ---
		s1 := ui.NewSpinner("[15:04:05] Executing plan...")
		s1.Start()
		time.Sleep(2 * time.Second)
		s1.Stop()

		s2 := ui.NewSpinner("[15:04:07] Reviewing...")
		s2.Start()
		time.Sleep(2 * time.Second)
		s2.Stop()

		ui.Phase("[15:04:09] Committed iteration 1")
		fmt.Println()

		// --- Iteration 2: guard warning ---
		ui.Iteration("=== Iteration 2 of 5 ===")
		fmt.Println()

		s3 := ui.NewSpinner("[15:04:10] Executing plan...")
		s3.Start()
		time.Sleep(2 * time.Second)
		s3.Stop()

		ui.Warn("No changes detected (1/2 before abort)")
		fmt.Println()

		// --- Spinner: aborted by Ctrl+C ---
		ui.Iteration("=== Iteration 3 of 5 ===")
		fmt.Println()

		s4 := ui.NewSpinner("[15:04:12] Executing plan...")
		s4.Start()
		time.Sleep(2 * time.Second)
		s4.Abort()

		fmt.Println()
		ui.Alert("Interrupted — committing partial work")
		fmt.Println()

		// --- Guard trigger ---
		ui.Alert("No changes in 2 consecutive iterations — agent appears stuck")
		ui.Alert("Aborting.")
		fmt.Println()

		// --- Errors ---
		ui.Error("Execution failed (code 1)")
		ui.Error("Review agent timeout")
		fmt.Println()

		// --- Max cycles ---
		ui.Alert("Max cycles (5) reached without approval")
		fmt.Println()

		// --- Success ---
		ui.Success("👷 Job's done - completed in 3 of 5 iterations")
		fmt.Println()

		// --- Git staging warning ---
		fmt.Println("--- git staging warning preview ---")
		confirmGitStaging("/Users/willmurray/Projects/my-app") //nolint
	},
}

func init() {
	rootCmd.AddCommand(demoCmd)
}
