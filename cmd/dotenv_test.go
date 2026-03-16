package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_StripsDoubleQuotes(t *testing.T) {
	f := writeTempDotEnv(t, `DOTENV_DQ_KEY="quoted_value"`)
	t.Cleanup(func() { os.Unsetenv("DOTENV_DQ_KEY") })
	loadDotEnv(f)
	if got := os.Getenv("DOTENV_DQ_KEY"); got != "quoted_value" {
		t.Errorf("got %q, want %q", got, "quoted_value")
	}
}

func TestLoadDotEnv_StripsSingleQuotes(t *testing.T) {
	f := writeTempDotEnv(t, `DOTENV_SQ_KEY='single_value'`)
	t.Cleanup(func() { os.Unsetenv("DOTENV_SQ_KEY") })
	loadDotEnv(f)
	if got := os.Getenv("DOTENV_SQ_KEY"); got != "single_value" {
		t.Errorf("got %q, want %q", got, "single_value")
	}
}

func TestLoadDotEnv_LoadsKeyValue(t *testing.T) {
	f := writeTempDotEnv(t, "DOTENV_PLAIN_KEY=plain_value")
	t.Cleanup(func() { os.Unsetenv("DOTENV_PLAIN_KEY") })
	loadDotEnv(f)
	if got := os.Getenv("DOTENV_PLAIN_KEY"); got != "plain_value" {
		t.Errorf("got %q, want %q", got, "plain_value")
	}
}

func TestLoadDotEnv_EnvVarWinsOverDotEnv(t *testing.T) {
	t.Setenv("DOTENV_WINS_KEY", "from_env")
	f := writeTempDotEnv(t, "DOTENV_WINS_KEY=from_file")
	loadDotEnv(f)
	if got := os.Getenv("DOTENV_WINS_KEY"); got != "from_env" {
		t.Errorf("got %q, want env value %q", got, "from_env")
	}
}

// Invariant: an explicit empty env var must prevent the .env file from overwriting it.
func TestLoadDotEnv_EmptyEnvVarPreventsOverride(t *testing.T) {
	t.Setenv("DOTENV_EMPTY_KEY", "")
	f := writeTempDotEnv(t, "DOTENV_EMPTY_KEY=from_file")
	loadDotEnv(f)
	if got := os.Getenv("DOTENV_EMPTY_KEY"); got != "" {
		t.Errorf("got %q, want empty (env presence must win)", got)
	}
}

func TestLoadDotEnv_MissingFileIsNoOp(t *testing.T) {
	t.Cleanup(func() { os.Unsetenv("DOTENV_MISSING_KEY") })
	loadDotEnv(filepath.Join(t.TempDir(), "no_such.env"))
	if got := os.Getenv("DOTENV_MISSING_KEY"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestLoadDotEnv_SkipsCommentsAndBlanks(t *testing.T) {
	f := writeTempDotEnv(t, "# comment\n\nDOTENV_COMMENT_KEY=value\n   \n# another")
	t.Cleanup(func() { os.Unsetenv("DOTENV_COMMENT_KEY") })
	loadDotEnv(f)
	if got := os.Getenv("DOTENV_COMMENT_KEY"); got != "value" {
		t.Errorf("got %q, want %q", got, "value")
	}
}

func TestLoadDotEnv_SkipsLinesWithoutEquals(t *testing.T) {
	f := writeTempDotEnv(t, "NOEQUALSSIGN")
	t.Cleanup(func() { os.Unsetenv("NOEQUALSSIGN") })
	loadDotEnv(f)
	if got := os.Getenv("NOEQUALSSIGN"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestLoadDotEnv_PermissionErrorIsReported(t *testing.T) {
	f := writeTempDotEnv(t, "DOTENV_PERM_KEY=value")
	if err := os.Chmod(f, 0000); err != nil {
		t.Skip("cannot set file permissions (may be root or unsupported fs)")
	}
	t.Cleanup(func() { os.Chmod(f, 0600) }) //nolint:errcheck

	var warned string
	loadDotEnvWithWarn(f, func(msg string) { warned = msg })
	if warned == "" {
		t.Error("expected a warning for unreadable .env, got none")
	}
}

func writeTempDotEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%s\n", content)), 0600); err != nil {
		t.Fatalf("writeTempDotEnv: %v", err)
	}
	return path
}
