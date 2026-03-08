package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Result holds the output and exit code from an agent run.
type Result struct {
	Output    string
	ExitCode  int
	TimedOut  bool
	Cancelled bool
}

// Run executes the agent CLI with the given prompt.
// ctx is used for cancellation (e.g. Ctrl+C); a separate timeout is applied on top.
// backend is "cursor" (calls `agent`) or "claude" (calls `claude`).
func Run(ctx context.Context, prompt string, timeoutSecs int, backend string) Result {
	binary := binaryFor(backend)
	return runArgs(ctx, binary, []string{"-p", prompt, "--output-format", "text"}, timeoutSecs)
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

	cmd := exec.CommandContext(timeoutCtx, binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// User cancellation takes priority over timeout check
	if ctx.Err() == context.Canceled {
		return Result{Output: stdout.String(), ExitCode: 130, Cancelled: true}
	}

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return Result{Output: stdout.String(), ExitCode: 124, TimedOut: true}
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return Result{
				Output:   fmt.Sprintf("Error running %s: %v\nStderr: %s", binary, err, stderr.String()),
				ExitCode: 1,
			}
		}
	}

	return Result{Output: stdout.String(), ExitCode: exitCode}
}

func binaryFor(backend string) string {
	switch backend {
	case "claude":
		return "claude"
	default:
		return "agent"
	}
}
