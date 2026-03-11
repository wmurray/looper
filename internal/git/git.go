package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// defaultIterationSubject is used when the agent returns an empty summary.
const defaultIterationSubject = "Apply iteration changes"


func run(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// RepoRoot returns the absolute path of the root of the current git repository,
// or an error if the working directory is not inside a git repository.
func RepoRoot() (string, error) {
	root, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}
	return root, nil
}

// AssertRepo returns an error if not in a git repository.
func AssertRepo() error {
	if _, err := run("rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
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
// Gotcha: empty summary falls back to "Apply iteration changes".
func CommitIteration(n int, summary string) error {
	diff := Diff()
	status, _ := run("status", "--porcelain")
	if strings.TrimSpace(diff) == "" && strings.TrimSpace(status) == "" {
		return nil
	}

	if _, err := exec.Command("git", "add", "-A").Output(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	subject, body := splitSummary(summary)
	trailer := fmt.Sprintf("looper-iteration: %d", n)
	args := []string{"commit", "--quiet", "-m", subject}
	if body != "" {
		args = append(args, "-m", body)
	}
	args = append(args, "-m", trailer)

	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Gotcha: empty or whitespace-only summary returns "Apply iteration changes".
func splitSummary(summary string) (subject, body string) {
	lines := strings.Split(summary, "\n")
	subjectIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			subject = strings.TrimSpace(line)
			subjectIdx = i
			break
		}
	}
	if subjectIdx == -1 {
		return defaultIterationSubject, ""
	}
	body = strings.TrimSpace(strings.Join(lines[subjectIdx+1:], "\n"))
	return subject, body
}

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
		return fmt.Errorf("git commit: %w\n%s", err, strings.TrimSpace(string(out)))
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
func HasIterationWork() bool {
	allRefs, _ := run("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	current, _ := run("rev-parse", "--abbrev-ref", "HEAD")

	var exclusions []string
	for _, b := range strings.Split(allRefs, "\n") {
		b = strings.TrimSpace(b)
		if b != "" && b != strings.TrimSpace(current) {
			exclusions = append(exclusions, "^"+b)
		}
	}

	iterArgs := append([]string{"log", "--oneline", "--grep=looper-iteration:"}, exclusions...)
	iterOut, _ := run(iterArgs...)
	if strings.TrimSpace(iterOut) != "" {
		return true
	}

	// Why: CommitWIP doesn't add the trailer; grep its prefix as a fallback.

	wipArgs := append([]string{"log", "--oneline", "--grep=^WIP: Iteration"}, exclusions...)
	wipOut, _ := run(wipArgs...)
	return strings.TrimSpace(wipOut) != ""
}

// CheckoutNewBranch creates and switches to a new branch.
func CheckoutNewBranch(name string) error {
	if out, err := exec.Command("git", "checkout", "-b", name).CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout -b: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

