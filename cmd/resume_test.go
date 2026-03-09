package cmd

import (
	"errors"
	"testing"
)

// resolveResumeState takes injected predicates so tests need no git repo or filesystem.

func TestResolveResumeState_HasIterations(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return true }, // hasWork: loop already ran
		func() error { return nil }, // plan exists (irrelevant when work found)
	)
	if state != resumeHasIterations {
		t.Errorf("got %v, want resumeHasIterations", state)
	}
}

func TestResolveResumeState_PlanExists(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return false }, // no iteration work
		func() error { return nil },  // plan file exists on disk
	)
	if state != resumePlanExists {
		t.Errorf("got %v, want resumePlanExists", state)
	}
}

func TestResolveResumeState_NoPlan(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return false },    // no iteration work
		func() error { return errors.New("not found") }, // no plan file
	)
	if state != resumeNoPlan {
		t.Errorf("got %v, want resumeNoPlan", state)
	}
}

// HasIterationWork takes priority over plan-file presence.
func TestResolveResumeState_IterationsTakesPriority(t *testing.T) {
	t.Parallel()
	state := resolveResumeState(
		func() bool { return true },               // work found
		func() error { return errors.New("gone") }, // plan missing — shouldn't matter
	)
	if state != resumeHasIterations {
		t.Errorf("got %v, want resumeHasIterations even when plan missing", state)
	}
}
