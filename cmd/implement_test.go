package cmd

import (
	"testing"

	"github.com/willmurray/looper/internal/progress"
)

func TestLastNRuns(t *testing.T) {
	sep := progress.RunSeparator
	header := "# Progress\n\n---\n"
	run1 := "1\nfirst\n"
	run2 := "2\nsecond\n"
	run3 := "3\nthird\n"
	// Construct fixtures using the real separator so tests stay in sync with
	// progress.BeginRun's format. If RunSeparator changes, these break loudly.
	full3 := header + sep + run1 + sep + run2 + sep + run3

	tests := []struct {
		name    string
		content string
		n       int
		want    string
	}{
		{
			name:    "n=0 returns full content",
			content: full3,
			n:       0,
			want:    full3,
		},
		{
			name:    "negative n returns full content",
			content: full3,
			n:       -1,
			want:    full3,
		},
		{
			name:    "n greater than runs returns full content",
			content: full3,
			n:       10,
			want:    full3,
		},
		{
			name:    "n equal to runs returns full content",
			content: full3,
			n:       3,
			want:    full3,
		},
		{
			name:    "n=1 with 3 runs returns header + last run",
			content: full3,
			n:       1,
			want:    header + sep + run3,
		},
		{
			name:    "n=2 with 3 runs returns header + last 2 runs",
			content: full3,
			n:       2,
			want:    header + sep + run2 + sep + run3,
		},
		{
			name:    "no runs present returns full content",
			content: header,
			n:       2,
			want:    header,
		},
		{
			name:    "empty content returns empty",
			content: "",
			n:       2,
			want:    "",
		},
		{
			name:    "single run n=1 returns full content",
			content: header + sep + run1,
			n:       1,
			want:    header + sep + run1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastNRuns(tt.content, tt.n)
			if got != tt.want {
				t.Errorf("\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}
