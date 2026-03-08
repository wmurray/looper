package cmd

import (
	"strings"
	"testing"
)

func TestBuildPlanPrompt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		ticket     string
		userPrompt string
		contains   string
		absent     string
	}{
		{name: "contains ticket", ticket: "DX-42", userPrompt: "Add JWT auth", contains: "DX-42"},
		{name: "contains user prompt", ticket: "DX-42", userPrompt: "Add JWT auth to the Rails API", contains: "Add JWT auth to the Rails API"},
		{name: "no unreplaced ticket placeholder", ticket: "DX-42", userPrompt: "Add JWT auth", absent: "{TICKET}"},
		{name: "no unreplaced prompt placeholder", ticket: "DX-42", userPrompt: "Add JWT auth", absent: "{PROMPT}"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := buildPlanPrompt(tc.ticket, tc.userPrompt)
			if tc.contains != "" && !strings.Contains(out, tc.contains) {
				t.Errorf("expected %q in output, got:\n%s", tc.contains, out)
			}
			if tc.absent != "" && strings.Contains(out, tc.absent) {
				t.Errorf("expected %q to be absent from output, got:\n%s", tc.absent, out)
			}
		})
	}
}

func TestPlanCmd_OpenFlag(t *testing.T) {
	t.Parallel()
	f := planCmd.Flags().Lookup("open")
	if f == nil {
		t.Fatal("expected --open flag to be registered on planCmd")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("expected --open to be a bool flag, got %q", f.Value.Type())
	}
	if f.DefValue != "false" {
		t.Errorf("expected --open default to be false, got %q", f.DefValue)
	}
}

func TestPlanCmd_PromptFlag(t *testing.T) {
	t.Parallel()
	f := planCmd.Flags().Lookup("prompt")
	if f == nil {
		t.Fatal("expected --prompt flag to be registered on planCmd")
	}
	if f.Value.Type() != "string" {
		t.Errorf("expected --prompt to be a string flag, got %q", f.Value.Type())
	}
	if f.DefValue != "" {
		t.Errorf("expected --prompt default to be empty string, got %q", f.DefValue)
	}
}
