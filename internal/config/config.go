package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const configDir = "looper"
const configFile = "config.json"

type Defaults struct {
	Cycles  int `json:"cycles"`
	Timeout int `json:"timeout"`
}

type Config struct {
	Backend       string   `json:"backend"`
	Defaults      Defaults `json:"defaults"`
	SkillPath     string   `json:"skill_path"`
	ReviewerAgent string   `json:"reviewer_agent"`
	TicketPattern string   `json:"ticket_pattern"`
	TrustedDirs   []string `json:"trusted_dirs,omitempty"`
	LinearAPIKey  string   `json:"linear_api_key,omitempty"`
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
		if os.IsNotExist(err) {
			return defaultConfig, nil
		}
		return defaultConfig, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig, fmt.Errorf("invalid config file: %w", err)
	}

	// Fill in zero values with defaults
	if cfg.Backend == "" {
		cfg.Backend = defaultConfig.Backend
	}
	if cfg.Defaults.Cycles == 0 {
		cfg.Defaults.Cycles = defaultConfig.Defaults.Cycles
	}
	if cfg.Defaults.Timeout < 10 {
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

	return os.WriteFile(path, data, 0644)
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
	case "linear_api_key":
		return cfg.LinearAPIKey, nil
	default:
		return "", fmt.Errorf("unknown key: %s (valid keys: backend, defaults.cycles, defaults.timeout, skill_path, reviewer_agent, ticket_pattern, linear_api_key)", key)
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
	case "linear_api_key":
		cfg.LinearAPIKey = value
	default:
		return cfg, fmt.Errorf("unknown key: %s (valid keys: backend, defaults.cycles, defaults.timeout, skill_path, reviewer_agent, ticket_pattern, linear_api_key)", key)
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
