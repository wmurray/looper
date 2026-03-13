package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeDiscoverTree creates files at the given relative paths under the home dir.
func makeDiscoverTree(t *testing.T, home string, paths []string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(home, p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(""), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func runSettingsDiscover(t *testing.T, home string, extraArgs ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", home)

	// Capture stdout
	rOut, wOut, _ := os.Pipe()
	oldOut := os.Stdout
	os.Stdout = wOut

	// Capture stderr
	rErr, wErr, _ := os.Pipe()
	oldErr := os.Stderr
	os.Stderr = wErr

	args := append([]string{"settings", "discover"}, extraArgs...)
	rootCmd.SetArgs(args)
	cmdErr := rootCmd.Execute()

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	return string(outBytes), string(errBytes), cmdErr
}

func TestSettingsDiscover_PrintsCommands(t *testing.T) {
	home := t.TempDir()
	makeDiscoverTree(t, home, []string{
		".claude/plugins/marketplaces/acme/plugins/tdd-plugin/skills/tdd-workflow/SKILL.md",
		".claude/plugins/marketplaces/acme/plugins/rails/agents/rails-reviewer.md",
	})

	out, _, err := runSettingsDiscover(t, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "looper settings set skill_path") {
		t.Errorf("expected skill_path command in output, got:\n%s", out)
	}
	if !strings.Contains(out, "looper settings set reviewer_agent") {
		t.Errorf("expected reviewer_agent command in output, got:\n%s", out)
	}
}

func TestSettingsDiscover_NothingFound(t *testing.T) {
	home := t.TempDir()

	// Suppress stderr (cobra error output)
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	out, _, err := runSettingsDiscover(t, home)

	w.Close()
	os.Stderr = old
	io.Copy(bytes.NewBuffer(nil), r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No skills or agents found") {
		t.Errorf("expected 'No skills or agents found' in output, got:\n%s", out)
	}
}

func TestSettingsDiscover_Apply_SingleSkill(t *testing.T) {
	home := t.TempDir()
	makeDiscoverTree(t, home, []string{
		".claude/skills/tdd-workflow/SKILL.md",
	})

	out, _, err := runSettingsDiscover(t, home, "--apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Set skill_path") {
		t.Errorf("expected 'Set skill_path' in output, got:\n%s", out)
	}
}
