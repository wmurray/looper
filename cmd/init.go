package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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

		return runInit(c, ".", yesFlag, skipGitignore, configOnly, skipConfig, dryRun)
	}

	return cmd
}

func runInit(cmd *cobra.Command, repoRoot string, yes, skipGitignore, configOnly, skipConfig, dryRun bool) error {
	out := cmd.OutOrStdout()

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

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(repoRoot, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			base := filepath.Base(match)
			if !seen[base] {
				candidates = append(candidates, base)
				seen[base] = true
			}
		}
	}

	return candidates
}

func verifyAndGuide(out io.Writer, repoRoot string) {
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
