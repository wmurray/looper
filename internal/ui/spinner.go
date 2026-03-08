package ui

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const clearLine = "\r\033[K"

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated spinner with elapsed time while a phase runs.
// When stopped, it replaces itself with a checkmark and final elapsed time.
// All output is written to stderr so that stdout remains clean for piped use.
//
// Start and Stop/Abort must each be called at most once. Use finishOnce and
// startOnce to enforce this. The stop channel is only used in TTY mode; in
// non-TTY mode only done is closed (by Start) and stop is left unused.
// Calling Stop/Abort before Start is safe and is a no-op.
type Spinner struct {
	msg        string
	start      time.Time
	stop       chan struct{} // closed by finish; only used when tty == true
	done       chan struct{} // closed by the TTY goroutine or by Start in non-TTY mode
	tty        bool
	started    bool // set inside startOnce; guards finish against pre-Start calls
	startOnce  sync.Once
	finishOnce sync.Once
}

// NewSpinner creates a spinner with the given message.
func NewSpinner(msg string) *Spinner {
	return &Spinner{
		msg:  msg,
		stop: make(chan struct{}),
		done: make(chan struct{}),
		tty:  isTTY(),
	}
}

// Start begins the spinner. It is safe to call at most once; subsequent calls are no-ops.
func (s *Spinner) Start() {
	s.startOnce.Do(func() {
		s.start = time.Now()
		s.started = true

		if !s.tty {
			fmt.Fprintf(os.Stderr, "%s%s%s\n", Gray, s.msg, Reset)
			close(s.done)
			return
		}

		go func() {
			defer close(s.done)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			i := 0
			for {
				select {
				case <-s.stop:
					return
				case <-ticker.C:
					elapsed := int(time.Since(s.start).Seconds())
					frame := spinFrames[i%len(spinFrames)]
					fmt.Fprintf(os.Stderr, clearLine+Gray+s.msg+"  "+Cyan+frame+"  "+Gray+"%ds"+Reset, elapsed)
					i++
				}
			}
		}()
	})
}

// Stop halts the spinner and prints the final elapsed time with a green checkmark.
func (s *Spinner) Stop() {
	s.finish(Green + "✓")
}

// Abort halts the spinner and prints the final elapsed time with a red ✗.
func (s *Spinner) Abort() {
	s.finish(Red + "✗")
}

func (s *Spinner) finish(marker string) {
	s.finishOnce.Do(func() {
		if !s.started {
			return // Start was never called; nothing to clean up
		}
		elapsed := int(time.Since(s.start).Seconds())
		if s.tty {
			close(s.stop)
			<-s.done
			fmt.Fprintf(os.Stderr, clearLine+Gray+s.msg+"  "+marker+Gray+"  %ds"+Reset+"\n", elapsed)
		} else {
			fmt.Fprintf(os.Stderr, "%s %ds\n", s.msg, elapsed)
		}
	})
}

// isTTY returns true if stderr is an interactive terminal.
func isTTY() bool {
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
