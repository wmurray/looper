package guards

import (
	"testing"
)

// --- extractSentences ---

func TestExtractSentences_Basic(t *testing.T) {
	sentences := extractSentences("There is a bug in the handler. It causes a crash!")
	if len(sentences) < 2 {
		t.Fatalf("expected at least 2 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestExtractSentences_Deduplicates(t *testing.T) {
	sentences := extractSentences("There is a bug. There is a bug.")
	if len(sentences) != 1 {
		t.Fatalf("expected 1 unique sentence, got %d: %v", len(sentences), sentences)
	}
}

// --- hashSentences ---

func TestHashSentences_SameInputSameOutput(t *testing.T) {
	h1 := hashSentences([]string{"there is a bug in the handler"})
	h2 := hashSentences([]string{"there is a bug in the handler"})
	if h1 != h2 {
		t.Fatal("same input should produce same hash")
	}
}

func TestHashSentences_DifferentInputDifferentOutput(t *testing.T) {
	h1 := hashSentences([]string{"there is a bug in the handler"})
	h2 := hashSentences([]string{"everything looks good here"})
	if h1 == h2 {
		t.Fatal("different input should produce different hash")
	}
}

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
	// Short text with no extractable sentences (< 8 chars each fragment)
	result := s.CheckRepeatedIssues("OK.")
	if result.Triggered || result.Warning {
		t.Fatal("should not fire on clean review with no extractable sentences")
	}
}

func TestCheckRepeatedIssues_FirstOccurrence(t *testing.T) {
	s := &State{}
	result := s.CheckRepeatedIssues("There is a serious bug in the authentication handler.")
	if result.Triggered || result.Warning {
		t.Fatal("should not fire on first occurrence of issues")
	}
	if s.PrevIssueHash == "" {
		t.Fatal("PrevIssueHash should be set after first occurrence")
	}
}

func TestCheckRepeatedIssues_RepeatWarns(t *testing.T) {
	s := &State{}
	review := "There is a serious bug in the authentication handler."
	s.CheckRepeatedIssues(review)
	result := s.CheckRepeatedIssues(review)
	if result.Triggered {
		t.Fatal("should not trigger on second occurrence, only warn")
	}
	if !result.Warning {
		t.Fatal("should warn on second consecutive identical review")
	}
}

func TestCheckRepeatedIssues_RepeatTwiceTriggers(t *testing.T) {
	s := &State{}
	review := "There is a serious bug in the authentication handler."
	s.CheckRepeatedIssues(review)
	s.CheckRepeatedIssues(review)
	result := s.CheckRepeatedIssues(review)
	if !result.Triggered {
		t.Fatal("should trigger after three consecutive identical reviews")
	}
}

func TestCheckRepeatedIssues_DifferentIssuesReset(t *testing.T) {
	s := &State{}
	s.CheckRepeatedIssues("There is a serious bug in the authentication handler.")
	s.CheckRepeatedIssues("There is a serious bug in the authentication handler.") // warning
	result := s.CheckRepeatedIssues("The payment module has a null pointer issue.")
	if result.Triggered {
		t.Fatal("should not trigger when sentence content changes")
	}
	if s.StuckCount != 0 {
		t.Fatalf("StuckCount should reset to 0 on new sentences, got %d", s.StuckCount)
	}
}

func TestCheckRepeatedIssues_ResetsWhenClean(t *testing.T) {
	s := &State{}
	review := "There is a serious bug in the authentication handler."
	s.CheckRepeatedIssues(review)
	s.CheckRepeatedIssues(review) // warning
	s.CheckRepeatedIssues("OK.")  // clean — no extractable sentences
	if s.StuckCount != 0 {
		t.Fatalf("StuckCount should reset to 0 after clean review, got %d", s.StuckCount)
	}
	if s.PrevIssueHash != "" {
		t.Fatalf("PrevIssueHash should clear after clean review, got %q", s.PrevIssueHash)
	}
}

func TestCheckRepeatedIssues_SameKeywordsDifferentSentencesNoFire(t *testing.T) {
	s := &State{}
	s.CheckRepeatedIssues("The error occurs in the auth layer.")
	result := s.CheckRepeatedIssues("A different error appears in the payment module.")
	if result.Triggered || result.Warning {
		t.Fatal("different sentences should not fire even if they share keywords")
	}
}

func TestCheckRepeatedIssues_ExactSentenceRepeatFires(t *testing.T) {
	s := &State{}
	review := "The handler returns a nil pointer on every request."
	s.CheckRepeatedIssues(review)
	result := s.CheckRepeatedIssues(review)
	if !result.Warning {
		t.Fatal("should warn on second consecutive identical sentence")
	}
	result = s.CheckRepeatedIssues(review)
	if !result.Triggered {
		t.Fatal("should trigger on third consecutive identical sentence")
	}
}

func TestCheckRepeatedIssues_EmptyInput(t *testing.T) {
	// Invariant: empty reviewer output (no reviewers configured) resets stuck state and must not panic or trigger.
	s := &State{}
	result := s.CheckRepeatedIssues("")
	if result.Triggered || result.Warning {
		t.Errorf("empty input should not trigger or warn, got %+v", result)
	}
	if s.StuckCount != 0 || s.PrevIssueHash != "" {
		t.Errorf("empty input should reset stuck state, got StuckCount=%d PrevIssueHash=%q", s.StuckCount, s.PrevIssueHash)
	}
}
