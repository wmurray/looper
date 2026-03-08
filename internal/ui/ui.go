package ui

import (
	"fmt"
	"os"
)

const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Gray   = "\033[90m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
)

// Header prints bold white — used for startup info.
func Header(format string, args ...any) {
	fmt.Printf(Bold+format+Reset+"\n", args...)
}

// Iteration prints bold cyan — used for the iteration banner.
func Iteration(format string, args ...any) {
	fmt.Printf(Bold+Cyan+format+Reset+"\n", args...)
}

// Phase prints gray — used for timestamped phase progress lines.
func Phase(format string, args ...any) {
	fmt.Printf(Gray+format+Reset+"\n", args...)
}

// Warn prints bold yellow with ⚠ prefix — guard warnings (threshold not yet crossed).
func Warn(format string, args ...any) {
	fmt.Printf(Bold+Yellow+"⚠  "+format+Reset+"\n", args...)
}

// Alert prints bold red with ⚠ prefix — guard triggers, timeouts, aborts.
func Alert(format string, args ...any) {
	fmt.Printf(Bold+Red+"⚠  "+format+Reset+"\n", args...)
}

// Error prints red to stderr — agent failures and system errors.
func Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, Red+format+Reset+"\n", args...)
}

// Success prints bold green — completion message.
func Success(format string, args ...any) {
	fmt.Printf(Bold+Green+format+Reset+"\n", args...)
}
