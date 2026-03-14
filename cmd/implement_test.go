package cmd

import "testing"

func TestLastNRuns(t *testing.T) {
	header := "# Progress\n\n---\n"
	run1 := "## RUN 1\nfirst\n"
	run2 := "## RUN 2\nsecond\n"
	run3 := "## RUN 3\nthird\n"
	full3 := header + run1 + run2 + run3

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
			name:    "n greater than runs returns full content",
			content: full3,
			n:       10,
			want:    full3,
		},
		{
			name:    "n=1 with 3 runs returns header + last run",
			content: full3,
			n:       1,
			want:    header + run3,
		},
		{
			name:    "n=2 with 3 runs returns header + last 2 runs",
			content: full3,
			n:       2,
			want:    header + run2 + run3,
		},
		{
			name:    "no runs present returns full content",
			content: header,
			n:       2,
			want:    header,
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
