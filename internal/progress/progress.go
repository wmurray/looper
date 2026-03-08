package progress

import (
	"fmt"
	"os"
	"time"
)

// Writer appends structured markdown to a progress file.
type Writer struct {
	path       string
	ticket     string
	planFile   string
	maxCycles  int
	timeoutSec int
}

func New(path, ticket, planFile string, maxCycles, timeoutSec int) *Writer {
	return &Writer{
		path:       path,
		ticket:     ticket,
		planFile:   planFile,
		maxCycles:  maxCycles,
		timeoutSec: timeoutSec,
	}
}

func (w *Writer) WriteHeader() error {
	content := fmt.Sprintf(`# Progress for Ticket %s

**Plan File:** %s
**Max Cycles:** %d
**Timeout per Iteration:** %ds
**Started:** %s

---

`, w.ticket, w.planFile, w.maxCycles, w.timeoutSec, time.Now().Format(time.RFC1123))
	return os.WriteFile(w.path, []byte(content), 0644)
}

func (w *Writer) BeginRun(i int) error {
	return w.append(fmt.Sprintf("\n## RUN %d\n\n", i))
}

func (w *Writer) WriteExecution(output, gitStatus, gitDiff string) error {
	return w.append(fmt.Sprintf(`### Execution

%s

**Git Status:**
`+"```"+`
%s
`+"```"+`

**Git Diff:**
`+"```diff"+`
%s
`+"```"+`

`, output, gitStatus, gitDiff))
}

func (w *Writer) WriteReview(output string) error {
	return w.append(fmt.Sprintf("### Review\n\n%s\n\n", output))
}

func (w *Writer) WriteGuardAlert(msg string) error {
	return w.append(fmt.Sprintf("⚠ **Guard Alert**: %s\n\n", msg))
}

func (w *Writer) WriteGuardTriggered(msg string) error {
	return w.append(fmt.Sprintf("⚠ **Guard Triggered**: %s\n\n", msg))
}

func (w *Writer) WriteIterationTime(secs int64) error {
	if secs > int64(w.timeoutSec) {
		return w.append(fmt.Sprintf("⚠ **Guard Alert**: Iteration took %ds (timeout: %ds)\n\n", secs, w.timeoutSec))
	}
	return nil
}

func (w *Writer) WriteSuccess(iteration int) error {
	return w.append(fmt.Sprintf("\n👷 Job's done - completed in %d of %d iterations\n", iteration, w.maxCycles))
}

func (w *Writer) WriteSummary(status string, finalIteration, thrashCount, stuckCount int, recentCommits string) error {
	var guards string
	if thrashCount > 0 || stuckCount > 0 {
		guards = "### Guards Triggered\n"
		if thrashCount > 0 {
			guards += fmt.Sprintf("- No changes detected: %d time(s)\n", thrashCount)
		}
		if stuckCount > 0 {
			guards += fmt.Sprintf("- Repeated issues: %d time(s)\n", stuckCount)
		}
		guards += "\n"
	}

	var nextSteps string
	if status == "complete" {
		nextSteps = `1. Review changes: ` + "`git log --patch`" + `
2. Manually test if needed
3. Optionally squash commits: ` + "`git rebase -i`" + `
4. Create PR and request human review`
	} else {
		nextSteps = fmt.Sprintf(`1. Review progress file: `+"`cat %s`"+`
2. Investigate guard triggers above
3. Fix issues and rerun, or continue manually`, w.path)
	}

	content := fmt.Sprintf(`
---

## Summary Report

**Status:** %s
**Iterations:** %d of %d
**Timeout per Iteration:** %ds
**Ticket:** %s
**Completed:** %s

%s### Commits Made
`+"```"+`
%s
`+"```"+`

### Next Steps
%s
`, status, finalIteration, w.maxCycles, w.timeoutSec, w.ticket,
		time.Now().Format(time.RFC1123), guards, recentCommits, nextSteps)

	return w.append(content)
}

func (w *Writer) append(s string) error {
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(s)
	return err
}
