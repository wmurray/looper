package selector_test

import (
	"testing"

	"github.com/willmurray/looper/internal/agent"
	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/detect"
	"github.com/willmurray/looper/internal/selector"
)

func TestSelectReviewersGeneralOnly(t *testing.T) {
	t.Parallel()
	reviewers := config.Reviewers{General: "general.md", Specialized: []string{"spec.md"}}
	strategy := config.ReviewStrategy{Mode: "general-only", GeneralEvery: 1, MajorityThreshold: 0.6}
	got := selector.SelectReviewers(reviewers, strategy, nil, detect.Detection{}, 1, 5)
	if len(got) != 1 || got[0] != "general.md" {
		t.Errorf("general-only: got %v, want [general.md]", got)
	}
}

func TestSelectReviewersAlways(t *testing.T) {
	t.Parallel()
	reviewers := config.Reviewers{General: "general.md", Specialized: []string{"spec1.md", "spec2.md"}}
	strategy := config.ReviewStrategy{Mode: "always", GeneralEvery: 1, MajorityThreshold: 0.6}
	got := selector.SelectReviewers(reviewers, strategy, nil, detect.Detection{}, 1, 5)
	if len(got) != 3 {
		t.Fatalf("always: got %v, want 3 reviewers", got)
	}
	if got[0] != "general.md" {
		t.Errorf("general should be first, got %v", got)
	}
}

func TestSelectReviewersSmartSchedule(t *testing.T) {
	t.Parallel()
	reviewers := config.Reviewers{General: "general.md", Specialized: []string{"spec.md"}}
	strategy := config.ReviewStrategy{Mode: "smart", GeneralEvery: 1, SpecializedEvery: 3, MajorityThreshold: 0.6}
	metadata := map[string]agent.Metadata{} // no language match

	// Iteration 3: specialized_every = 3, so should include specialized.
	got := selector.SelectReviewers(reviewers, strategy, metadata, detect.Detection{}, 3, 5)
	found := false
	for _, r := range got {
		if r == "spec.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("iteration 3: specialized should appear at i%%3==0, got %v", got)
	}

	// Iteration 2: should not include specialized.
	got2 := selector.SelectReviewers(reviewers, strategy, metadata, detect.Detection{}, 2, 5)
	for _, r := range got2 {
		if r == "spec.md" {
			t.Errorf("iteration 2: specialized should not appear, got %v", got2)
		}
	}
}

func TestSelectReviewersLanguageMatch(t *testing.T) {
	t.Parallel()
	reviewers := config.Reviewers{General: "general.md", Specialized: []string{"go-reviewer.md"}}
	strategy := config.ReviewStrategy{Mode: "smart", GeneralEvery: 1, SpecializedEvery: 10, MajorityThreshold: 0.6}
	metadata := map[string]agent.Metadata{
		"go-reviewer.md": {Languages: []string{"go"}},
	}
	detected := detect.Detection{Languages: []string{"go"}}

	// Iteration 1: not on schedule, but language matches.
	got := selector.SelectReviewers(reviewers, strategy, metadata, detected, 1, 5)
	found := false
	for _, r := range got {
		if r == "go-reviewer.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("language match: go-reviewer.md should appear when go detected, got %v", got)
	}
}

func TestSelectReviewersDedup(t *testing.T) {
	t.Parallel()
	// general and specialized are the same path.
	reviewers := config.Reviewers{General: "agent.md", Specialized: []string{"agent.md"}}
	strategy := config.ReviewStrategy{Mode: "always", GeneralEvery: 1, MajorityThreshold: 0.6}
	got := selector.SelectReviewers(reviewers, strategy, nil, detect.Detection{}, 1, 5)
	if len(got) != 1 {
		t.Errorf("dedup: expected 1 reviewer, got %v", got)
	}
}

func TestMajorityApproved(t *testing.T) {
	t.Parallel()
	cases := []struct {
		approvals, total int
		threshold        float64
		want             bool
	}{
		{2, 3, 0.6, true},
		{1, 3, 0.6, false},
		{3, 3, 0.6, true},
		{0, 3, 0.6, false},
		{0, 0, 0.6, false}, // no reviewers → not approved
	}
	for _, tc := range cases {
		got := selector.MajorityApproved(tc.approvals, tc.total, tc.threshold)
		if got != tc.want {
			t.Errorf("MajorityApproved(%d, %d, %.1f) = %v, want %v", tc.approvals, tc.total, tc.threshold, got, tc.want)
		}
	}
}
