package cmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTempGitRepoForSettings creates a temp dir with a git repo and returns its path.
func initTempGitRepoForSettings(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("hi"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	return dir
}

// captureStdout redirects os.Stdout to a pipe, runs fn, and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(out)
}

// runSettingsGetAt changes to dir and executes "settings get <key>", returning stdout.
func runSettingsGetAt(t *testing.T, dir string, key string) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	return captureStdout(t, func() {
		rootCmd.SetArgs([]string{"settings", "get", key})
		if err := rootCmd.Execute(); err != nil {
			t.Errorf("command error: %v", err)
		}
	})
}

func TestSettingsGet_GlobalValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepoForSettings(t)

	out := runSettingsGetAt(t, repoDir, "backend")
	if strings.Contains(out, "[repo]") {
		t.Errorf("expected no [repo] annotation for global value, got %q", out)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("expected non-empty output for backend")
	}
}

func TestSettingsGet_RepoValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepoForSettings(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`{"backend":"cursor"}`), 0644); err != nil {
		t.Fatalf("write .looper.json: %v", err)
	}

	out := runSettingsGetAt(t, repoDir, "backend")
	if !strings.Contains(out, "[repo]") {
		t.Errorf("expected [repo] annotation for repo-overridden value, got %q", out)
	}
	if !strings.Contains(out, "cursor") {
		t.Errorf("expected value 'cursor' in output, got %q", out)
	}
}
