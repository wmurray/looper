package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/runner"
)

// makeAITree creates a skill and an agent file under the given home dir and returns their paths.
func makeAITree(t *testing.T, home string) (skillPath, agentPath string) {
	t.Helper()
	skillPath = filepath.Join(home, ".claude", "skills", "tdd-workflow", "SKILL.md")
	agentPath = filepath.Join(home, ".claude", "agents", "go-reviewer.md")
	for _, p := range []string{skillPath, agentPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte("content of "+filepath.Base(p)), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return
}

// runSettingsDiscoverWithInput is like runSettingsDiscover but also injects stdin content.
func runSettingsDiscoverWithInput(t *testing.T, home, stdinContent string, extraArgs ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", home)

	// Inject stdin
	rIn, wIn, _ := os.Pipe()
	if _, err := io.WriteString(wIn, stdinContent); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	wIn.Close()
	oldIn := os.Stdin
	os.Stdin = rIn
	defer func() { os.Stdin = oldIn }()

	// Capture stdout
	rOut, wOut, _ := os.Pipe()
	oldOut := os.Stdout
	os.Stdout = wOut

	// Capture stderr
	rErr, wErr, _ := os.Pipe()
	oldErr := os.Stderr
	os.Stderr = wErr

	args := append([]string{"settings", "discover"}, extraArgs...)
	rootCmd.SetArgs(args)
	cmdErr := rootCmd.Execute()

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	return string(outBytes), string(errBytes), cmdErr
}

// stubRunnerWith returns a discoverRunFn replacement that returns a fixed output.
func stubRunnerWith(output string) func(context.Context, string, int, string) runner.Result {
	return func(_ context.Context, _ string, _ int, _ string) runner.Result {
		return runner.Result{Output: output, ExitCode: 0}
	}
}

// resetDiscoverFlagsOnCleanup schedules a cleanup that resets the cobra bool flag variables to
// their defaults. Without this, cobra does not reset bound variables between test runs and a test
// that passes --ai will corrupt subsequent tests.
func resetDiscoverFlagsOnCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		discoverAI = false
		discoverYes = false
	})
}

// TestValidateAISuggestions: table-driven tests for all validation branches.
func TestValidateAISuggestions(t *testing.T) {
	scanned := map[string]bool{
		"/real/SKILL.md":  true,
		"/real/agent1.md": true,
	}

	cases := []struct {
		name        string
		raw         string
		wantKeys    []string
		wantMissing []string
	}{
		{
			name:     "valid both keys",
			raw:      `{"skill_path": "/real/SKILL.md", "reviewer_agent": "/real/agent1.md"}`,
			wantKeys: []string{"skill_path", "reviewer_agent"},
		},
		{
			name:     "unknown key rejected",
			raw:      `{"skill_path": "/real/SKILL.md", "bad_key": "/real/SKILL.md"}`,
			wantKeys: []string{"skill_path"},
			wantMissing: []string{"bad_key"},
		},
		{
			name:        "hallucinated path rejected",
			raw:         `{"skill_path": "/invented/SKILL.md"}`,
			wantMissing: []string{"skill_path"},
		},
		{
			name:        "preamble stripped",
			raw:         `Here is my recommendation: {"skill_path": "/real/SKILL.md"} done.`,
			wantKeys:    []string{"skill_path"},
		},
		{
			name:        "invalid json returns empty",
			raw:         `not json at all`,
			wantMissing: []string{"skill_path", "reviewer_agent"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateAISuggestions(tc.raw, scanned)
			for _, k := range tc.wantKeys {
				if _, ok := got[k]; !ok {
					t.Errorf("expected key %q in result, got %v", k, got)
				}
			}
			for _, k := range tc.wantMissing {
				if _, ok := got[k]; ok {
					t.Errorf("expected key %q to be absent, got %v", k, got)
				}
			}
		})
	}
}

// TestBuildAIDiscoverPrompt: prompt contains stack, current values, file contents, and format instruction.
func TestBuildAIDiscoverPrompt(t *testing.T) {
	contents := map[string]string{
		"/path/to/SKILL.md":  "## TDD Workflow\nRed-green-refactor.",
		"/path/to/agent1.md": "# Go Reviewer\nReviews Go code.",
	}
	prompt := buildAIDiscoverPrompt(
		"go",
		"/current/skill.md",
		"/current/reviewer.md",
		contents,
	)

	checks := []string{
		"go",                    // stack detected
		"/current/skill.md",    // current skill_path
		"/current/reviewer.md", // current reviewer_agent
		"/path/to/SKILL.md",    // discovered path
		"TDD Workflow",          // file content
		"/path/to/agent1.md",   // discovered agent path
		"Go Reviewer",          // agent file content
		`{"skill_path"`,        // output format instruction
	}
	for _, want := range checks {
		if !strings.Contains(prompt, want) {
			t.Errorf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

// TestSettingsDiscover_AI_NothingFound: --ai with empty home exits 0 with "No skills or agents found".
func TestSettingsDiscover_AI_NothingFound(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()

	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
		return config.Config{Backend: "claude"}, nil
	}
	defer func() { discoverConfigLoadFn = origLoad }()

	out, _, err := runSettingsDiscover(t, home, "--ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No skills or agents found") {
		t.Errorf("expected 'No skills or agents found' in output, got:\n%s", out)
	}
}

// TestSettingsDiscover_AI_NoBackend: --ai with no backend configured exits 1 with the right message.
func TestSettingsDiscover_AI_NoBackend(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()

	// Inject a config with no backend set.
	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
		return config.Config{}, nil // Backend == ""
	}
	defer func() { discoverConfigLoadFn = origLoad }()

	out, _, err := runSettingsDiscover(t, home, "--ai")
	if err == nil {
		t.Fatal("expected error when backend is not configured, got nil")
	}
	if !strings.Contains(out, "backend is not configured") {
		t.Errorf("expected 'backend is not configured' in output, got:\n%s", out)
	}
}

// TestSettingsDiscover_AI_ValidSuggestion: stub runner returns valid JSON; diff is printed and confirmation prompt appears.
func TestSettingsDiscover_AI_ValidSuggestion(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()
	skillPath, agentPath := makeAITree(t, home)

	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
		return config.Config{Backend: "claude"}, nil
	}
	defer func() { discoverConfigLoadFn = origLoad }()

	origRun := discoverRunFn
	discoverRunFn = stubRunnerWith(`{"skill_path": "` + skillPath + `", "reviewer_agent": "` + agentPath + `"}`)
	defer func() { discoverRunFn = origRun }()

	// Pass "n" to decline confirmation.
	out, _, _ := runSettingsDiscoverWithInput(t, home, "n\n", "--ai")

	if !strings.Contains(out, "skill_path") {
		t.Errorf("expected 'skill_path' in diff output, got:\n%s", out)
	}
	if !strings.Contains(out, "Apply these changes?") {
		t.Errorf("expected confirmation prompt in output, got:\n%s", out)
	}
}

// TestSettingsDiscover_AI_YesFlag: --yes skips prompt and config file is updated.
func TestSettingsDiscover_AI_YesFlag(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()
	skillPath, agentPath := makeAITree(t, home)

	// On macOS os.UserConfigDir() returns $HOME/Library/Application Support,
	// so setting HOME is the right way to redirect config writes in tests.
	t.Setenv("HOME", home)

	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
		return config.Config{Backend: "claude"}, nil
	}
	defer func() { discoverConfigLoadFn = origLoad }()

	origRun := discoverRunFn
	discoverRunFn = stubRunnerWith(`{"skill_path": "` + skillPath + `", "reviewer_agent": "` + agentPath + `"}`)
	defer func() { discoverRunFn = origRun }()

	out, _, err := runSettingsDiscover(t, home, "--ai", "--yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Applied") {
		t.Errorf("expected 'Applied' in output, got:\n%s", out)
	}

	// Derive config path the same way config.ConfigPath() does on the current platform.
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "looper", "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}
	var saved config.Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parsing saved config: %v", err)
	}
	if saved.SkillPath != skillPath {
		t.Errorf("skill_path: want %q, got %q", skillPath, saved.SkillPath)
	}
	if saved.ReviewerAgent != agentPath {
		t.Errorf("reviewer_agent: want %q, got %q", agentPath, saved.ReviewerAgent)
	}
}

// TestSettingsDiscover_AI_HallucinatedPath: stub returns a path not in scanned set; suggestion is rejected.
func TestSettingsDiscover_AI_HallucinatedPath(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()
	makeAITree(t, home)

	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
		return config.Config{Backend: "claude"}, nil
	}
	defer func() { discoverConfigLoadFn = origLoad }()

	origRun := discoverRunFn
	discoverRunFn = stubRunnerWith(`{"skill_path": "/invented/path/SKILL.md"}`)
	defer func() { discoverRunFn = origRun }()

	out, _, err := runSettingsDiscover(t, home, "--ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "no usable suggestions") {
		t.Errorf("expected 'no usable suggestions' in output, got:\n%s", out)
	}
}

// TestSettingsDiscover_AI_UnknownKey: stub returns an unknown key; key is skipped.
func TestSettingsDiscover_AI_UnknownKey(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()
	skillPath, _ := makeAITree(t, home)

	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
		return config.Config{Backend: "claude"}, nil
	}
	defer func() { discoverConfigLoadFn = origLoad }()

	origRun := discoverRunFn
	// Returns one valid key and one unknown key.
	discoverRunFn = stubRunnerWith(`{"skill_path": "` + skillPath + `", "bad_key": "` + skillPath + `"}`)
	defer func() { discoverRunFn = origRun }()

	// Pass "n" to decline.
	out, _, _ := runSettingsDiscoverWithInput(t, home, "n\n", "--ai")

	if !strings.Contains(out, "skill_path") {
		t.Errorf("expected 'skill_path' in diff, got:\n%s", out)
	}
	if strings.Contains(out, "bad_key") {
		t.Errorf("expected 'bad_key' to be absent from diff, got:\n%s", out)
	}
}

// TestSettingsDiscover_AI_AlreadyOptimal: suggested values equal current values; prints "Settings are already optimal."
func TestSettingsDiscover_AI_AlreadyOptimal(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()
	skillPath, agentPath := makeAITree(t, home)

	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
		// Current config already matches what the agent will suggest.
		return config.Config{Backend: "claude", SkillPath: skillPath, ReviewerAgent: agentPath}, nil
	}
	defer func() { discoverConfigLoadFn = origLoad }()

	origRun := discoverRunFn
	discoverRunFn = stubRunnerWith(`{"skill_path": "` + skillPath + `", "reviewer_agent": "` + agentPath + `"}`)
	defer func() { discoverRunFn = origRun }()

	out, _, err := runSettingsDiscover(t, home, "--ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "already optimal") {
		t.Errorf("expected 'already optimal' in output, got:\n%s", out)
	}
}
