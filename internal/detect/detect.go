package detect

import (
	"path/filepath"
	"sort"
	"strings"
)

// Detection holds the set of languages inferred from changed files.
type Detection struct {
	Languages  []string
	Frameworks []string
}

var extToLang = map[string]string{
	".go":   "go",
	".ts":   "typescript",
	".tsx":  "typescript",
	".js":   "javascript",
	".jsx":  "javascript",
	".py":   "python",
	".rb":   "ruby",
	".rs":   "rust",
	".java": "java",
	".cs":   "csharp",
	".swift": "swift",
	".kt":   "kotlin",
	".kts":  "kotlin",
}

// FromFileExtensions maps a list of file paths to languages.
func FromFileExtensions(paths []string) Detection {
	seen := map[string]bool{}
	for _, p := range paths {
		ext := strings.ToLower(filepath.Ext(p))
		if lang, ok := extToLang[ext]; ok {
			seen[lang] = true
		}
	}
	langs := make([]string, 0, len(seen))
	for l := range seen {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	return Detection{Languages: langs}
}

// FromGitDiff returns the set of languages inferred from file extensions
// present in the unified diff output.
func FromGitDiff(diff string) Detection {
	var paths []string
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			paths = append(paths, strings.TrimPrefix(line, "+++ b/"))
		}
	}
	return FromFileExtensions(paths)
}
