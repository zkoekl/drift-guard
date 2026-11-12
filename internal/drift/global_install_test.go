package drift

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func isolatedEnv(home string) []string {
	var out []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") ||
			strings.HasPrefix(e, "CURSOR_PROJECT_DIR=") ||
			strings.HasPrefix(e, "DRIFT_GUARD_") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "HOME="+home)
}

func runScript(t *testing.T, home, script string) (string, int) {
	t.Helper()
	cmd := exec.Command("sh", script)
	cmd.Env = isolatedEnv(home)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("%s: %v\n%s", script, err, out)
	}
	return string(out), ee.ExitCode()
}

func TestGlobalInstallScripts(t *testing.T) {
	root := repoRoot(t)
	install := filepath.Join(root, "docs/install/global/install.sh")
	uninstall := filepath.Join(root, "docs/install/global/uninstall.sh")
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("shellcheck"); err == nil {
		out, err := exec.Command("shellcheck", "-s", "sh", install, uninstall).CombinedOutput()
		if err != nil {
			t.Fatalf("shellcheck: %v\n%s", err, out)
		}
	} else {
		t.Log("shellcheck not installed; skipped")
	}

	base := t.TempDir()
	home := filepath.Join(base, "home dir")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(home, ".cursor", "skills", "check-ambiguity")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept, "MARKER"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runScript(t, home, install)
	if code != 0 {
		t.Fatalf("install exit %d\n%s", code, out)
	}
	wrapper := filepath.Join(home, ".drift-guard", "bin", "drift-guard-hook")
	bin := filepath.Join(home, ".drift-guard", "bin", "drift-guard")
	for _, path := range []string{wrapper, bin} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode()&0o111 == 0 {
			t.Fatalf("not executable: %s", path)
		}
	}
	wrapperBody, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	text := string(wrapperBody)
	for _, needle := range []string{
		"DRIFT_GUARD_ACTIVATION=opt-in",
		"DRIFT_GUARD_LOG=",
		"DRIFT_GUARD_STATE=",
		"DRIFT_GUARD_DUMP=",
		bin,
		home,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("wrapper missing %q:\n%s", needle, text)
		}
	}
	if strings.Contains(text, "$HOME") {
		t.Fatalf("wrapper kept $HOME:\n%s", text)
	}
	hooksPath := filepath.Join(home, ".cursor", "hooks.json")
	hooks, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hooks), wrapper) || strings.Count(string(hooks), `"failClosed": true`) != 4 {
		t.Fatalf("hooks.json:\n%s", hooks)
	}
	if !strings.Contains(string(hooks), `"loop_limit": 3`) {
		t.Fatalf("hooks.json missing loop_limit:\n%s", hooks)
	}
	marker, err := os.ReadFile(filepath.Join(kept, "MARKER"))
	if err != nil || string(marker) != "keep\n" {
		t.Fatalf("existing skill was replaced: %v %q", err, marker)
	}
	for _, name := range []string{"handoff-constraints", "verify-before-done"} {
		skill := filepath.Join(home, ".cursor", "skills", name, "SKILL.md")
		if _, err := os.Stat(skill); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "check-ambiguity", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("existing skill directory was filled in")
	}

	proj := filepath.Join(home, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	runHook := func(payload string) string {
		t.Helper()
		cmd := exec.Command(wrapper)
		cmd.Env = isolatedEnv(home)
		cmd.Dir = proj
		cmd.Stdin = strings.NewReader(payload)
		got, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("hook: %v\n%s", err, got)
		}
		return string(got)
	}
	first := runHook(`{"hook_event_name":"beforeShellExecution","command":"git push origin","conversation_id":"e2e","prompt":"zzPromptSecret"}`)
	if strings.TrimSpace(first) != `{"permission":"allow"}` {
		t.Fatalf("inactive push: %s", first)
	}
	second := runHook(`{"hook_event_name":"stop","conversation_id":"e2e","text":"zzPromptSecret"}`)
	if strings.TrimSpace(second) != "{}" {
		t.Fatalf("inactive stop: %s", second)
	}
	logPath := filepath.Join(home, ".drift-guard", "events.jsonl")
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(logged), "\n") != 1 || strings.Contains(string(logged), "zzPromptSecret") {
		t.Fatalf("log:\n%s", logged)
	}
	if tree := listTree(t, proj); tree != "" {
		t.Fatalf("hook wrote into the project:\n%s", tree)
	}
	on := runHook(`{"hook_event_name":"beforeSubmitPrompt","conversation_id":"e2e","prompt":"start #dg"}`)
	if strings.TrimSpace(on) != "{}" {
		t.Fatalf("activate: %s", on)
	}
	denied := runHook(`{"hook_event_name":"beforeShellExecution","command":"git push","conversation_id":"e2e"}`)
	if !strings.Contains(denied, `"permission":"deny"`) {
		t.Fatalf("active push: %s", denied)
	}
	if tree := listTree(t, proj); tree != "" {
		t.Fatalf("active hook wrote into the project:\n%s", tree)
	}

	again, code := runScript(t, home, install)
	if code != 0 {
		t.Fatalf("second install exit %d\n%s", code, again)
	}
	hooks2, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(hooks2) != string(hooks) {
		t.Fatal("second install changed hooks.json")
	}

	if _, code := runScript(t, home, uninstall); code != 0 {
		t.Fatalf("uninstall exit %d", code)
	}
	if _, err := os.Stat(wrapper); !os.IsNotExist(err) {
		t.Fatal("wrapper still installed")
	}
	if _, err := os.Stat(hooksPath); !os.IsNotExist(err) {
		t.Fatal("hooks.json still installed")
	}
	if _, err := os.Stat(filepath.Join(kept, "MARKER")); err != nil {
		t.Fatal("pre-existing skill was removed")
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "verify-before-done")); !os.IsNotExist(err) {
		t.Fatal("installed skill was left behind")
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatal("uninstall removed the event log")
	}

	blockedHome := filepath.Join(t.TempDir(), "blocked")
	if err := os.MkdirAll(filepath.Join(blockedHome, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("{\n  \"hooks\": { \"stop\": [ { \"command\": \"other\" } ] }\n}\n")
	blockedHooks := filepath.Join(blockedHome, ".cursor", "hooks.json")
	if err := os.WriteFile(blockedHooks, original, 0o644); err != nil {
		t.Fatal(err)
	}
	msg, code := runScript(t, blockedHome, install)
	if code == 0 {
		t.Fatalf("install overwrote an existing hooks.json:\n%s", msg)
	}
	got, err := os.ReadFile(blockedHooks)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("existing hooks.json changed:\n%s", got)
	}
	backup := filepath.Join(blockedHome, ".cursor", "hooks.json.drift-guard-backup")
	backed, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backed) != string(original) {
		t.Fatal("backup does not match the original")
	}
	if _, code := runScript(t, blockedHome, uninstall); code != 0 {
		t.Fatalf("uninstall blocked exit %d", code)
	}
	got, err = os.ReadFile(blockedHooks)
	if err != nil || string(got) != string(original) {
		t.Fatalf("original hooks.json not kept: %v %s", err, got)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("backup left after uninstall")
	}

	trivialHome := filepath.Join(t.TempDir(), "trivial")
	if err := os.MkdirAll(filepath.Join(trivialHome, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	trivialPath := filepath.Join(trivialHome, ".cursor", "hooks.json")
	trivial := []byte("{ \"version\": 1, \"hooks\": {} }\n")
	if err := os.WriteFile(trivialPath, trivial, 0o644); err != nil {
		t.Fatal(err)
	}
	msg, code = runScript(t, trivialHome, install)
	if code != 0 {
		t.Fatalf("trivial install exit %d\n%s", code, msg)
	}
	merged, err := os.ReadFile(trivialPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(merged), `"failClosed": true`) || !strings.Contains(string(merged), "drift-guard-hook") {
		t.Fatalf("merged hooks:\n%s", merged)
	}
	trivialBackup, err := os.ReadFile(filepath.Join(trivialHome, ".cursor", "hooks.json.drift-guard-backup"))
	if err != nil || string(trivialBackup) != string(trivial) {
		t.Fatalf("trivial backup: %v %s", err, trivialBackup)
	}
	if _, code := runScript(t, trivialHome, uninstall); code != 0 {
		t.Fatalf("uninstall trivial exit %d", code)
	}
	restored, err := os.ReadFile(trivialPath)
	if err != nil || string(restored) != string(trivial) {
		t.Fatalf("trivial hooks not restored: %v %s", err, restored)
	}
}
