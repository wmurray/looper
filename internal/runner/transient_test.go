package runner

import (
	"context"
	"testing"
)

func TestIsTransient_RateLimitKeyword(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "error: rate limit exceeded"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for rate limit stderr")
	}
}

func TestIsTransient_429Keyword(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "HTTP 429 Too Many Requests"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for 429 in stderr")
	}
}

func TestIsTransient_TooManyRequests(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "too many requests, please back off"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for too many requests")
	}
}

func TestIsTransient_Overloaded(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "Claude is currently overloaded"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for overloaded")
	}
}

func TestIsTransient_ConnectionRefused(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "dial tcp: connection refused"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for connection refused")
	}
}

func TestIsTransient_Timeout(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "request timeout"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for timeout in stderr")
	}
}

func TestIsTransient_NoSuchHost(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "no such host: api.anthropic.com"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for no such host")
	}
}

func TestIsTransient_CaseInsensitive(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "RATE LIMIT hit"}
	if !isTransient(r) {
		t.Error("expected isTransient=true for uppercase rate limit")
	}
}

func TestIsTransient_NonMatchingStderr(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: "syntax error: unexpected token"}
	if isTransient(r) {
		t.Error("expected isTransient=false for non-matching stderr")
	}
}

func TestIsTransient_ExitCodeNotOne(t *testing.T) {
	r := Result{ExitCode: 2, Stderr: "rate limit exceeded"}
	if isTransient(r) {
		t.Error("expected isTransient=false when ExitCode != 1")
	}
}

func TestIsTransient_ExitCodeZero(t *testing.T) {
	r := Result{ExitCode: 0, Stderr: "rate limit exceeded"}
	if isTransient(r) {
		t.Error("expected isTransient=false for ExitCode 0")
	}
}

func TestIsTransient_EmptyStderr(t *testing.T) {
	r := Result{ExitCode: 1, Stderr: ""}
	if isTransient(r) {
		t.Error("expected isTransient=false for empty stderr")
	}
}

// --- runWithRetry ---

// stubRunner returns a function matching RunFn that cycles through the given results.
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
	transient := Result{ExitCode: 1, Stderr: "rate limit exceeded"}
	fn := stubRunner([]Result{transient, transient, transient})
	got := runWithRetry(context.Background(), fn, "prompt", 5, "claude", 2, "execution", nil)
	if got.ExitCode != 1 {
		t.Errorf("expected ExitCode 1, got %d", got.ExitCode)
	}
}

func TestRunWithRetry_TransientThenSuccess_ReturnsSuccess(t *testing.T) {
	transient := Result{ExitCode: 1, Stderr: "rate limit exceeded"}
	success := Result{ExitCode: 0, Output: "done"}
	fn := stubRunner([]Result{transient, success})
	got := runWithRetry(context.Background(), fn, "prompt", 5, "claude", 3, "execution", nil)
	if got.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", got.ExitCode)
	}
	if got.Output != "done" {
		t.Errorf("expected output 'done', got %q", got.Output)
	}
}

func TestRunWithRetry_NonTransientNeverRetries(t *testing.T) {
	calls := 0
	fn := func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		calls++
		ch := make(chan Result, 1)
		ch <- Result{ExitCode: 1, Stderr: "syntax error"}
		return ch
	}
	runWithRetry(context.Background(), fn, "prompt", 5, "claude", 3, "execution", nil)
	if calls != 1 {
		t.Errorf("expected exactly 1 call for non-transient error, got %d", calls)
	}
}

func TestRunWithRetry_Success_NeverRetries(t *testing.T) {
	calls := 0
	fn := func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		calls++
		ch := make(chan Result, 1)
		ch <- Result{ExitCode: 0, Output: "ok"}
		return ch
	}
	runWithRetry(context.Background(), fn, "prompt", 5, "claude", 3, "execution", nil)
	if calls != 1 {
		t.Errorf("expected exactly 1 call on success, got %d", calls)
	}
}

func TestRunWithRetry_ZeroRetries_NoRetry(t *testing.T) {
	calls := 0
	fn := func(ctx context.Context, prompt string, timeoutSecs int, backend string) <-chan Result {
		calls++
		ch := make(chan Result, 1)
		ch <- Result{ExitCode: 1, Stderr: "rate limit exceeded"}
		return ch
	}
	runWithRetry(context.Background(), fn, "prompt", 5, "claude", 0, "execution", nil)
	if calls != 1 {
		t.Errorf("expected exactly 1 call with retries=0, got %d", calls)
	}
}
