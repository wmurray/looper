package runlog

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const logsFile = "runs.jsonl"

// RunEntry records the outcome of one looper implement invocation.
type RunEntry struct {
	Ticket          string   `json:"ticket"`
	StartedAt       string   `json:"started_at"`
	FinishedAt      string   `json:"finished_at"`
	Outcome         string   `json:"outcome"`
	CyclesUsed      int      `json:"cycles_used"`
	CyclesMax       int      `json:"cycles_max"`
	GuardEvents     []string `json:"guard_events"`
	LastReviewerMsg string   `json:"last_reviewer_msg"`
}

// Append serializes e as a single JSON line and appends it to runs.jsonl.
func Append(e RunEntry) error {
	dir, err := dataDir()
	if err != nil {
		return fmt.Errorf("runlog.Append: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runlog.Append: mkdir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, logsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("runlog.Append: open: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("runlog.Append: marshal: %w", err)
	}
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

// ReadAll reads all entries from runs.jsonl.
// Returns an empty slice (no error) when the file does not exist.
func ReadAll() ([]RunEntry, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, fmt.Errorf("runlog.ReadAll: %w", err)
	}
	f, err := os.Open(filepath.Join(dir, logsFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []RunEntry{}, nil
		}
		return nil, fmt.Errorf("runlog.ReadAll: open: %w", err)
	}
	defer f.Close()

	var entries []RunEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e RunEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("runlog.ReadAll: unmarshal line: %w", err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("runlog.ReadAll: scan: %w", err)
	}
	return entries, nil
}

func dataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "looper"), nil
}
