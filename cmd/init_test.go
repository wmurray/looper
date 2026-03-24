package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd_DirectoryCreation(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	looperDir := filepath.Join(dir, ".looper")
	if _, err := os.Stat(looperDir); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist")
	}
}

func TestInitCmd_DirectoryCreation_Idempotent(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{}); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	if err := runInit(cmd, dir, initOptions{}); err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	looperDir := filepath.Join(dir, ".looper")
	if _, err := os.Stat(looperDir); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist after second init")
	}
}

func TestInitCmd_GitignoreCreation(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("expected .gitignore to exist: %v", err)
	}

	if !strings.Contains(string(content), ".looper/") {
		t.Errorf("expected .gitignore to contain '.looper/', got %q", string(content))
	}
}

func TestInitCmd_GitignoreNoDuplicates(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("expected .gitignore to exist: %v", err)
	}

	count := strings.Count(string(content), ".looper/")
	if count != 1 {
		t.Errorf("expected .looper/ pattern once in .gitignore, got %d times", count)
	}
}

func TestInitCmd_GitignoreSkip(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, SkipGitignore: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); !os.IsNotExist(err) {
		t.Errorf("expected .gitignore not to be created with --skip-gitignore")
	}
}

func TestInitCmd_DryRun_CreatesNothing(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, DryRun: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, name := range []string{".looper", ".gitignore", ".looper.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s not to be created in --dry-run mode", name)
		}
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "dry-run") && !strings.Contains(strings.ToLower(output), "would") {
		t.Errorf("expected dry-run message in output, got %q", output)
	}
}

func TestInitCmd_StackDetection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		file    string
		content string
		wantKw  string
	}{
		{"Go", "go.mod", "module test", "go"},
		{"Node", "package.json", `{"name":"test"}`, "node"},
		{"Python", "pyproject.toml", "", "python"},
		{"Rails", "Gemfile", "", "rails"},
		{"Rust", "Cargo.toml", "", "rust"},
		{"Java Maven", "pom.xml", "", "java"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tt.file), []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			stack := detectStack(dir)
			if stack != tt.wantKw {
				t.Errorf("expected %q for %s, got %q", tt.wantKw, tt.file, stack)
			}
		})
	}
}

func TestInitCmd_LooperConfigCreation(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	configPath := filepath.Join(dir, ".looper.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected .looper.json to exist: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Errorf("expected valid JSON in .looper.json: %v", err)
	}
}

func TestInitCmd_ConfigOnlyFlag(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, ConfigOnly: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".looper.json")); os.IsNotExist(err) {
		t.Errorf("expected .looper.json to exist with --config-only")
	}

	// Invariant: --config-only must not create .looper/ or .gitignore.
	if _, err := os.Stat(filepath.Join(dir, ".looper")); !os.IsNotExist(err) {
		t.Errorf("expected .looper/ not to be created with --config-only")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("expected .gitignore not to be created with --config-only")
	}
}

func TestInitCmd_SkipConfigFlag(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, SkipConfig: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".looper")); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist")
	}

	if _, err := os.Stat(filepath.Join(dir, ".looper.json")); !os.IsNotExist(err) {
		t.Errorf("expected .looper.json not to be created with --skip-config")
	}
}

func TestInitCmd_DryRunFlag(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, DryRun: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".looper")); !os.IsNotExist(err) {
		t.Errorf("expected .looper directory not to be created in --dry-run mode")
	}

	if _, err := os.Stat(filepath.Join(dir, ".looper.json")); !os.IsNotExist(err) {
		t.Errorf("expected .looper.json not to be created in --dry-run mode")
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "dry-run") && !strings.Contains(strings.ToLower(output), "would") {
		t.Errorf("expected dry-run message in output, got %q", output)
	}
}

func TestInitCmd_Flags(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()

	tests := []struct {
		flag string
		name string
	}{
		{"yes", "--yes flag"},
		{"skip-gitignore", "--skip-gitignore flag"},
		{"config-only", "--config-only flag"},
		{"skip-config", "--skip-config flag"},
		{"dry-run", "--dry-run flag"},
		{"migrate", "--migrate flag"},
	}

	for _, tt := range tests {
		f := cmd.Flags().Lookup(tt.flag)
		if f == nil {
			t.Errorf("missing %s", tt.name)
		}
	}
}

func TestInitCmd_YesFlag(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".looper")); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist")
	}
}

func TestInitCmd_ExistingGitignore(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("expected .gitignore to exist: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "node_modules/") {
		t.Errorf("expected original .gitignore content to be preserved")
	}
	if !strings.Contains(contentStr, ".looper/") {
		t.Errorf("expected .looper/ pattern to be added")
	}
}

func TestInitCmd_MultipleStacks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	stacks := detectAllStacks(dir)
	if len(stacks) != 2 {
		t.Errorf("expected detection of 2 stacks, got %d: %v", len(stacks), stacks)
	}

	stackStr := strings.Join(stacks, ",")
	if !strings.Contains(stackStr, "node") || !strings.Contains(stackStr, "go") {
		t.Errorf("expected 'node' and 'go' in detected stacks, got %q", stackStr)
	}
}

func TestInitCmd_DetectMigrationCandidates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Why: TKT (3 chars) used deliberately — TICKET (6 chars) exceeds the 4-char limit and won't be detected.
	for _, name := range []string{"TKT_PLAN.md", "TKT_PROGRESS.md", "TKT_STATE.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	candidates := findMigrationCandidates(dir)
	if len(candidates) != 3 {
		t.Errorf("expected 3 migration candidates, got %d: %v", len(candidates), candidates)
	}
}

func TestInitCmd_MigrateFlag_MovesFiles(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	planFile := filepath.Join(dir, "TKT_PLAN.md")
	if err := os.WriteFile(planFile, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, Migrate: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	movedFile := filepath.Join(dir, ".looper", "TKT", "TKT_PLAN.md")
	if _, err := os.Stat(movedFile); os.IsNotExist(err) {
		t.Errorf("expected migrated file at %s", movedFile)
	}

	if _, err := os.Stat(planFile); !os.IsNotExist(err) {
		t.Errorf("expected original file to be removed after migration")
	}
}

func TestMigrateFlag_DoesNothingIfNoFiles(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, Migrate: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := out.String()
	if strings.Contains(output, "error") || strings.Contains(output, "Error") {
		t.Errorf("expected no error output when no migration candidates found")
	}
}

func TestInitCmd_ExtendedConfig_WithReviewers(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".looper.json"))
	if err != nil {
		t.Fatalf("expected .looper.json to exist: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Errorf("expected valid JSON in .looper.json: %v", err)
	}

	if _, hasReviewers := cfg["reviewers"]; !hasReviewers {
		t.Errorf("expected .looper.json to have 'reviewers' field")
	}
}

func TestInitCmd_VerifyGuidance_RootFilesDetection(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "TKT_PLAN.md"), []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "TKT_PLAN") && !strings.Contains(output, "migrate") {
		t.Errorf("expected migration guidance in output: %q", output)
	}
}

func TestInitCmd_VerifyGuidance_GlobalConfigCheck(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestInitCmd_MigrateFlag_WithMultipleFiles(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	srcFiles := []string{
		filepath.Join(dir, "TKT_PLAN.md"),
		filepath.Join(dir, "TKT_PROGRESS.md"),
		filepath.Join(dir, "TKT_STATE.json"),
	}
	for _, f := range srcFiles {
		if err := os.WriteFile(f, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, Migrate: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ticketDir := filepath.Join(dir, ".looper", "TKT")
	for _, name := range []string{"TKT_PLAN.md", "TKT_PROGRESS.md", "TKT_STATE.json"} {
		dst := filepath.Join(ticketDir, name)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			t.Errorf("expected migrated file at %s", dst)
		}
	}

	for _, f := range srcFiles {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("expected original file %s to be removed", f)
		}
	}
}

func TestMigrateFlag_HyphenatedTicketID(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	planFile := filepath.Join(dir, "IMP-123_PLAN.md")
	if err := os.WriteFile(planFile, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runInit(cmd, dir, initOptions{Yes: true, Migrate: true}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Invariant: full ticket ID preserved as directory — .looper/IMP-123/, not .looper/IMP/.
	correctPath := filepath.Join(dir, ".looper", "IMP-123", "IMP-123_PLAN.md")
	wrongPath := filepath.Join(dir, ".looper", "IMP", "IMP-123_PLAN.md")

	if _, err := os.Stat(correctPath); os.IsNotExist(err) {
		t.Errorf("expected migrated file at correct path: %s", correctPath)
	}
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Errorf("file migrated to wrong path (extracted IMP instead of IMP-123)")
	}
	if _, err := os.Stat(planFile); !os.IsNotExist(err) {
		t.Errorf("expected original file to be removed")
	}
}

func TestFindMigrationCandidates_SkipsNonTicketFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, name := range []string{"DATABASE_PLAN.md", "IMP-123_PLAN.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	candidates := findMigrationCandidates(dir)

	// Invariant: long-word filenames (DATABASE_PLAN.md) must not be treated as ticket files.
	found := false
	nonTicketFound := false
	for _, c := range candidates {
		if c == "IMP-123_PLAN.md" {
			found = true
		}
		if c == "DATABASE_PLAN.md" {
			nonTicketFound = true
		}
	}

	if !found {
		t.Errorf("expected to find IMP-123_PLAN.md in candidates")
	}
	if nonTicketFound {
		t.Errorf("should not find DATABASE_PLAN.md (non-ticket file) in candidates")
	}
}

func TestMoveFileToLooperStructure_ExtractsHyphenatedTicketID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, ".looper"), 0755); err != nil {
		t.Fatal(err)
	}

	srcFile := filepath.Join(dir, "IMP-123_PLAN.md")
	if err := os.WriteFile(srcFile, []byte("plan content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := moveFileToLooperStructure(dir, "IMP-123_PLAN.md"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedPath := filepath.Join(dir, ".looper", "IMP-123", "IMP-123_PLAN.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s", expectedPath)
	}

	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Errorf("expected source file to be removed")
	}
}

func TestVerifyAndGuide_ChecksCorrectConfigPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out bytes.Buffer

	// Gotcha: UserHomeDir can't be mocked here; test only asserts the function runs without panic.
	verifyAndGuide(&out, dir)
}

func TestInitCmd_ConflictingFlags_ConfigOnlyAndSkipConfig(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, initOptions{Yes: true, ConfigOnly: true, SkipConfig: true})
	if err == nil {
		t.Errorf("expected error when using --config-only and --skip-config together")
	}
	if !strings.Contains(err.Error(), "cannot use") {
		t.Errorf("expected helpful error message about conflicting flags, got: %v", err)
	}
}
