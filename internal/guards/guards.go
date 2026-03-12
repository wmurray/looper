package guards

import (
	"regexp"
	"sort"
	"strings"
)

// GuardResult describes the outcome of a guard check.
type GuardResult struct {
	// Triggered means the guard threshold was crossed — abort the loop.
	Triggered bool
	// Warning means the guard fired but hasn't reached the abort threshold yet.
	Warning bool
	Message string
}

// State tracks running guard counters across loop iterations.
type State struct {
	ThrashCount int
	StuckCount  int
	PrevIssues  string
}

// CheckNoChanges fires if neither the diff nor HEAD changed (no work done).
// 2 consecutive no-work iterations triggers an abort.
func (s *State) CheckNoChanges(gitDiff string, headChanged bool) GuardResult {
	if strings.TrimSpace(gitDiff) != "" || headChanged {
		s.ThrashCount = 0
		return GuardResult{}
	}

	s.ThrashCount++
	if s.ThrashCount >= 2 {
		return GuardResult{
			Triggered: true,
			Message:   "No changes in 2 consecutive iterations — agent appears stuck",
		}
	}
	return GuardResult{
		Warning: true,
		Message: "No changes detected (1/2 before abort)",
	}
}

var issueKeywords = regexp.MustCompile(`(?i)\b(TODO|FIXME|bug|issue|error|fail|undefined|nil)\b`)

// CheckRepeatedIssues fires if the same issue keywords appear in consecutive reviews.
// 2 consecutive matches triggers an abort.
func (s *State) CheckRepeatedIssues(reviewOutput string) GuardResult {
	matches := issueKeywords.FindAllString(reviewOutput, -1)

	// Normalize: lowercase + deduplicate + sort
	seen := map[string]bool{}
	for _, m := range matches {
		seen[strings.ToLower(m)] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	current := strings.Join(keys, ",")

	if current == "" {
		s.StuckCount = 0
		s.PrevIssues = ""
		return GuardResult{}
	}

	if s.PrevIssues != "" && current == s.PrevIssues {
		s.StuckCount++
		if s.StuckCount >= 2 {
			return GuardResult{
				Triggered: true,
				Message:   "Same issues repeated in 2 consecutive reviews: " + current,
			}
		}
		return GuardResult{
			Warning: true,
			Message: "Same issues appearing again (1/2 before abort): " + current,
		}
	}

	s.StuckCount = 0
	s.PrevIssues = current
	return GuardResult{}
}
