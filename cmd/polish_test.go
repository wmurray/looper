package cmd

import (
	"strings"
	"testing"

	"github.com/willmurray/looper/internal/config"
)

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
