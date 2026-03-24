package agent

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Metadata holds YAML frontmatter parsed from an agent .md file.
type Metadata struct {
	Role        string   `yaml:"role"`
	Languages   []string `yaml:"languages"`
	Frameworks  []string `yaml:"frameworks"`
	Level       string   `yaml:"level"`
	Description string   `yaml:"description"`
	Path        string   // set to the file path by ParseMetadata; not from frontmatter
}

// ParseMetadata reads the file at path and returns its YAML frontmatter.
// If no frontmatter is present, returns a zero Metadata (not an error).
// Languages and frameworks are normalised to lowercase.
func ParseMetadata(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return Metadata{Path: path}, nil
	}

	rest := content[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		// Gotcha: opening --- found but no closing --- means the file is malformed, not just frontmatter-free.
		return Metadata{}, errors.New("agent.ParseMetadata: unclosed frontmatter block (opening --- has no matching closing ---)")
	}
	frontmatter := rest[:end]

	var m Metadata
	if err := yaml.Unmarshal([]byte(frontmatter), &m); err != nil {
		return Metadata{}, err
	}

	for i, l := range m.Languages {
		m.Languages[i] = strings.ToLower(l)
	}
	for i, f := range m.Frameworks {
		m.Frameworks[i] = strings.ToLower(f)
	}
	m.Path = path
	return m, nil
}
