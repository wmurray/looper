package plan

import (
	"regexp"
	"strings"
)

// ValidationError represents a single plan validation issue.
type ValidationError struct {
	Section string
	Message string
	Fatal   bool
}

var htmlCommentRe = regexp.MustCompile(`<!--.*?-->`)

// sectionContent extracts the content of a markdown section (between its heading
// and the next same-or-higher-level heading) and strips HTML comments.
func sectionContent(content, heading string) (string, bool) {
	lines := strings.Split(content, "\n")
	var inSection bool
	var buf []string
	headingPrefix := heading + " " // e.g. "## Objective "
	headingMarker := heading        // bare match too

	for _, line := range lines {
		if strings.HasPrefix(line, headingPrefix) || line == headingMarker || strings.HasPrefix(line, heading+"\r") {
			inSection = true
			continue
		}
		if inSection {
			// Stop at next ## heading
			if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
				break
			}
			buf = append(buf, line)
		}
	}
	if !inSection {
		return "", false
	}
	raw := strings.Join(buf, "\n")
	stripped := htmlCommentRe.ReplaceAllString(raw, "")
	return strings.TrimSpace(stripped), true
}

// Validate checks the plan content for missing or placeholder sections.
// It returns a slice of ValidationErrors; Fatal=true errors should abort the loop.
func Validate(content string) []ValidationError {
	var errs []ValidationError

	// Rule 1: ## Objective must be present and non-empty
	objContent, objFound := sectionContent(content, "## Objective")
	if !objFound || objContent == "" {
		errs = append(errs, ValidationError{
			Section: "Objective",
			Message: "## Objective section is missing or empty — fill it in before running the loop",
			Fatal:   true,
		})
	}

	// Rule 3: ## Implementation Steps must have at least one numbered step with text
	stepsContent, stepsFound := sectionContent(content, "## Implementation Steps")
	numberedStepWithTextRe := regexp.MustCompile(`(?m)^\d+\.[ \t]+\S`)
	hasNumberedStep := stepsFound && numberedStepWithTextRe.MatchString(stepsContent)
	if !stepsFound || !hasNumberedStep {
		errs = append(errs, ValidationError{
			Section: "Implementation Steps",
			Message: "## Implementation Steps section must contain at least one numbered step (1. something)",
			Fatal:   true,
		})
	}

	// Rule 2: ## Acceptance Criteria must have at least one - [ ] item with text
	acContent, acFound := sectionContent(content, "## Acceptance Criteria")
	checkboxWithTextRe := regexp.MustCompile(`(?m)^- \[ \][ \t]+\S`)
	hasCheckbox := acFound && checkboxWithTextRe.MatchString(acContent)
	if !acFound || !hasCheckbox {
		errs = append(errs, ValidationError{
			Section: "Acceptance Criteria",
			Message: "## Acceptance Criteria section must contain at least one unchecked item with text (- [ ] something)",
			Fatal:   true,
		})
	}

	// Rule 4: warn (non-fatal) on any section whose raw content is only placeholder lines
	// (HTML comments, bare "- ", bare "1." with no trailing text)
	errs = append(errs, warnPlaceholderSections(content)...)

	return errs
}

// knownSections are the headings we scan for placeholder warnings.
var knownSections = []string{
	"## Objective",
	"## Context",
	"## Implementation Steps",
	"## Acceptance Criteria",
	"## Out of Scope",
}

var (
	bareItemRe        = regexp.MustCompile(`(?m)^-\s*$`)
	bareNumberedRe    = regexp.MustCompile(`(?m)^\d+\.\s*$`)
	placeholderLineRe = regexp.MustCompile(`(?m)^<!--.*?-->$`)
)

// isOnlyPlaceholders returns true if the raw section content (before stripping
// HTML comments) consists only of HTML comment lines, bare "- " lines, or bare "1." lines.
func isOnlyPlaceholders(raw string) bool {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		isComment := placeholderLineRe.MatchString(line)
		isBareItem := bareItemRe.MatchString(line)
		isBareNum := bareNumberedRe.MatchString(line)
		if !isComment && !isBareItem && !isBareNum {
			return false
		}
	}
	return true
}

// rawSectionContent extracts the raw (un-stripped) content of a section.
func rawSectionContent(content, heading string) (string, bool) {
	lines := strings.Split(content, "\n")
	var inSection bool
	var buf []string

	for _, line := range lines {
		if strings.HasPrefix(line, heading+" ") || line == heading || strings.HasPrefix(line, heading+"\r") {
			inSection = true
			continue
		}
		if inSection {
			if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
				break
			}
			buf = append(buf, line)
		}
	}
	if !inSection {
		return "", false
	}
	return strings.Join(buf, "\n"), true
}

func warnPlaceholderSections(content string) []ValidationError {
	var errs []ValidationError
	for _, heading := range knownSections {
		raw, found := rawSectionContent(content, heading)
		if !found {
			continue
		}
		if isOnlyPlaceholders(raw) {
			name := strings.TrimPrefix(heading, "## ")
			errs = append(errs, ValidationError{
				Section: name,
				Message: heading + " still contains only placeholder text — fill it in",
				Fatal:   false,
			})
		}
	}
	return errs
}
