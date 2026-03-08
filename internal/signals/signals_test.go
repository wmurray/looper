package signals

import (
	"context"
	"testing"
	"time"
)

func TestWithInterrupt_CancelPropagates(t *testing.T) {
	t.Parallel()
	ctx, cancel := WithInterrupt(context.Background())
	defer cancel()

	cancel()
	select {
	case <-ctx.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled after cancel() was called")
	}
}

func TestWithInterrupt_ParentCancelPropagates(t *testing.T) {
	t.Parallel()
	parent, parentCancel := context.WithCancel(context.Background())
	ctx, cancel := WithInterrupt(parent)
	defer cancel()

	parentCancel()
	select {
	case <-ctx.Done():
		// expected: child context cancelled when parent is cancelled
	case <-time.After(time.Second):
		t.Fatal("child context not cancelled when parent was cancelled")
	}
}
