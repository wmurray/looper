package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanCmd_Flags(t *testing.T) {
	cmd := newCleanCmd()
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

func TestCleanGlobs(t *testing.T) {
	want := map[string]bool{
		"*_PLAN.md":     false,
		"*_PROGRESS.md": false,
		"*_STATE.json":  false,
	}
	for _, g := range cleanGlobs {
		want[g] = true
	}
	for pattern, found := range want {
		if !found {
			t.Errorf("missing pattern %q in cleanGlobs", pattern)
		}
	}
	if len(cleanGlobs) != 3 {
		t.Errorf("expected 3 globs, got %d", len(cleanGlobs))
	}
}

func TestRunClean_NothingToClean(t *testing.T) {
	cmd := newCleanCmd()
	dir := t.TempDir()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := runClean(cmd, nil, dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(buf.String(), "Nothing to clean") {
		t.Errorf("expected 'Nothing to clean' in output, got %q", buf.String())
	}
}

func TestRunClean_InteractiveYes(t *testing.T) {
	cmd := newCleanCmd()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "TICKET-1_PLAN.md"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("y\n"))

	if err := runClean(cmd, nil, dir); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "TICKET-1_PLAN.md")); !os.IsNotExist(err) {
		t.Error("expected TICKET-1_PLAN.md to be removed")
	}
}

func TestRunClean_InteractiveNo(t *testing.T) {
	cmd := newCleanCmd()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "TICKET-1_PLAN.md"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("n\n"))

	if err := runClean(cmd, nil, dir); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "TICKET-1_PLAN.md")); os.IsNotExist(err) {
		t.Error("expected TICKET-1_PLAN.md to still exist after 'n' answer")
	}
}

func TestRunClean_ErrorIncludesFilename(t *testing.T) {
	cmd := newCleanCmd()
	dir := t.TempDir()

	// Gotcha: chmod 0555 on the dir (not the file) blocks os.Remove.
	target := filepath.Join(dir, "TICKET-1_PLAN.md")
	if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}

	err := runClean(cmd, nil, dir)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "TICKET-1_PLAN.md") {
		t.Errorf("expected error to contain filename, got: %v", err)
	}
}

func TestRunClean_YesFlag(t *testing.T) {
	cmd := newCleanCmd()
	dir := t.TempDir()

	files := []string{"TICKET-1_PLAN.md", "TICKET-1_PROGRESS.md", "TICKET-1_STATE.json"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}

	if err := runClean(cmd, nil, dir); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", f)
		}
	}
}
