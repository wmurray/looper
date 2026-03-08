package git

import (
	"os"
	"path/filepath"
	"testing"
)

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
	result := InferTicketFromPlan(path)
	if result != "DX-123" {
		t.Errorf("expected DX-123, got %q", result)
	}
}

func TestInferTicketFromPlan_InlineFormat(t *testing.T) {
	path := writePlanFile(t, "# My Plan\n\nThis relates to ticket: ABC-456\n")
	result := InferTicketFromPlan(path)
	if result != "ABC-456" {
		t.Errorf("expected ABC-456, got %q", result)
	}
}

func TestInferTicketFromPlan_NoTicket(t *testing.T) {
	path := writePlanFile(t, "# My Plan\n\nNo ticket mentioned here.\n")
	result := InferTicketFromPlan(path)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestInferTicketFromPlan_TicketBeyondLine10(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nTicket: LATE-999\n"
	path := writePlanFile(t, content)
	result := InferTicketFromPlan(path)
	if result != "" {
		t.Errorf("ticket beyond line 10 should not be found, got %q", result)
	}
}

func TestInferTicketFromPlan_NonexistentFile(t *testing.T) {
	result := InferTicketFromPlan("/nonexistent/path/PLAN.md")
	if result != "" {
		t.Errorf("expected empty string for missing file, got %q", result)
	}
}

func TestInferTicketFromPlan_CaseInsensitiveHeader(t *testing.T) {
	path := writePlanFile(t, "TICKET: XY-789\n")
	result := InferTicketFromPlan(path)
	if result != "XY-789" {
		t.Errorf("expected XY-789, got %q", result)
	}
}

// --- AssertRepo / AssertClean (require real git) ---
// These are integration-level and run only when git is available.

func TestAssertRepo_NotARepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Skipf("could not chdir to temp dir: %v", err)
	}
	err := AssertRepo()
	if err == nil {
		t.Fatal("expected error when not in a git repo")
	}
}
