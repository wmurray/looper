package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/willmurray/looper/internal/discover"
	"github.com/willmurray/looper/internal/ui"
)

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

// Why: silent when no stack is detected so greenfield repos don't generate noise.
func warnOnStackMismatch(projectDir, reviewerBasename string) {
	keyword := detectStack(projectDir)
	if keyword == "" {
		return
	}

	reviewerLower := strings.ToLower(reviewerBasename)
	if strings.Contains(reviewerLower, keyword) {
		return
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

func detectStack(projectDir string) string {
	stacks := detectAllStacks(projectDir)
	if len(stacks) == 0 {
		return ""
	}
	return stacks[0]
}

func detectAllStacks(projectDir string) []string {
	var stacks []string
	seen := make(map[string]bool)
	for _, ind := range stackIndicators {
		if _, err := os.Stat(filepath.Join(projectDir, ind.file)); err == nil {
			if !seen[ind.keyword] {
				stacks = append(stacks, ind.keyword)
				seen[ind.keyword] = true
			}
		}
	}
	return stacks
}
