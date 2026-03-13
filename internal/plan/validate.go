package plan

import (
	"regexp"
	"strings"
)

type ValidationError struct {
	Section string
	Message string
	Fatal   bool
}

var htmlCommentRe = regexp.MustCompile(`<!--.*?-->`)

// Gotcha: stops at the next # or ## heading, not just the same level.
func sectionContent(content, heading string) (string, bool) {
	lines := strings.Split(content, "\n")
	var inSection bool
	var buf []string
	headingPrefix := heading + " "
	headingMarker := heading

	for _, line := range lines {
		if strings.HasPrefix(line, headingPrefix) || line == headingMarker || strings.HasPrefix(line, heading+"\r") {
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
	raw := strings.Join(buf, "\n")
	stripped := htmlCommentRe.ReplaceAllString(raw, "")
	return strings.TrimSpace(stripped), true
}

// Why: Fatal=true errors should abort the loop; non-fatal ones are warnings only.
func Validate(content string) []ValidationError {
	var errs []ValidationError

	objContent, objFound := sectionContent(content, "## Objective")
	if !objFound || objContent == "" {
		errs = append(errs, ValidationError{
			Section: "Objective",
			Message: "## Objective section is missing or empty — fill it in before running the loop",
			Fatal:   true,
		})
	}

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

	errs = append(errs, warnPlaceholderSections(content)...)

	return errs
}

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

// Gotcha: operates on raw (un-stripped) content; HTML comment lines count as placeholders.
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

// Why: returns raw content (with HTML comments) so isOnlyPlaceholders can classify them.
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
