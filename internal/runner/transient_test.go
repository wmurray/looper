package runner

import (
	"context"
	"testing"
)

func TestIsTransient(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		result    Result
		wantTrue  bool
	}{
		{"rate limit keyword", Result{ExitCode: 1, Stderr: "error: rate limit exceeded"}, true},
		{"429 keyword", Result{ExitCode: 1, Stderr: "HTTP 429 Too Many Requests"}, true},
		{"too many requests", Result{ExitCode: 1, Stderr: "too many requests, please back off"}, true},
		{"overloaded", Result{ExitCode: 1, Stderr: "Claude is currently overloaded"}, true},
		{"connection refused", Result{ExitCode: 1, Stderr: "dial tcp: connection refused"}, true},
		{"timeout in stderr", Result{ExitCode: 1, Stderr: "request timeout"}, true},
		{"no such host", Result{ExitCode: 1, Stderr: "no such host: api.anthropic.com"}, true},
		{"case insensitive", Result{ExitCode: 1, Stderr: "RATE LIMIT hit"}, true},
		{"non-matching stderr", Result{ExitCode: 1, Stderr: "syntax error: unexpected token"}, false},
		{"exit code not one", Result{ExitCode: 2, Stderr: "rate limit exceeded"}, false},
		{"exit code zero", Result{ExitCode: 0, Stderr: "rate limit exceeded"}, false},
		{"empty stderr", Result{ExitCode: 1, Stderr: ""}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isTransient(tc.result)
			if got != tc.wantTrue {
				t.Errorf("isTransient(%+v) = %v, want %v", tc.result, got, tc.wantTrue)
			}
		})
	}
}

// stubRunner returns a RunFn that cycles through the given results.
func stubRunner(results []Result) RunFn {
	i := 0
	return func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		ch := make(chan Result, 1)
		if i < len(results) {
			ch <- results[i]
			i++
		} else {
			ch <- results[len(results)-1]
		}
		return ch
	}
}

func TestRunWithRetry_AllRetriesFail_ReturnsLastResult(t *testing.T) {
	t.Parallel()
	transient := Result{ExitCode: 1, Stderr: "rate limit exceeded"}
	fn := stubRunner([]Result{transient, transient, transient})
	got := RunWithRetry(context.Background(), fn, "prompt", 5, "claude", 2, "execution", nil, nil)
	if got.ExitCode != 1 {
		t.Errorf("expected ExitCode 1, got %d", got.ExitCode)
	}
}

func TestRunWithRetry_TransientThenSuccess_ReturnsSuccess(t *testing.T) {
	t.Parallel()
	transient := Result{ExitCode: 1, Stderr: "rate limit exceeded"}
	success := Result{ExitCode: 0, Output: "done"}
	fn := stubRunner([]Result{transient, success})
	got := RunWithRetry(context.Background(), fn, "prompt", 5, "claude", 3, "execution", nil, nil)
	if got.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", got.ExitCode)
	}
	if got.Output != "done" {
		t.Errorf("expected output 'done', got %q", got.Output)
	}
}

func TestRunWithRetry_NonTransientNeverRetries(t *testing.T) {
	t.Parallel()
	calls := 0
	fn := func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		calls++
		ch := make(chan Result, 1)
		ch <- Result{ExitCode: 1, Stderr: "syntax error"}
		return ch
	}
	RunWithRetry(context.Background(), fn, "prompt", 5, "claude", 3, "execution", nil, nil)
	if calls != 1 {
		t.Errorf("expected exactly 1 call for non-transient error, got %d", calls)
	}
}

func TestRunWithRetry_Success_NeverRetries(t *testing.T) {
	t.Parallel()
	calls := 0
	fn := func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		calls++
		ch := make(chan Result, 1)
		ch <- Result{ExitCode: 0, Output: "ok"}
		return ch
	}
	RunWithRetry(context.Background(), fn, "prompt", 5, "claude", 3, "execution", nil, nil)
	if calls != 1 {
		t.Errorf("expected exactly 1 call on success, got %d", calls)
	}
}

func TestRunWithRetry_ZeroRetries_NoRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	fn := func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		calls++
		ch := make(chan Result, 1)
		ch <- Result{ExitCode: 1, Stderr: "rate limit exceeded"}
		return ch
	}
	RunWithRetry(context.Background(), fn, "prompt", 5, "claude", 0, "execution", nil, nil)
	if calls != 1 {
		t.Errorf("expected exactly 1 call with retries=0, got %d", calls)
	}
}

func TestRunWithRetry_WarnFnCalledOnTransient(t *testing.T) {
	t.Parallel()
	transient := Result{ExitCode: 1, Stderr: "rate limit exceeded"}
	success := Result{ExitCode: 0}
	fn := stubRunner([]Result{transient, success})
	var warnCalled bool
	warnFn := func(format string, args ...any) {
		warnCalled = true
	}
	RunWithRetry(context.Background(), fn, "prompt", 5, "claude", 1, "execution", nil, warnFn)
	if !warnCalled {
		t.Error("expected warnFn to be called on transient error")
	}
}

func TestRunWithRetry_WarnFnNotCalledOnNonTransient(t *testing.T) {
	t.Parallel()
	fn := stubRunner([]Result{{ExitCode: 1, Stderr: "syntax error"}})
	warnCalled := false
	warnFn := func(format string, args ...any) {
		warnCalled = true
	}
	RunWithRetry(context.Background(), fn, "prompt", 5, "claude", 3, "execution", nil, warnFn)
	if warnCalled {
		t.Error("expected warnFn not to be called for non-transient error")
	}
}
