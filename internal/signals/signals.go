package signals

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// WithInterrupt returns a context that is cancelled when SIGINT or SIGTERM is
// received. The returned cancel must be called to release resources (use defer).
//
// The goroutine exits via ctx.Done() when the caller's defer cancel() fires on
// normal completion, so it does not leak in the happy path.
func WithInterrupt(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			signal.Stop(sigCh)
			cancel()
		case <-ctx.Done():
			signal.Stop(sigCh)
		}
	}()
	return ctx, cancel
}
