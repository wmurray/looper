package detect_test

import (
	"testing"

	"github.com/willmurray/looper/internal/detect"
)

func TestFromFileExtensions_Go(t *testing.T) {
	t.Parallel()
	d := detect.FromFileExtensions([]string{"main.go", "util.go"})
	if len(d.Languages) != 1 || d.Languages[0] != "go" {
		t.Errorf("Languages = %v, want [go]", d.Languages)
	}
}

func TestFromFileExtensions_Mixed(t *testing.T) {
	t.Parallel()
	d := detect.FromFileExtensions([]string{"main.go", "app.ts"})
	langs := d.Languages
	if len(langs) != 2 {
		t.Fatalf("Languages = %v, want [go typescript]", langs)
	}
	found := map[string]bool{}
	for _, l := range langs {
		found[l] = true
	}
	if !found["go"] || !found["typescript"] {
		t.Errorf("Languages = %v, want go and typescript", langs)
	}
}

func TestFromGitDiff(t *testing.T) {
	t.Parallel()
	diff := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// change
`
	d := detect.FromGitDiff(diff)
	if len(d.Languages) != 1 || d.Languages[0] != "go" {
		t.Errorf("Languages = %v, want [go]", d.Languages)
	}
}
