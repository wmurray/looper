package ui

import (
	"fmt"
	"os"
	"time"
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated spinner with elapsed time while a phase runs.
// When stopped, it replaces itself with a checkmark and final elapsed time.
type Spinner struct {
	msg   string
	start time.Time
	stop  chan struct{}
	done  chan struct{}
	tty   bool
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

// Start begins the spinner in a background goroutine.
func (s *Spinner) Start() {
	s.start = time.Now()

	if !s.tty {
		fmt.Printf("%s%s%s\n", Gray, s.msg, Reset)
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
				fmt.Printf("\r\033[K"+Gray+s.msg+"  "+Cyan+frame+"  "+Gray+"%ds"+Reset, elapsed)
				i++
			}
		}
	}()
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
	if s.tty {
		close(s.stop)
		<-s.done
		elapsed := int(time.Since(s.start).Seconds())
		fmt.Printf("\r\033[K"+Gray+s.msg+"  "+marker+Gray+"  %ds"+Reset+"\n", elapsed)
	}
}

// isTTY returns true if stdout is an interactive terminal.
func isTTY() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
