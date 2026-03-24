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
	// Point CONFIG lookup at a nonexistent path by using a fresh temp dir
	// We test Load() by calling it directly; it falls back to defaults on ENOENT.
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

func TestGet_LinearAPIKey_ReturnsUnknownError(t *testing.T) {
	cfg := Config{}
	_, err := Get(cfg, "linear_api_key")
	if err == nil {
		t.Fatal("expected error for linear_api_key — key must not be stored in config")
	}
}

func TestSet_LinearAPIKey_ReturnsUnknownError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "linear_api_key", "lin_api_abc123")
	if err == nil {
		t.Fatal("expected error for linear_api_key — key must not be stored in config")
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

func TestGet_PolishAgent(t *testing.T) {
	cfg := Config{PolishAgent: "/some/polish-agent.md"}
	val, err := Get(cfg, "polish_agent")
	if err != nil {
		t.Fatalf("Get(polish_agent): unexpected error: %v", err)
	}
	if val != "/some/polish-agent.md" {
		t.Errorf("Get(polish_agent) = %q, want %q", val, "/some/polish-agent.md")
	}
}

func TestGet_PolishCmds(t *testing.T) {
	cfg := Config{PolishCmds: []string{"go fmt ./...", "go vet ./..."}}
	val, err := Get(cfg, "polish_cmds")
	if err != nil {
		t.Fatalf("Get(polish_cmds): unexpected error: %v", err)
	}
	if val != "go fmt ./..., go vet ./..." {
		t.Errorf("Get(polish_cmds) = %q, want %q", val, "go fmt ./..., go vet ./...")
	}
}

func TestGet_PolishCmds_Empty(t *testing.T) {
	cfg := Config{}
	val, err := Get(cfg, "polish_cmds")
	if err != nil {
		t.Fatalf("Get(polish_cmds): unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("Get(polish_cmds) on empty slice = %q, want %q", val, "")
	}
}

func TestSet_PolishAgent(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "polish_agent", "/my/agent.md")
	if err != nil {
		t.Fatalf("Set(polish_agent): unexpected error: %v", err)
	}
	if updated.PolishAgent != "/my/agent.md" {
		t.Errorf("PolishAgent = %q, want %q", updated.PolishAgent, "/my/agent.md")
	}
}

func TestSet_PolishCmds(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "polish_cmds", "go fmt ./...,go vet ./...")
	if err != nil {
		t.Fatalf("Set(polish_cmds): unexpected error: %v", err)
	}
	if len(updated.PolishCmds) != 2 {
		t.Fatalf("PolishCmds length = %d, want 2", len(updated.PolishCmds))
	}
	if updated.PolishCmds[0] != "go fmt ./..." {
		t.Errorf("PolishCmds[0] = %q, want %q", updated.PolishCmds[0], "go fmt ./...")
	}
	if updated.PolishCmds[1] != "go vet ./..." {
		t.Errorf("PolishCmds[1] = %q, want %q", updated.PolishCmds[1], "go vet ./...")
	}
}

func TestSet_PolishCmds_EmptyRejectsBlank(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "polish_cmds", "   ,  ,  ")
	if err == nil {
		t.Fatal("expected error for blank-only commands in polish_cmds")
	}
}

func TestApplyRepoOverlay_PolishFields(t *testing.T) {
	dst := Config{}
	src := Config{PolishAgent: "/src/agent.md", PolishCmds: []string{"go fmt ./..."}}
	result, keys := applyRepoOverlay(dst, src)
	if result.PolishAgent != "/src/agent.md" {
		t.Errorf("PolishAgent = %q, want %q", result.PolishAgent, "/src/agent.md")
	}
	if len(result.PolishCmds) != 1 || result.PolishCmds[0] != "go fmt ./..." {
		t.Errorf("PolishCmds = %v, want [go fmt ./...]", result.PolishCmds)
	}
	wantKeys := map[string]bool{"polish_agent": true, "polish_cmds": true}
	for _, k := range keys {
		if wantKeys[k] {
			delete(wantKeys, k)
		}
	}
	if len(wantKeys) != 0 {
		t.Errorf("overlay keys missing: %v", wantKeys)
	}
}

func TestSet_Notify_True(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "notify", "true")
	if err != nil {
		t.Fatalf("Set(notify, true): unexpected error: %v", err)
	}
	if !updated.Notify {
		t.Error("expected Notify = true")
	}
}

func TestSet_Notify_False(t *testing.T) {
	cfg := Config{Notify: true}
	updated, err := Set(cfg, "notify", "false")
	if err != nil {
		t.Fatalf("Set(notify, false): unexpected error: %v", err)
	}
	if updated.Notify {
		t.Error("expected Notify = false")
	}
}

func TestSet_Notify_Invalid(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "notify", "yes")
	if err == nil {
		t.Fatal("expected error for invalid notify value")
	}
}

func TestGet_Notify_True(t *testing.T) {
	cfg := Config{Notify: true}
	val, err := Get(cfg, "notify")
	if err != nil {
		t.Fatalf("Get(notify): unexpected error: %v", err)
	}
	if val != "true" {
		t.Errorf("Get(notify) = %q, want %q", val, "true")
	}
}

func TestGet_Notify_False(t *testing.T) {
	cfg := Config{Notify: false}
	val, err := Get(cfg, "notify")
	if err != nil {
		t.Fatalf("Get(notify): unexpected error: %v", err)
	}
	if val != "false" {
		t.Errorf("Get(notify) = %q, want %q", val, "false")
	}
}

func TestSet_NotifyWebhook(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "notify_webhook", "https://hooks.slack.com/test")
	if err != nil {
		t.Fatalf("Set(notify_webhook): unexpected error: %v", err)
	}
	if updated.NotifyWebhook != "https://hooks.slack.com/test" {
		t.Errorf("NotifyWebhook = %q, want %q", updated.NotifyWebhook, "https://hooks.slack.com/test")
	}
}

// --- retries key ---

func TestSet_Retries_Valid(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "retries", "3")
	if err != nil {
		t.Fatalf("Set(retries, 3): unexpected error: %v", err)
	}
	if updated.Retries == nil || *updated.Retries != 3 {
		t.Errorf("Retries = %v, want pointer to 3", updated.Retries)
	}
}

func TestSet_Retries_Zero(t *testing.T) {
	cfg := Config{Retries: intPtr(2)}
	updated, err := Set(cfg, "retries", "0")
	if err != nil {
		t.Fatalf("Set(retries, 0): unexpected error: %v", err)
	}
	if updated.Retries == nil || *updated.Retries != 0 {
		t.Errorf("Retries = %v, want pointer to 0", updated.Retries)
	}
}

func TestSet_Retries_Negative_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "retries", "-1")
	if err == nil {
		t.Fatal("expected error for negative retries")
	}
}

func TestSet_Retries_NonInt_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "retries", "abc")
	if err == nil {
		t.Fatal("expected error for non-integer retries")
	}
}

func TestGet_Retries(t *testing.T) {
	cfg := Config{Retries: intPtr(3)}
	val, err := Get(cfg, "retries")
	if err != nil {
		t.Fatalf("Get(retries): unexpected error: %v", err)
	}
	if val != "3" {
		t.Errorf("Get(retries) = %q, want %q", val, "3")
	}
}

func TestGet_Retries_Nil(t *testing.T) {
	cfg := Config{} // Retries is nil
	val, err := Get(cfg, "retries")
	if err != nil {
		t.Fatalf("Get(retries) on nil: unexpected error: %v", err)
	}
	if val != "0" {
		t.Errorf("Get(retries) on nil = %q, want %q", val, "0")
	}
}

func TestGet_Retries_RoundTrip(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "retries", "3")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := Get(updated, "retries")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "3" {
		t.Errorf("round-trip: got %q, want %q", val, "3")
	}
}

func TestApplyRepoOverlay_Retries(t *testing.T) {
	dst := Config{}
	src := Config{Retries: intPtr(2)}
	result, keys := applyRepoOverlay(dst, src)
	if result.Retries == nil || *result.Retries != 2 {
		t.Errorf("Retries = %v, want pointer to 2", result.Retries)
	}
	found := false
	for _, k := range keys {
		if k == "retries" {
			found = true
		}
	}
	if !found {
		t.Errorf("overlay keys = %v, want retries to be present", keys)
	}
}

// TestApplyRepoOverlay_RetriesZeroNotOverridden documents that retries: 0 in a
// repo config cannot clear a non-zero global value. This is intentional: the
// same > 0 sentinel used by cycles and timeout is applied here for consistency.
// A follow-up could allow zero to mean "disable" if that use case arises.
// TestApplyRepoOverlay_RetriesZeroOverridesGlobal documents that retries: 0 in
// a repo config must override a non-zero global value. This requires *int so
// that a nil pointer (absent from JSON) is distinguishable from a pointer to 0
// (explicitly set to zero).
func TestApplyRepoOverlay_RetriesZeroOverridesGlobal(t *testing.T) {
	dst := Config{Retries: intPtr(2)}
	src := Config{Retries: intPtr(0)}
	result, keys := applyRepoOverlay(dst, src)
	if result.Retries == nil || *result.Retries != 0 {
		t.Errorf("Retries = %v, want pointer to 0 (explicit zero must override global non-zero)", result.Retries)
	}
	found := false
	for _, k := range keys {
		if k == "retries" {
			found = true
		}
	}
	if !found {
		t.Errorf("overlay keys = %v, want 'retries' to be present when explicitly set to 0", keys)
	}
}

// TestApplyRepoOverlay_RetriesAbsentDoesNotOverride documents that a repo
// config with no retries field (nil pointer) leaves the global value intact.
func TestApplyRepoOverlay_RetriesAbsentDoesNotOverride(t *testing.T) {
	dst := Config{Retries: intPtr(2)}
	src := Config{} // Retries is nil — not set in repo config
	result, keys := applyRepoOverlay(dst, src)
	if result.Retries == nil || *result.Retries != 2 {
		t.Errorf("Retries = %v, want pointer to 2 (absent repo config must not override global)", result.Retries)
	}
	for _, k := range keys {
		if k == "retries" {
			t.Errorf("overlay keys should not include 'retries' when src.Retries is nil, got %v", keys)
		}
	}
}

func intPtr(n int) *int { return &n }

// --- review_every key ---

func TestSet_ReviewEvery_Valid(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "review_every", "3")
	if err != nil {
		t.Fatalf("Set(review_every, 3): unexpected error: %v", err)
	}
	if updated.ReviewEvery == nil || *updated.ReviewEvery != 3 {
		t.Errorf("ReviewEvery = %v, want pointer to 3", updated.ReviewEvery)
	}
}

func TestSet_ReviewEvery_One(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "review_every", "1")
	if err != nil {
		t.Fatalf("Set(review_every, 1): unexpected error: %v", err)
	}
	if updated.ReviewEvery == nil || *updated.ReviewEvery != 1 {
		t.Errorf("ReviewEvery = %v, want pointer to 1", updated.ReviewEvery)
	}
}

func TestSet_ReviewEvery_Zero_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "review_every", "0")
	if err == nil {
		t.Fatal("expected error for review_every=0")
	}
}

func TestSet_ReviewEvery_Negative_ReturnsError(t *testing.T) {
	cfg := Config{}
	_, err := Set(cfg, "review_every", "-1")
	if err == nil {
		t.Fatal("expected error for negative review_every")
	}
}

func TestGet_ReviewEvery_Nil(t *testing.T) {
	cfg := Config{} // ReviewEvery is nil
	val, err := Get(cfg, "review_every")
	if err != nil {
		t.Fatalf("Get(review_every) on nil: unexpected error: %v", err)
	}
	if val != "1" {
		t.Errorf("Get(review_every) on nil = %q, want %q", val, "1")
	}
}

func TestGet_ReviewEvery_Set(t *testing.T) {
	cfg := Config{ReviewEvery: intPtr(4)}
	val, err := Get(cfg, "review_every")
	if err != nil {
		t.Fatalf("Get(review_every): unexpected error: %v", err)
	}
	if val != "4" {
		t.Errorf("Get(review_every) = %q, want %q", val, "4")
	}
}

func TestGet_ReviewEvery_RoundTrip(t *testing.T) {
	cfg := Config{}
	updated, err := Set(cfg, "review_every", "3")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := Get(updated, "review_every")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "3" {
		t.Errorf("round-trip: got %q, want %q", val, "3")
	}
}

func TestApplyRepoOverlay_ReviewEvery(t *testing.T) {
	dst := Config{}
	src := Config{ReviewEvery: intPtr(2)}
	result, keys := applyRepoOverlay(dst, src)
	if result.ReviewEvery == nil || *result.ReviewEvery != 2 {
		t.Errorf("ReviewEvery = %v, want pointer to 2", result.ReviewEvery)
	}
	found := false
	for _, k := range keys {
		if k == "review_every" {
			found = true
		}
	}
	if !found {
		t.Errorf("overlay keys = %v, want review_every to be present", keys)
	}
}

func TestApplyRepoOverlay_ReviewEveryAbsentDoesNotOverride(t *testing.T) {
	dst := Config{ReviewEvery: intPtr(3)}
	src := Config{} // ReviewEvery is nil
	result, keys := applyRepoOverlay(dst, src)
	if result.ReviewEvery == nil || *result.ReviewEvery != 3 {
		t.Errorf("ReviewEvery = %v, want pointer to 3", result.ReviewEvery)
	}
	for _, k := range keys {
		if k == "review_every" {
			t.Errorf("overlay keys should not include review_every when src.ReviewEvery is nil, got %v", keys)
		}
	}
}

func TestGet_NotifyWebhook(t *testing.T) {
	cfg := Config{NotifyWebhook: "https://hooks.slack.com/test"}
	val, err := Get(cfg, "notify_webhook")
	if err != nil {
		t.Fatalf("Get(notify_webhook): unexpected error: %v", err)
	}
	if val != "https://hooks.slack.com/test" {
		t.Errorf("Get(notify_webhook) = %q, want %q", val, "https://hooks.slack.com/test")
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

// --- MigrateReviewerAgent ---

func TestMigrateReviewerAgent(t *testing.T) {
	cfg := Config{ReviewerAgent: "path/to/agent.md"}
	MigrateReviewerAgent(&cfg)
	if cfg.Reviewers == nil {
		t.Fatal("Reviewers should not be nil after migration")
	}
	if cfg.Reviewers.General != "path/to/agent.md" {
		t.Errorf("Reviewers.General = %q, want %q", cfg.Reviewers.General, "path/to/agent.md")
	}
}

func TestMigrateNoOp(t *testing.T) {
	existing := &Reviewers{General: "other/agent.md"}
	cfg := Config{ReviewerAgent: "path/to/agent.md", Reviewers: existing}
	MigrateReviewerAgent(&cfg)
	if cfg.Reviewers.General != "other/agent.md" {
		t.Errorf("migration must not overwrite existing Reviewers: got %q", cfg.Reviewers.General)
	}
}

func TestEffectiveReviewersNilFallback(t *testing.T) {
	t.Parallel()
	cfg := Config{ReviewerAgent: "path/to/reviewer.md"}
	r := EffectiveReviewers(cfg)
	if r.General != "path/to/reviewer.md" {
		t.Errorf("EffectiveReviewers with nil Reviewers should use ReviewerAgent, got %q", r.General)
	}
}

func TestEffectiveReviewersUsesReviewersField(t *testing.T) {
	t.Parallel()
	cfg := Config{
		ReviewerAgent: "legacy.md",
		Reviewers:     &Reviewers{General: "new.md", Specialized: []string{"spec.md"}},
	}
	r := EffectiveReviewers(cfg)
	if r.General != "new.md" {
		t.Errorf("EffectiveReviewers should prefer Reviewers field, got %q", r.General)
	}
	if len(r.Specialized) != 1 || r.Specialized[0] != "spec.md" {
		t.Errorf("Specialized = %v, want [spec.md]", r.Specialized)
	}
}

func TestEffectiveReviewStrategyPartial(t *testing.T) {
	t.Parallel()
	cfg := Config{ReviewStrategy: &ReviewStrategy{Mode: "always"}}
	s := EffectiveReviewStrategy(cfg)
	if s.Mode != "always" {
		t.Errorf("Mode = %q, want always", s.Mode)
	}
	if s.GeneralEvery != 1 {
		t.Errorf("GeneralEvery = %d, want 1 (default)", s.GeneralEvery)
	}
	if s.SpecializedEvery != 3 {
		t.Errorf("SpecializedEvery = %d, want 3 (default)", s.SpecializedEvery)
	}
	if s.MajorityThreshold != 0.6 {
		t.Errorf("MajorityThreshold = %f, want 0.6 (default)", s.MajorityThreshold)
	}
}

func TestEffectiveReviewStrategyDefaults(t *testing.T) {
	cfg := Config{}
	s := EffectiveReviewStrategy(cfg)
	if s.Mode != "smart" {
		t.Errorf("Mode = %q, want %q", s.Mode, "smart")
	}
	if s.GeneralEvery != 1 {
		t.Errorf("GeneralEvery = %d, want 1", s.GeneralEvery)
	}
	if s.SpecializedEvery != 3 {
		t.Errorf("SpecializedEvery = %d, want 3", s.SpecializedEvery)
	}
	if s.MajorityThreshold != 0.6 {
		t.Errorf("MajorityThreshold = %f, want 0.6", s.MajorityThreshold)
	}
}
