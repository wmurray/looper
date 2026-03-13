package plan

import "testing"

func TestValidate_BareItemPlaceholderWarns(t *testing.T) {
	// A section with only a bare "- " or "1." placeholder (no text after) should
	// produce a non-fatal warning. The three required sections are fine here.
	content := `# Ticket: TEST-1

## Objective
Real objective text

## Context
<!-- background here -->

## Implementation Steps
1. Do the real thing

## Acceptance Criteria
- [ ] It works

## Out of Scope
-
`
	errs := Validate(content)
	var hasFatal bool
	var hasWarnForContext bool
	for _, e := range errs {
		if e.Fatal {
			hasFatal = true
		}
		if !e.Fatal && e.Section == "Context" {
			hasWarnForContext = true
		}
	}
	if hasFatal {
		t.Errorf("expected no fatal errors for otherwise valid plan, got: %v", errs)
	}
	if !hasWarnForContext {
		t.Errorf("expected non-fatal warning for placeholder Context section, got: %v", errs)
	}
}

func TestValidate_AIPromptTemplate(t *testing.T) {
	// This is the planPromptTemplate structure before AI fills it in.
	content := `# Ticket: TEST-1

## Objective
<!-- clear statement of what needs to be built -->

## Context
<!-- background, relevant code areas, related tickets if mentioned -->

## Implementation Steps
1. ...

## Acceptance Criteria
- [ ] ...

## Out of Scope
- ...
`
	errs := Validate(content)
	// Objective: HTML comment only → stripped → empty → fatal
	var hasFatalObjective bool
	for _, e := range errs {
		if e.Section == "Objective" && e.Fatal {
			hasFatalObjective = true
		}
	}
	if !hasFatalObjective {
		t.Errorf("expected fatal error for AI prompt template Objective, got: %v", errs)
	}
	// Implementation Steps: "1. ..." — "..." is non-whitespace, so hasNumberedStep=true → no fatal
	// Acceptance Criteria: "- [ ] ..." — same → no fatal
	// This is acceptable — the prompt template has placeholder "..." text.
}

func TestValidate_FreshTemplate(t *testing.T) {
	// This is the exact output of planTemplateBytes — should produce fatal errors.
	content := `# Ticket: TEST-1

## Objective
<!-- What needs to be done -->

## Context
<!-- Background, links to ticket, related files -->

## Implementation Steps
1.
2.
3.

## Acceptance Criteria
- [ ]
- [ ]

## Out of Scope
-
`
	errs := Validate(content)
	var fatalSections []string
	for _, e := range errs {
		if e.Fatal {
			fatalSections = append(fatalSections, e.Section)
		}
	}
	// Must have fatal errors for Objective (comment-only → empty), Implementation Steps (bare "1." no text),
	// and Acceptance Criteria (bare "- [ ]" with nothing after)
	wantFatal := []string{"Objective", "Implementation Steps", "Acceptance Criteria"}
	for _, want := range wantFatal {
		found := false
		for _, s := range fatalSections {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected fatal error for %q section in fresh template, got fatal sections: %v", want, fatalSections)
		}
	}
}

func TestValidate_ValidPlanPassesClean(t *testing.T) {
	content := `# Ticket: TEST-1

## Objective
Add a Validate function that checks plan files.

## Context
The plan package needs validation logic.

## Implementation Steps
1. Create validate.go
2. Add tests

## Acceptance Criteria
- [ ] Validate returns no errors for a valid plan
- [ ] Validate returns fatal errors for missing required sections

## Out of Scope
- Linting prose quality
`
	errs := Validate(content)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid plan, got: %v", errs)
	}
}

func TestValidate_MissingImplementationSteps(t *testing.T) {
	content := `# Ticket: TEST-1

## Objective
Something real

## Acceptance Criteria
- [ ] It works
`
	errs := Validate(content)
	var found bool
	for _, e := range errs {
		if e.Section == "Implementation Steps" && e.Fatal {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fatal error for Implementation Steps section, got: %v", errs)
	}
}

func TestValidate_MissingAcceptanceCriteria(t *testing.T) {
	content := `# Ticket: TEST-1

## Objective
Something real

## Implementation Steps
1. Do the thing
`
	errs := Validate(content)
	var found bool
	for _, e := range errs {
		if e.Section == "Acceptance Criteria" && e.Fatal {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fatal error for Acceptance Criteria section, got: %v", errs)
	}
}

func TestValidate_MissingObjective(t *testing.T) {
	content := `# Ticket: TEST-1

## Acceptance Criteria
- [ ] Something

## Implementation Steps
1. Do the thing
`
	errs := Validate(content)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	var found bool
	for _, e := range errs {
		if e.Section == "Objective" && e.Fatal {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fatal error for Objective section, got: %v", errs)
	}
}
