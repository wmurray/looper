package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State persists loop progress after each completed cycle so looper resume can
// restart from the last completed cycle rather than cycle 1.
//
// State is serialized to {TICKET}_STATE.json in the working directory.
type State struct {
	Ticket            string          `json:"ticket"`
	PlanFile          string          `json:"plan_file"`
	CyclesTotal       int             `json:"cycles_total"`
	CycleCompleted    int             `json:"cycle_completed"`
	ThrashCount       int             `json:"thrash_count"`
	StuckCount        int             `json:"stuck_count"`
	PrevIssues        string          `json:"prev_issues"`
	StartedAt         time.Time       `json:"started_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ReviewerApprovals map[string]bool `json:"reviewer_approvals,omitempty"`
}

// Path returns the legacy state file path at the repo root.
// For ticket "IMP-6", it returns "IMP-6_STATE.json".
func Path(ticket string) string {
	return ticket + "_STATE.json"
}

// NewPath returns the canonical state file path under .looper/{ticket}/.
func NewPath(ticket string) string {
	return filepath.Join(".looper", ticket, ticket+"_STATE.json")
}

// Write marshals s to JSON and writes it atomically to NewPath(s.Ticket) using
// a temp file and rename to prevent corruption on failure.
func Write(s State) error {
	dest := NewPath(s.Ticket)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("state.Write: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state.Write: marshal: %w", err)
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("state.Write: write temp: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp) // Gotcha: best-effort cleanup; rename failure is the real error.
		return fmt.Errorf("state.Write: rename: %w", err)
	}
	return nil
}

// Read reads and unmarshals the state file for the given ticket.
// It tries NewPath first, then falls back to the legacy Path.
// Returns an error wrapping os.ErrNotExist if no file is found.
func Read(ticket string) (State, error) {
	for _, p := range []string{NewPath(ticket), Path(ticket)} {
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
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
	return State{}, fmt.Errorf("state.Read: %w", os.ErrNotExist)
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

// Delete removes the state file for the given ticket from both NewPath and legacy Path.
// It is a no-op if neither file exists, making it safe to call unconditionally.
func Delete(ticket string) error {
	for _, p := range []string{NewPath(ticket), Path(ticket)} {
		err := os.Remove(p)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
