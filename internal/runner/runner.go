package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const backendClaude = "claude"

type Result struct {
	Output    string
	Stderr    string
	Err       error  // Gotcha: non-nil when the binary could not be started — distinct from a non-zero exit code.
	ExitCode  int
	TimedOut  bool
	Cancelled bool
}

func Run(ctx context.Context, prompt string, timeoutSecs int, backend string) Result {
	binary := binaryFor(backend)
	args := []string{"-p", prompt, "--output-format", "text"}
	if backend == backendClaude {
		args = append(args, "--dangerously-skip-permissions")
	}
	return runArgs(ctx, binary, args, timeoutSecs)
}

// Invariant: delivers exactly one Result regardless of success, timeout, or cancellation.
func RunAsync(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		ch <- Run(ctx, prompt, timeoutSecs, backend)
	}()
	return ch
}

// Why: extracted so Run and RunStreamAsync share one execution path and tests can call it directly.
func runArgs(ctx context.Context, binary string, args []string, timeoutSecs int) Result {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	// Gotcha: exec.CommandContext sends SIGKILL only to the direct child; grandchild processes are orphaned.
	cmd := exec.CommandContext(timeoutCtx, binary, args...)
	cmd.Env = removeEnv(os.Environ(), "CLAUDECODE")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		// Why: parent context checked first so user cancellation takes priority over timeout.
		// Gotcha: only reached when cmd.Run errors, so a clean exit is never misreported as cancelled.
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
			// Why: start failure is a runner error, not agent output — set Err rather than ExitCode.
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

// Invariant: Result.Output is fully populated even when streaming to out.
func runArgsStream(ctx context.Context, binary string, args []string, timeoutSecs int, out io.Writer) Result {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, binary, args...)
	cmd.Env = removeEnv(os.Environ(), "CLAUDECODE")

	var stdout bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, out)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
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

// Invariant: delivers exactly one Result; Result.Output is fully populated regardless of streaming.
func RunStreamAsync(ctx context.Context, prompt string, timeoutSecs int, backend string, out io.Writer) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		binary := binaryFor(backend)
		args := []string{"-p", prompt, "--output-format", "text"}
		if backend == backendClaude {
			args = append(args, "--dangerously-skip-permissions")
		}
		ch <- runArgsStream(ctx, binary, args, timeoutSecs, out)
	}()
	return ch
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
