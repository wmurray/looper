package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willmurray/looper/internal/discover"
	"github.com/willmurray/looper/internal/ui"
)

// Gotcha: strict pattern — 1-4-letter prefix OR letter-prefix with hyphen-digits. Rejects words like DATABASE.
var ticketFileRe = regexp.MustCompile(`^([A-Z]{1,4}(?:-[0-9]+)?|[A-Z][A-Z0-9]*-[0-9]+)_`)

type initOptions struct {
	Yes           bool
	SkipGitignore bool
	ConfigOnly    bool
	SkipConfig    bool
	DryRun        bool
	Migrate       bool
}

type repoConfigDefaults struct {
	Cycles  int `json:"cycles"`
	Timeout int `json:"timeout"`
}

type repoConfigReviewers struct {
	General string `json:"general"`
}

type repoConfig struct {
	Defaults  repoConfigDefaults  `json:"defaults"`
	Reviewers repoConfigReviewers `json:"reviewers"`
	Stack     string              `json:"stack,omitempty"`
}

var initCmd = newInitCmd()

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize repository for looper",
		Long: `Initialize a repository for looper by creating a .looper/ directory structure,
configuring .gitignore, and optionally creating a .looper.json config file.

This command:
  - Creates .looper/ directory at repository root
  - Updates .gitignore to exclude looper files
  - Detects project stack and suggests configuration
  - Creates optional .looper.json for repo-specific settings

The command is idempotent and safe to run multiple times.`,
		Args: cobra.NoArgs,
	}

	cmd.Flags().BoolP("yes", "y", false, "Auto-accept all defaults without prompts")
	cmd.Flags().Bool("skip-gitignore", false, "Skip .gitignore configuration")
	cmd.Flags().Bool("config-only", false, "Only create .looper.json, skip directory and gitignore setup")
	cmd.Flags().Bool("skip-config", false, "Skip .looper.json creation")
	cmd.Flags().Bool("dry-run", false, "Preview changes without applying them")
	cmd.Flags().Bool("migrate", false, "Migrate existing root-level files to .looper/ structure")

	cmd.RunE = func(c *cobra.Command, args []string) error {
		yes, _ := c.Flags().GetBool("yes")
		skipGitignore, _ := c.Flags().GetBool("skip-gitignore")
		configOnly, _ := c.Flags().GetBool("config-only")
		skipConfig, _ := c.Flags().GetBool("skip-config")
		dryRun, _ := c.Flags().GetBool("dry-run")
		migrate, _ := c.Flags().GetBool("migrate")

		return runInit(c, ".", initOptions{
			Yes:           yes,
			SkipGitignore: skipGitignore,
			ConfigOnly:    configOnly,
			SkipConfig:    skipConfig,
			DryRun:        dryRun,
			Migrate:       migrate,
		})
	}

	return cmd
}

func runInit(cmd *cobra.Command, repoRoot string, opts initOptions) error {
	out := cmd.OutOrStdout()

	if opts.ConfigOnly && opts.SkipConfig {
		return fmt.Errorf("cannot use --config-only and --skip-config together")
	}

	if opts.DryRun {
		fmt.Fprintln(out, "→ DRY RUN: no changes will be applied")
		fmt.Fprintln(out)
	}

	if !opts.ConfigOnly {
		if err := createLooperDir(out, repoRoot, opts.DryRun); err != nil {
			return err
		}
	}

	if !opts.ConfigOnly && !opts.SkipGitignore {
		if err := setupGitignore(cmd, out, repoRoot, opts.Yes, opts.DryRun); err != nil {
			return err
		}
	}

	if !opts.SkipConfig {
		if err := createLooperConfig(cmd, out, repoRoot, opts.Yes, opts.DryRun); err != nil {
			return err
		}
	}

	if opts.Migrate {
		if err := migrateRootFiles(cmd, out, repoRoot, opts.Yes, opts.DryRun); err != nil {
			return err
		}
	}

	if !opts.DryRun {
		verifyAndGuide(out, repoRoot)
	}

	if opts.DryRun {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "✓ DRY RUN complete. No files were modified.")
	} else {
		fmt.Fprintln(out)
		ui.Success("Repository initialized for looper!")
	}

	return nil
}

func createLooperDir(out io.Writer, repoRoot string, dryRun bool) error {
	looperDir := filepath.Join(repoRoot, ".looper")

	if _, err := os.Stat(looperDir); err == nil {
		fmt.Fprintln(out, "✓ .looper/ directory already exists")
		return nil
	}

	if dryRun {
		fmt.Fprintln(out, "→ Would create .looper/ directory")
		return nil
	}

	if err := os.MkdirAll(looperDir, 0755); err != nil {
		return fmt.Errorf("failed to create .looper directory (check write permissions): %w", err)
	}

	fmt.Fprintln(out, "✓ Created .looper/ directory")
	return nil
}

func setupGitignore(cmd *cobra.Command, out io.Writer, repoRoot string, yes, dryRun bool) error {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	looperPattern := ".looper/"

	var content []byte

	if _, statErr := os.Stat(gitignorePath); statErr == nil {
		var readErr error
		content, readErr = os.ReadFile(gitignorePath)
		if readErr != nil {
			return fmt.Errorf("failed to read .gitignore (check file permissions): %w", readErr)
		}

		if strings.Contains(string(content), looperPattern) {
			fmt.Fprintln(out, "✓ .gitignore already contains .looper/ pattern")
			return nil
		}
	}

	newContent := string(content)
	if !strings.HasSuffix(newContent, "\n") && newContent != "" {
		newContent += "\n"
	}
	newContent += "# looper-cli generated files\n.looper/\n"

	if !yes && !dryRun {
		fmt.Fprintln(out, "\n.gitignore changes:")
		fmt.Fprintln(out, "  + # looper-cli generated files")
		fmt.Fprintln(out, "  + .looper/")
		fmt.Fprint(out, "\nApply changes? [y/N] ")

		scanner := bufio.NewScanner(cmd.InOrStdin())
		if !scanner.Scan() {
			fmt.Fprintln(out, "Skipped.")
			return nil
		}

		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Skipped.")
			return nil
		}
	}

	if dryRun {
		fmt.Fprintln(out, "→ Would update .gitignore with .looper/ pattern")
		return nil
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore (check write permissions): %w", err)
	}

	fmt.Fprintln(out, "✓ Updated .gitignore")
	return nil
}

func createLooperConfig(cmd *cobra.Command, out io.Writer, repoRoot string, yes, dryRun bool) error {
	configPath := filepath.Join(repoRoot, ".looper.json")

	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintln(out, "✓ .looper.json already exists")
		return nil
	}

	stack := getStackDescription(repoRoot)

	cfg := repoConfig{
		Defaults: repoConfigDefaults{
			Cycles:  5,
			Timeout: 420,
		},
		Reviewers: repoConfigReviewers{
			General: "",
		},
		Stack: stack,
	}

	if !yes && !dryRun {
		if stack != "" {
			fmt.Fprintf(out, "\nProject stack detected: %s\n", stack)
		}
		fmt.Fprint(out, "Create .looper.json with defaults? [y/N] ")

		scanner := bufio.NewScanner(cmd.InOrStdin())
		if !scanner.Scan() {
			fmt.Fprintln(out, "Skipped.")
			return nil
		}

		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Skipped.")
			return nil
		}
	}

	if dryRun {
		fmt.Fprintln(out, "→ Would create .looper.json")
		return nil
	}

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write .looper.json (check write permissions): %w", err)
	}

	fmt.Fprintln(out, "✓ Created .looper.json")
	return nil
}

func getStackDescription(repoRoot string) string {
	var names []string
	seen := make(map[string]bool)
	for _, ind := range stackIndicators {
		if _, err := os.Stat(filepath.Join(repoRoot, ind.file)); err == nil {
			if !seen[ind.displayName] {
				names = append(names, ind.displayName)
				seen[ind.displayName] = true
			}
		}
	}
	return strings.Join(names, " + ")
}

func findMigrationCandidates(repoRoot string) []string {
	patterns := []string{
		"*_PLAN.md",
		"*_PROGRESS.md",
		"*_STATE.json",
	}

	var candidates []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(repoRoot, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			base := filepath.Base(match)
			if ticketFileRe.MatchString(base) && !seen[base] {
				candidates = append(candidates, base)
				seen[base] = true
			}
		}
	}

	return candidates
}

func migrateRootFiles(cmd *cobra.Command, out io.Writer, repoRoot string, yes, dryRun bool) error {
	candidates := findMigrationCandidates(repoRoot)
	if len(candidates) == 0 {
		fmt.Fprintln(out, "✓ No root-level looper files found to migrate")
		return nil
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Found %d file(s) to migrate:\n", len(candidates))
	for _, f := range candidates {
		fmt.Fprintf(out, "  • %s\n", f)
	}

	if !yes && !dryRun {
		fmt.Fprint(out, "\nMigrate these files to .looper/:ticket/? [y/N] ")
		scanner := bufio.NewScanner(cmd.InOrStdin())
		if !scanner.Scan() {
			fmt.Fprintln(out, "Skipped.")
			return nil
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Skipped.")
			return nil
		}
	}

	if dryRun {
		fmt.Fprintf(out, "→ Would migrate %d file(s) to .looper/ structure\n", len(candidates))
		return nil
	}

	for _, candidate := range candidates {
		if err := moveFileToLooperStructure(repoRoot, candidate); err != nil {
			return fmt.Errorf("failed to migrate %s: %w", candidate, err)
		}
		fmt.Fprintf(out, "✓ Migrated %s\n", candidate)
	}

	return nil
}

func moveFileToLooperStructure(repoRoot, filename string) error {
	srcPath := filepath.Join(repoRoot, filename)

	matches := ticketFileRe.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return fmt.Errorf("cannot determine ticket from filename: %s", filename)
	}
	ticket := matches[1]

	ticketDir := filepath.Join(repoRoot, ".looper", ticket)
	if err := os.MkdirAll(ticketDir, 0755); err != nil {
		return err
	}

	return os.Rename(srcPath, filepath.Join(ticketDir, filename))
}

func verifyAndGuide(out io.Writer, repoRoot string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	var globalCfgPath string
	if runtime.GOOS == "darwin" {
		globalCfgPath = filepath.Join(home, "Library", "Application Support", "looper", "config.json")
	} else {
		globalCfgPath = filepath.Join(home, ".config", "looper", "config.json")
	}
	if _, err := os.Stat(globalCfgPath); os.IsNotExist(err) {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "• Global config not found")
		fmt.Fprintln(out, "  Initialize with: looper settings set backend claude")
	}

	available, err := discover.Scan(home)
	if err == nil && len(available) > 0 {
		agents := 0
		for _, f := range available {
			if f.Kind == discover.KindAgent {
				agents++
			}
		}
		if agents > 0 {
			fmt.Fprintln(out)
			fmt.Fprintf(out, "• Found %d agent(s) available\n", agents)
			fmt.Fprintln(out, "  Run 'looper settings discover --ai' to configure reviewers")
		}
	}

	candidates := findMigrationCandidates(repoRoot)
	if len(candidates) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "• Detected root-level looper files:")
		for _, f := range candidates {
			fmt.Fprintf(out, "  - %s\n", f)
		}
		fmt.Fprintln(out, "  Run 'looper init --migrate' to move these to .looper/ structure")
	}
}
