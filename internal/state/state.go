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
//
// State is serialized to {TICKET}_STATE.json in the working directory.
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
// For ticket "IMP-6", it returns "IMP-6_STATE.json".
func Path(ticket string) string {
	return ticket + "_STATE.json"
}

// Write marshals s to JSON and writes it atomically to the state file using
// a temp file and rename to prevent corruption on failure.
// The state file path is determined by s.Ticket via Path().
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
		_ = os.Remove(tmp) // Gotcha: best-effort cleanup; rename failure is the real error.
		return fmt.Errorf("state.Write: rename: %w", err)
	}
	return nil
}

// Read reads and unmarshals the state file for the given ticket.
// It returns an error wrapping os.ErrNotExist if the file is not found,
// allowing callers to distinguish missing state from I/O errors.
func Read(ticket string) (State, error) {
	data, err := os.ReadFile(Path(ticket))
	if err != nil {
		return State{}, fmt.Errorf("state.Read: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("state.Read: unmarshal: %w", err)
	}
	if err := s.Validate(); err != nil {
		return State{}, fmt.Errorf("state.Read: validate: %w", err)
	}
	return s, nil
}

// Validate checks the state for logical consistency.
// It returns an error if CycleCompleted > CyclesTotal or if required fields are missing.
func (s State) Validate() error {
	if s.Ticket == "" {
		return errors.New("state.Validate: ticket is required")
	}
	if s.CyclesTotal <= 0 {
		return fmt.Errorf("state.Validate: cycles_total must be positive, got %d", s.CyclesTotal)
	}
	if s.CycleCompleted > s.CyclesTotal {
		return fmt.Errorf("state.Validate: cycle_completed (%d) exceeds cycles_total (%d)",
			s.CycleCompleted, s.CyclesTotal)
	}
	return nil
}

// Delete removes the state file for the given ticket.
// It is a no-op if the file does not exist, making it safe to call unconditionally.
func Delete(ticket string) error {
	err := os.Remove(Path(ticket))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
