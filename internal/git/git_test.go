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

// --- SplitSummary ---

func TestSplitSummary_Normal(t *testing.T) {
	subject, _ := SplitSummary("Added authentication middleware\nand updated routes")
	if subject != "Added authentication middleware" {
		t.Errorf("got %q", subject)
	}
}

func TestSplitSummary_LeadingBlankLines(t *testing.T) {
	subject, _ := SplitSummary("\n\n  \nActual content here")
	if subject != "Actual content here" {
		t.Errorf("got %q", subject)
	}
}

func TestSplitSummary_EmptyString(t *testing.T) {
	subject, _ := SplitSummary("")
	if subject != defaultIterationSubject {
		t.Errorf("expected default subject, got %q", subject)
	}
}

func TestSplitSummary_WhitespaceOnly(t *testing.T) {
	subject, _ := SplitSummary("   \n  \n  ")
	if subject != defaultIterationSubject {
		t.Errorf("expected default subject for whitespace-only input, got %q", subject)
	}
}

func TestSplitSummary_SingleLine(t *testing.T) {
	subject, _ := SplitSummary("  trimmed  ")
	if subject != "trimmed" {
		t.Errorf("got %q", subject)
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
		t.Fatalf("could not chdir to temp dir: %v", err)
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
	writeFile(t, "impl.txt", "implementation")
	if err := CommitIteration(1, "Implement the feature"); err != nil {
		t.Fatalf("CommitIteration: %v", err)
	}

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

// --- RepoRoot ---

func TestRepoRoot_InRepo(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")

	root, err := RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot() error: %v", err)
	}

	// Resolve symlinks on both sides: os.TempDir() on macOS returns a path
	// under /var/folders which is a symlink to /private/var/folders, while
	// git rev-parse --show-toplevel returns the resolved path.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	evalCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatalf("EvalSymlinks(cwd): %v", err)
	}
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root): %v", err)
	}
	if evalRoot != evalCwd {
		t.Errorf("RepoRoot() = %q, want %q", evalRoot, evalCwd)
	}
}

func TestRepoRoot_NotInRepo(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("could not chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	_, err = RepoRoot()
	if err == nil {
		t.Fatal("RepoRoot() expected error outside a git repo, got nil")
	}
}

// --- CommitIteration ---

func getLastCommitMessage(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "log", "--format=%B", "-n", "1").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func countCommits(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("git", "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-list --count HEAD: %v", err)
	}
	n := 0
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
	return n
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestCommitIteration_SingleLineSummary(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "change.txt", "change")

	if err := CommitIteration(1, "Fix Load to return zero Config"); err != nil {
		t.Fatalf("CommitIteration: %v", err)
	}

	msg := getLastCommitMessage(t)
	lines := strings.Split(msg, "\n")
	if lines[0] != "Fix Load to return zero Config" {
		t.Errorf("subject = %q, want %q", lines[0], "Fix Load to return zero Config")
	}
	if !strings.Contains(msg, "looper-iteration: 1") {
		t.Errorf("commit message missing trailer; full message:\n%s", msg)
	}
}

func TestCommitIteration_MultiLineSummary(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "change.txt", "change")

	summary := "Fix Load to return zero Config\n\nUpdated the config loader to handle edge cases.\nAdded tests for empty input."
	if err := CommitIteration(1, summary); err != nil {
		t.Fatalf("CommitIteration: %v", err)
	}

	msg := getLastCommitMessage(t)
	lines := strings.Split(msg, "\n")
	if lines[0] != "Fix Load to return zero Config" {
		t.Errorf("subject = %q, want %q", lines[0], "Fix Load to return zero Config")
	}
	if !strings.Contains(msg, "Updated the config loader to handle edge cases.") {
		t.Errorf("body missing expected content; full message:\n%s", msg)
	}
	if !strings.Contains(msg, "looper-iteration: 1") {
		t.Errorf("commit message missing trailer; full message:\n%s", msg)
	}
}

func TestCommitIteration_EmptySummary(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "change.txt", "change")

	if err := CommitIteration(1, ""); err != nil {
		t.Fatalf("CommitIteration: %v", err)
	}

	msg := getLastCommitMessage(t)
	lines := strings.Split(msg, "\n")
	if lines[0] != defaultIterationSubject {
		t.Errorf("subject = %q, want %q", lines[0], defaultIterationSubject)
	}
	if !strings.Contains(msg, "looper-iteration: 1") {
		t.Errorf("commit message missing trailer; full message:\n%s", msg)
	}
}

func TestCommitIteration_TrailerFormat(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "change.txt", "change")

	if err := CommitIteration(3, "Do something"); err != nil {
		t.Fatalf("CommitIteration: %v", err)
	}

	msg := getLastCommitMessage(t)
	// Ref: git trailer spec — trailer paragraph must be preceded by a blank line.
	if !strings.Contains(msg, "\n\nlooper-iteration: 3") {
		t.Errorf("trailer not preceded by blank line; full message:\n%s", msg)
	}
}

func TestCommitIteration_NoChanges(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	before := countCommits(t)

	if err := CommitIteration(1, "Fix something"); err != nil {
		t.Fatalf("CommitIteration: %v", err)
	}

	after := countCommits(t)
	if after != before {
		t.Errorf("commit count: got %d, want %d (no new commit expected)", after, before)
	}
}

func TestHasIterationWork_WithNormalIterationCommit(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "fix.txt", "fix content")
	if err := CommitIteration(2, "Fix the thing"); err != nil {
		t.Fatalf("CommitIteration: %v", err)
	}

	if !HasIterationWork() {
		t.Error("HasIterationWork() = false, want true after a normal iteration commit")
	}
}

// --- Head ---

func TestHead_ReturnsCommitHash(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")

	h := Head()
	if len(h) != 40 {
		t.Errorf("Head() = %q, want a 40-char hex SHA", h)
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Head() = %q, contains non-hex character %q", h, c)
			break
		}
	}
}

// --- Checkout error path ---

// --- CommitPolish ---

func TestCommitPolish_SubjectAndTrailer(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "change.txt", "polished")

	if err := CommitPolish("Refactor: tighten comments", ""); err != nil {
		t.Fatalf("CommitPolish: %v", err)
	}

	msg := getLastCommitMessage(t)
	lines := strings.Split(msg, "\n")
	if lines[0] != "Refactor: tighten comments" {
		t.Errorf("subject = %q, want %q", lines[0], "Refactor: tighten comments")
	}
	if !strings.Contains(msg, "looper-polish: true") {
		t.Errorf("commit message missing looper-polish trailer; full message:\n%s", msg)
	}
	if strings.Contains(msg, "looper-iteration:") {
		t.Errorf("polish commit must not have looper-iteration trailer; full message:\n%s", msg)
	}
}

func TestCommitPolish_WithBody(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "change.txt", "polished with body")

	subject := "Refactor: apply linters"
	body := "Ran go fmt and go vet; fixed alignment in config.go"
	if err := CommitPolish(subject, body); err != nil {
		t.Fatalf("CommitPolish: %v", err)
	}

	msg := getLastCommitMessage(t)
	if !strings.HasPrefix(msg, subject) {
		t.Errorf("subject = %q, want prefix %q", msg, subject)
	}
	if !strings.Contains(msg, body) {
		t.Errorf("commit message missing body %q; full message:\n%s", body, msg)
	}
	if !strings.Contains(msg, "looper-polish: true") {
		t.Errorf("commit message missing looper-polish trailer; full message:\n%s", msg)
	}
}

func TestCommitPolish_EmptySummaryFallsBack(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	writeFile(t, "change.txt", "polish fallback")

	if err := CommitPolish("", ""); err != nil {
		t.Fatalf("CommitPolish: %v", err)
	}

	msg := getLastCommitMessage(t)
	lines := strings.Split(msg, "\n")
	if lines[0] != defaultPolishSubject {
		t.Errorf("subject = %q, want %q", lines[0], defaultPolishSubject)
	}
}

func TestCommitPolish_NoChanges(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")
	before := countCommits(t)

	if err := CommitPolish("Refactor: nothing", ""); err != nil {
		t.Fatalf("CommitPolish: %v", err)
	}

	after := countCommits(t)
	if after != before {
		t.Errorf("commit count: got %d, want %d (no new commit expected when no changes)", after, before)
	}
}

func TestCheckout_NonexistentBranch(t *testing.T) {
	cleanup := initTempRepo(t)
	defer cleanup()

	makeCommit(t, "initial commit")

	err := Checkout("branch-that-does-not-exist")
	if err == nil {
		t.Fatal("Checkout(nonexistent) returned nil, want error")
	}
}
