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

func TestAgentDecision_NoChanges_Legacy(t *testing.T) {
	t.Parallel()
	// empty diff + empty status + same HEAD → neither self-commit nor pending changes
	isSelf, hasPending := agentDecision("", "", "abc123", "abc123")
	if isSelf || hasPending {
		t.Errorf("expected (false, false) when diff/status empty and HEAD unchanged; got (%v, %v)", isSelf, hasPending)
	}
}

func TestAgentDecision_DiffChange_Legacy(t *testing.T) {
	t.Parallel()
	_, hasPending := agentDecision("diff --git a/foo", "", "abc123", "abc123")
	if !hasPending {
		t.Error("expected hasPendingChanges=true when diff is non-empty")
	}
}

func TestAgentDecision_NewCommit_Legacy(t *testing.T) {
	t.Parallel()
	isSelf, _ := agentDecision("", "", "abc123", "def456")
	if !isSelf {
		t.Error("expected isSelfCommit=true when HEAD changed")
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

// --- Dry-run empty ticket defaults to "(none)" ---

func TestBuildDryRunOutput_EmptyTicketShowsNone(t *testing.T) {
	t.Parallel()
	out := buildDryRunOutput("", "/my/agent.md", nil, 300, "claude")
	if !strings.Contains(out, "(none)") {
		t.Errorf("dry-run output with empty ticket should show (none); got:\n%s", out)
	}
	if strings.Contains(out, "Ticket:         \n") {
		t.Errorf("dry-run output should not have bare 'Ticket:' with no value; got:\n%s", out)
	}
}

// --- agentDecision: self-commit vs working-tree dirty ---

func TestAgentDecision_SelfCommit(t *testing.T) {
	t.Parallel()
	// HEAD changed → agent self-committed; working tree is clean
	isSelfCommit, hasPendingChanges := agentDecision("", "", "abc123", "def456")
	if !isSelfCommit {
		t.Error("expected isSelfCommit=true when HEAD changed")
	}
	if hasPendingChanges {
		t.Error("expected hasPendingChanges=false when working tree is clean")
	}
}

func TestAgentDecision_PendingChanges(t *testing.T) {
	t.Parallel()
	// HEAD unchanged, dirty working tree → need to commit
	isSelfCommit, hasPendingChanges := agentDecision("diff --git a/foo", "", "abc123", "abc123")
	if isSelfCommit {
		t.Error("expected isSelfCommit=false when HEAD unchanged")
	}
	if !hasPendingChanges {
		t.Error("expected hasPendingChanges=true when diff is non-empty")
	}
}

func TestAgentDecision_NoChanges(t *testing.T) {
	t.Parallel()
	// HEAD unchanged, clean working tree → nothing to do
	isSelfCommit, hasPendingChanges := agentDecision("", "", "abc123", "abc123")
	if isSelfCommit {
		t.Error("expected isSelfCommit=false when HEAD unchanged")
	}
	if hasPendingChanges {
		t.Error("expected hasPendingChanges=false when working tree is clean")
	}
}
