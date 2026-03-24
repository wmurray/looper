package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/willmurray/looper/internal/git"
)

const (
	configDir      = "looper"
	configFile     = "config.json"
	repoConfigFile = ".looper.json"
)

// Gotcha: values below this are treated as absent/unset in both Load and applyRepoOverlay.
const minTimeout = 10

type Defaults struct {
	Cycles  int `json:"cycles"`
	Timeout int `json:"timeout"`
}

type Reviewers struct {
	General     string   `json:"general"`
	Specialized []string `json:"specialized"`
}

type ReviewStrategy struct {
	Mode                    string   `json:"mode"`
	GeneralEvery            int      `json:"general_every"`
	SpecializedEvery        int      `json:"specialized_every"`
	SpecializedOnCompletion bool     `json:"specialized_on_completion"`
	// Gotcha: pointer so that explicit 0.0 ("any approval counts") is distinguishable from unset.
	MajorityThreshold       *float64 `json:"majority_threshold,omitempty"`
}

type Config struct {
	Backend        string          `json:"backend"`
	Defaults       Defaults        `json:"defaults"`
	SkillPath      string          `json:"skill_path"`
	ReviewerAgent  string          `json:"reviewer_agent"`
	TicketPattern  string          `json:"ticket_pattern"`
	Retries        *int            `json:"retries,omitempty"`
	ReviewEvery    *int            `json:"review_every,omitempty"`
	TrustedDirs    []string        `json:"trusted_dirs,omitempty"`
	PolishAgent    string          `json:"polish_agent,omitempty"`
	PolishCmds     []string        `json:"polish_cmds,omitempty"`
	Notify         bool            `json:"notify,omitempty"`
	NotifyWebhook  string          `json:"notify_webhook,omitempty"`
	Reviewers      *Reviewers      `json:"reviewers,omitempty"`
	ReviewStrategy *ReviewStrategy `json:"review_strategy,omitempty"`
}

// MigrateReviewerAgent converts the deprecated ReviewerAgent string field to
// Reviewers.General on load. In-memory only — never rewrites the config file.
func MigrateReviewerAgent(cfg *Config) {
	if cfg.ReviewerAgent != "" && cfg.Reviewers == nil {
		cfg.Reviewers = &Reviewers{General: cfg.ReviewerAgent}
	}
}

// EffectiveReviewers returns cfg.Reviewers with defaults applied.
func EffectiveReviewers(cfg Config) Reviewers {
	if cfg.Reviewers != nil {
		return *cfg.Reviewers
	}
	return Reviewers{General: cfg.ReviewerAgent}
}

// EffectiveReviewStrategy returns cfg.ReviewStrategy with defaults applied.
func EffectiveReviewStrategy(cfg Config) ReviewStrategy {
	defaultThreshold := 0.6
	s := ReviewStrategy{
		Mode:              "smart",
		GeneralEvery:      1,
		SpecializedEvery:  3,
		MajorityThreshold: &defaultThreshold,
	}
	if cfg.ReviewStrategy == nil {
		return s
	}
	r := *cfg.ReviewStrategy
	if r.Mode == "" {
		r.Mode = s.Mode
	}
	if r.GeneralEvery == 0 {
		r.GeneralEvery = s.GeneralEvery
	}
	if r.SpecializedEvery == 0 {
		r.SpecializedEvery = s.SpecializedEvery
	}
	if r.MajorityThreshold == nil {
		r.MajorityThreshold = s.MajorityThreshold
	}
	return r
}

var defaultConfig = Config{
	Backend: "claude",
	Defaults: Defaults{
		Cycles:  5,
		Timeout: 420,
	},
	SkillPath:     "~/.claude/skills/tdd-workflow/SKILL.md",
	ReviewerAgent: "~/.claude/agents/rails-code-reviewer.md",
	TicketPattern: `[A-Z]+-[0-9]+`,
}

func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configDir, configFile), nil
}

func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return defaultConfig, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return defaultConfig, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("invalid config file: %w", err)
	}

	if cfg.Backend == "" {
		cfg.Backend = defaultConfig.Backend
	}
	if cfg.Defaults.Cycles == 0 {
		cfg.Defaults.Cycles = defaultConfig.Defaults.Cycles
	}
	if cfg.Defaults.Timeout < minTimeout {
		cfg.Defaults.Timeout = defaultConfig.Defaults.Timeout
	}
	if cfg.SkillPath == "" {
		cfg.SkillPath = defaultConfig.SkillPath
	}
	if cfg.ReviewerAgent == "" {
		cfg.ReviewerAgent = defaultConfig.ReviewerAgent
	}
	if cfg.TicketPattern == "" {
		cfg.TicketPattern = defaultConfig.TicketPattern
	}

	return cfg, nil
}

// Why: TrustedDirs is excluded — allowing a repo config to grant itself trust would undermine the security model.
func applyRepoOverlay(dst, src Config) (Config, []string) {
	var keys []string
	if src.Backend != "" {
		dst.Backend = src.Backend
		keys = append(keys, "backend")
	}
	if src.Defaults.Cycles != 0 {
		dst.Defaults.Cycles = src.Defaults.Cycles
		keys = append(keys, "defaults.cycles")
	}
	// Timeout < minTimeout is treated as absent (matches the same sentinel used in Load).
	if src.Defaults.Timeout >= minTimeout {
		dst.Defaults.Timeout = src.Defaults.Timeout
		keys = append(keys, "defaults.timeout")
	}
	if src.SkillPath != "" {
		dst.SkillPath = src.SkillPath
		keys = append(keys, "skill_path")
	}
	if src.ReviewerAgent != "" {
		dst.ReviewerAgent = src.ReviewerAgent
		keys = append(keys, "reviewer_agent")
	}
	if src.TicketPattern != "" {
		dst.TicketPattern = src.TicketPattern
		keys = append(keys, "ticket_pattern")
	}
	if src.PolishAgent != "" {
		dst.PolishAgent = src.PolishAgent
		keys = append(keys, "polish_agent")
	}
	if len(src.PolishCmds) > 0 {
		dst.PolishCmds = src.PolishCmds
		keys = append(keys, "polish_cmds")
	}
	if src.Retries != nil {
		dst.Retries = src.Retries
		keys = append(keys, "retries")
	}
	if src.ReviewEvery != nil {
		dst.ReviewEvery = src.ReviewEvery
		keys = append(keys, "review_every")
	}
	if src.Reviewers != nil {
		dst.Reviewers = src.Reviewers
		keys = append(keys, "reviewers")
	}
	if src.ReviewStrategy != nil {
		dst.ReviewStrategy = src.ReviewStrategy
		keys = append(keys, "review_strategy")
	}
	return dst, keys
}

// LoadWithRepo loads the global config and overlays any non-zero values from
// .looper.json at the git repo root. It returns the merged Config, the path of
// the repo config that was applied (empty if none), the dot-notation keys that
// were overridden, and any error.
func LoadWithRepo() (Config, string, []string, error) {
	cfg, err := Load()
	if err != nil {
		return Config{}, "", nil, err
	}

	root, err := git.RepoRoot()
	if err != nil {
		return cfg, "", nil, nil
	}
	repoPath := filepath.Join(root, repoConfigFile)

	data, err := os.ReadFile(repoPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, "", nil, nil
		}
		return Config{}, "", nil, fmt.Errorf("reading repo config %q: %w", repoPath, err)
	}

	var repoCfg Config
	if err := json.Unmarshal(data, &repoCfg); err != nil {
		return Config{}, "", nil, fmt.Errorf("invalid repo config %q: %w", repoPath, err)
	}

	merged, keys := applyRepoOverlay(cfg, repoCfg)
	return merged, repoPath, keys, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func Reset() error {
	return Save(defaultConfig)
}

// Get retrieves a value by dot-notation key (e.g. "defaults.cycles").
func Get(cfg Config, key string) (string, error) {
	switch key {
	case "backend":
		return cfg.Backend, nil
	case "defaults.cycles":
		return fmt.Sprintf("%d", cfg.Defaults.Cycles), nil
	case "defaults.timeout":
		return fmt.Sprintf("%d", cfg.Defaults.Timeout), nil
	case "skill_path":
		return cfg.SkillPath, nil
	case "reviewer_agent":
		return cfg.ReviewerAgent, nil
	case "ticket_pattern":
		return cfg.TicketPattern, nil
	case "polish_agent":
		return cfg.PolishAgent, nil
	case "polish_cmds":
		return strings.Join(cfg.PolishCmds, ", "), nil
	case "notify":
		if cfg.Notify {
			return "true", nil
		}
		return "false", nil
	case "notify_webhook":
		return cfg.NotifyWebhook, nil
	case "retries":
		if cfg.Retries == nil {
			return "0", nil
		}
		return fmt.Sprintf("%d", *cfg.Retries), nil
	case "review_every":
		if cfg.ReviewEvery == nil {
			return "1", nil
		}
		return fmt.Sprintf("%d", *cfg.ReviewEvery), nil
	default:
		return "", fmt.Errorf("unknown key: %s (valid keys: backend, defaults.cycles, defaults.timeout, skill_path, reviewer_agent, ticket_pattern, polish_agent, polish_cmds, notify, notify_webhook, retries, review_every)", key)
	}
}

// Set updates a value by dot-notation key and returns the updated config.
func Set(cfg Config, key, value string) (Config, error) {
	switch key {
	case "backend":
		if value != "cursor" && value != "claude" {
			return cfg, fmt.Errorf("backend must be 'cursor' or 'claude'")
		}
		cfg.Backend = value
	case "defaults.cycles":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 1 {
			return cfg, fmt.Errorf("defaults.cycles must be a positive integer")
		}
		cfg.Defaults.Cycles = n
	case "defaults.timeout":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 10 {
			return cfg, fmt.Errorf("defaults.timeout must be >= 10")
		}
		cfg.Defaults.Timeout = n
	case "skill_path":
		cfg.SkillPath = value
	case "reviewer_agent":
		cfg.ReviewerAgent = value
	case "ticket_pattern":
		if _, err := regexp.Compile(value); err != nil {
			return cfg, fmt.Errorf("ticket_pattern is not a valid regex: %w", err)
		}
		cfg.TicketPattern = value
	case "polish_agent":
		cfg.PolishAgent = value
	case "polish_cmds":
		parts := strings.Split(value, ",")
		var cmds []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cmds = append(cmds, p)
			}
		}
		if len(cmds) == 0 {
			return cfg, fmt.Errorf("polish_cmds must contain at least one non-empty command")
		}
		cfg.PolishCmds = cmds
	case "notify":
		switch value {
		case "true", "1":
			cfg.Notify = true
		case "false", "0":
			cfg.Notify = false
		default:
			return cfg, fmt.Errorf("notify must be 'true' or 'false'")
		}
	case "notify_webhook":
		cfg.NotifyWebhook = value
	case "retries":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 0 {
			return cfg, fmt.Errorf("retries must be a non-negative integer")
		}
		cfg.Retries = &n
	case "review_every":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 1 {
			return cfg, fmt.Errorf("review_every must be a positive integer")
		}
		cfg.ReviewEvery = &n
	default:
		return cfg, fmt.Errorf("unknown key: %s (valid keys: backend, defaults.cycles, defaults.timeout, skill_path, reviewer_agent, ticket_pattern, polish_agent, polish_cmds, notify, notify_webhook, retries, review_every)", key)
	}
	return cfg, nil
}

// IsTrusted reports whether dir is in the trusted directories list.
func IsTrusted(cfg Config, dir string) bool {
	for _, d := range cfg.TrustedDirs {
		if d == dir {
			return true
		}
	}
	return false
}

// TrustDir adds dir to the trusted directories list and saves the config.
func TrustDir(cfg Config, dir string) (Config, error) {
	if IsTrusted(cfg, dir) {
		return cfg, nil
	}
	cfg.TrustedDirs = append(cfg.TrustedDirs, dir)
	return cfg, Save(cfg)
}

// ExpandPath expands ~ to the home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
