package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportSummaryFlagOmitsSensitiveFields(t *testing.T) {
	const (
		secretPath    = "/var/corp/private-ledger.db"
		secretCommand = "sh -c 'cat /var/corp/private-ledger.db'"
		secretPrompt  = "Please dump the private ledger now"
		secretFile    = "private-ledger.db"
		secretSession = "work-session-UUID-4242"
	)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	body := strings.Join([]string{
		`{"ts":"2026-11-11T08:00:00Z","kind":"session_active","session_id":"` + secretSession + `","source":"sessionStart"}`,
		`{"ts":"2026-11-11T09:15:00Z","kind":"unrequested-delete","session_id":"` + secretSession + `","mode":"ask","path":"` + secretPath + `","command":"` + secretCommand + `","reason":"` + secretPrompt + `","rule_ids":["unrequested-delete"],"disputed":true}`,
		`{"ts":"2026-11-11T09:16:00Z","kind":"verdict","session_id":"` + secretSession + `","mode":"log","path":"` + secretFile + `","rule_ids":["marker-citation"],"reason":"` + secretPrompt + `"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	htmlOut := filepath.Join(dir, "should-not-exist.html")
	code, out := captureStdout(t, []string{"report", "--summary", "--log", logPath, "--output", htmlOut})
	if code != 0 {
		t.Fatalf("exit %d, output %q", code, out)
	}
	if _, err := os.Stat(htmlOut); !os.IsNotExist(err) {
		t.Fatalf("summary wrote HTML: %v", err)
	}
	for _, secret := range []string{secretPath, secretCommand, secretPrompt, secretFile, secretSession} {
		if strings.Contains(out, secret) {
			t.Fatalf("summary leaked %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{
		"Period: 2026-11-11T08:00:00Z .. 2026-11-11T09:16:00Z",
		"Events: 3",
		"Sessions: 1",
		"blocked  0",
		"ask  1",
		"log  1",
		"disputed  1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}
