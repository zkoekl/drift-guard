package drift

import (
	"os"
	"testing"
)

// Claude Code PostToolUse input (https://code.claude.com/docs/en/hooks)
// carries the result in tool_response. For Bash the documented shape is
// {stdout, stderr, interrupted, isImage}, with no exit code. PostToolUse
// fires after a successful tool call. Reading only tool_output used to miss
// a passing test run and fire done-without-tests and marker-citation.
func TestClaudeToolResponseCountsAsPassingTest(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"test_command":"go test","test_path_globs":["**/*.go"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	const sid = "claude-tr"
	mustHandle(t, p, `{"hook_event_name":"PostToolUse","session_id":"`+sid+`","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"stdout":"PASS\nok\texample\t0.12s\n","stderr":"","interrupted":false,"isImage":false}}`)
	mustHandle(t, p, `{"hook_event_name":"PostToolUse","session_id":"`+sid+`","tool_name":"Edit","tool_input":{"file_path":"widget.go"}}`)
	mustHandle(t, p, `{"hook_event_name":"PostToolUse","session_id":"`+sid+`","tool_name":"Read","tool_input":{"path":"widget.go"}}`)
	text := "Updated widget.go.\nЦЕЛЬ: fix widget\nИСТОЧНИК: widget.go:1\nДОКАЗАНО: go test ./...\n"
	out := mustHandle(t, p, `{"hook_event_name":"Stop","session_id":"`+sid+`","text":`+jsonString(text)+`}`)
	if _, ok := out["followup_message"]; ok {
		t.Fatalf("clean claude reply followed up: %#v", out)
	}
	if out["decision"] != nil && out["decision"] != "" {
		t.Fatalf("clean claude reply blocked: %#v", out)
	}
	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if hasRule(ev, "done-without-tests") || hasRule(ev, "marker-citation") {
			t.Fatalf("false alarm after tool_response: %#v", ev)
		}
	}
}

func TestClaudeInterruptedToolResponseIsNotSuccess(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"test_command":"go test","test_path_globs":["**/*.go"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	const sid = "claude-int"
	mustHandle(t, p, `{"hook_event_name":"PostToolUse","session_id":"`+sid+`","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"stdout":"","stderr":"","interrupted":true,"isImage":false}}`)
	mustHandle(t, p, `{"hook_event_name":"PostToolUse","session_id":"`+sid+`","tool_name":"Edit","tool_input":{"file_path":"widget.go"}}`)
	text := "Updated widget.go.\nЦЕЛЬ: fix widget\nИСТОЧНИК: widget.go:1\nДОКАЗАНО: go test ./...\n"
	mustHandle(t, p, `{"hook_event_name":"Stop","session_id":"`+sid+`","text":`+jsonString(text)+`}`)
	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	var sawDone, sawCite bool
	for _, ev := range events {
		if hasRule(ev, "done-without-tests") {
			sawDone = true
		}
		if hasRule(ev, "marker-citation") {
			sawCite = true
		}
	}
	if !sawDone || !sawCite {
		t.Fatalf("interrupted command should not count as proof: done=%v cite=%v events=%#v", sawDone, sawCite, events)
	}
}
