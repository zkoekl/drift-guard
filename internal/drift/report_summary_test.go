package drift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportSummaryOmitsSensitiveFields(t *testing.T) {
	const (
		secretPath    = "/opt/acme/secret-payroll.csv"
		secretCommand = "rm -rf /opt/acme/secret-payroll.csv"
		secretPrompt  = "User said: delete the payroll file please"
		secretFile    = "q3-bonus-sheet.xlsx"
		secretSession = "sess-CORP-SECRET-991"
	)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	events := []Event{
		{
			TS:         "2026-11-09T10:00:00Z",
			Kind:       "session_active",
			SessionID:  secretSession,
			Source:     "beforeSubmitPrompt",
			Activation: "tag",
		},
		{
			TS:        "2026-11-10T18:30:00Z",
			Kind:      "unrequested-delete",
			SessionID: secretSession,
			Mode:      "ask",
			Path:      secretPath,
			Command:   secretCommand,
			Tool:      "rm",
			Reason:    secretPrompt + " " + secretFile,
			RuleIDs:   []string{"unrequested-delete"},
			Disputed:  true,
		},
		{
			TS:        "2026-11-10T18:31:00Z",
			Kind:      "verdict",
			SessionID: "other-session-id-zzzz",
			Source:    "hook-stop",
			Mode:      "block",
			RuleIDs:   []string{"done-without-tests"},
			Reason:    "changed " + secretFile + " with no test",
			Path:      secretPath,
		},
	}
	for _, ev := range events {
		if err := AppendJSONL(logPath, ev); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := LoadEvents(logPath)
	if err != nil {
		t.Fatal(err)
	}
	out := ReportSummary(loaded)
	for _, secret := range []string{secretPath, secretCommand, secretPrompt, secretFile, secretSession, "other-session-id-zzzz"} {
		if strings.Contains(out, secret) {
			t.Fatalf("summary leaked %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{
		"Period: 2026-11-09T10:00:00Z .. 2026-11-10T18:31:00Z",
		"Events: 3",
		"Sessions: 2",
		"unrequested-delete  1",
		"verdict  1",
		"session_active  1",
		"done-without-tests  1",
		"blocked  1",
		"ask  1",
		"disputed  1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

func TestReportSummaryEmpty(t *testing.T) {
	out := ReportSummary(nil)
	if !strings.Contains(out, "Period: none") || !strings.Contains(out, "Events: 0") || !strings.Contains(out, "Sessions: 0") {
		t.Fatalf("empty summary:\n%s", out)
	}
	if _, err := os.Stat("events.jsonl"); err == nil {
		// not part of this test; ignore if a repo-local log exists
	}
}
