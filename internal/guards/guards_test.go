package guards

import (
	"testing"
)

// --- CheckNoChanges ---

func TestCheckNoChanges_NoChangeFirstTime(t *testing.T) {
	s := &State{}
	result := s.CheckNoChanges("", false)
	if result.Triggered {
		t.Fatal("should not trigger on first empty diff")
	}
	if !result.Warning {
		t.Fatal("should warn on first empty diff")
	}
	if s.ThrashCount != 1 {
		t.Fatalf("expected ThrashCount 1, got %d", s.ThrashCount)
	}
}

func TestCheckNoChanges_NoChangeTwice_Triggers(t *testing.T) {
	s := &State{}
	s.CheckNoChanges("", false)
	result := s.CheckNoChanges("", false)
	if !result.Triggered {
		t.Fatal("should trigger after two consecutive empty diffs")
	}
	if result.Warning {
		t.Fatal("Triggered and Warning should not both be true")
	}
}

func TestCheckNoChanges_ResetsOnChange(t *testing.T) {
	s := &State{}
	s.CheckNoChanges("", false) // strike 1
	result := s.CheckNoChanges("some diff content", false)
	if result.Triggered || result.Warning {
		t.Fatal("should not fire when diff is non-empty")
	}
	if s.ThrashCount != 0 {
		t.Fatalf("ThrashCount should reset to 0, got %d", s.ThrashCount)
	}
}

func TestCheckNoChanges_NoFireOnNonEmptyDiff(t *testing.T) {
	s := &State{}
	result := s.CheckNoChanges("diff --git a/foo.go b/foo.go\n+added line", false)
	if result.Triggered || result.Warning {
		t.Fatal("should not fire when diff has content")
	}
}

func TestCheckNoChanges_StrikeResetAfterChange(t *testing.T) {
	s := &State{}
	s.CheckNoChanges("", false) // strike 1
	s.CheckNoChanges("real change", false)
	result := s.CheckNoChanges("", false)
	if result.Triggered {
		t.Fatal("should not trigger — counter was reset")
	}
	if !result.Warning {
		t.Fatal("should warn on first empty diff after reset")
	}
}

func TestCheckNoChanges_HeadChangedResetsCounter(t *testing.T) {
	s := &State{}
	s.CheckNoChanges("", false) // strike 1
	result := s.CheckNoChanges("", true)
	if result.Triggered || result.Warning {
		t.Fatal("should not fire when HEAD changed (agent committed its own work)")
	}
	if s.ThrashCount != 0 {
		t.Fatalf("ThrashCount should reset to 0 when HEAD changed, got %d", s.ThrashCount)
	}
}

func TestCheckNoChanges_HeadUnchangedStillCounts(t *testing.T) {
	s := &State{}
	result := s.CheckNoChanges("", false)
	if result.Triggered {
		t.Fatal("should not trigger on first empty diff with no HEAD change")
	}
	if !result.Warning {
		t.Fatal("should warn on first empty diff with no HEAD change")
	}
	if s.ThrashCount != 1 {
		t.Fatalf("ThrashCount should be 1, got %d", s.ThrashCount)
	}
}

// --- CheckRepeatedIssues ---

func TestCheckRepeatedIssues_NoIssues(t *testing.T) {
	s := &State{}
	result := s.CheckRepeatedIssues("Everything looks great, all tests pass.")
	if result.Triggered || result.Warning {
		t.Fatal("should not fire when no issue keywords present")
	}
}

func TestCheckRepeatedIssues_FirstOccurrence(t *testing.T) {
	s := &State{}
	result := s.CheckRepeatedIssues("There is a bug in the handler")
	if result.Triggered || result.Warning {
		t.Fatal("should not fire on first occurrence of issues")
	}
	if s.PrevIssues == "" {
		t.Fatal("PrevIssues should be set after first occurrence")
	}
}

func TestCheckRepeatedIssues_RepeatWarns(t *testing.T) {
	s := &State{}
	s.CheckRepeatedIssues("There is a bug in the handler")
	result := s.CheckRepeatedIssues("Found a bug that needs fixing")
	if result.Triggered {
		t.Fatal("should not trigger on second occurrence, only warn")
	}
	if !result.Warning {
		t.Fatal("should warn on second consecutive same issues")
	}
}

func TestCheckRepeatedIssues_RepeatTwiceTriggers(t *testing.T) {
	s := &State{}
	s.CheckRepeatedIssues("There is a bug in the handler")
	s.CheckRepeatedIssues("Found a bug that needs fixing")
	result := s.CheckRepeatedIssues("Still a bug here")
	if !result.Triggered {
		t.Fatal("should trigger after three consecutive identical issue sets")
	}
}

func TestCheckRepeatedIssues_DifferentIssuesReset(t *testing.T) {
	s := &State{}
	s.CheckRepeatedIssues("There is a bug")
	s.CheckRepeatedIssues("Found a bug")    // same keyword → warning
	result := s.CheckRepeatedIssues("TODO: fix this undefined reference")
	if result.Triggered {
		t.Fatal("should not trigger when issue keywords change")
	}
	if s.StuckCount != 0 {
		t.Fatalf("StuckCount should reset to 0 on new issues, got %d", s.StuckCount)
	}
}

func TestCheckRepeatedIssues_CaseInsensitive(t *testing.T) {
	s := &State{}
	s.CheckRepeatedIssues("BUG found")
	result := s.CheckRepeatedIssues("bug found")
	if result.Triggered {
		t.Fatal("should not trigger on second occurrence")
	}
	if !result.Warning {
		t.Fatal("should warn — same keywords regardless of case")
	}
}

func TestCheckRepeatedIssues_ResetsWhenClean(t *testing.T) {
	s := &State{}
	s.CheckRepeatedIssues("bug bug bug")
	s.CheckRepeatedIssues("bug again")
	s.CheckRepeatedIssues("Everything looks great")
	if s.StuckCount != 0 {
		t.Fatalf("StuckCount should reset to 0 after clean review, got %d", s.StuckCount)
	}
	if s.PrevIssues != "" {
		t.Fatalf("PrevIssues should clear after clean review, got %q", s.PrevIssues)
	}
}
