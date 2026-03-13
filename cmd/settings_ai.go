package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/willmurray/looper/internal/config"
	"github.com/willmurray/looper/internal/discover"
	"github.com/willmurray/looper/internal/runner"
)

var allowedAIKeys = map[string]bool{
	"skill_path":     true,
	"reviewer_agent": true,
}

// Why: extracts the first JSON object so agent preamble/postamble doesn't break parsing.
var jsonObjectRE = regexp.MustCompile(`\{[^{}]*\}`)

// Invariant: only keys in allowedAIKeys and paths in scannedPaths are returned — guards against hallucinated paths.
func validateAISuggestions(raw string, scannedPaths map[string]bool) map[string]string {
	match := jsonObjectRE.FindString(raw)
	if match == "" {
		return nil
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(match), &parsed); err != nil {
		return nil
	}
	result := make(map[string]string)
	for k, v := range parsed {
		if !allowedAIKeys[k] {
			continue
		}
		if !scannedPaths[v] {
			continue
		}
		result[k] = v
	}
	return result
}

// Why: injected so tests can supply a fake config without touching the filesystem.
var discoverConfigLoadFn = func() (config.Config, error) {
	return config.Load()
}

// Why: injected so tests can supply a stub runner without calling a real agent binary.
var discoverRunFn = func(ctx context.Context, prompt string, timeoutSecs int, backend string) runner.Result {
	return runner.Run(ctx, prompt, timeoutSecs, backend)
}

func runAIDiscover(home string, yes bool) error {
	cfg, err := discoverConfigLoadFn()
	if err != nil {
		return err
	}
	if cfg.Backend == "" {
		fmt.Println("backend is not configured — run: looper settings set backend claude")
		return fmt.Errorf("backend is not configured")
	}

	found, err := discover.Scan(home)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	if len(found) == 0 {
		fmt.Println("No skills or agents found.")
		return nil
	}

	contents := make(map[string]string, len(found))
	scanned := make(map[string]bool, len(found))
	for _, f := range found {
		scanned[f.Path] = true
		data, err := os.ReadFile(f.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n", f.Path, err)
			continue
		}
		contents[f.Path] = string(data)
	}

	cwd, _ := os.Getwd()
	stack := detectStack(cwd)
	if stack != "" {
		fmt.Printf("Detected stack: %s\n", stack)
	}

	prompt := buildAIDiscoverPrompt(stack, cfg.SkillPath, cfg.ReviewerAgent, contents)

	timeout := cfg.Defaults.Timeout
	if timeout < 10 {
		timeout = 420
	}
	result := discoverRunFn(context.Background(), prompt, timeout, cfg.Backend)
	if result.Err != nil || result.ExitCode != 0 {
		if result.Stderr != "" {
			fmt.Fprint(os.Stderr, result.Stderr)
		}
		return fmt.Errorf("agent failed (exit %d)", result.ExitCode)
	}

	suggestions := validateAISuggestions(result.Output, scanned)
	if len(suggestions) == 0 {
		fmt.Println("Agent returned no usable suggestions.")
		return nil
	}

	type row struct{ key, suggested, current string }
	var rows []row
	for _, k := range []string{"skill_path", "reviewer_agent"} {
		v, ok := suggestions[k]
		if !ok {
			continue
		}
		var current string
		switch k {
		case "skill_path":
			current = cfg.SkillPath
		case "reviewer_agent":
			current = cfg.ReviewerAgent
		}
		if v == current {
			continue
		}
		currentLabel := current
		if currentLabel == "" {
			currentLabel = "(unset)"
		}
		rows = append(rows, row{k, v, currentLabel})
	}
	if len(rows) == 0 {
		fmt.Println("Settings are already optimal.")
		return nil
	}

	fmt.Println("Suggested settings:")
	for _, r := range rows {
		display := strings.Replace(r.suggested, home, "~", 1)
		fmt.Printf("  %-16s %s  (currently: %s)\n", r.key, display, r.current)
	}

	if !yes {
		fmt.Print("Apply these changes? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() || strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			return nil
		}
	}

	saveCfg, err := config.Load()
	if err != nil {
		return err
	}
	for k, v := range suggestions {
		if saveCfg, err = config.Set(saveCfg, k, v); err != nil {
			return err
		}
	}
	if err := config.Save(saveCfg); err != nil {
		return err
	}
	fmt.Println("Applied.")
	return nil
}

func buildAIDiscoverPrompt(stack, currentSkillPath, currentReviewerAgent string, contents map[string]string) string {
	var b strings.Builder

	stackLine := stack
	if stackLine == "" {
		stackLine = "none detected"
	}
	fmt.Fprintf(&b, "You are helping configure a CLI tool called looper.\n\n")
	fmt.Fprintf(&b, "Detected project stack: %s\n\n", stackLine)
	fmt.Fprintf(&b, "Current settings:\n")
	fmt.Fprintf(&b, "  skill_path     = %s\n", currentSkillPath)
	fmt.Fprintf(&b, "  reviewer_agent = %s\n\n", currentReviewerAgent)

	fmt.Fprintf(&b, "Discovered files (you may ONLY recommend paths from this list):\n\n")
	for path, content := range contents {
		fmt.Fprintf(&b, "--- %s ---\n%s\n\n", path, content)
	}

	fmt.Fprintf(&b, "Allowed settings keys: skill_path, reviewer_agent\n\n")
	fmt.Fprintf(&b, "Based on the detected stack and file contents, recommend which paths to assign.\n")
	fmt.Fprintf(&b, "Respond with ONLY a JSON object, e.g.:\n")
	fmt.Fprintf(&b, `{"skill_path": "/abs/path/SKILL.md", "reviewer_agent": "/abs/path/agent.md"}`)
	fmt.Fprintf(&b, "\nOmit a key entirely if no good candidate exists.\n")
	fmt.Fprintf(&b, "Do not invent or guess paths — only use paths from the discovered files list above.\n")

	return b.String()
}
