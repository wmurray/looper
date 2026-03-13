package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// desktopCommand returns the command name and arguments for a desktop notification on the given OS.
// Returns empty strings/slice for unsupported platforms.
func desktopCommand(goos, title, body string) (string, []string) {
	switch goos {
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		return "osascript", []string{"-e", script}
	case "linux":
		return "notify-send", []string{title, body}
	default:
		return "", nil
	}
}

func sendDesktop(title, body string) error {
	name, args := desktopCommand(runtime.GOOS, title, body)
	if name == "" {
		return nil
	}
	return exec.Command(name, args...).Run()
}
