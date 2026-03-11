package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const backendClaude = "claude"

// Result holds the output and exit code from an agent run.
type Result struct {
	Output    string // stdout from the agent
	Stderr    string // stderr from the agent process
	Err       error  // non-nil when the agent binary could not be started (distinct from a non-zero exit)
	ExitCode  int
	TimedOut  bool
	Cancelled bool
}

// Run executes the agent CLI with the given prompt.
// ctx is used for cancellation (e.g. Ctrl+C); a separate timeout is applied on top.
// backend is "cursor" (calls `agent`) or "claude" (calls `claude`).
func Run(ctx context.Context, prompt string, timeoutSecs int, backend string) Result {
	binary := binaryFor(backend)
	args := []string{"-p", prompt, "--output-format", "text"}
	if backend == backendClaude {
		args = append(args, "--dangerously-skip-permissions")
	}
	return runArgs(ctx, binary, args, timeoutSecs)
}

// RunAsync executes the agent CLI in a goroutine and returns a channel that
// delivers exactly one Result when the agent completes, times out, or is cancelled.
func RunAsync(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		ch <- Run(ctx, prompt, timeoutSecs, backend)
	}()
	return ch
}

// runArgs is the core execution primitive, extracted for testability.
func runArgs(ctx context.Context, binary string, args []string, timeoutSecs int) Result {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	// Note: on timeout/cancel, exec.CommandContext sends SIGKILL only to the
	// direct child process. Any grandchild processes spawned by the agent will
	// be orphaned. A process-group kill would be needed to clean those up.
	cmd := exec.CommandContext(timeoutCtx, binary, args...)
	cmd.Env = removeEnv(os.Environ(), "CLAUDECODE")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		// User cancellation takes priority over timeout — check parent context first.
		// These checks are only relevant when cmd.Run returned an error, so a clean
		// exit (code 0) is never misreported as cancelled even if both fire.
		if errors.Is(ctx.Err(), context.Canceled) {
			return Result{Output: stdout.String(), Stderr: stderr.String(), ExitCode: 130, Cancelled: true}
		}
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			return Result{Output: stdout.String(), Stderr: stderr.String(), ExitCode: 124, TimedOut: true}
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// Binary could not be started — this is a runner error, not agent output.
			return Result{
				Output:   stdout.String(),
				Stderr:   stderr.String(),
				Err:      fmt.Errorf("error running %s: %w", binary, err),
				ExitCode: 1,
			}
		}
	}

	return Result{Output: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
}

func removeEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

func binaryFor(backend string) string {
	switch backend {
	case backendClaude:
		return backendClaude
	default:
		return "agent"
	}
}
