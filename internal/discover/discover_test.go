package discover_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/willmurray/looper/internal/discover"
)

// makeTree creates files at the given relative paths under root.
func makeTree(t *testing.T, root string, paths []string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(""), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func TestScan_AllPathsAndKinds(t *testing.T) {
	home := t.TempDir()
	makeTree(t, home, []string{
		".claude/plugins/marketplaces/acme/plugins/tdd-plugin/skills/tdd-workflow/SKILL.md",
		".claude/plugins/marketplaces/acme/plugins/rails/agents/rails-reviewer.md",
		".claude/skills/my-skill/SKILL.md",
		".claude/agents/my-agent.md",
	})

	results, err := discover.Scan(home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	counts := map[discover.Kind]int{}
	for _, r := range results {
		counts[r.Kind]++
	}
	if counts[discover.KindSkill] != 2 {
		t.Errorf("expected 2 skills, got %d", counts[discover.KindSkill])
	}
	if counts[discover.KindAgent] != 2 {
		t.Errorf("expected 2 agents, got %d", counts[discover.KindAgent])
	}
}

func TestScan_EmptyHome(t *testing.T) {
	home := t.TempDir()
	results, err := discover.Scan(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty home, got %d", len(results))
	}
}

func TestScan_FindsMarketplaceSkill(t *testing.T) {
	home := t.TempDir()
	makeTree(t, home, []string{
		".claude/plugins/marketplaces/acme-workflows/plugins/tdd-plugin/skills/tdd-workflow/SKILL.md",
	})

	results, err := discover.Scan(home)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Kind != discover.KindSkill {
		t.Errorf("expected KindSkill, got %v", results[0].Kind)
	}
}
