package drift

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellDeletions(t *testing.T) {
	cases := []struct {
		cmd  string
		want []string
	}{
		{"rm hello.py", []string{"rm:hello.py"}},
		{"rm -rf -- hello.py main.go", []string{"rm:hello.py", "rm:main.go"}},
		{"/bin/rm -f hello.py", []string{"rm:hello.py"}},
		{`echo "rm hello.py"`, nil},
		{"git rm hello.py", []string{"git-rm:hello.py"}},
		{"git -C repo rm hello.py", []string{"git-rm:hello.py"}},
		{"git status", nil},
		{"git push origin", nil},
		{"unlink hello.py", []string{"unlink:hello.py"}},
		{"mv hello.py main.go", []string{"mv-away:hello.py", "mv-over:main.go"}},
		{"git mv hello.py main.go", []string{"mv-away:hello.py", "mv-over:main.go"}},
		{"echo hi > hello.py", []string{"trunc:hello.py"}},
		{"echo hi >> hello.py", nil},
		{"echo hi 2>&1", nil},
		{`echo 'keep > this'`, nil},
		{"rm *.py", nil},
		{"rm $FILE", nil},
		{"rm hello.py && git status", []string{"rm:hello.py"}},
		{"sudo rm hello.py", nil},
		{"bash -c 'rm hello.py'", nil},
		{"find . -delete", nil},
	}
	for _, tc := range cases {
		if got := opSummary(shellDeletions(tc.cmd)); !equalStrings(got, tc.want) {
			t.Errorf("%s\n got %v\nwant %v", tc.cmd, got, tc.want)
		}
	}
}

func TestDeletionAction(t *testing.T) {
	hello := statSet([]string{"hello.py", "main.go", "src/pkg/a.go"}, []string{"src"})
	vague := "ну и сделай там заодно что нужно, ты понял"
	cases := []struct {
		name   string
		cmd    string
		prompt string
		known  bool
		want   []string
		action int
	}{
		{"named file", "rm hello.py", "#dg убери лишние пробелы в hello.py", true, nil, actSkip},
		{"other file", "rm main.go", "#dg убери лишние пробелы в hello.py", true, []string{"main.go"}, actAsk},
		{"verb no file", "rm main.go", "удали лишнее", true, []string{"main.go"}, actLog},
		{"under dir", "rm src/pkg/a.go", "remove old files in src/pkg", true, nil, actSkip},
		{"outside dir", "rm hello.py", "remove old files in src/pkg", true, []string{"hello.py"}, actAsk},
		{"vague", "rm hello.py", vague, true, []string{"hello.py"}, actAsk},
		{"unknown", "rm hello.py", "", false, []string{"hello.py"}, actAsk},
		{"create names it", "rm hello.py", "создай hello.py", true, nil, actSkip},
		{"trunc other", "echo hi > hello.py", "создай main.go", true, []string{"hello.py"}, actAsk},
		{"append", "echo hi >> hello.py", "создай main.go", true, nil, actSkip},
		{"mv away", "mv hello.py gone.go", "создай readme", true, []string{"hello.py"}, actAsk},
		{"ignored", "rm bin/app", "создай readme", true, nil, actSkip},
		{"tmp", "rm /tmp/hello.py", "создай readme", true, nil, actSkip},
		{"project in tmp", "rm hello.py", "", false, []string{"hello.py"}, actAsk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := "/proj"
			if tc.name == "project in tmp" {
				root = "/tmp/proj"
			}
			ops := selectUnrequested(shellDeletions(tc.cmd), nil, root, hello)
			var got []string
			for _, op := range ops {
				action, _ := deletionAction(tc.prompt, tc.known, op.Path)
				if action == actSkip {
					if len(tc.want) != 0 {
						t.Fatalf("skipped %s", op.Path)
					}
					continue
				}
				if action != tc.action {
					t.Fatalf("action %d on %s", action, op.Path)
				}
				got = append(got, op.Path)
			}
			if !equalStrings(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if !vaguePrompt(vague) || vaguePrompt("а разве не лучше было на Go?") {
		t.Fatal("vague list")
	}
	if hasDestructiveIntent("создай hello.py") || !hasDestructiveIntent("please remove the old one") {
		t.Fatal("intent list")
	}
	if covers("edit hello.py.bak", "hello.py") || !covers("edit hello.py", "pkg/hello.py") {
		t.Fatal("name boundary")
	}
	if ignoredDeletePath("hello.py", "/tmp/proj") || ignoredDeletePath("/tmp/proj/hello.py", "/tmp/proj") {
		t.Fatal("project under /tmp")
	}
}

func TestToolDeletions(t *testing.T) {
	del := toolDeletions(map[string]any{"tool_name": "Delete", "tool_input": map[string]any{"path": "hello.py"}})
	if !equalStrings(opSummary(del), []string{"tool:hello.py"}) {
		t.Fatalf("%v", opSummary(del))
	}
	wipe := toolDeletions(map[string]any{"tool_name": "Write", "tool_input": map[string]any{"path": "hello.py", "contents": ""}})
	if !equalStrings(opSummary(wipe), []string{"trunc:hello.py"}) {
		t.Fatalf("wipe %v", opSummary(wipe))
	}
	if toolDeletions(map[string]any{"tool_name": "Write", "tool_input": map[string]any{"path": "main.go", "contents": "package main\n"}}) != nil {
		t.Fatal("non-empty write")
	}
	if toolDeletions(map[string]any{"tool_name": "StrReplace", "tool_input": map[string]any{"path": "hello.py", "new_string": ""}}) != nil {
		t.Fatal("strreplace")
	}
}

func TestHookVerbScope(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"reply_language":"ru"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hello.py", "main.go"} {
		if err := os.WriteFile(filepath.Join(p.Root, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const sid = "scope"
	prompt := "#dg убери лишние пробелы в hello.py"
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt", "conversation_id": sid, "prompt": prompt,
	}))
	named := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": sid, "command": "rm hello.py",
	}))
	if named["permission"] != "allow" || len(deleteEvents(t, p)) != 0 {
		t.Fatalf("named file %#v events %v", named, deleteEvents(t, p))
	}
	other := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": sid, "command": "rm main.go",
	}))
	if other["permission"] != "ask" {
		t.Fatalf("other file %#v", other)
	}
	msg, _ := other["user_message"].(string)
	if !strings.Contains(msg, "Удаление/замена main.go") {
		t.Fatalf("message %q", msg)
	}

	bare := testPaths(t)
	if err := os.WriteFile(filepath.Join(bare.Root, "main.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustHandle(t, bare, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt", "conversation_id": "bare", "prompt": "удали лишнее",
	}))
	got := mustHandle(t, bare, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "bare", "command": "rm main.go",
	}))
	if got["permission"] != "allow" {
		t.Fatalf("bare verb %#v", got)
	}
	evs := deleteEvents(t, bare)
	if len(evs) != 1 || !evs[0].Disputed || !evs[0].PromptKnown || evs[0].Mode != "log" {
		t.Fatalf("bare event %#v", evs)
	}
	if !strings.Contains(evs[0].Reason, "destructive verb names no file") {
		t.Fatalf("reason %q", evs[0].Reason)
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
		"hook_event_name": "beforeSubmitPrompt", "conversation_id": sid,
		"prompt": "#dg создай hello.py, который печатает привет, и проверь",
	}))
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "postToolUse", "conversation_id": sid, "tool_name": "Write",
		"tool_input": map[string]any{"path": "hello.py", "contents": "print('hi')\n"},
	}))
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt", "conversation_id": sid,
		"prompt": "а разве не лучше было на Go?",
	}))
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt", "conversation_id": sid,
		"prompt": "ну и сделай там заодно что нужно, ты понял",
	}))
	deleted := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse", "conversation_id": sid, "tool_name": "Delete",
		"tool_input": map[string]any{"path": "hello.py"},
	}))
	want := "Удаление/замена hello.py не было в задаче. Размытый запрос, нужно было спросить. Уточни у пользователя, прежде чем удалять."
	if deleted["permission"] != "ask" || deleted["user_message"] != want {
		t.Fatalf("%#v", deleted)
	}
	mainGo := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse", "conversation_id": sid, "tool_name": "Write",
		"tool_input": map[string]any{"path": "main.go", "contents": "package main\n"},
	}))
	if mainGo["permission"] != "allow" {
		t.Fatalf("main.go %#v", mainGo)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 || evs[0].Path != "hello.py" || !evs[0].VaguePrompt || evs[0].Disputed {
		t.Fatalf("%#v", evs)
	}
	if !strings.Contains(evs[0].Reason, "vague request, should have asked") {
		t.Fatalf("reason %q", evs[0].Reason)
	}
	page := ReportHTML(evs)
	if !strings.Contains(page, "Unrequested deletes") || !strings.Contains(page, "hello.py") {
		t.Fatalf("report\n%s", page)
	}
}

func TestHookUnrequestedDeleteCLIAndModes(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "cli", "command": "rm hello.py",
	}))
	if out["permission"] != "ask" {
		t.Fatalf("%#v", out)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 || evs[0].PromptKnown || !evs[0].Disputed || evs[0].Tool != "rm" {
		t.Fatalf("%#v", evs)
	}

	created := testPaths(t)
	mustHandle(t, created, hookJSON(t, map[string]any{
		"hook_event_name": "postToolUse", "conversation_id": "made", "tool_name": "Write",
		"tool_input": map[string]any{"path": "hello.py", "contents": "x\n"},
	}))
	got := mustHandle(t, created, hookJSON(t, map[string]any{
		"hook_event_name": "preToolUse", "conversation_id": "made", "tool_name": "Delete",
		"tool_input": map[string]any{"path": "hello.py"},
	}))
	if got["permission"] != "ask" || len(deleteEvents(t, created)) != 1 {
		t.Fatalf("created %#v", got)
	}

	logged := testPaths(t)
	if err := os.WriteFile(logged.Rules, []byte(`{"unrequested_delete":"log"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logged.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if perm := mustHandle(t, logged, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "log", "command": "rm hello.py",
	})); perm["permission"] != "allow" || len(deleteEvents(t, logged)) != 1 {
		t.Fatalf("log mode %#v", perm)
	}

	off := testPaths(t)
	if err := os.WriteFile(off.Rules, []byte(`{"unrequested_delete":"off"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(off.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if perm := mustHandle(t, off, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "off", "command": "rm hello.py",
	})); perm["permission"] != "allow" || len(deleteEvents(t, off)) != 0 {
		t.Fatalf("off %#v", perm)
	}
}

func TestHookInactiveAndPush(t *testing.T) {
	p := testPaths(t)
	t.Setenv("DRIFT_GUARD_ACTIVATION", "opt-in")
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if perm := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "asleep", "command": "rm hello.py",
	})); perm["permission"] != "allow" || len(deleteEvents(t, p)) != 0 {
		t.Fatalf("inactive %#v", perm)
	}
	mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeSubmitPrompt", "conversation_id": "awake", "prompt": "продолжи #dg работу",
	}))
	if perm := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "awake", "command": "rm hello.py",
	})); perm["permission"] != "ask" {
		t.Fatalf("active %#v", perm)
	}
	pushed := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "awake", "command": "rm hello.py && git push",
	}))
	if pushed["permission"] != "deny" {
		t.Fatalf("push %#v", pushed)
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

func TestHookUnrequestedDeleteCloudDeniesAsk(t *testing.T) {
	t.Setenv("CURSOR_AGENT_WORKER_ID", "worker-test-1")
	t.Setenv("CURSOR_WORKER_POOL_NAME", "")
	p := testPaths(t)
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "cloud", "command": "rm hello.py",
	}))
	if out["permission"] != "deny" {
		t.Fatalf("cloud should deny ask: %#v", out)
	}
	msg, _ := out["user_message"].(string)
	if !strings.Contains(msg, "cloud agent") && !strings.Contains(msg, "cannot prompt") {
		t.Fatalf("deny message: %q", msg)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 || evs[0].Mode != "deny" {
		t.Fatalf("event %#v", evs)
	}
	if !strings.Contains(evs[0].Reason, "cannot honor ask") {
		t.Fatalf("reason %q", evs[0].Reason)
	}

	// The local CLI sets CURSOR_AGENT, not a worker id, and must still ask.
	t.Setenv("CURSOR_AGENT_WORKER_ID", "")
	t.Setenv("CURSOR_WORKER_POOL_NAME", "")
	t.Setenv("CURSOR_AGENT", "1")
	t.Setenv("CURSOR_AGENT_CLI_LOCAL_MODE", "true")
	local := testPaths(t)
	if err := os.WriteFile(filepath.Join(local.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := mustHandle(t, local, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "local-cli", "command": "rm hello.py",
	}))
	if got["permission"] != "ask" {
		t.Fatalf("local CLI should still ask: %#v", got)
	}
}

func TestHookUnrequestedDeleteCloudLogModeAllows(t *testing.T) {
	t.Setenv("CURSOR_AGENT_WORKER_ID", "worker-test-1")
	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"unrequested_delete":"log"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "hello.py"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustHandle(t, p, hookJSON(t, map[string]any{
		"hook_event_name": "beforeShellExecution", "conversation_id": "cloud-log", "command": "rm hello.py",
	}))
	if out["permission"] != "allow" {
		t.Fatalf("log mode should not deny: %#v", out)
	}
	evs := deleteEvents(t, p)
	if len(evs) != 1 || evs[0].Mode != "log" {
		t.Fatalf("event %#v", evs)
	}
}
