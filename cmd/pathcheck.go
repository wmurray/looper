package cmd

import (
	"os"
	"path/filepath"

	"github.com/willmurray/looper/internal/discover"
	"github.com/willmurray/looper/internal/ui"
)

// Gotcha: returns true when the path is missing so callers can gate further checks.
func warnIfPathMissing(key, configuredPath string) bool {
	if _, err := os.Stat(configuredPath); err == nil {
		return false
	}

	ui.Warn("%s not found: %s", key, configuredPath)

	home, err := os.UserHomeDir()
	if err != nil {
		ui.Warn("Set it with: looper settings set %s <path>", key)
		return true
	}

	found, err := discover.Scan(home)
	if err != nil || len(found) == 0 {
		ui.Warn("Set it with: looper settings set %s <path>", key)
		return true
	}

	want := filepath.Base(configuredPath)
	var matches []string
	for _, f := range found {
		if filepath.Base(f.Path) == want {
			matches = append(matches, f.Path)
		}
	}

	switch len(matches) {
	case 0:
		ui.Warn("Set it with: looper settings set %s <path>", key)
	case 1:
		ui.Warn("Did you mean? Run: looper settings set %s %s", key, matches[0])
	default:
		ui.Warn("Multiple candidates found. Run one of:")
		for _, m := range matches {
			ui.Warn("  looper settings set %s %s", key, m)
		}
	}
	return true
}
