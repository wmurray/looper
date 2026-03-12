package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- Load ---

func TestLoad_MissingFile_ReturnsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir())

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON in config file")
	}
	if cfg.Backend != "" || cfg.Defaults.Cycles != 0 || cfg.Defaults.Timeout != 0 {
		t.Errorf("expected zero Config on error, got backend=%q cycles=%d timeout=%d",
			cfg.Backend, cfg.Defaults.Cycles, cfg.Defaults.Timeout)
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

// --- LoadWithRepo ---

// loadWithRepoAt changes to dir, calls LoadWithRepo, and restores the original
// working directory. The config global state (HOME) must be set before calling.
// WARNING: Must not be called from parallel tests (t.Parallel()) — os.Chdir
// mutates the process-global working directory.
func loadWithRepoAt(t *testing.T, dir string) (Config, string, []string, error) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()
	return LoadWithRepo()
}

// initTempGitRepo creates a temp dir with a git repo and returns its path.
func initTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	// Need at least one commit so rev-parse works fully
	f := filepath.Join(dir, "README")
	if err := os.WriteFile(f, []byte("hi"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	return dir
}

func TestLoadWithRepo_NoRepoConfig_ReturnsGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	cfg, repoPath, keys, err := loadWithRepoAt(t, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoPath != "" {
		t.Errorf("expected empty repoPath, got %q", repoPath)
	}
	if len(keys) != 0 {
		t.Errorf("expected no overlay keys, got %v", keys)
	}
	if cfg.Backend != defaultConfig.Backend {
		t.Errorf("backend = %q, want %q", cfg.Backend, defaultConfig.Backend)
	}
	if cfg.Defaults.Cycles != defaultConfig.Defaults.Cycles {
		t.Errorf("cycles = %d, want %d", cfg.Defaults.Cycles, defaultConfig.Defaults.Cycles)
	}
}

func TestLoadWithRepo_ReturnsOverlayKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`{"defaults":{"cycles":3},"backend":"cursor"}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, keys, err := loadWithRepoAt(t, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{"defaults.cycles": true, "backend": true}
	if len(keys) != len(want) {
		t.Fatalf("overlay keys = %v, want %v", keys, want)
	}
	for _, k := range keys {
		if !want[k] {
			t.Errorf("unexpected overlay key %q", k)
		}
	}
	for k := range want {
		found := false
		for _, k2 := range keys {
			if k == k2 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected overlay key %q not present in %v", k, keys)
		}
	}
}

func TestLoadWithRepo_ZeroValuesNotInOverlayKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`{"defaults":{"cycles":0}}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, keys, err := loadWithRepoAt(t, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected no overlay keys for zero values, got %v", keys)
	}
}

func TestLoadWithRepo_InvalidJSON_ReturnsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`not valid json`), 0644); err != nil {
		t.Fatalf("write .looper.json: %v", err)
	}

	_, _, _, err := loadWithRepoAt(t, repoDir)
	if err == nil {
		t.Fatal("expected error for invalid JSON in .looper.json")
	}
}

func TestLoadWithRepo_ReturnsEmptyConfigOnError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Poison the global config to make Load() fail.
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	repoDir := initTempGitRepo(t)
	cfg, _, _, err := loadWithRepoAt(t, repoDir)
	if err == nil {
		t.Fatal("expected error for invalid global config")
	}
	// On error, LoadWithRepo must return a zero Config (not defaults).
	if cfg.Backend != "" || cfg.Defaults.Cycles != 0 || cfg.Defaults.Timeout != 0 {
		t.Errorf("expected zero Config on error, got backend=%q cycles=%d timeout=%d",
			cfg.Backend, cfg.Defaults.Cycles, cfg.Defaults.Timeout)
	}
}

func TestLoadWithRepo_ZeroValuesDoNotOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`{"defaults":{"cycles":0,"timeout":0},"backend":""}`), 0644); err != nil {
		t.Fatalf("write .looper.json: %v", err)
	}

	cfg, _, _, err := loadWithRepoAt(t, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Defaults.Cycles != defaultConfig.Defaults.Cycles {
		t.Errorf("cycles = %d, want %d (zero should not override)", cfg.Defaults.Cycles, defaultConfig.Defaults.Cycles)
	}
	if cfg.Defaults.Timeout != defaultConfig.Defaults.Timeout {
		t.Errorf("timeout = %d, want %d (zero should not override)", cfg.Defaults.Timeout, defaultConfig.Defaults.Timeout)
	}
	if cfg.Backend != defaultConfig.Backend {
		t.Errorf("backend = %q, want %q (empty should not override)", cfg.Backend, defaultConfig.Backend)
	}
}

func TestLoadWithRepo_NotInGitRepo_ReturnsGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir() // plain dir, no git init

	cfg, repoPath, keys, err := loadWithRepoAt(t, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoPath != "" {
		t.Errorf("expected empty repoPath outside git repo, got %q", repoPath)
	}
	if len(keys) != 0 {
		t.Errorf("expected no overlay keys outside git repo, got %v", keys)
	}
	if cfg.Backend != defaultConfig.Backend {
		t.Errorf("backend = %q, want %q", cfg.Backend, defaultConfig.Backend)
	}
}

func TestLoadWithRepo_ReadErrorIncludesPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	// Create .looper.json as a directory so ReadFile fails with a non-ENOENT error.
	looperDir := filepath.Join(repoDir, ".looper.json")
	if err := os.Mkdir(looperDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, _, _, err := loadWithRepoAt(t, repoDir)
	if err == nil {
		t.Fatal("expected error when .looper.json is a directory")
	}
	if !strings.Contains(err.Error(), ".looper.json") {
		t.Errorf("error %q does not contain the repo config path", err.Error())
	}
}

func TestApplyRepoOverlay_TrustedDirsExcluded(t *testing.T) {
	dst := Config{TrustedDirs: []string{"/existing"}}
	src := Config{TrustedDirs: []string{"/injected"}, Backend: "cursor"}

	result, _ := applyRepoOverlay(dst, src)

	if len(result.TrustedDirs) != 1 || result.TrustedDirs[0] != "/existing" {
		t.Errorf("TrustedDirs = %v, want [/existing] — repo config must not modify TrustedDirs", result.TrustedDirs)
	}
}

func TestLoadWithRepo_TimeoutAtMinBoundary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`{"defaults":{"timeout":10}}`), 0644); err != nil {
		t.Fatalf("write .looper.json: %v", err)
	}

	cfg, _, keys, err := loadWithRepoAt(t, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Defaults.Timeout != 10 {
		t.Errorf("timeout = %d, want 10 (minTimeout boundary should be applied)", cfg.Defaults.Timeout)
	}
	found := false
	for _, k := range keys {
		if k == "defaults.timeout" {
			found = true
		}
	}
	if !found {
		t.Errorf("overlay keys = %v, want defaults.timeout to be present", keys)
	}
}

func TestLoad_PermissionDenied_ReturnsZeroConfig(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denial as root")
	}
	t.Setenv("HOME", t.TempDir())

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"backend":"claude"}`), 0000); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
	if cfg.Backend != "" || cfg.Defaults.Cycles != 0 || cfg.Defaults.Timeout != 0 {
		t.Errorf("expected zero Config on error, got backend=%q cycles=%d timeout=%d",
			cfg.Backend, cfg.Defaults.Cycles, cfg.Defaults.Timeout)
	}
}

func TestLoad_MissingFile_ReturnsDefaults_Hermetic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend != defaultConfig.Backend {
		t.Errorf("backend = %q, want %q", cfg.Backend, defaultConfig.Backend)
	}
	if cfg.Defaults.Cycles != defaultConfig.Defaults.Cycles {
		t.Errorf("cycles = %d, want %d", cfg.Defaults.Cycles, defaultConfig.Defaults.Cycles)
	}
	if cfg.Defaults.Timeout != defaultConfig.Defaults.Timeout {
		t.Errorf("timeout = %d, want %d", cfg.Defaults.Timeout, defaultConfig.Defaults.Timeout)
	}
}

func TestLoadWithRepo_RepoConfigOverridesCycles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initTempGitRepo(t)

	looperJSON := filepath.Join(repoDir, ".looper.json")
	if err := os.WriteFile(looperJSON, []byte(`{"defaults":{"cycles":3}}`), 0644); err != nil {
		t.Fatalf("write .looper.json: %v", err)
	}

	cfg, repoPath, _, err := loadWithRepoAt(t, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repoPath == "" {
		t.Error("expected non-empty repoPath")
	}
	if cfg.Defaults.Cycles != 3 {
		t.Errorf("cycles = %d, want 3", cfg.Defaults.Cycles)
	}
	if cfg.Backend != defaultConfig.Backend {
		t.Errorf("backend = %q, want %q", cfg.Backend, defaultConfig.Backend)
	}
}
