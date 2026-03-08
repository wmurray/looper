package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- binaryFor ---

func TestBinaryFor(t *testing.T) {
	t.Parallel()
	cases := []struct{ backend, want string }{
		{"cursor", "agent"},
		{"claude", "claude"},
		{"", "agent"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.backend, func(t *testing.T) {
			t.Parallel()
			if got := binaryFor(tc.backend); got != tc.want {
				t.Errorf("binaryFor(%q) = %q, want %q", tc.backend, got, tc.want)
			}
		})
	}
}

// --- runArgs: success ---

func TestRunArgs_Success(t *testing.T) {
	ctx := context.Background()
	result := runArgs(ctx, "echo", []string{"hello"}, 5)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.TimedOut || result.Cancelled {
		t.Fatal("expected clean success result")
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", result.Output)
	}
}

func TestRunArgs_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	result := runArgs(ctx, "false", []string{}, 5)
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code from 'false'")
	}
	if result.TimedOut || result.Cancelled {
		t.Fatal("expected plain failure, not timeout or cancellation")
	}
}

func TestRunArgs_BinaryNotFound(t *testing.T) {
	ctx := context.Background()
	result := runArgs(ctx, "binary_that_does_not_exist_xyz", []string{}, 5)
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for missing binary")
	}
	if result.Err == nil {
		t.Fatal("expected Err to be set for missing binary")
	}
}

// --- runArgs: timeout ---

func TestRunArgs_Timeout(t *testing.T) {
	ctx := context.Background()
	result := runArgs(ctx, "sleep", []string{"10"}, 1)
	if !result.TimedOut {
		t.Fatal("expected TimedOut to be true")
	}
	if result.ExitCode != 124 {
		t.Errorf("expected exit code 124 for timeout, got %d", result.ExitCode)
	}
	if result.Cancelled {
		t.Fatal("Cancelled should be false on timeout")
	}
}

// --- runArgs: cancellation ---

func TestRunArgs_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Use time.AfterFunc so the cancel fires after the subprocess is running,
	// without relying on a fixed sleep that may be too short on loaded CI.
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()

	result := runArgs(ctx, "sleep", []string{"10"}, 30)
	if !result.Cancelled {
		t.Fatal("expected Cancelled to be true")
	}
	if result.ExitCode != 130 {
		t.Errorf("expected exit code 130 for cancellation, got %d", result.ExitCode)
	}
	if result.TimedOut {
		t.Fatal("TimedOut should be false on cancellation")
	}
}

func TestRunArgs_CancelledImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before even starting

	result := runArgs(ctx, "sleep", []string{"10"}, 30)
	if !result.Cancelled {
		t.Fatal("expected Cancelled to be true for pre-cancelled context")
	}
}

// --- RunAsync ---

func TestRunAsync_ReturnsChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so no real agent binary is needed
	ch := RunAsync(ctx, "irrelevant", 5, "cursor")
	select {
	case result := <-ch:
		if !result.Cancelled {
			t.Errorf("expected Cancelled result for pre-cancelled context, got %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunAsync did not deliver result within timeout")
	}
}
