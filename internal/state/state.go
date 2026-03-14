package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// State persists loop progress after each completed cycle so looper resume can
// restart from the last completed cycle rather than cycle 1.
type State struct {
	Ticket         string    `json:"ticket"`
	PlanFile       string    `json:"plan_file"`
	CyclesTotal    int       `json:"cycles_total"`
	CycleCompleted int       `json:"cycle_completed"`
	ThrashCount    int       `json:"thrash_count"`
	StuckCount     int       `json:"stuck_count"`
	PrevIssues     string    `json:"prev_issues"`
	StartedAt      time.Time `json:"started_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Path returns the state file path for the given ticket.
func Path(ticket string) string {
	return ticket + "_STATE.json"
}

// Write marshals s to JSON and writes it atomically (temp file + rename).
func Write(s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state.Write: marshal: %w", err)
	}
	tmp := Path(s.Ticket) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("state.Write: write temp: %w", err)
	}
	if err := os.Rename(tmp, Path(s.Ticket)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("state.Write: rename: %w", err)
	}
	return nil
}

// Read reads and unmarshals the state file for ticket.
// Returns an error wrapping os.ErrNotExist if the file is absent.
func Read(ticket string) (State, error) {
	data, err := os.ReadFile(Path(ticket))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, fmt.Errorf("state.Read: %w", os.ErrNotExist)
		}
		return State{}, fmt.Errorf("state.Read: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("state.Read: unmarshal: %w", err)
	}
	return s, nil
}

// Delete removes the state file for ticket. No-op if the file is missing.
func Delete(ticket string) error {
	err := os.Remove(Path(ticket))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
