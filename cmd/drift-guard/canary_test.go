package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zkoekl/drift-guard/internal/drift"
)

func TestCanaryRefreshCommandOmitsCode(t *testing.T) {
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(rules, []byte(`{"canary":{"files":["AGENTS.md","missing.md"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("drift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(dir, "secret")
	stdout, code := captureRun(t, []string{
		"canary", "refresh",
		"--session", "cli-sess",
		"--rules", rules,
		"--root", dir,
		"--secret-file", secretFile,
	})
	if code != 1 {
		t.Fatalf("exit %d stdout %s", code, stdout)
	}
	if !strings.Contains(stdout, "refreshed AGENTS.md") || !strings.Contains(stdout, "missing missing.md") {
		t.Fatalf("stdout %s", stdout)
	}
	secret, err := os.ReadFile(secretFile)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := drift.CanaryCode("cli-sess", "AGENTS.md", strings.TrimSpace(string(secret)))
	if !strings.Contains(string(got), "drift-guard-canary: "+want) {
		t.Fatalf("file %s", got)
	}
	if strings.Contains(stdout, want) || strings.Contains(stdout, strings.TrimSpace(string(secret))) {
		t.Fatalf("command printed the code or the secret: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.md")); !os.IsNotExist(err) {
		t.Fatal("missing file was created")
	}

	offDir := t.TempDir()
	offRules := filepath.Join(offDir, "rules.json")
	if err := os.WriteFile(offRules, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, code = captureRun(t, []string{
		"canary", "refresh",
		"--session", "cli-sess",
		"--rules", offRules,
		"--root", offDir,
		"--secret-file", filepath.Join(offDir, "secret"),
	})
	if code != 0 || !strings.Contains(stdout, "canary off") {
		t.Fatalf("off: exit %d stdout %s", code, stdout)
	}
	if code := run([]string{"canary", "refresh"}); code != 2 {
		t.Fatalf("usage exit %d", code)
	}
}

func captureRun(t *testing.T, args []string) (string, int) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := run(args)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return buf.String(), code
}
