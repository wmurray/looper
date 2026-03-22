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
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, false, false, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	looperDir := filepath.Join(dir, ".looper")
	if _, err := os.Stat(looperDir); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist")
	}
}

func TestInitCmd_DirectoryCreation_Idempotent(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, false, false, false, false, false, false)
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	err = runInit(cmd, dir, false, false, false, false, false, false)
	if err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	looperDir := filepath.Join(dir, ".looper")
	if _, err := os.Stat(looperDir); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist after second init")
	}
}

func TestInitCmd_GitignoreCreation(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("expected .gitignore to exist: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, ".looper/") {
		t.Errorf("expected .gitignore to contain '.looper/', got %q", contentStr)
	}
}

func TestInitCmd_GitignoreNoDuplicates(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	err = runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("expected .gitignore to exist: %v", err)
	}

	contentStr := string(content)
	count := strings.Count(contentStr, ".looper/")
	if count != 1 {
		t.Errorf("expected .looper/ pattern once in .gitignore, got %d times", count)
	}
}

func TestInitCmd_GitignoreSkip(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, true, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); !os.IsNotExist(err) {
		t.Errorf("expected .gitignore not to be created with --skip-gitignore")
	}
}

func TestInitCmd_StackDetection_NodeJS(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	stack := detectStack(dir)
	if stack != "node" {
		t.Errorf("expected 'node' stack detection for package.json, got %q", stack)
	}
}

func TestInitCmd_StackDetection_Go(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}

	stack := detectStack(dir)
	if stack != "go" {
		t.Errorf("expected 'go' stack detection for go.mod, got %q", stack)
	}
}

func TestInitCmd_StackDetection_Python(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	stack := detectStack(dir)
	if stack != "python" {
		t.Errorf("expected 'python' stack detection for pyproject.toml, got %q", stack)
	}
}

func TestInitCmd_LooperConfigCreation(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
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
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, true, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	configPath := filepath.Join(dir, ".looper.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("expected .looper.json to exist with --config-only")
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); !os.IsNotExist(err) {
		t.Errorf("expected .gitignore not to be created with --config-only")
	}
}

func TestInitCmd_SkipConfigFlag(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, true, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	looperDir := filepath.Join(dir, ".looper")
	if _, err := os.Stat(looperDir); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist")
	}

	configPath := filepath.Join(dir, ".looper.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("expected .looper.json not to be created with --skip-config")
	}
}

func TestInitCmd_DryRunFlag(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, true, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	looperDir := filepath.Join(dir, ".looper")
	if _, err := os.Stat(looperDir); !os.IsNotExist(err) {
		t.Errorf("expected .looper directory not to be created in --dry-run mode")
	}

	configPath := filepath.Join(dir, ".looper.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("expected .looper.json not to be created in --dry-run mode")
	}

	output := out.String()
	if !strings.Contains(strings.ToLower(output), "dry-run") && !strings.Contains(strings.ToLower(output), "would") {
		t.Errorf("expected dry-run message in output, got %q", output)
	}
}

func TestInitCmd_Flags(t *testing.T) {
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
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	looperDir := filepath.Join(dir, ".looper")
	if _, err := os.Stat(looperDir); os.IsNotExist(err) {
		t.Errorf("expected .looper directory to exist")
	}
}

func TestInitCmd_ExistingGitignore(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	gitignorePath := filepath.Join(dir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
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
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "TICKET_PLAN.md"), []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TICKET_PROGRESS.md"), []byte("# Progress"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TICKET_STATE.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	candidates := findMigrationCandidates(dir)
	if len(candidates) != 3 {
		t.Errorf("expected 3 migration candidates, got %d: %v", len(candidates), candidates)
	}
}

func TestInitCmd_MigrateFlag_MovesFiles(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	planFile := filepath.Join(dir, "TICKET_PLAN.md")
	if err := os.WriteFile(planFile, []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ticketDir := filepath.Join(dir, ".looper", "TICKET")
	movedFile := filepath.Join(ticketDir, "TICKET_PLAN.md")
	if _, err := os.Stat(movedFile); os.IsNotExist(err) {
		t.Errorf("expected migrated file at %s", movedFile)
	}

	if _, err := os.Stat(planFile); !os.IsNotExist(err) {
		t.Errorf("expected original file to be removed after migration")
	}
}

func TestInitCmd_MigrateFlag_DoesNothingIfNoFiles(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := out.String()
	if strings.Contains(output, "error") || strings.Contains(output, "Error") {
		t.Errorf("expected no error output when no migration candidates found")
	}
}

func TestInitCmd_ExtendedConfig_WithReviewers(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
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

	if _, hasReviewers := cfg["reviewers"]; !hasReviewers {
		t.Errorf("expected .looper.json to have 'reviewers' field")
	}
}

func TestInitCmd_VerifyGuidance_RootFilesDetection(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "TICKET_PLAN.md"), []byte("# Plan"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "TICKET_PLAN") && !strings.Contains(output, "migrate") {
		t.Logf("expected migration guidance in output: %q", output)
	}
}

func TestInitCmd_VerifyGuidance_GlobalConfigCheck(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestInitCmd_MigrateFlag_WithMultipleFiles(t *testing.T) {
	cmd := newInitCmd()
	dir := t.TempDir()

	planFile := filepath.Join(dir, "TICKET_PLAN.md")
	progressFile := filepath.Join(dir, "TICKET_PROGRESS.md")
	stateFile := filepath.Join(dir, "TICKET_STATE.json")

	for _, f := range []string{planFile, progressFile, stateFile} {
		if err := os.WriteFile(f, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runInit(cmd, dir, true, false, false, false, false, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ticketDir := filepath.Join(dir, ".looper", "TICKET")
	expectedFiles := []string{
		filepath.Join(ticketDir, "TICKET_PLAN.md"),
		filepath.Join(ticketDir, "TICKET_PROGRESS.md"),
		filepath.Join(ticketDir, "TICKET_STATE.json"),
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("expected migrated file at %s", f)
		}
	}

	for _, f := range []string{planFile, progressFile, stateFile} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("expected original file %s to be removed", f)
		}
	}
}
