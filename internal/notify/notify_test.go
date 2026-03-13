package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend_NoopWhenDisabledAndNoWebhook(t *testing.T) {
	// Should not panic or spawn any process; just verifies no-op contract.
	Send(false, "", "title", "body")
}

func TestSendSlack_PayloadContainsTitleAndBody(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := sendSlack(srv.URL, "Looper — IMP-13", "Loop finished successfully"); err != nil {
		t.Fatalf("sendSlack returned error: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}
	text := payload["text"]
	if text == "" {
		t.Fatal("expected non-empty text field")
	}
	for _, want := range []string{"Looper — IMP-13", "Loop finished successfully"} {
		if !strings.Contains(text, want) {
			t.Errorf("payload text %q does not contain %q", text, want)
		}
	}
}

func TestDesktopCommand_Darwin(t *testing.T) {
	name, args := desktopCommand("darwin", "My Title", "My Body")
	if name != "osascript" {
		t.Errorf("expected osascript, got %q", name)
	}
	if len(args) < 2 || args[0] != "-e" {
		t.Errorf("expected osascript -e <script>, got %v", args)
	}
	script := args[1]
	if !strings.Contains(script, `"My Title"`) {
		t.Errorf("script missing title: %q", script)
	}
	if !strings.Contains(script, `"My Body"`) {
		t.Errorf("script missing body: %q", script)
	}
}

func TestDesktopCommand_Linux(t *testing.T) {
	name, args := desktopCommand("linux", "My Title", "My Body")
	if name != "notify-send" {
		t.Errorf("expected notify-send, got %q", name)
	}
	if len(args) != 2 || args[0] != "My Title" || args[1] != "My Body" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestDesktopCommand_Other(t *testing.T) {
	name, args := desktopCommand("windows", "t", "b")
	if name != "" || len(args) != 0 {
		t.Errorf("expected no-op for unsupported OS, got name=%q args=%v", name, args)
	}
}
