package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/willmurray/looper/internal/config"
)

// --- Dry-run output ---

func TestBuildDryRunOutput_ContainsAgentPath(t *testing.T) {
	t.Parallel()
	out := buildDryRunOutput("IMP-18", "/my/agent.md", []string{"go fmt ./..."}, 300, "claude")
	if !strings.Contains(out, "/my/agent.md") {
		t.Errorf("dry-run output does not contain agent path; got:\n%s", out)
	}
}

func TestBuildDryRunOutput_ContainsTicket(t *testing.T) {
	t.Parallel()
	out := buildDryRunOutput("IMP-18", "/my/agent.md", []string{"go fmt ./..."}, 300, "claude")
	if !strings.Contains(out, "IMP-18") {
		t.Errorf("dry-run output does not contain ticket; got:\n%s", out)
	}
}

func TestBuildDryRunOutput_ContainsLintCmds(t *testing.T) {
	t.Parallel()
	out := buildDryRunOutput("IMP-18", "/my/agent.md", []string{"go fmt ./...", "go vet ./..."}, 300, "claude")
	if !strings.Contains(out, "go fmt ./...") || !strings.Contains(out, "go vet ./...") {
		t.Errorf("dry-run output does not contain lint commands; got:\n%s", out)
	}
}

// --- No-changes idempotency guard ---

func TestAgentHasChanges_NoChanges(t *testing.T) {
	t.Parallel()
	// empty diff + empty status + same HEAD → no changes
	if agentHasChanges("", "", "abc123", "abc123") {
		t.Error("expected agentHasChanges=false when diff/status empty and HEAD unchanged")
	}
}

func TestAgentHasChanges_DiffChange(t *testing.T) {
	t.Parallel()
	if !agentHasChanges("diff --git a/foo", "", "abc123", "abc123") {
		t.Error("expected agentHasChanges=true when diff is non-empty")
	}
}

func TestAgentHasChanges_NewCommit(t *testing.T) {
	t.Parallel()
	if !agentHasChanges("", "", "abc123", "def456") {
		t.Error("expected agentHasChanges=true when HEAD changed")
	}
}

// --- Lint phase ---

func TestRunLintCmds_FailurePropagates(t *testing.T) {
	t.Parallel()
	err := runLintCmds(context.Background(), []string{"false"}, t.TempDir())
	if err == nil {
		t.Error("expected error from failing lint command, got nil")
	}
}

func TestRunLintCmds_SuccessNoError(t *testing.T) {
	t.Parallel()
	err := runLintCmds(context.Background(), []string{"true"}, t.TempDir())
	if err != nil {
		t.Errorf("expected no error from succeeding lint command, got %v", err)
	}
}

// --- Flag registration ---

func TestPolishCmd_YesFlag(t *testing.T) {
	t.Parallel()
	cmd := newPolishCmd()
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("--yes flag not registered")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("expected bool type, got %s", f.Value.Type())
	}
	if f.DefValue != "false" {
		t.Errorf("expected default false, got %s", f.DefValue)
	}
}

func TestPolishCmd_DryRunFlag(t *testing.T) {
	t.Parallel()
	cmd := newPolishCmd()
	f := cmd.Flags().Lookup("dry-run")
	if f == nil {
		t.Fatal("--dry-run flag not registered")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("expected bool type, got %s", f.Value.Type())
	}
	if f.DefValue != "false" {
		t.Errorf("expected default false, got %s", f.DefValue)
	}
}

func TestPolishCmd_TimeoutFlag(t *testing.T) {
	t.Parallel()
	cmd := newPolishCmd()
	f := cmd.Flags().Lookup("timeout")
	if f == nil {
		t.Fatal("--timeout flag not registered")
	}
	if f.Value.Type() != "int" {
		t.Errorf("expected int type, got %s", f.Value.Type())
	}
	if f.DefValue != "0" {
		t.Errorf("expected default 0, got %s", f.DefValue)
	}
}

// --- PolishAgent fallback ---

func TestResolvePolishAgent_UsesPolishAgent(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		PolishAgent:   "/path/to/polish.md",
		ReviewerAgent: "/path/to/reviewer.md",
	}
	got := resolvePolishAgent(cfg)
	if got != "/path/to/polish.md" {
		t.Errorf("resolvePolishAgent = %q, want /path/to/polish.md", got)
	}
}

func TestResolvePolishAgent_FallsBackToReviewer(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		PolishAgent:   "",
		ReviewerAgent: "/path/to/reviewer.md",
	}
	got := resolvePolishAgent(cfg)
	if got != "/path/to/reviewer.md" {
		t.Errorf("resolvePolishAgent = %q, want /path/to/reviewer.md", got)
	}
}

// --- Prompt construction ---

func TestBuildPolishPrompt_ContainsAgentPath(t *testing.T) {
	t.Parallel()
	prompt := buildPolishPrompt("/my/agent.md")
	if !strings.Contains(prompt, "/my/agent.md") {
		t.Errorf("prompt does not contain agent path; prompt:\n%s", prompt)
	}
}

func TestBuildPolishPrompt_ContainsConstraintBullets(t *testing.T) {
	t.Parallel()
	prompt := buildPolishPrompt("/my/agent.md")

	bullets := []string{
		"Do NOT add new features",
		"Do NOT refactor anything",
		"Do NOT change public APIs",
		"go build ./...",
	}
	for _, b := range bullets {
		if !strings.Contains(prompt, b) {
			t.Errorf("prompt missing constraint bullet %q; prompt:\n%s", b, prompt)
		}
	}
}

func TestBuildPolishPrompt_InstructsCommitMessage(t *testing.T) {
	t.Parallel()
	prompt := buildPolishPrompt("/my/agent.md")
	if !strings.Contains(prompt, "imperative commit message") {
		t.Errorf("prompt does not instruct commit message format; prompt:\n%s", prompt)
	}
}
