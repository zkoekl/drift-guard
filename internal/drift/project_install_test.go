package drift

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDogfoodProjectInstall(t *testing.T) {
	root := repoRoot(t)
	hookPath := filepath.Join(root, ".cursor", "hooks.json")
	raw, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int                       `json:"version"`
		Hooks   map[string][]hookEntryDoc `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 {
		t.Fatalf("version %d", doc.Version)
	}
	want := []string{
		"beforeShellExecution",
		"preToolUse",
		"postToolUse",
		"sessionStart",
		"beforeSubmitPrompt",
		"afterAgentResponse",
		"stop",
	}
	const cmd = ".cursor/hooks/drift-guard.sh"
	for _, name := range want {
		entries := doc.Hooks[name]
		if len(entries) == 0 {
			t.Fatalf("missing %s", name)
		}
		for _, e := range entries {
			if e.Command != cmd {
				t.Fatalf("%s command %q", name, e.Command)
			}
			if e.Timeout < 30 {
				t.Fatalf("%s timeout %d", name, e.Timeout)
			}
		}
	}
	pushClosed := false
	for _, e := range doc.Hooks["beforeShellExecution"] {
		if e.FailClosed && strings.Contains(e.Matcher, "push") {
			pushClosed = true
		}
	}
	if !pushClosed {
		t.Fatal("git push hook is not failClosed")
	}
	shellClosed := false
	for _, e := range doc.Hooks["preToolUse"] {
		if e.FailClosed && e.Matcher == "Shell" {
			shellClosed = true
		}
	}
	if !shellClosed {
		t.Fatal("preToolUse Shell is not failClosed")
	}
	if doc.Hooks["stop"][0].LoopLimit == nil || *doc.Hooks["stop"][0].LoopLimit != 3 {
		t.Fatalf("stop loop_limit %#v", doc.Hooks["stop"][0].LoopLimit)
	}

	script := filepath.Join(root, ".cursor", "hooks", "drift-guard.sh")
	st, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0o111 == 0 {
		t.Fatal("hook script is not executable")
	}
	if _, err := exec.LookPath("shellcheck"); err == nil {
		out, err := exec.Command("shellcheck", "-s", "sh", script).CombinedOutput()
		if err != nil {
			t.Fatalf("shellcheck: %v\n%s", err, out)
		}
	}

	claudeRaw, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeRaw), "${CLAUDE_PROJECT_DIR}/.cursor/hooks/drift-guard.sh") ||
		!strings.Contains(string(claudeRaw), `"onFailure": "block"`) {
		t.Fatalf("claude settings:\n%s", claudeRaw)
	}

	rulesRaw, err := os.ReadFile(filepath.Join(root, "drift-guard.rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rules ruleFile
	if err := json.Unmarshal(rulesRaw, &rules); err != nil {
		t.Fatal(err)
	}
	if rules.TestCommand != "go test ./..." || len(rules.Canary.Files) != 1 || rules.Canary.Files[0] != "AGENTS.md" {
		t.Fatalf("rules: %#v", rules)
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	run := func(payload string) map[string]any {
		t.Helper()
		c := exec.Command(script)
		c.Dir = root
		c.Stdin = strings.NewReader(payload)
		c.Env = append(os.Environ(),
			"PATH="+filepath.Dir(goBin)+":/usr/bin:/bin",
			"DRIFT_GUARD_LOG="+filepath.Join(dir, "events.jsonl"),
			"DRIFT_GUARD_STATE="+filepath.Join(dir, "state"),
			"DRIFT_GUARD_DUMP="+filepath.Join(dir, "dump.jsonl"),
			"DRIFT_GUARD_RULES="+filepath.Join(dir, "none.json"),
			"DRIFT_GUARD_ROOT="+root,
		)
		out, err := c.Output()
		if err != nil {
			t.Fatalf("hook: %v\n%s", err, out)
		}
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("stdout %s: %v", out, err)
		}
		return got
	}
	allow := run(`{"hook_event_name":"beforeShellExecution","command":"git status"}`)
	if allow["permission"] != "allow" {
		t.Fatalf("status: %#v", allow)
	}
	deny := run(`{"hook_event_name":"beforeShellExecution","command":"git push"}`)
	if deny["permission"] != "deny" {
		t.Fatalf("push: %#v", deny)
	}
}

type hookEntryDoc struct {
	Command    string `json:"command"`
	Timeout    int    `json:"timeout"`
	FailClosed bool   `json:"failClosed"`
	Matcher    string `json:"matcher"`
	LoopLimit  *int   `json:"loop_limit"`
}
