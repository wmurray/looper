package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

var defaultTicketRe = regexp.MustCompile(`[A-Z]+-[0-9]+`)

// --- firstLine ---

func TestFirstLine_Normal(t *testing.T) {
	result := firstLine("Added authentication middleware\nand updated routes")
	if result != "Added authentication middleware" {
		t.Errorf("got %q", result)
	}
}

func TestFirstLine_LeadingBlankLines(t *testing.T) {
	result := firstLine("\n\n  \nActual content here")
	if result != "Actual content here" {
		t.Errorf("got %q", result)
	}
}

func TestFirstLine_EmptyString(t *testing.T) {
	result := firstLine("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFirstLine_WhitespaceOnly(t *testing.T) {
	result := firstLine("   \n  \n  ")
	if result != "" {
		t.Errorf("expected empty string for whitespace-only input, got %q", result)
	}
}

func TestFirstLine_SingleLine(t *testing.T) {
	result := firstLine("  trimmed  ")
	if result != "trimmed" {
		t.Errorf("got %q", result)
	}
}

// --- InferTicketFromPlan ---

func writePlanFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "PLAN.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	return path
}

func TestInferTicketFromPlan_HeaderFormat(t *testing.T) {
	path := writePlanFile(t, "# Ticket: DX-123\n\n## Objective\nDo a thing\n")
	result := InferTicketFromPlan(path, defaultTicketRe)
	if result != "DX-123" {
		t.Errorf("expected DX-123, got %q", result)
	}
}

func TestInferTicketFromPlan_InlineFormat(t *testing.T) {
	path := writePlanFile(t, "# My Plan\n\nThis relates to ticket: ABC-456\n")
	result := InferTicketFromPlan(path, defaultTicketRe)
	if result != "ABC-456" {
		t.Errorf("expected ABC-456, got %q", result)
	}
}

func TestInferTicketFromPlan_NoTicket(t *testing.T) {
	path := writePlanFile(t, "# My Plan\n\nNo ticket mentioned here.\n")
	result := InferTicketFromPlan(path, defaultTicketRe)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestInferTicketFromPlan_TicketBeyondLine10(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nTicket: LATE-999\n"
	path := writePlanFile(t, content)
	result := InferTicketFromPlan(path, defaultTicketRe)
	if result != "" {
		t.Errorf("ticket beyond line 10 should not be found, got %q", result)
	}
}

func TestInferTicketFromPlan_NonexistentFile(t *testing.T) {
	result := InferTicketFromPlan("/nonexistent/path/PLAN.md", defaultTicketRe)
	if result != "" {
		t.Errorf("expected empty string for missing file, got %q", result)
	}
}

func TestInferTicketFromPlan_CaseInsensitiveHeader(t *testing.T) {
	path := writePlanFile(t, "TICKET: XY-789\n")
	result := InferTicketFromPlan(path, defaultTicketRe)
	if result != "XY-789" {
		t.Errorf("expected XY-789, got %q", result)
	}
}

// --- AssertRepo / AssertClean (require real git) ---
// These are integration-level and run only when git is available.

func TestAssertRepo_NotARepo(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Skipf("could not chdir to temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := AssertRepo(); err == nil {
		t.Fatal("expected error when not in a git repo")
	}
}

// --- helpers for git integration tests ---

// initTempRepo creates a temp dir, inits a git repo in it, chdirs into it,
// and returns a cleanup function that restores the original working directory.
//
// WARNING: uses os.Chdir which mutates the process-wide working directory.
// Do NOT call t.Parallel() in any test that uses this helper.
func initTempRepo(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to temp repo: %v", err)
	}

	mustRun := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %s", name, args, out)
		}
	}
	mustRun("git", "init")
	mustRun("git", "config", "user.email", "test@example.com")
	mustRun("git", "config", "user.name", "Test")

	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("could not restore working directory: %v", err)
		}
	}
}

var makeCommitCounter atomic.Int64

// makeCommit creates a uniquely named file and commits it with the given message.
// Each call writes a distinct file so successive commits don't no-op.
// Uses an atomic counter so it is safe against future concurrent callers.
func makeCommit(t *testing.T, message string) {
	t.Helper()
	n := makeCommitCounter.Add(1)
	filename := fmt.Sprintf("file_%d.txt", n)
	if err := os.WriteFile(filename, []byte(message), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if out, err := exec.Command("git", "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("git add: %s", out)
	}
	if out, err := exec.Command("git", "commit", "-m", message).CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s", out)
	}
}

// defaultBranch returns the current branch name (works across git versions).
func defaultBranch(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// --- BranchExists ---

func TestBranchExists_Exists(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	branch := defaultBranch(t)

	if !BranchExists(branch) {
		t.Errorf("BranchExists(%q) = false, want true", branch)
	}
}

func TestBranchExists_NotExists(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")

	if BranchExists("nonexistent-branch-xyz") {
		t.Error("BranchExists(nonexistent) = true, want false")
	}
}

// --- Checkout ---

func TestCheckout_SwitchesBranch(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	orig := defaultBranch(t)

	// create a new branch
	if out, err := exec.Command("git", "checkout", "-b", "feature-branch").CombinedOutput(); err != nil {
		t.Fatalf("create branch: %s", out)
	}
	makeCommit(t, "feature commit")

	// switch back to orig
	if out, err := exec.Command("git", "checkout", orig).CombinedOutput(); err != nil {
		t.Fatalf("checkout orig: %s", out)
	}

	// now use Checkout to switch to feature-branch
	if err := Checkout("feature-branch"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	got := defaultBranch(t)
	if got != "feature-branch" {
		t.Errorf("after Checkout, branch = %q, want feature-branch", got)
	}
}

// --- HasIterationWork ---

func TestHasIterationWork_WithIterations(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	makeCommit(t, "Iteration 1: implement the feature")

	if !HasIterationWork() {
		t.Error("HasIterationWork() = false, want true after iteration commit")
	}
}

func TestHasIterationWork_WithWIPCommit(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	makeCommit(t, "WIP: Iteration 2 - timeout during implement")

	if !HasIterationWork() {
		t.Error("HasIterationWork() = false, want true after WIP commit")
	}
}

func TestHasIterationWork_WithoutIterations(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "Add plan file")

	if HasIterationWork() {
		t.Error("HasIterationWork() = true, want false with no iteration commits")
	}
}

// TestHasIterationWork_ScopedToCurrentBranch verifies that iteration commits
// on the base branch do not trigger HasIterationWork on a branch cut from it.
func TestHasIterationWork_ScopedToCurrentBranch(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	// Base branch gets an iteration-matching commit (simulates a naming collision).
	makeCommit(t, "initial commit")
	makeCommit(t, "Iteration 1: something committed on base branch")
	base := defaultBranch(t)

	// Cut a new feature branch — no new iteration commits on it.
	if out, err := exec.Command("git", "checkout", "-b", "feature-branch").CombinedOutput(); err != nil {
		t.Fatalf("create branch: %s", out)
	}
	makeCommit(t, "Add TICKET-1 plan file")

	// HasIterationWork must return false: the matching commit is on the base branch,
	// not on this branch.
	if HasIterationWork() {
		t.Errorf("HasIterationWork() = true, want false (matching commit is on %q, not on current branch)", base)
	}
}

// --- Checkout error path ---

func TestCheckout_NonexistentBranch(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")

	err := Checkout("branch-that-does-not-exist")
	if err == nil {
		t.Fatal("Checkout(nonexistent) returned nil, want error")
	}
}
