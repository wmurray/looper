package notify

// Send fires a desktop notification and optionally POSTs to a Slack webhook.
// Best-effort: all errors are silently swallowed so the caller's exit code is unaffected.
func Send(enabled bool, webhookURL, title, body string) {
	if !enabled && webhookURL == "" {
		return
	}
	if enabled {
		_ = sendDesktop(title, body)
	}
	if webhookURL != "" {
		_ = sendSlack(webhookURL, title, body)
	}
}
