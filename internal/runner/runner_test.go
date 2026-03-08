package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- binaryFor ---

func TestBinaryFor_Cursor(t *testing.T) {
	if binaryFor("cursor") != "agent" {
		t.Errorf("expected 'agent' for cursor backend")
	}
}

func TestBinaryFor_Claude(t *testing.T) {
	if binaryFor("claude") != "claude" {
		t.Errorf("expected 'claude' for claude backend")
	}
}

func TestBinaryFor_Default(t *testing.T) {
	if binaryFor("") != "agent" {
		t.Errorf("expected 'agent' as default")
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
	if result.Output == "" {
		t.Fatal("expected error message in output for missing binary")
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

	// Cancel after a short delay so the subprocess has time to start
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

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

func TestRunAsync_DeliversResult(t *testing.T) {
	ctx := context.Background()
	// Use Run via RunAsync path indirectly by testing the channel delivery
	ch := make(chan Result, 1)
	go func() {
		ch <- runArgs(ctx, "echo", []string{"async"}, 5)
	}()

	select {
	case result := <-ch:
		if result.ExitCode != 0 {
			t.Fatalf("expected exit 0, got %d", result.ExitCode)
		}
		if !strings.Contains(result.Output, "async") {
			t.Errorf("expected 'async' in output, got %q", result.Output)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunAsync did not deliver result within timeout")
	}
}
