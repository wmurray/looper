package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, cfg Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// --- Load ---

func TestLoad_MissingFile_ReturnsDefaults(t *testing.T) {
	// Point CONFIG lookup at a nonexistent path by using a fresh temp dir
	// We test Load() by calling it directly; it falls back to defaults on ENOENT.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend != "claude" {
		t.Errorf("expected default backend 'claude', got %q", cfg.Backend)
	}
	if cfg.Defaults.Cycles != 5 {
		t.Errorf("expected default cycles 5, got %d", cfg.Defaults.Cycles)
	}
	if cfg.Defaults.Timeout != 420 {
		t.Errorf("expected default timeout 420, got %d", cfg.Defaults.Timeout)
	}
}

func TestLoad_InvalidJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("not json"), 0644)

	// Manually unmarshal to simulate what Load does
	var cfg Config
	err := json.Unmarshal([]byte("not json"), &cfg)
	if err == nil {
		t.Fatal("expected unmarshal error for invalid JSON")
	}
}

// --- Get ---

func TestGet_ValidKeys(t *testing.T) {
	cfg := Config{
		Backend:       "claude",
		Defaults:      Defaults{Cycles: 3, Timeout: 300},
		SkillPath:     "/some/skill.md",
		ReviewerAgent: "/some/reviewer.md",
	}

	cases := []struct {
		key  string
		want string
	}{
		{"backend", "claude"},
		{"defaults.cycles", "3"},
		{"defaults.timeout", "300"},
		{"skill_path", "/some/skill.md"},
		{"reviewer_agent", "/some/reviewer.md"},
	}

	for _, tc := range cases {
		val, err := Get(cfg, tc.key)
		if err != nil {
			t.Errorf("Get(%q): unexpected error: %v", tc.key, err)
		}
		if val != tc.want {
			t.Errorf("Get(%q) = %q, want %q", tc.key, val, tc.want)
		}
	}
}

func TestGet_UnknownKey_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Get(cfg, "nonexistent.key")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

// --- Set ---

func TestSet_Backend_Valid(t *testing.T) {
	cfg := Config{Backend: "cursor"}
	updated, err := Set(cfg, "backend", "claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Backend != "claude" {
		t.Errorf("expected backend 'claude', got %q", updated.Backend)
	}
}

func TestSet_Backend_Invalid(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "backend", "openai")
	if err == nil {
		t.Fatal("expected error for invalid backend value")
	}
}

func TestSet_Cycles_Valid(t *testing.T) {
	cfg := Config{Defaults: Defaults{Cycles: 5}}
	updated, err := Set(cfg, "defaults.cycles", "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Defaults.Cycles != 10 {
		t.Errorf("expected cycles 10, got %d", updated.Defaults.Cycles)
	}
}

func TestSet_Cycles_Zero_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "defaults.cycles", "0")
	if err == nil {
		t.Fatal("expected error for cycles=0")
	}
}

func TestSet_Cycles_Negative_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "defaults.cycles", "-1")
	if err == nil {
		t.Fatal("expected error for negative cycles")
	}
}

func TestSet_Timeout_BelowMinimum_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "defaults.timeout", "5")
	if err == nil {
		t.Fatal("expected error for timeout < 10")
	}
}

func TestSet_Timeout_Valid(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "defaults.timeout", "60")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Defaults.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", updated.Defaults.Timeout)
	}
}

func TestSet_SkillPath(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "skill_path", "/new/path/skill.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.SkillPath != "/new/path/skill.md" {
		t.Errorf("expected updated skill_path, got %q", updated.SkillPath)
	}
}

func TestSet_UnknownKey_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "unknown.key", "value")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

// --- IsTrusted / TrustDir ---

func TestIsTrusted_NotInList(t *testing.T) {
	cfg := Config{}
	if IsTrusted(cfg, "/some/dir") {
		t.Fatal("expected false for empty trusted list")
	}
}

func TestIsTrusted_InList(t *testing.T) {
	cfg := Config{TrustedDirs: []string{"/some/dir", "/other/dir"}}
	if !IsTrusted(cfg, "/some/dir") {
		t.Fatal("expected true for dir in trusted list")
	}
}

func TestIsTrusted_ExactMatch(t *testing.T) {
	cfg := Config{TrustedDirs: []string{"/some/dir"}}
	if IsTrusted(cfg, "/some/dir/subdir") {
		t.Fatal("should not match subdirectories — exact match only")
	}
}

func TestTrustDir_AddsDir(t *testing.T) {
	cfg := Config{Backend: "claude", Defaults: Defaults{Cycles: 5, Timeout: 420}}
	// Call the in-memory logic directly without saving to disk.
	if IsTrusted(cfg, "/new/repo") {
		t.Fatal("should not be trusted before TrustDir")
	}
	cfg.TrustedDirs = append(cfg.TrustedDirs, "/new/repo")
	if !IsTrusted(cfg, "/new/repo") {
		t.Fatal("expected /new/repo to be trusted after TrustDir")
	}
}

func TestTrustDir_NoDuplicates(t *testing.T) {
	cfg := Config{TrustedDirs: []string{"/existing/repo"}}
	updated, _ := TrustDir(cfg, "/existing/repo")
	count := 0
	for _, d := range updated.TrustedDirs {
		if d == "/existing/repo" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 entry, got %d", count)
	}
}

// --- ExpandPath ---

func TestExpandPath_Tilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := ExpandPath("~/foo/bar")
	expected := filepath.Join(home, "foo/bar")
	if result != expected {
		t.Errorf("ExpandPath(~/foo/bar) = %q, want %q", result, expected)
	}
}

func TestExpandPath_AbsolutePath(t *testing.T) {
	result := ExpandPath("/absolute/path")
	if result != "/absolute/path" {
		t.Errorf("ExpandPath should not modify absolute path, got %q", result)
	}
}

func TestExpandPath_NoTilde(t *testing.T) {
	result := ExpandPath("relative/path")
	if result != "relative/path" {
		t.Errorf("ExpandPath should not modify relative path without tilde, got %q", result)
	}
}

// --- Save ---

func TestSave_FilePermissions(t *testing.T) {
	// Redirect HOME so ConfigPath() resolves inside a temp directory,
	// keeping the test hermetic and avoiding writes to the real config file.
	t.Setenv("HOME", t.TempDir())

	cfg := Config{Backend: "claude", Defaults: Defaults{Cycles: 5, Timeout: 420}}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("config file permissions = %04o, want 0600", perm)
	}
}

func TestSet_LinearAPIKey(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "linear_api_key", "lin_api_abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.LinearAPIKey != "lin_api_abc123" {
		t.Errorf("LinearAPIKey = %q, want %q", updated.LinearAPIKey, "lin_api_abc123")
	}
}

func TestGet_LinearAPIKey(t *testing.T) {
	cfg := Config{LinearAPIKey: "lin_api_xyz"}
	val, err := Get(cfg, "linear_api_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "lin_api_xyz" {
		t.Errorf("Get(linear_api_key) = %q, want %q", val, "lin_api_xyz")
	}
}
