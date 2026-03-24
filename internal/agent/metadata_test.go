package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/willmurray/looper/internal/agent"
)

func TestParseMetadata(t *testing.T) {
	t.Parallel()
	content := `---
role: reviewer
languages:
  - Go
  - TypeScript
frameworks:
  - Gin
level: senior
description: Reviews Go and TypeScript code
---

# Agent content here
`
	path := filepath.Join(t.TempDir(), "agent.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m, err := agent.ParseMetadata(path)
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	m.Path = "" // clear for comparison

	if m.Role != "reviewer" {
		t.Errorf("Role = %q, want %q", m.Role, "reviewer")
	}
	if len(m.Languages) != 2 || m.Languages[0] != "go" || m.Languages[1] != "typescript" {
		t.Errorf("Languages = %v, want [go typescript]", m.Languages)
	}
	if len(m.Frameworks) != 1 || m.Frameworks[0] != "gin" {
		t.Errorf("Frameworks = %v, want [gin]", m.Frameworks)
	}
	if m.Level != "senior" {
		t.Errorf("Level = %q, want %q", m.Level, "senior")
	}
	if m.Description != "Reviews Go and TypeScript code" {
		t.Errorf("Description = %q", m.Description)
	}
}

func TestParseMetadataNoFrontmatter(t *testing.T) {
	t.Parallel()
	content := "# Just a plain agent file\n\nNo frontmatter here.\n"
	path := filepath.Join(t.TempDir(), "agent.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m, err := agent.ParseMetadata(path)
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	if m.Role != "" || len(m.Languages) != 0 {
		t.Errorf("expected zero Metadata for file without frontmatter, got %+v", m)
	}
}

func TestParseMetadataDashPrefix(t *testing.T) {
	t.Parallel()
	// "----" must not be treated as a frontmatter opener.
	content := "----\nkey: val\n---\n# content"
	path := filepath.Join(t.TempDir(), "agent.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m, err := agent.ParseMetadata(path)
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	if m.Role != "" || len(m.Languages) != 0 {
		t.Errorf("expected zero Metadata for '----' prefix, got %+v", m)
	}
}

func TestParseMetadataLowercaseNormalization(t *testing.T) {
	t.Parallel()
	content := "---\nlanguages:\n  - Go\n  - TypeScript\n---\n"
	path := filepath.Join(t.TempDir(), "agent.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m, err := agent.ParseMetadata(path)
	if err != nil {
		t.Fatalf("ParseMetadata: %v", err)
	}
	for _, lang := range m.Languages {
		for _, r := range lang {
			if r >= 'A' && r <= 'Z' {
				t.Errorf("language %q should be lowercase", lang)
			}
		}
	}
}
