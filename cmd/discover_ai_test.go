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

func runSettingsDiscoverWithInput(t *testing.T, home, stdinContent string, extraArgs ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", home)

	rIn, wIn, _ := os.Pipe()
	if _, err := io.WriteString(wIn, stdinContent); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	wIn.Close()
	oldIn := os.Stdin
	os.Stdin = rIn
	defer func() { os.Stdin = oldIn }()

	rOut, wOut, _ := os.Pipe()
	oldOut := os.Stdout
	os.Stdout = wOut

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

func stubRunnerWith(output string) func(context.Context, string, int, string) runner.Result {
	return func(_ context.Context, _ string, _ int, _ string) runner.Result {
		return runner.Result{Output: output, ExitCode: 0}
	}
}

// Gotcha: cobra does not reset bound flag variables between test runs; without this, --ai in one test corrupts the next.
func resetDiscoverFlagsOnCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		discoverAI = false
		discoverYes = false
	})
}

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

func TestSettingsDiscover_AI_NoBackend(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()

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

	out, _, _ := runSettingsDiscoverWithInput(t, home, "n\n", "--ai")

	if !strings.Contains(out, "skill_path") {
		t.Errorf("expected 'skill_path' in diff output, got:\n%s", out)
	}
	if !strings.Contains(out, "Apply these changes?") {
		t.Errorf("expected confirmation prompt in output, got:\n%s", out)
	}
}

func TestSettingsDiscover_AI_YesFlag(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()
	skillPath, agentPath := makeAITree(t, home)

	// Why: on macOS os.UserConfigDir() derives from $HOME, so setting HOME redirects config writes.
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
	discoverRunFn = stubRunnerWith(`{"skill_path": "` + skillPath + `", "bad_key": "` + skillPath + `"}`)
	defer func() { discoverRunFn = origRun }()

	out, _, _ := runSettingsDiscoverWithInput(t, home, "n\n", "--ai")

	if !strings.Contains(out, "skill_path") {
		t.Errorf("expected 'skill_path' in diff, got:\n%s", out)
	}
	if strings.Contains(out, "bad_key") {
		t.Errorf("expected 'bad_key' to be absent from diff, got:\n%s", out)
	}
}

func TestSettingsDiscover_AI_AlreadyOptimal(t *testing.T) {
	resetDiscoverFlagsOnCleanup(t)
	home := t.TempDir()
	skillPath, agentPath := makeAITree(t, home)

	origLoad := discoverConfigLoadFn
	discoverConfigLoadFn = func() (config.Config, error) {
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
