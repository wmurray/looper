package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/willmurray/looper/internal/discover"
	"github.com/willmurray/looper/internal/ui"
)

// stackIndicator maps a project root filename to the stack keyword it signals.
var stackIndicators = []struct {
	file    string
	keyword string
}{
	{"go.mod", "go"},
	{"Gemfile", "rails"},
	{"package.json", "node"},
	{"pyproject.toml", "python"},
	{"requirements.txt", "python"},
	{"Cargo.toml", "rust"},
	{"pom.xml", "java"},
	{"build.gradle", "java"},
}

// warnOnStackMismatch checks whether the configured reviewer agent's basename
// contains a keyword that matches the detected project stack. If no stack
// indicators are found the function is silent (greenfield). If a mismatch is
// detected it warns and, when available, suggests a better agent from
// ~/.claude/.
func warnOnStackMismatch(projectDir, reviewerBasename string) {
	keyword := detectStack(projectDir)
	if keyword == "" {
		return // greenfield — nothing to check
	}

	reviewerLower := strings.ToLower(reviewerBasename)
	if strings.Contains(reviewerLower, keyword) {
		return // reviewer matches detected stack
	}

	ui.Warn("reviewer_agent may not match the project stack (detected: %s, reviewer: %s)", keyword, reviewerBasename)

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	found, err := discover.Scan(home)
	if err != nil || len(found) == 0 {
		return
	}

	for _, f := range found {
		if f.Kind != discover.KindAgent {
			continue
		}
		if strings.Contains(strings.ToLower(filepath.Base(f.Path)), keyword) {
			ui.Warn("Consider: looper settings set reviewer_agent %s", f.Path)
			return
		}
	}
}

// detectStack returns the first stack keyword matched by a known indicator
// file in projectDir, or "" if none are found.
func detectStack(projectDir string) string {
	for _, ind := range stackIndicators {
		if _, err := os.Stat(filepath.Join(projectDir, ind.file)); err == nil {
			return ind.keyword
		}
	}
	return ""
}
