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
		yesFlag, _ := c.Flags().GetBool("yes")
		skipGitignore, _ := c.Flags().GetBool("skip-gitignore")
		configOnly, _ := c.Flags().GetBool("config-only")
		skipConfig, _ := c.Flags().GetBool("skip-config")
		dryRun, _ := c.Flags().GetBool("dry-run")
		migrateFlag, _ := c.Flags().GetBool("migrate")

		return runInit(c, ".", yesFlag, skipGitignore, configOnly, skipConfig, dryRun, migrateFlag)
	}

	return cmd
}

func runInit(cmd *cobra.Command, repoRoot string, yes, skipGitignore, configOnly, skipConfig, dryRun, migrate bool) error {
	out := cmd.OutOrStdout()

	if configOnly && skipConfig {
		return fmt.Errorf("cannot use --config-only and --skip-config together")
	}

	if dryRun {
		fmt.Fprintln(out, "→ DRY RUN: no changes will be applied")
		fmt.Fprintln(out)
	}

	if !configOnly {
		if err := createLooperDir(out, repoRoot, dryRun); err != nil {
			return err
		}
	}

	if !configOnly && !skipGitignore {
		if err := setupGitignore(cmd, out, repoRoot, yes, dryRun); err != nil {
			return err
		}
	}

	if !skipConfig {
		if err := createLooperConfig(cmd, out, repoRoot, yes, dryRun); err != nil {
			return err
		}
	}

	if migrate {
		if err := migrateRootFiles(cmd, out, repoRoot, yes, dryRun); err != nil {
			return err
		}
	}

	if !dryRun {
		verifyAndGuide(out, repoRoot)
	}

	if dryRun {
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
		fmt.Fprintf(out, "✓ .looper/ directory already exists\n")
		return nil
	}

	if dryRun {
		fmt.Fprintf(out, "→ Would create .looper/ directory\n")
		return nil
	}

	if err := os.MkdirAll(looperDir, 0755); err != nil {
		return fmt.Errorf("failed to create .looper directory: %w", err)
	}

	fmt.Fprintf(out, "✓ Created .looper/ directory\n")
	return nil
}

func setupGitignore(cmd *cobra.Command, out io.Writer, repoRoot string, yes, dryRun bool) error {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	looperPattern := ".looper/"

	var content []byte

	if _, err := os.Stat(gitignorePath); err == nil {
		var err error
		content, err = os.ReadFile(gitignorePath)
		if err != nil {
			return fmt.Errorf("failed to read .gitignore: %w", err)
		}

		if strings.Contains(string(content), looperPattern) {
			fmt.Fprintf(out, "✓ .gitignore already contains .looper/ pattern\n")
			return nil
		}
	}

	newContent := string(content)
	if !strings.HasSuffix(newContent, "\n") && newContent != "" {
		newContent += "\n"
	}
	newContent += "# looper-cli generated files\n.looper/\n"

	if !yes && !dryRun {
		fmt.Fprintf(out, "\n.gitignore changes:\n")
		fmt.Fprintf(out, "  + # looper-cli generated files\n")
		fmt.Fprintf(out, "  + .looper/\n")
		fmt.Fprintf(out, "\nApply changes? [y/N] ")

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
		fmt.Fprintf(out, "→ Would update .gitignore with .looper/ pattern\n")
		return nil
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}

	fmt.Fprintf(out, "✓ Updated .gitignore\n")
	return nil
}

func createLooperConfig(cmd *cobra.Command, out io.Writer, repoRoot string, yes, dryRun bool) error {
	configPath := filepath.Join(repoRoot, ".looper.json")

	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(out, "✓ .looper.json already exists\n")
		return nil
	}

	stack := getStackDescription(repoRoot)

	cfg := map[string]interface{}{
		"defaults": map[string]interface{}{
			"cycles":  5,
			"timeout": 420,
		},
		"reviewers": map[string]interface{}{
			"general": "",
		},
	}

	if stack != "" {
		cfg["stack"] = stack
	}

	if !yes && !dryRun {
		fmt.Fprintf(out, "\nProject stack detected: %s\n", stack)
		fmt.Fprintf(out, "Create .looper.json with defaults? [y/N] ")

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
		fmt.Fprintf(out, "→ Would create .looper.json\n")
		return nil
	}

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write .looper.json: %w", err)
	}

	fmt.Fprintf(out, "✓ Created .looper.json\n")
	return nil
}

func getStackDescription(repoRoot string) string {
	stacks := detectAllStacks(repoRoot)
	if len(stacks) == 0 {
		return "Unknown"
	}

	keywordToName := map[string]string{
		"go":     "Go",
		"rails":  "Ruby/Rails",
		"node":   "Node.js/JavaScript",
		"python": "Python",
		"rust":   "Rust",
		"java":   "Java/Maven",
	}

	var names []string
	for _, kw := range stacks {
		if name, ok := keywordToName[kw]; ok {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		return "Unknown"
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

	// Regex to match ticket ID format: either short IDs (1-4 uppercase letters) or hyphenated format (with numbers)
	// Examples: IMP, TEST, ABC (1-4 chars) or IMP-123, LIN-456, TICKET-1 (any length with hyphen-number)
	// This avoids matching long dictionary words like DATABASE_PLAN.md
	ticketIDRegex := regexp.MustCompile(`^([A-Z]{1,4}(?:-[0-9]+)?|[A-Z][A-Z0-9]*-[0-9]+)_`)

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(repoRoot, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			base := filepath.Base(match)
			// Only include files that match the ticket ID pattern
			if ticketIDRegex.MatchString(base) && !seen[base] {
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
		fmt.Fprintf(out, "✓ No root-level looper files found to migrate\n")
		return nil
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Found %d file(s) to migrate:\n", len(candidates))
	for _, f := range candidates {
		fmt.Fprintf(out, "  • %s\n", f)
	}

	if !yes && !dryRun {
		fmt.Fprintf(out, "\nMigrate these files to .looper/:ticket/? [y/N] ")
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

	// Extract ticket ID using regex to handle hyphenated IDs like IMP-123
	re := regexp.MustCompile(`^([A-Z][A-Z0-9]*(?:-[0-9]+)?)_`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return fmt.Errorf("cannot determine ticket from filename: %s", filename)
	}
	ticket := matches[1]

	ticketDir := filepath.Join(repoRoot, ".looper", ticket)
	if err := os.MkdirAll(ticketDir, 0755); err != nil {
		return err
	}

	dstPath := filepath.Join(ticketDir, filename)

	content, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dstPath, content, 0644); err != nil {
		return err
	}

	return os.Remove(srcPath)
}

func verifyAndGuide(out io.Writer, repoRoot string) {
	home, err := os.UserHomeDir()
	if err == nil {
		var globalCfgPath string
		if runtime.GOOS == "darwin" {
			globalCfgPath = filepath.Join(home, "Library", "Application Support", "looper", "config.json")
		} else {
			globalCfgPath = filepath.Join(home, ".config", "looper", "config.json")
		}
		if _, err := os.Stat(globalCfgPath); os.IsNotExist(err) {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "• Global config not found")
			fmt.Fprintf(out, "  Initialize with: looper settings set backend claude\n")
		}
	}

	if home != "" {
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
				fmt.Fprintf(out, "  Run 'looper settings discover --ai' to configure reviewers\n")
			}
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
