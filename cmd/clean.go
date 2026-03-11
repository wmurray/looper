package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/ui"
)

var cleanGlobs = []string{"*_PLAN.md", "*_PROGRESS.md", "*_STATE.json"}

var cleanCmd = newCleanCmd()

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove looper working files (*_PLAN.md, *_PROGRESS.md, *_STATE.json)",
		Long: `Remove looper-generated working files from the current directory.

Matches:
  *_PLAN.md
  *_PROGRESS.md
  *_STATE.json

These files are working state, not source code. Run after a ticket is done.`,
		Args: cobra.NoArgs,
	}
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runClean(c, args, ".")
	}
	return cmd
}

func runClean(cmd *cobra.Command, args []string, dir string) error {
	var files []string
	for _, pattern := range cleanGlobs {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return fmt.Errorf("glob %q: %w", pattern, err)
		}
		files = append(files, matches...)
	}

	out := cmd.OutOrStdout()

	if len(files) == 0 {
		fmt.Fprintln(out, "Nothing to clean.")
		return nil
	}

	for _, f := range files {
		fmt.Fprintf(out, "  %s\n", f)
	}

	yesFlag, _ := cmd.Flags().GetBool("yes")
	if !yesFlag {
		fmt.Fprintf(out, "\nRemove %d file(s)? [y/N] ", len(files))
		scanner := bufio.NewScanner(cmd.InOrStdin())
		if !scanner.Scan() {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	var errs []string
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("some files could not be removed:\n%s", strings.Join(errs, "\n"))
	}

	ui.Success("Removed %d file(s).", len(files))
	return nil
}
