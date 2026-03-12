package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

// Gotcha: os.Chdir is process-wide; these tests cannot run with t.Parallel().
func runSettingsGetAt(t *testing.T, dir string, key string) (string, error) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	var cmdErr error
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"settings", "get", key})
		cmdErr = rootCmd.Execute()
	})
	return out, cmdErr
}

func TestSettingsGet_GlobalValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepoForSettings(t)

	out, err := runSettingsGetAt(t, repoDir, "backend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "[repo]") {
		t.Errorf("expected no [repo] annotation for global value, got %q", out)
	}
	if strings.TrimSpace(out) != "claude" {
		t.Errorf("expected default backend 'claude', got %q", strings.TrimSpace(out))
	}
}

func TestSettingsGet_RepoValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepoForSettings(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`{"backend":"cursor"}`), 0644); err != nil {
		t.Fatalf("write .looper.json: %v", err)
	}

	out, err := runSettingsGetAt(t, repoDir, "backend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "[repo]") {
		t.Errorf("expected [repo] annotation for repo-overridden value, got %q", out)
	}
	if !strings.Contains(out, "cursor") {
		t.Errorf("expected value 'cursor' in output, got %q", out)
	}
}

func TestSettingsGet_UnknownKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepoForSettings(t)

	// Gotcha: Cobra writes error text to os.Stderr even during tests; redirect to suppress.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, err := runSettingsGetAt(t, repoDir, "nosuchkey")

	w.Close()
	os.Stderr = oldStderr
	io.Copy(bytes.NewBuffer(nil), r) // drain

	if err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}
