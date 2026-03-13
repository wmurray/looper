package runner

import (
	"context"
	"io"
	"strings"
)

var transientKeywords = []string{
	"rate limit",
	"429",
	"too many requests",
	"overloaded",
	"connection refused",
	"timeout",
	"no such host",
}

// RunFn is the function signature for async runner invocations. It allows
// RunWithRetry to be tested with a stub without spawning real subprocesses.
type RunFn func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result

// RetryWriter is satisfied by progress.Writer and allows RunWithRetry to record
// retry attempts without importing the progress package.
type RetryWriter interface {
	WriteRetry(phase string, attempt, maxRetries int, reason string) error
}

// RunAsyncFn returns a RunFn backed by RunAsync.
func RunAsyncFn() RunFn {
	return func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		return RunAsync(ctx, prompt, timeoutSecs, backend)
	}
}

// RunStreamAsyncFn returns a RunFn backed by RunStreamAsync writing to out.
func RunStreamAsyncFn(out io.Writer) RunFn {
	return func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		return RunStreamAsync(ctx, prompt, timeoutSecs, backend, out)
	}
}

// RunWithRetry calls fn and retries up to maxRetries times when isTransient
// detects a transient error. pw and warnFn may be nil. Non-transient failures
// and success return immediately. Retry count resets per call.
func RunWithRetry(ctx context.Context, fn RunFn, prompt string, timeoutSecs int, backend string, maxRetries int, phase string, pw RetryWriter, warnFn func(string, ...any)) Result {
	var result Result
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result = <-fn(ctx, prompt, timeoutSecs, backend)
		if result.ExitCode == 0 || result.Cancelled || result.TimedOut {
			return result
		}
		if !isTransient(result) {
			return result
		}
		if attempt < maxRetries {
			reason := strings.TrimSpace(result.Stderr)
			if pw != nil {
				_ = pw.WriteRetry(phase, attempt+1, maxRetries, reason)
			}
			if warnFn != nil {
				warnFn("Transient error on %s (attempt %d/%d): %s — retrying", phase, attempt+1, maxRetries, reason)
			}
		}
	}
	return result
}

func isTransient(result Result) bool {
	if result.ExitCode != 1 {
		return false
	}
	lower := strings.ToLower(result.Stderr)
	for _, kw := range transientKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
