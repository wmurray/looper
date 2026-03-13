// Package discover scans canonical ~/.claude/ locations for installed skill
// and agent files, so looper can auto-suggest or auto-apply configuration.
package discover

import (
	"os"
	"path/filepath"
)

// Kind identifies whether a found file is a skill or an agent.
type Kind int

const (
	KindSkill Kind = iota
	KindAgent
)

func (k Kind) String() string {
	if k == KindSkill {
		return "skill"
	}
	return "agent"
}

// Found represents a single discovered skill or agent file.
type Found struct {
	Kind Kind
	Path string // absolute path
}

// Scan globs all canonical ~/.claude/ locations and returns every skill/agent
// file found. homeDir is the home directory to use (normally the result of
// os.UserHomeDir(); injected here so tests can use a temp directory).
func Scan(homeDir string) ([]Found, error) {
	base := filepath.Join(homeDir, ".claude")

	patterns := []struct {
		glob string
		kind Kind
	}{
		// Marketplace plugins layout: ~/.claude/plugins/marketplaces/<market>/plugins/<plugin>/skills/<name>/SKILL.md
		{filepath.Join(base, "plugins", "marketplaces", "*", "plugins", "*", "skills", "*", "SKILL.md"), KindSkill},
		// Marketplace plugins layout: ~/.claude/plugins/marketplaces/<market>/plugins/<plugin>/agents/*.md
		{filepath.Join(base, "plugins", "marketplaces", "*", "plugins", "*", "agents", "*.md"), KindAgent},
		// Flat layout: ~/.claude/skills/<name>/SKILL.md
		{filepath.Join(base, "skills", "*", "SKILL.md"), KindSkill},
		// Flat layout: ~/.claude/agents/*.md
		{filepath.Join(base, "agents", "*.md"), KindAgent},
	}

	var results []Found
	for _, p := range patterns {
		matches, err := filepath.Glob(p.glob)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			abs, err := filepath.Abs(m)
			if err != nil {
				return nil, err
			}
			// Skip directories that accidentally match the glob.
			info, err := os.Stat(abs)
			if err != nil || info.IsDir() {
				continue
			}
			results = append(results, Found{Kind: p.kind, Path: abs})
		}
	}
	return results, nil
}
