package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)


func run(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// AssertRepo returns an error if not in a git repository.
func AssertRepo() error {
	if _, err := run("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not in a git repository")
	}
	return nil
}

// AssertClean returns an error if there are uncommitted changes.
func AssertClean() error {
	out, err := run("status", "--porcelain")
	if err != nil {
		return fmt.Errorf("could not get git status: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("repository has uncommitted changes\nPlease commit or stash changes before running")
	}
	return nil
}

// InferTicketFromBranch returns a ticket ID from the current branch name, or "".
func InferTicketFromBranch(re *regexp.Regexp) string {
	branch, err := run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return re.FindString(branch)
}

// InferTicketFromPlan scans the first 10 lines of a plan file for a ticket ID.
func InferTicketFromPlan(path string, re *regexp.Regexp) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < 10 && scanner.Scan(); i++ {
		line := scanner.Text()
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "ticket:") || strings.Contains(lower, "ticket:") {
			if match := re.FindString(line); match != "" {
				return match
			}
		}
	}
	return ""
}

// Diff returns the output of `git diff` (unstaged changes).
func Diff() string {
	out, _ := run("diff")
	return out
}

// StatusShort returns the output of `git status --short`.
func StatusShort() string {
	out, _ := run("status", "--short")
	return out
}

// CommitIteration commits all changes for a given iteration number.
// summary is a brief description of what was done (first line used).
// No-ops if the working tree is clean.
func CommitIteration(n int, summary string) error {
	diff := Diff()
	status, _ := run("status", "--porcelain")
	if strings.TrimSpace(diff) == "" && strings.TrimSpace(status) == "" {
		return nil
	}

	if _, err := exec.Command("git", "add", "-A").Output(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	desc := firstLine(summary)
	var msg string
	if desc != "" {
		msg = fmt.Sprintf("Iteration %d: %s", n, desc)
	} else {
		msg = fmt.Sprintf("Iteration %d: execution and review cycle", n)
	}

	if out, err := exec.Command("git", "commit", "-m", msg, "--quiet").CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s", string(out))
	}
	return nil
}

// firstLine returns the first non-empty line of s, trimmed.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// CommitWIP commits with a WIP message (used on timeout/failure).
func CommitWIP(iteration int, phase string) error {
	status, _ := run("status", "--porcelain")
	if strings.TrimSpace(status) == "" {
		return nil
	}

	if _, err := exec.Command("git", "add", "-A").Output(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	msg := fmt.Sprintf("WIP: Iteration %d - timeout during %s", iteration, phase)
	if out, err := exec.Command("git", "commit", "-m", msg).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s", string(out))
	}
	return nil
}

// RecentCommits returns the last n commit messages as a string.
func RecentCommits(n int) string {
	out, _ := run("log", "--oneline", fmt.Sprintf("-n%d", n))
	return out
}

// BranchExists reports whether a local branch with the given name exists.
func BranchExists(name string) bool {
	_, err := run("rev-parse", "--verify", name)
	return err == nil
}

// Checkout switches to an existing branch.
func Checkout(name string) error {
	if out, err := exec.Command("git", "checkout", name).CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// HasIterationWork reports whether the current branch has any iteration or WIP
// commits (i.e. the implement loop has run at least once).
//
// To avoid false positives from matching commits on the base branch, only
// commits reachable from HEAD but not from any other local branch are searched.
func HasIterationWork() bool {
	// Build an exclusion list of all other local branches.
	// git log HEAD ^branchA ^branchB ... scopes output to this branch's unique commits.
	allRefs, _ := run("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	current, _ := run("rev-parse", "--abbrev-ref", "HEAD")

	// Multiple --grep patterns use OR semantics by default (not AND).
	// Do not add --all-match here — we want either pattern to be a match.
	args := []string{"log", "--oneline", "--grep=^Iteration ", "--grep=^WIP: Iteration"}
	for _, b := range strings.Split(allRefs, "\n") {
		b = strings.TrimSpace(b)
		if b != "" && b != strings.TrimSpace(current) {
			args = append(args, "^"+b)
		}
	}

	// Ignore the error: if git log fails, conservatively report no iteration work.
	out, _ := run(args...)
	return strings.TrimSpace(out) != ""
}

// CheckoutNewBranch creates and switches to a new branch.
func CheckoutNewBranch(name string) error {
	if out, err := exec.Command("git", "checkout", "-b", name).CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout -b: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

