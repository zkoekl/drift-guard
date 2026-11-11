package drift

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The hook writes events.jsonl in the project root and dumps under
// .drift-guard/. Both patterns are in the repo's .gitignore so a session
// does not dirty git status.
func TestGitignoreCoversHookLogs(t *testing.T) {
	root := repoRoot(t)
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), ignore, 0o644); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(dir, "README.md")
	if err := os.WriteFile(tracked, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".drift-guard"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"events.jsonl",
		"drift-guard-report.html",
		"drift-guard.log.jsonl",
		filepath.Join(".drift-guard", "hook-dump.jsonl"),
		filepath.Join(".drift-guard", "canary.md"),
		filepath.Join(".drift-guard", "session-unknown.json"),
	} {
		path := filepath.Join(dir, rel)
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("add", "-A")
	run("commit", "-qm", "init")
	status := run("status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("hook logs should be ignored, status:\n%s", status)
	}
	trackedFiles := run("ls-files")
	for _, name := range []string{"events.jsonl", "hook-dump.jsonl", "canary.md", "drift-guard-report.html"} {
		if strings.Contains(trackedFiles, name) {
			t.Fatalf("tracked ignored file %s:\n%s", name, trackedFiles)
		}
	}
}
