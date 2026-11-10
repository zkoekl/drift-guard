package drift

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	dir := t.TempDir()
	return Paths{
		Log:   filepath.Join(dir, "events.jsonl"),
		Dump:  filepath.Join(dir, "dump.jsonl"),
		State: filepath.Join(dir, "state"),
		Rules: filepath.Join(dir, "rules.json"),
		Root:  dir,
	}
}

func mustHandle(t *testing.T, p Paths, payload string) map[string]any {
	t.Helper()
	out, code := Handle([]byte(payload), p, time.Now().UTC())
	if code != 0 {
		t.Fatalf("exit %d stdout %s", code, out)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("stdout %s: %v", out, err)
	}
	return m
}

func TestHookDeniesGitPush(t *testing.T) {
	p := testPaths(t)
	cursor := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","command":"git push origin","user_email":"secret@example.com","conversation_id":"c1"}`)
	if cursor["permission"] != "deny" {
		t.Fatalf("cursor deny: %#v", cursor)
	}
	dump, err := os.ReadFile(p.Dump)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dump), "secret@example.com") {
		t.Fatal("user_email leaked into the dump")
	}
	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !hasRule(events[0], "git-push") {
		t.Fatalf("events: %#v", events)
	}

	claude := mustHandle(t, p, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git status && git push"},"session_id":"s1"}`)
	spec, _ := claude["hookSpecificOutput"].(map[string]any)
	if spec["permissionDecision"] != "deny" {
		t.Fatalf("claude deny: %#v", claude)
	}

	allow := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","command":"git status","conversation_id":"c1"}`)
	if allow["permission"] != "allow" {
		t.Fatalf("allow: %#v", allow)
	}
	quoted := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","command":"echo \"git push\"","conversation_id":"c1"}`)
	if quoted["permission"] != "allow" {
		t.Fatalf("quoted: %#v", quoted)
	}
}

func TestStopFollowupCapAndTranscript(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"reply_language":"ru","test_command":"pytest","test_path_globs":["**/*.py"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	english := "This report covers three numbered rounds. Each round is written in English and describes the series."
	mustHandle(t, p, `{"hook_event_name":"afterAgentResponse","conversation_id":"cap","text":`+jsonString(english)+`}`)
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"cap","tool_name":"Write","tool_input":{"path":"src/a.py"},"tool_output":"ok"}`)

	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte("{\"role\":\"user\",\"text\":\"go\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stop := `{"hook_event_name":"stop","conversation_id":"cap","loop_count":0,"status":"completed","transcript_path":` + jsonString(transcript) + `}`

	first := mustHandle(t, p, stop)
	if first["followup_message"] == "" || first["followup_message"] == nil {
		t.Fatalf("first followup missing: %#v", first)
	}
	second := mustHandle(t, p, stop)
	if second["followup_message"] == nil || second["followup_message"] == "" {
		t.Fatalf("second followup missing: %#v", second)
	}
	third := mustHandle(t, p, stop)
	if _, ok := third["followup_message"]; ok {
		t.Fatalf("third followup should be absent: %#v", third)
	}

	dump, err := os.ReadFile(p.Dump)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dump), "does not contain the assistant text") {
		t.Fatalf("expected an incomplete transcript note:\n%s", dump)
	}
	if err := os.WriteFile(transcript, []byte("{\"role\":\"assistant\",\"text\":"+jsonString(english)+"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A new session so the follow-up cap does not matter. The text is already in state.
	mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"cap2","transcript_path":`+jsonString(transcript)+`,"text":`+jsonString(english)+`}`)
	dump, err = os.ReadFile(p.Dump)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dump), "transcript contains the assistant text from this turn") {
		t.Fatalf("expected a complete transcript note:\n%s", dump)
	}

	// Markers are present and the cited file was read, so the only finding is
	// reply language. That finding stays log-only.
	marked := english + "\nЦЕЛЬ: three numbered rounds\nИСТОЧНИК: series.md:1\nДОКАЗАНО: true\n"
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"lang-only","tool_name":"Read","tool_input":{"path":"series.md"}}`)
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"lang-only","tool_name":"Shell","tool_input":{"command":"true"},"tool_output":{"exitCode":0}}`)
	langOnly := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"lang-only","text":`+jsonString(marked)+`}`)
	if _, ok := langOnly["followup_message"]; ok {
		t.Fatalf("language finding must not follow up: %#v", langOnly)
	}
	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	var sawLang, sawDone bool
	for _, ev := range events {
		if hasRule(ev, "reply-language") {
			sawLang = true
		}
		if hasRule(ev, "done-without-tests") {
			sawDone = true
			if ev.Mode != "block" {
				t.Fatalf("done-without-tests mode: %#v", ev)
			}
		}
	}
	if !sawLang || !sawDone {
		t.Fatalf("missing findings lang=%v done=%v", sawLang, sawDone)
	}
}

func TestLoopCountSuppressesFollowup(t *testing.T) {
	p := testPaths(t)
	out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"loop","loop_count":2,"test_command":"pytest","changed_paths":["a.py"]}`)
	if _, ok := out["followup_message"]; ok {
		t.Fatalf("loop_count 2 should not follow up: %#v", out)
	}
}

func TestClaudeStopContinues(t *testing.T) {
	p := testPaths(t)
	out := mustHandle(t, p, `{"hook_event_name":"Stop","session_id":"cl","test_command":"pytest","changed_paths":["a.py"],"last_assistant_message":"The cleanup is done."}`)
	if out["decision"] != "block" || out["reason"] == "" {
		t.Fatalf("claude stop: %#v", out)
	}
}

func TestFlip(t *testing.T) {
	ok, _ := Flip([]Message{
		{Role: "assistant", Text: "The correct approach is to edit the file in place."},
		{Role: "user", Text: "No. That is wrong."},
		{Role: "assistant", Text: "You are right, I was wrong. I will switch to a different approach."},
	})
	if !ok {
		t.Fatal("expected a flip")
	}
	ok, _ = Flip([]Message{
		{Role: "assistant", Text: "The correct approach is to edit the file in place."},
		{Role: "user", Text: "No. That is wrong."},
		{Role: "assistant", Text: "You are right, I was wrong.", ToolCalls: []string{"read"}},
	})
	if ok {
		t.Fatal("a tool call is new evidence")
	}
	ok, _ = Flip([]Message{
		{Role: "assistant", Text: "The correct approach is to edit the file in place."},
		{Role: "user", Text: "Please continue."},
		{Role: "assistant", Text: "Still editing in place."},
	})
	if ok {
		t.Fatal("no objection")
	}
}

func TestReportHasFiveChartsAndNoURL(t *testing.T) {
	events := []Event{
		{
			TS: "2026-11-09T00:00:00Z", Kind: "verdict", SessionID: "s", Source: "scan",
			Channel: "conversation", Mode: "log", Violations: 1,
			RuleIDs: []string{"flip-after-pushback"}, Reason: "conclusion changed",
			TokensJudge: 0,
		},
		{
			TS: "2026-11-09T00:00:01Z", Kind: "verdict", SessionID: "s", Source: "cli",
			Channel: "code", Mode: "block", Violations: 1,
			RuleIDs: []string{"done-without-tests"}, Reason: "no test",
		},
	}
	page := ReportHTML(events)
	for _, title := range []string{
		"Catches over time",
		"False alarms over time",
		"Precision and recall",
		"User-caught vs tool-caught",
		"Tokens per session",
		"Rule churn",
		"Ambiguous",
	} {
		if !strings.Contains(page, title) {
			t.Errorf("missing %q", title)
		}
	}
	if strings.Contains(page, "http://") || strings.Contains(page, "https://") {
		t.Fatal("report contains a network URL")
	}
	if !strings.Contains(page, "flip-after-pushback") && !strings.Contains(page, "conclusion changed") {
		t.Fatal("ambiguous row missing")
	}
	empty := ReportHTML(nil)
	if strings.Count(empty, "No data yet.") < 5 {
		t.Fatalf("empty states: %d", strings.Count(empty, "No data yet."))
	}
}

func TestScopeFixtureShape(t *testing.T) {
	globs := []string{"gameplay/**"}
	outside := Outside([]string{".github/workflows/build.yml", "deploy/Dockerfile"}, globs)
	if len(outside) != 2 {
		t.Fatalf("outside: %v", outside)
	}
	if len(Outside([]string{"gameplay/loop.js"}, globs)) != 0 {
		t.Fatal("in-scope file flagged")
	}
	body := "<!-- drift-guard-scope:start -->\n- cmd/**\n- docs/**\n<!-- drift-guard-scope:end -->\n"
	got, src := ResolveGlobs(t.TempDir(), "", body)
	if src != "pull request body" || len(got) != 2 || got[0] != "cmd/**" {
		t.Fatalf("body scope %q %v", src, got)
	}
	dir := t.TempDir()
	plan := "---\nscope:\n  - internal/**\n---\n\nProse is ignored.\n"
	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	got, src = ResolveGlobs(dir, "", "")
	if src != "plan.md" || len(got) != 1 {
		t.Fatalf("plan scope %q %v", src, got)
	}
}

func TestScanFixtures(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")
	cases := []struct {
		file string
		want []string
	}{
		{"fixtures/phase0/transcripts/reply-language-synthetic.json", []string{"reply-language", "answer-markers"}},
		{"fixtures/phase0/transcripts/restart-claim-synthetic.json", []string{"answer-markers"}},
		{"fixtures/phase0/transcripts/flip-synthetic.json", []string{"answer-markers", "sycophancy"}},
		{"fixtures/phase0/transcripts/canary-synthetic.json", []string{"answer-markers", "answered-from-memory", "canary-read"}},
	}
	for _, tc := range cases {
		res, err := ScanFile(filepath.Join(root, tc.file), logPath, time.Now().UTC())
		if err != nil {
			t.Fatal(tc.file, err)
		}
		if res.Exit != 0 {
			t.Fatalf("%s exit %d", tc.file, res.Exit)
		}
		if strings.Join(res.RuleIDs, ",") != strings.Join(tc.want, ",") {
			t.Fatalf("%s rules %v want %v", tc.file, res.RuleIDs, tc.want)
		}
	}
	events, err := LoadEvents(logPath)
	if err != nil {
		t.Fatal(err)
	}
	page := ReportHTML(events)
	if strings.Contains(page, "http://") || strings.Contains(page, "https://") {
		t.Fatal("scan report contains a network URL")
	}
	htmlPath := filepath.Join(dir, "report.html")
	if err := os.WriteFile(htmlPath, []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
