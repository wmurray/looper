package guards

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

type GuardResult struct {
	// Why: signals the caller to abort the loop; threshold was crossed.
	Triggered bool
	// Why: signals the caller to warn but continue; threshold not yet reached.
	Warning bool
	Message string
}

type State struct {
	ThrashCount   int
	StuckCount    int
	PrevIssueHash string
}

// Gotcha: triggers after 2 consecutive no-work iterations, not 1.
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

var sentenceSplitter = regexp.MustCompile(`[.!?]+(?:\s+|$)`)

// Gotcha: filters sentences shorter than 8 chars to avoid matching trivial words.
func extractSentences(text string) []string {
	parts := sentenceSplitter.Split(text, -1)
	seen := map[string]bool{}
	result := []string{}
	for _, p := range parts {
		s := strings.ToLower(strings.TrimSpace(p))
		if len(s) < 8 || seen[s] {
			continue
		}
		seen[s] = true
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

// Invariant: output is order-independent — sentences are sorted before hashing.
func hashSentences(sentences []string) string {
	digests := make([]string, len(sentences))
	for i, s := range sentences {
		sum := sha256.Sum256([]byte(s))
		digests[i] = hex.EncodeToString(sum[:])
	}
	sort.Strings(digests)
	return strings.Join(digests, ",")
}

// Gotcha: triggers after 2 consecutive reviews with identical sentence fingerprints, not 1.
func (s *State) CheckRepeatedIssues(reviewOutput string) GuardResult {
	sentences := extractSentences(reviewOutput)

	if len(sentences) == 0 {
		s.StuckCount = 0
		s.PrevIssueHash = ""
		return GuardResult{}
	}

	current := hashSentences(sentences)

	if s.PrevIssueHash != "" && current == s.PrevIssueHash {
		s.StuckCount++
		if s.StuckCount >= 2 {
			return GuardResult{
				Triggered: true,
				Message:   "Same issues repeated in 2 consecutive reviews",
			}
		}
		return GuardResult{
			Warning: true,
			Message: "Same issues appearing again (1/2 before abort)",
		}
	}

	s.StuckCount = 0
	s.PrevIssueHash = current
	return GuardResult{}
}
