package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(out)
}

func TestWarnIfPathMissing_SilentWhenPresent(t *testing.T) {
	home := t.TempDir()
	f := filepath.Join(home, "SKILL.md")
	if err := os.WriteFile(f, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	var missing bool
	out := captureStdout(t, func() {
		missing = warnIfPathMissing("skill_path", f)
	})
	if out != "" {
		t.Errorf("expected no output for existing path, got: %q", out)
	}
	if missing {
		t.Error("expected false for existing path")
	}
}

func TestWarnIfPathMissing_WarnWhenMissing_NoSuggestion(t *testing.T) {
	home := t.TempDir() // empty, no ~/.claude/ at all
	t.Setenv("HOME", home)

	var missing bool
	out := captureStdout(t, func() {
		missing = warnIfPathMissing("skill_path", "/nonexistent/SKILL.md")
	})
	if !strings.Contains(out, "skill_path not found") {
		t.Errorf("expected 'skill_path not found' in stdout, got: %q", out)
	}
	if !missing {
		t.Error("expected true for missing path")
	}
}

func TestWarnIfPathMissing_SuggestsWhenMatchFound(t *testing.T) {
	home := t.TempDir()
	// Put a SKILL.md under the flat layout.
	skillFile := filepath.Join(home, ".claude", "skills", "tdd-workflow", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	// The configured path has the same basename (SKILL.md) so it should match.
	out := captureStdout(t, func() {
		warnIfPathMissing("skill_path", "/old/path/SKILL.md")
	})
	if !strings.Contains(out, "looper settings set skill_path") {
		t.Errorf("expected suggestion in stdout, got: %q", out)
	}
	if !strings.Contains(out, skillFile) {
		t.Errorf("expected discovered path %q in suggestion, got: %q", skillFile, out)
	}

	var _ = bytes.NewBuffer(nil)
}
