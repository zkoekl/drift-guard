package drift

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestShellDeletions(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{"rm hello.py", []string{"rm:hello.py"}},
		{"rm -rf hello.py main.go", []string{"rm:hello.py", "rm:main.go"}},
		{"rm -rf -- hello.py", []string{"rm:hello.py"}},
		{"/bin/rm -f hello.py", []string{"rm:hello.py"}},
		{"sudo rm hello.py", []string{"rm:hello.py"}},
		{"sudo -u root rm -f hello.py", []string{"rm:hello.py"}},
		{"env FOO=1 rm hello.py", []string{"rm:hello.py"}},
		{"echo rm hello.py", nil},
		{`echo "rm hello.py"`, nil},
		{`echo 'rm hello.py'`, nil},
		{"git rm hello.py", []string{"git-rm:hello.py"}},
		{"git -C repo rm --cached hello.py", []string{"git-rm:hello.py"}},
		{"git status", nil},
		{"git push origin", nil},
		{"unlink hello.py", []string{"unlink:hello.py"}},
		{"mv hello.py main.go", []string{"mv-away:hello.py", "mv-over:main.go"}},
		{"mv -t dest a.go b.go", []string{"mv-away:a.go", "mv-away:b.go", "mv-over:dest"}},
		{"git mv hello.py main.go", []string{"mv-away:hello.py", "mv-over:main.go"}},
		{"echo hi > hello.py", []string{"trunc:hello.py"}},
		{"echo hi>hello.py", []string{"trunc:hello.py"}},
		{": > hello.py", []string{"trunc:hello.py"}},
		{"echo hi >| hello.py", []string{"trunc:hello.py"}},
		{"echo hi >> hello.py", nil},
		{"echo hi 2>&1", nil},
		{"echo hi > /dev/null", []string{"trunc:/dev/null"}},
		{`echo 'keep > this'`, nil},
		{`echo "keep > this"`, nil},
		{"echo hi # > other.py", nil},
		{"rm hello.py # && rm other.go", []string{"rm:hello.py"}},
		{"rm *.py", nil},
		{"rm $FILE", nil},
		{`rm "$FILE"`, nil},
		{"bash -c 'rm hello.py'", []string{"rm:hello.py"}},
		{`bash -lc "rm hello.py && mv a.go b.go"`, []string{"rm:hello.py", "mv-away:a.go", "mv-over:b.go"}},
		{"rm hello.py && git status", []string{"rm:hello.py"}},
		{"echo hi | rm hello.py", []string{"rm:hello.py"}},
		{"Remove-Item hello.py", nil},
		{"find . -delete", nil},
		{"rm -rf node_modules/pkg", []string{"rm:node_modules/pkg"}},
		{"cp hello.py main.go", nil},
	}
	for _, tc := range cases {
		got := opSummary(shellDeletions(tc.cmd))
		if !equalStrings(got, tc.want) {
			t.Errorf("%s\n got %v\nwant %v", tc.cmd, got, tc.want)
		}
	}
}

func TestShellDeletionOperandCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("rm")
	for i := 0; i < 50; i++ {
		b.WriteString(" f")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(".py")
	}
	ops := shellDeletions(b.String())
	if len(ops) != maxDeleteOperands {
		t.Fatalf("operands %d", len(ops))
	}
	if shellDeletions("rm "+strings.Repeat("a", maxShellScan)) != nil {
		t.Fatal("oversized command was parsed")
	}
}

func TestSelectUnrequested(t *testing.T) {
	hello := statSet([]string{"hello.py", "src/hello.py", "main.go", "subdir/hello.py"}, []string{"subdir"})
	vague := "ну и сделай там заодно что нужно, ты понял"
	cases := []struct {
		name    string
		cmd     string
		prompt  string
		known   bool
		created []string
		stat    pathStat
		root    string
		want    []string
	}{
		{"unknown existing", "rm hello.py", "", false, nil, hello, "/proj", []string{"hello.py"}},
		{"unknown missing", "rm missing.py", "", false, nil, hello, "/proj", nil},
		{"unknown created", "rm hello.py", "", false, []string{"hello.py"}, statSet(nil, nil), "/proj", []string{"hello.py"}},
		{"named", "rm hello.py", "создай hello.py, который печатает привет", true, nil, hello, "/proj", nil},
		{"other file", "rm hello.py", "создай main.go", true, nil, hello, "/proj", []string{"hello.py"}},
		{"intent ru", "rm hello.py", "удали лишнее", true, nil, hello, "/proj", nil},
		{"intent en", "rm hello.py", "please remove the old scripts", true, nil, hello, "/proj", nil},
		{"rewrite allows", "rm hello.py", "перепиши readme", true, nil, hello, "/proj", nil},
		{"vague still flags", "rm hello.py", vague, true, nil, hello, "/proj", []string{"hello.py"}},
		{"basename", "rm src/hello.py", "edit hello.py", true, nil, hello, "/proj", nil},
		{"basename boundary", "rm hello.py", "edit hello.py.bak", true, nil, hello, "/proj", []string{"hello.py"}},
		{"trunc exists", "echo hi > hello.py", "создай main.go", true, nil, hello, "/proj", []string{"hello.py"}},
		{"trunc missing", "echo hi > missing.py", "создай main.go", true, nil, hello, "/proj", nil},
		{"append", "echo hi >> hello.py", "создай main.go", true, nil, hello, "/proj", nil},
		{"mv away only", "mv hello.py main.go", "создай readme", true, nil, statSet([]string{"hello.py"}, nil), "/proj", []string{"hello.py"}},
		{"mv over", "mv hello.py main.go", "создай readme", true, nil, hello, "/proj", []string{"hello.py", "main.go"}},
		{"mv into temp", "mv hello.py /tmp/x", "создай readme", true, nil, hello, "/proj", []string{"hello.py"}},
		{"tmp file", "rm /tmp/hello.py", "создай readme", true, nil, statSet([]string{"/tmp/hello.py"}, nil), "/proj", nil},
		{"build paths", "rm -rf bin/app dist/a.js node_modules/pkg .git/config", "создай readme", true, nil, statSet([]string{"bin/app", "dist/a.js", "node_modules/pkg", ".git/config"}, nil), "/proj", nil},
		{"dev null", "echo hi > /dev/null", "создай readme", true, nil, statSet([]string{"/dev/null"}, nil), "/proj", nil},
		{"mv into dir", "mv hello.py subdir", "создай readme", true, nil, hello, "/proj", []string{"hello.py", "subdir/hello.py"}},
		{"project under tmp", "rm hello.py", "", false, nil, statSet([]string{"hello.py"}, nil), "/tmp/proj", []string{"hello.py"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := flaggedPaths(selectUnrequested(shellDeletions(tc.cmd), tc.prompt, tc.known, tc.created, tc.root, tc.stat))
			if !equalStrings(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestVagueAndIntent(t *testing.T) {
	if !vaguePrompt("ну и сделай там заодно что нужно, ты понял") {
		t.Fatal("motivating prompt should be vague")
	}
	for _, s := range []string{
		"сделай что нужно",
		"Ты понял",
		"как считаешь нужным",
		"do whatever is needed",
		"you know what I mean",
		"while you're at it",
	} {
		if !vaguePrompt(s) {
			t.Errorf("vague %q", s)
		}
	}
	for _, s := range []string{
		"#dg создай hello.py, который печатает привет, и проверь",
		"а разве не лучше было на Go?",
		"add a test for ParseWidget",
	} {
		if vaguePrompt(s) {
			t.Errorf("not vague %q", s)
		}
	}
	if !hasDestructiveIntent("please Delete the old file") || !hasDestructiveIntent("замени это") {
		t.Fatal("intent words")
	}
	if hasDestructiveIntent("создай hello.py") || hasDestructiveIntent("а разве не лучше было на Go?") {
		t.Fatal("create and question are not destructive intent")
	}
	if !promptNamesFile("создай src/hello.py", "src/hello.py") || !promptNamesFile("создай hello.py", "pkg/hello.py") {
		t.Fatal("prompt should name the file")
	}
	if promptNamesFile("edit hello.py.bak", "hello.py") || promptNamesFile("edit nothello.py", "hello.py") {
		t.Fatal("boundary")
	}
}

func TestIgnoredDeletePaths(t *testing.T) {
	if ignoredDeletePath("hello.py", "/tmp/proj") {
		t.Fatal("project file under /tmp must not be ignored")
	}
	if ignoredDeletePath("/tmp/proj/hello.py", "/tmp/proj") {
		t.Fatal("absolute path inside the project must not be ignored")
	}
	for _, p := range []string{"/tmp/hello.py", "bin/app", "web/node_modules/pkg/index.js", ".git/config", "dist/app.js", "/dev/null"} {
		if !ignoredDeletePath(p, "/tmp/proj") {
			t.Errorf("expected ignore %s", p)
		}
	}
}

func TestToolDeletions(t *testing.T) {
	del := toolDeletions(map[string]any{
		"tool_name":  "Delete",
		"tool_input": map[string]any{"path": "hello.py"},
	})
	if !equalStrings(opSummary(del), []string{"tool:hello.py"}) {
		t.Fatalf("delete tool %v", opSummary(del))
	}
	asString := toolDeletions(map[string]any{
		"tool_name":  "Delete",
		"tool_input": `{"path":"hello.py"}`,
	})
	if !equalStrings(opSummary(asString), []string{"tool:hello.py"}) {
		t.Fatalf("string input %v", opSummary(asString))
	}
	wipe := toolDeletions(map[string]any{
		"tool_name":  "Write",
		"tool_input": map[string]any{"path": "hello.py", "contents": ""},
	})
	if !equalStrings(opSummary(wipe), []string{"trunc:hello.py"}) {
		t.Fatalf("empty write %v", opSummary(wipe))
	}
	keep := toolDeletions(map[string]any{
		"tool_name":  "Write",
		"tool_input": map[string]any{"path": "main.go", "contents": "package main\n"},
	})
	if keep != nil {
		t.Fatalf("non-empty write %v", opSummary(keep))
	}
	edit := toolDeletions(map[string]any{
		"tool_name": "StrReplace",
		"tool_input": map[string]any{
			"path":       "hello.py",
			"old_string": "a",
			"new_string": "",
		},
	})
	if edit != nil {
		t.Fatalf("strreplace %v", opSummary(edit))
	}
	patch := toolDeletions(map[string]any{
		"tool_name": "Edit",
		"tool_input": map[string]any{
			"path":  "other.txt",
			"patch": "*** Delete File: hello.py\n",
		},
	})
	if !equalStrings(opSummary(patch), []string{"tool:hello.py"}) {
		t.Fatalf("patch %v", opSummary(patch))
	}
}

func TestHookUnrequestedDeleteHelloGo(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"reply_language":"ru"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const sid = "hello-go"
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt",
		"conversation_id": sid,
		"prompt":          "#dg создай hello.py, который печатает привет, и проверь",
	}))
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "postToolUse",
		"conversation_id": sid,
		"tool_name":       "Write",
		"tool_input":      map[string]any{"path": "hello.py", "contents": "print('hi')\n"},
	}))
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt",
		"conversation_id": sid,
		"prompt":          "а разве не лучше было на Go?",
	}))
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt",
		"conversation_id": sid,
		"prompt":          "ну и сделай там заодно что нужно, ты понял",
	}))
	deleted := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse",
		"conversation_id": sid,
		"tool_name":       "Delete",
		"tool_input":      map[string]any{"path": "hello.py"},
	}))
	if deleted["permission"] != "ask" {
		t.Fatalf("delete permission %#v", deleted)
	}
	msg, _ := deleted["user_message"].(string)
	wantMsg := "Удаление/замена hello.py не было в задаче. Размытый запрос, нужно было спросить. Уточни у пользователя, прежде чем удалять."
	if msg != wantMsg {
		t.Fatalf("message %q", msg)
	}
	if deleted["agent_message"] != msg {
		t.Fatalf("agent message %#v", deleted["agent_message"])
	}
	mainGo := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse",
		"conversation_id": sid,
		"tool_name":       "Write",
		"tool_input":      map[string]any{"path": "main.go", "contents": "package main\n"},
	}))
	if mainGo["permission"] != "allow" {
		t.Fatalf("main.go %#v", mainGo)
	}
	goMod := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse",
		"conversation_id": sid,
		"tool_name":       "Write",
		"tool_input":      map[string]any{"path": "go.mod", "contents": "module example\n"},
	}))
	if goMod["permission"] != "allow" {
		t.Fatalf("go.mod %#v", goMod)
	}

	evs := deleteEvents(t, p)
	if len(evs) != 1 {
		t.Fatalf("events %#v", evs)
	}
	ev := evs[0]
	if ev.Path != "hello.py" || ev.Tool != "Delete" || !ev.PromptKnown || !ev.VaguePrompt || ev.Disputed {
		t.Fatalf("event %#v", ev)
	}
	if !strings.Contains(ev.Reason, "vague request, should have asked") {
		t.Fatalf("reason %q", ev.Reason)
	}
	raw, err := os.ReadFile(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"kind":"unrequested-delete"`, `"prompt_known":true`, `"vague_prompt":true`, `"disputed":false`, `"path":"hello.py"`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("log missing %s\n%s", field, raw)
		}
	}
	page := ReportHTML(evs)
	if !strings.Contains(page, "Unrequested deletes") || !strings.Contains(page, "hello.py") || !strings.Contains(page, "vague request, should have asked") {
		t.Fatalf("report\n%s", page)
	}
}

func TestHookUnrequestedDeleteCLI(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const sid = "cli-rm"
	cmd := hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": sid,
		"command":         "rm hello.py",
	})
	first := mustHandle(t, p, cmd)
	if first["permission"] != "ask" {
		t.Fatalf("first %#v", first)
	}
	msg, _ := first["user_message"].(string)
	if !strings.Contains(msg, "Deleting or replacing hello.py") || !strings.Contains(msg, "Ask the user before deleting it.") {
		t.Fatalf("english message %q", msg)
	}
	second := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse",
		"conversation_id": sid,
		"tool_name":       "Shell",
		"tool_input":      map[string]any{"command": "rm hello.py"},
	}))
	if second["permission"] != "ask" {
		t.Fatalf("second %#v", second)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 {
		t.Fatalf("deduped events %d %#v", len(evs), evs)
	}
	if evs[0].PromptKnown || evs[0].VaguePrompt || !evs[0].Disputed || evs[0].Tool != "rm" {
		t.Fatalf("event %#v", evs[0])
	}
	raw, err := os.ReadFile(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"prompt_known":false`) || !strings.Contains(string(raw), `"disputed":true`) {
		t.Fatalf("raw %s", raw)
	}

	// Created earlier in the session, file not on disk, prompt unknown.
	p2 := testPaths(t)
	const sid2 = "cli-created"
	mustHandle(t, p2, hookJSON(t, map[string]any{
		"hook_event_name": "postToolUse",
		"conversation_id": sid2,
		"tool_name":       "Write",
		"tool_input":      map[string]any{"path": "hello.py", "contents": "print(1)\n"},
	}))
	created := mustHandle(t, p2, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse",
		"conversation_id": sid2,
		"tool_name":       "Delete",
		"tool_input":      map[string]any{"path": "hello.py"},
	}))
	if created["permission"] != "ask" {
		t.Fatalf("created %#v", created)
	}
	ev2 := deleteEvents(t, p2)
	if len(ev2) != 1 || !ev2[0].Disputed || ev2[0].PromptKnown {
		t.Fatalf("created event %#v", ev2)
	}
}

func TestHookUnrequestedDeleteModes(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"unrequested_delete":"log","reply_language":"en"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logged := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "logmode",
		"command":         "rm hello.py",
	}))
	if logged["permission"] != "allow" {
		t.Fatalf("log mode %#v", logged)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 || evs[0].Mode != "log" {
		t.Fatalf("log event %#v", evs)
	}

	off := testPaths(t)
	if err := os.WriteFile(off.Rules, []byte(`{"unrequested_delete":"off"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(off.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skipped := mustHandle(t, off, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "offmode",
		"command":         "rm hello.py",
	}))
	if skipped["permission"] != "allow" || len(deleteEvents(t, off)) != 0 {
		t.Fatalf("off %#v events %v", skipped, deleteEvents(t, off))
	}

	named := testPaths(t)
	if err := os.WriteFile(filepath.Join(named.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustHandle(t, named, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt",
		"conversation_id": "named",
		"prompt":          "удали hello.py",
	}))
	allow := mustHandle(t, named, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "named",
		"command":         "rm hello.py",
	}))
	if allow["permission"] != "allow" || len(deleteEvents(t, named)) != 0 {
		t.Fatalf("requested %#v", allow)
	}
}

func TestHookUnrequestedDeleteInactiveAndPush(t *testing.T) {
	p := testPaths(t)
	t.Setenv("DRIFT_GUARD_ACTIVATION", "opt-in")
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inactive := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "asleep",
		"command":         "rm hello.py",
	}))
	if inactive["permission"] != "allow" || len(deleteEvents(t, p)) != 0 {
		t.Fatalf("inactive %#v", inactive)
	}

	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt",
		"conversation_id": "awake",
		"prompt":          "продолжи #dg работу",
	}))
	awake := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "awake",
		"command":         "rm hello.py",
	}))
	if awake["permission"] != "ask" {
		t.Fatalf("active %#v", awake)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 || !evs[0].PromptKnown || evs[0].VaguePrompt || evs[0].Disputed {
		t.Fatalf("active event %#v", evs)
	}

	pushed := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "awake",
		"command":         "rm hello.py && git push",
	}))
	if pushed["permission"] != "deny" {
		t.Fatalf("push %#v", pushed)
	}
	all, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	sawPush := false
	sawDelete := 0
	for _, ev := range all {
		if ev.Kind == "unrequested-delete" && ev.SessionID == "awake" {
			sawDelete++
		}
		if hasRule(ev, "git-push") {
			sawPush = true
		}
	}
	if !sawPush || sawDelete < 2 {
		t.Fatalf("push log delete=%d push=%v %#v", sawDelete, sawPush, all)
	}
}

func TestHookDeleteClaudeAndAbsolute(t *testing.T) {
	p := testPaths(t)
	abs := filepath.Join(p.Root, "hello.py")
	if err := os.WriteFile(abs, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "claude",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "rm " + abs},
	}))
	spec, _ := out["hookSpecificOutput"].(map[string]any)
	if spec["permissionDecision"] != "ask" {
		t.Fatalf("claude %#v", out)
	}
	reason, _ := spec["permissionDecisionReason"].(string)
	if !strings.Contains(reason, "Ask the user before deleting it.") {
		t.Fatalf("reason %q", reason)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 || evs[0].Path == "" {
		t.Fatalf("absolute %#v", evs)
	}
}

func TestHookReplyLanguageEnv(t *testing.T) {
	p := testPaths(t)
	t.Setenv("DRIFT_GUARD_REPLY_LANGUAGE", "ru")
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "envlang",
		"command":         "unlink hello.py",
	}))
	msg, _ := out["user_message"].(string)
	if !strings.Contains(msg, "Удаление/замена hello.py") {
		t.Fatalf("env language %q", msg)
	}
}

func opSummary(ops []deleteOp) []string {
	var s []string
	for _, op := range ops {
		s = append(s, op.Kind+":"+op.Path)
	}
	return s
}

func equalStrings(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func statSet(files, dirs []string) pathStat {
	f := map[string]bool{}
	d := map[string]bool{}
	for _, p := range files {
		f[normDeletePath(p)] = true
	}
	for _, p := range dirs {
		d[normDeletePath(p)] = true
	}
	return func(p string) (bool, bool) {
		p = normDeletePath(p)
		if d[p] {
			return true, true
		}
		if f[p] {
			return true, false
		}
		return false, false
	}
}

func deleteEvents(t *testing.T, p Paths) []Event {
	t.Helper()
	evs, err := LoadEvents(p.Log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []Event
	for _, ev := range evs {
		if ev.Kind == kindUnrequestedDelete {
			out = append(out, ev)
		}
	}
	return out
}

func hookJSON(t *testing.T, v map[string]any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDeleteDedupeWindow(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution",
		"conversation_id": "later",
		"command":         "rm hello.py",
	})
	t0 := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	first, code := Handle([]byte(payload), p, t0)
	if code != 0 {
		t.Fatal(code)
	}
	var m map[string]any
	if err := json.Unmarshal(first, &m); err != nil {
		t.Fatal(err)
	}
	if m["permission"] != "ask" {
		t.Fatalf("first %#v", m)
	}
	second, _ := Handle([]byte(payload), p, t0.Add(6*time.Second))
	if err := json.Unmarshal(second, &m); err != nil {
		t.Fatal(err)
	}
	if m["permission"] != "ask" {
		t.Fatalf("second %#v", m)
	}
	if len(deleteEvents(t, p)) != 2 {
		t.Fatalf("expected a second event after the window, got %d", len(deleteEvents(t, p)))
	}
}
