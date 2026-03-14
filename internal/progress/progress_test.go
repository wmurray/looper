package progress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestWriter(t *testing.T) (*Writer, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "PROGRESS.md")
	w := New(path, "DX-123", "DX-123_PLAN.md", 5, 420)
	return w, path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read progress file: %v", err)
	}
	return string(data)
}

func assertContains(t *testing.T, content, substr string) {
	t.Helper()
	if !strings.Contains(content, substr) {
		t.Errorf("expected progress file to contain %q\n\ngot:\n%s", substr, content)
	}
}

// --- WriteHeader ---

func TestWriteHeader_ContainsTicket(t *testing.T) {
	w, path := newTestWriter(t)
	if err := w.WriteHeader(); err != nil {
		t.Fatalf("WriteHeader error: %v", err)
	}
	content := readFile(t, path)
	assertContains(t, content, "DX-123")
	assertContains(t, content, "DX-123_PLAN.md")
	assertContains(t, content, "Max Cycles:** 5")
	assertContains(t, content, "Timeout per Iteration:** 420s")
}

func TestWriteHeader_OverwritesExisting(t *testing.T) {
	w, path := newTestWriter(t)
	os.WriteFile(path, []byte("old content"), 0644)
	w.WriteHeader()
	content := readFile(t, path)
	if strings.Contains(content, "old content") {
		t.Error("WriteHeader should overwrite existing file")
	}
}

// --- BeginRun ---

func TestBeginRun_AppendsRunHeader(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	if err := w.BeginRun(2); err != nil {
		t.Fatalf("BeginRun error: %v", err)
	}
	assertContains(t, readFile(t, path), "## RUN 2")
}

// --- WriteExecution ---

func TestWriteExecution_ContainsOutput(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.BeginRun(1)
	err := w.WriteExecution("Added auth middleware")
	if err != nil {
		t.Fatalf("WriteExecution error: %v", err)
	}
	content := readFile(t, path)
	assertContains(t, content, "### Execution")
	assertContains(t, content, "Added auth middleware")
}

// --- WriteReview ---

func TestWriteReview_ContainsOutput(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.BeginRun(1)
	err := w.WriteReview("Needs work: missing tests")
	if err != nil {
		t.Fatalf("WriteReview error: %v", err)
	}
	content := readFile(t, path)
	assertContains(t, content, "### Review")
	assertContains(t, content, "Needs work: missing tests")
}

// --- WriteGuardAlert / WriteGuardTriggered ---

func TestWriteGuardAlert(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.WriteGuardAlert("No changes detected (1/2 before abort)")
	assertContains(t, readFile(t, path), "Guard Alert")
	assertContains(t, readFile(t, path), "No changes detected")
}

func TestWriteGuardTriggered(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.WriteGuardTriggered("No changes in 2 consecutive iterations")
	assertContains(t, readFile(t, path), "Guard Triggered")
	assertContains(t, readFile(t, path), "No changes in 2 consecutive iterations")
}

// --- WriteSummary ---

func TestWriteSummary_Complete(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	err := w.WriteSummary("complete", 3, 0, 0, "abc1234 Iteration 3")
	if err != nil {
		t.Fatalf("WriteSummary error: %v", err)
	}
	content := readFile(t, path)
	assertContains(t, content, "## Summary Report")
	assertContains(t, content, "complete")
	assertContains(t, content, "3 of 5")
	assertContains(t, content, "abc1234 Iteration 3")
	assertContains(t, content, "Review changes")
}

func TestWriteSummary_GuardTriggered(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.WriteSummary("aborted — no changes", 2, 2, 0, "")
	content := readFile(t, path)
	assertContains(t, content, "Guards Triggered")
	assertContains(t, content, "No changes detected: 2 time(s)")
}

func TestWriteSummary_Interrupted(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.WriteSummary("interrupted", 1, 0, 0, "")
	content := readFile(t, path)
	assertContains(t, content, "interrupted")
	assertContains(t, content, "Fix issues and rerun")
}

// --- WriteRetry ---

func TestWriteRetry_AppendsWarningLine(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	err := w.WriteRetry("execution", 1, 3, "rate limit exceeded")
	if err != nil {
		t.Fatalf("WriteRetry error: %v", err)
	}
	content := readFile(t, path)
	assertContains(t, content, "⚠")
	assertContains(t, content, "Retry")
	assertContains(t, content, "execution phase")
	assertContains(t, content, "attempt 1 of 3")
	assertContains(t, content, `"rate limit exceeded"`)
}

func TestWriteRetry_IncludesPhaseAndAttempt(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.WriteRetry("review", 2, 5, "overloaded")
	content := readFile(t, path)
	assertContains(t, content, "review phase")
	assertContains(t, content, "attempt 2 of 5")
}

// --- WriteIterationTime ---

func TestWriteIterationTime_OverTimeout(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.WriteIterationTime(500)
	assertContains(t, readFile(t, path), "Guard Alert")
}

func TestWriteIterationTime_UnderTimeout(t *testing.T) {
	w, path := newTestWriter(t)
	w.WriteHeader()
	w.WriteIterationTime(100)
	content := readFile(t, path)
	if strings.Contains(content, "Guard Alert") {
		t.Error("should not write guard alert when under timeout")
	}
}
