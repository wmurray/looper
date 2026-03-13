package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWarnOnStackMismatch_SilentWhenNoIndicators(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	out := captureStdout(t, func() {
		warnOnStackMismatch(dir, "rails-code-reviewer.md")
	})
	if out != "" {
		t.Errorf("expected no output for greenfield project, got: %q", out)
	}
}

func TestWarnOnStackMismatch_SilentWhenReviewerMatches(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Ruby/Rails project.
	if err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		warnOnStackMismatch(dir, "rails-code-reviewer.md")
	})
	if out != "" {
		t.Errorf("expected no output when reviewer matches stack, got: %q", out)
	}
}

func TestWarnOnStackMismatch_WarnsWhenMismatch(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Go project.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		warnOnStackMismatch(dir, "rails-code-reviewer.md")
	})
	if !strings.Contains(out, "reviewer_agent") {
		t.Errorf("expected reviewer_agent warning, got: %q", out)
	}
}

func TestWarnOnStackMismatch_SuggestsAlternativeAgent(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Go project.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Put a Go reviewer in ~/.claude/agents/.
	agentFile := filepath.Join(home, ".claude", "agents", "go-code-reviewer.md")
	if err := os.MkdirAll(filepath.Dir(agentFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		warnOnStackMismatch(dir, "rails-code-reviewer.md")
	})
	if !strings.Contains(out, "go-code-reviewer.md") {
		t.Errorf("expected go-code-reviewer.md suggestion, got: %q", out)
	}
}
