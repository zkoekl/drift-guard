package drift

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func enableOptIn(t *testing.T) {
	t.Helper()
	t.Setenv("DRIFT_GUARD_ACTIVATION", "opt-in")
	t.Setenv("DRIFT_GUARD_TAG", "")
	t.Setenv("DRIFT_GUARD_ROOT", "")
	t.Setenv("CURSOR_PROJECT_DIR", "")
	t.Setenv("DRIFT_GUARD_REPLY_LANGUAGE", "")
	t.Setenv("DRIFT_GUARD_RULES", "")
}

func optInPaths(t *testing.T) (Paths, string) {
	t.Helper()
	project := t.TempDir()
	outside := t.TempDir()
	p := Paths{
		Log:   filepath.Join(outside, "events.jsonl"),
		Dump:  filepath.Join(outside, "hook-dump.jsonl"),
		State: filepath.Join(outside, "state"),
		Root:  project,
		Rules: filepath.Join(project, "unused-rules.json"),
	}
	return p, project
}

func listTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			lines = append(lines, rel+"/")
			return nil
		}
		lines = append(lines, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestActivationTagToken(t *testing.T) {
	cases := []struct {
		text string
		tag  string
		want bool
	}{
		{"#dg", "#dg", true},
		{"please #DG now", "#dg", true},
		{"(#dg)", "#dg", true},
		{"#dg, next", "#dg", true},
		{"#dgs", "#dg", false},
		{"x#dg", "#dg", false},
		{"my#dg", "#dg", false},
		{"#dg_more", "#dg", false},
		{"see #guard.", "#guard", true},
		{"#guardrail", "#guard", false},
		{"", "#dg", false},
		{"no tag", "#dg", false},
	}
	for _, tc := range cases {
		if got := hasActivationTag(tc.text, tc.tag); got != tc.want {
			t.Errorf("hasActivationTag(%q, %q) = %v, want %v", tc.text, tc.tag, got, tc.want)
		}
	}
}

func TestScanHookMetaSkipsNestedAndLargeOutput(t *testing.T) {
	raw := []byte(`{
		"tool_output": {"prompt": "#dg", "path": "/tmp/zzPathSecret"},
		"hook_event_name": "beforeSubmitPrompt",
		"conversation_id": "conv-1",
		"workspace_roots": ["/proj/a", "/proj/b"],
		"prompt": "please \u0023dg now",
		"blob": "` + strings.Repeat("Z", 50000) + `"
	}`)
	meta, err := scanHookMeta(raw)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Event != "beforeSubmitPrompt" || meta.ConversationID != "conv-1" {
		t.Fatalf("meta: %#v", meta)
	}
	if strings.Join(meta.WorkspaceRoots, ",") != "/proj/a,/proj/b" {
		t.Fatalf("roots: %#v", meta.WorkspaceRoots)
	}
	if meta.prompt() != "please #dg now" {
		t.Fatalf("prompt: %q", meta.prompt())
	}
	nested := []byte(`{"hook_event_name":"postToolUse","conversation_id":"c","tool_output":{"prompt":"#dg"}}`)
	meta, err = scanHookMeta(nested)
	if err != nil {
		t.Fatal(err)
	}
	if meta.prompt() != "" || meta.Event != "postToolUse" {
		t.Fatalf("nested prompt leaked: %#v", meta)
	}
	if _, err := scanHookMeta([]byte(`{`)); err == nil {
		t.Fatal("truncated json should fail")
	}
}

func TestOptInInactiveIsNeutralOnce(t *testing.T) {
	enableOptIn(t)
	p, project := optInPaths(t)
	if err := os.WriteFile(filepath.Join(project, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := listTree(t, project)
	now := time.Date(2026, 11, 10, 1, 2, 3, 0, time.UTC)
	secretPrompt := "zzPromptSecret please look at zzPathSecret"
	steps := []struct {
		name    string
		payload string
		want    string
	}{
		{"sessionStart", `{"hook_event_name":"sessionStart","conversation_id":"ina","workspace_roots":["/tmp/zzPathSecret"]}`, "{}"},
		{"beforeShell", `{"hook_event_name":"beforeShellExecution","conversation_id":"ina","command":"git push origin zzPathSecret","prompt":"` + secretPrompt + `"}`, "allow"},
		{"preTool", `{"hook_event_name":"preToolUse","conversation_id":"ina","tool_name":"Shell","tool_input":{"command":"git push"}}`, "allow"},
		{"preClaude", `{"hook_event_name":"PreToolUse","conversation_id":"ina","tool_name":"Bash","tool_input":{"command":"git push"}}`, "{}"},
		{"post", `{"hook_event_name":"postToolUse","conversation_id":"ina","tool_name":"Write","tool_input":{"path":"/tmp/zzPathSecret"}}`, "{}"},
		{"after", `{"hook_event_name":"afterAgentResponse","conversation_id":"ina","text":"#dg zzPromptSecret"}`, "{}"},
		{"prompt", `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"ina","prompt":"hello #dgs"}`, "{}"},
		{"userPrompt", `{"hook_event_name":"UserPromptSubmit","session_id":"ina","prompt":"still no token"}`, "{}"},
		{"stop", `{"hook_event_name":"stop","conversation_id":"ina","text":"done zzPromptSecret","transcript_path":"/tmp/zzPathSecret"}`, "{}"},
		{"stopClaude", `{"hook_event_name":"Stop","conversation_id":"ina","last_assistant_message":"done"}`, "{}"},
	}
	for _, ev := range steps {
		out, code := Handle([]byte(ev.payload), p, now)
		if code != 0 {
			t.Fatalf("%s exit %d", ev.name, code)
		}
		text := strings.TrimSpace(string(out))
		switch ev.want {
		case "allow":
			if text != `{"permission":"allow"}` {
				t.Fatalf("%s: %s", ev.name, text)
			}
		case "{}":
			if text != "{}" {
				t.Fatalf("%s: %s", ev.name, text)
			}
			var m map[string]any
			if err := json.Unmarshal(out, &m); err != nil {
				t.Fatal(err)
			}
			if _, ok := m["followup_message"]; ok {
				t.Fatalf("%s followed up: %s", ev.name, text)
			}
			if _, ok := m["permission"]; ok && ev.name != "beforeShell" && ev.name != "preTool" {
				t.Fatalf("%s permission: %s", ev.name, text)
			}
			if m["decision"] != nil || m["hookSpecificOutput"] != nil {
				t.Fatalf("%s not neutral: %s", ev.name, text)
			}
		}
	}
	if _, err := os.Stat(p.Dump); !os.IsNotExist(err) {
		t.Fatal("inactive session wrote a dump")
	}
	if listTree(t, project) != before {
		t.Fatalf("project dir changed:\n%s", listTree(t, project))
	}
	body, err := os.ReadFile(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "zzPromptSecret") || strings.Contains(string(body), "zzPathSecret") || strings.Contains(string(body), "git push") {
		t.Fatalf("inactive log leaked prompt or path:\n%s", body)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want one inactive event, got %d:\n%s", len(lines), body)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["kind"] != "session_inactive" || rec["session_id"] != "ina" || rec["source"] != "sessionStart" {
		t.Fatalf("note: %#v", rec)
	}
	if !strings.HasPrefix(rec["ts"].(string), "2026-11-10T01:02:03") {
		t.Fatalf("ts: %#v", rec["ts"])
	}
	for k := range rec {
		switch k {
		case "ts", "kind", "session_id", "source":
		default:
			t.Fatalf("extra field %s in %#v", k, rec)
		}
	}
	state, err := os.ReadFile(filepath.Join(p.State, "session-ina.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "zzPromptSecret") || strings.Contains(string(state), "zzPathSecret") {
		t.Fatalf("state leaked: %s", state)
	}
	// A second session gets its own single note, and invalid JSON writes nothing.
	out, code := Handle([]byte(`{"hook_event_name":"stop","conversation_id":"other"}`), p, now)
	if code != 0 || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("other: %d %s", code, out)
	}
	out, code = Handle([]byte("{"), p, now)
	if code != 0 || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("bad json: %d %s", code, out)
	}
	loaded, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].Kind != "session_inactive" || loaded[1].SessionID != "other" {
		t.Fatalf("events: %#v", loaded)
	}
}

func TestOptInTagAndProjectActivation(t *testing.T) {
	enableOptIn(t)
	p, _ := optInPaths(t)

	// No tag: push is allowed. A tag on a later prompt stays active and denies push.
	allow := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","conversation_id":"tag1","command":"git push"}`)
	if allow["permission"] != "allow" {
		t.Fatalf("inactive push: %#v", allow)
	}
	mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"tag1","prompt":"please #DG check"}`)
	mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"tag1","prompt":"continue without the tag"}`)
	deny := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","conversation_id":"tag1","command":"git push origin"}`)
	if deny["permission"] != "deny" {
		t.Fatalf("active push: %#v", deny)
	}
	bare := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"tag1","text":"This reply is long enough to be a final answer and it cites nothing."}`)
	if bare["followup_message"] != nil {
		t.Fatalf("active stop must not ask for markers by default: %#v", bare)
	}

	// A near-miss tag does not activate.
	near := mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"near","prompt":"see #dgs and x#dg"}`)
	if _, ok := near["followup_message"]; ok {
		t.Fatalf("near miss: %#v", near)
	}
	still := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","conversation_id":"near","command":"git push"}`)
	if still["permission"] != "allow" {
		t.Fatalf("near miss activated: %#v", still)
	}

	t.Setenv("DRIFT_GUARD_TAG", "#guard")
	miss := mustHandle(t, p, `{"hook_event_name":"UserPromptSubmit","session_id":"custom","prompt":"#guardrail"}`)
	if len(miss) != 0 {
		t.Fatalf("custom miss: %#v", miss)
	}
	hit := mustHandle(t, p, `{"hook_event_name":"UserPromptSubmit","session_id":"custom","prompt":"use #Guard here"}`)
	if len(hit) != 0 {
		t.Fatalf("custom hit output: %#v", hit)
	}
	customDeny := mustHandle(t, p, `{"hook_event_name":"PreToolUse","session_id":"custom","tool_name":"Bash","tool_input":{"command":"git push"}}`)
	spec, _ := customDeny["hookSpecificOutput"].(map[string]any)
	if spec["permissionDecision"] != "deny" {
		t.Fatalf("custom tag did not activate: %#v", customDeny)
	}

	// Project file activates without a tag, including via env and workspace_roots.
	proj := t.TempDir()
	rules := `{"reply_language":"en","test_command":"go test"}`
	if err := os.WriteFile(filepath.Join(proj, "drift-guard.rules.json"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DRIFT_GUARD_TAG", "")
	t.Setenv("DRIFT_GUARD_ROOT", proj)
	rooted := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","conversation_id":"byroot","command":"git push"}`)
	if rooted["permission"] != "deny" {
		t.Fatalf("DRIFT_GUARD_ROOT: %#v", rooted)
	}
	t.Setenv("DRIFT_GUARD_ROOT", "")
	t.Setenv("CURSOR_PROJECT_DIR", proj)
	cursed := mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"bycursor"}`)
	if len(cursed) != 0 {
		t.Fatalf("cursor dir output: %#v", cursed)
	}
	pushed := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","conversation_id":"bycursor","command":"git status && git push"}`)
	if pushed["permission"] != "deny" {
		t.Fatalf("CURSOR_PROJECT_DIR: %#v", pushed)
	}
	t.Setenv("CURSOR_PROJECT_DIR", "")
	other := t.TempDir()
	p2, project2 := optInPaths(t)
	_ = project2
	p2.Root = other
	ws := mustHandle(t, p2, `{"hook_event_name":"beforeShellExecution","conversation_id":"byws","command":"git push","workspace_roots":[`+jsonString(proj)+`]}`)
	if ws["permission"] != "deny" {
		t.Fatalf("workspace_roots: %#v", ws)
	}

	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	var sawTag, sawProject bool
	for _, ev := range events {
		if ev.Kind != "session_active" {
			continue
		}
		switch ev.SessionID {
		case "tag1":
			if ev.Activation != "tag" || ev.Source != "beforeSubmitPrompt" {
				t.Fatalf("tag activation: %#v", ev)
			}
			sawTag = true
		case "byroot", "bycursor":
			if ev.Activation != "project" {
				t.Fatalf("project activation: %#v", ev)
			}
			sawProject = true
		}
	}
	if !sawTag || !sawProject {
		t.Fatalf("missing activation events tag=%v project=%v\n%#v", sawTag, sawProject, events)
	}
	page := ReportHTML(events)
	if !strings.Contains(page, "Active sessions:") || !strings.Contains(page, "Inactive sessions:") {
		t.Fatal("report missing session counts")
	}
	if !strings.Contains(page, ">tag<") || !strings.Contains(page, ">project<") {
		t.Fatalf("report activations:\n%s", page)
	}
	if strings.Contains(page, "http://") || strings.Contains(page, "https://") {
		t.Fatal("report contains a network URL")
	}
}

func TestOptInGlobalModeWritesNothingInProject(t *testing.T) {
	enableOptIn(t)
	p, project := optInPaths(t)
	now := time.Date(2026, 11, 10, 7, 8, 9, 0, time.UTC)
	if err := os.WriteFile(filepath.Join(project, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := listTree(t, project)
	big := strings.Repeat("Q", 200000)
	payload := `{"hook_event_name":"postToolUse","conversation_id":"quiet","tool_output":"` + big + `","user_email":"secret@example.com","prompt":"#dg"}`
	out, code := Handle([]byte(payload), p, now)
	if code != 0 || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("big inactive: %d %s", code, out)
	}
	if listTree(t, project) != before {
		t.Fatalf("inactive wrote into the project:\n%s", listTree(t, project))
	}
	logBody, err := os.ReadFile(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logBody), "secret@example.com") || strings.Contains(string(logBody), strings.Repeat("Q", 100)) || strings.Contains(string(logBody), "#dg") {
		t.Fatal("inactive log kept payload bytes")
	}

	// Active via tag only: still no events.jsonl or .drift-guard inside the project.
	mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"quiet","prompt":"turn on #dg"}`)
	mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"quiet","text":"This reply is long enough to be a final answer and it cites nothing."}`)
	if listTree(t, project) != before {
		t.Fatalf("active tag wrote into the project:\n%s", listTree(t, project))
	}
	if _, err := os.Stat(p.Dump); err != nil {
		t.Fatal("active session should dump outside the project")
	}
	if _, err := os.Stat(filepath.Join(project, ".drift-guard")); !os.IsNotExist(err) {
		t.Fatal(".drift-guard appeared in the project")
	}
	if _, err := os.Stat(filepath.Join(project, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatal("events.jsonl appeared in the project")
	}

	// Project rules with a canary: secret stays in the global state dir.
	// sessionStart does not see the project. The tag on the next prompt does,
	// and that is when the canary line is refreshed.
	canaryRoot := t.TempDir()
	outside := t.TempDir()
	cp := Paths{
		Log:   filepath.Join(outside, "events.jsonl"),
		Dump:  filepath.Join(outside, "hook-dump.jsonl"),
		State: filepath.Join(outside, "state"),
		Root:  t.TempDir(),
		Rules: filepath.Join(canaryRoot, "drift-guard.rules.json"),
	}
	rules := `{"require_markers":true,"canary":{"files":["AGENTS.md"]}}`
	if err := os.WriteFile(filepath.Join(canaryRoot, "drift-guard.rules.json"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(canaryRoot, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# Rules\n\ndrift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeCanary := listTree(t, canaryRoot)
	mustHandle(t, cp, `{"hook_event_name":"sessionStart","conversation_id":"can1"}`)
	body, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "drift-guard-canary: pending") {
		t.Fatalf("sessionStart refreshed too early: %s", body)
	}
	if _, err := os.Stat(filepath.Join(cp.State, "canary-secret")); !os.IsNotExist(err) {
		t.Fatal("secret written before activation")
	}
	if listTree(t, canaryRoot) != beforeCanary {
		t.Fatalf("sessionStart wrote into the project:\n%s", listTree(t, canaryRoot))
	}
	prompt := `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"can1","prompt":"enable #dg","workspace_roots":[` + jsonString(canaryRoot) + `]}`
	started := mustHandle(t, cp, prompt)
	raw, _ := json.Marshal(started)
	secret, err := os.ReadFile(filepath.Join(cp.State, "canary-secret"))
	if err != nil {
		t.Fatal(err)
	}
	canary := CanaryCode("can1", "AGENTS.md", strings.TrimSpace(string(secret)))
	body, err = os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "drift-guard-canary: "+canary) {
		t.Fatalf("tag prompt did not refresh canary: %s", body)
	}
	if strings.Contains(string(raw), canary) || strings.Contains(string(raw), strings.TrimSpace(string(secret))) {
		t.Fatal("stdout leaked the canary")
	}
	if _, err := os.Stat(filepath.Join(canaryRoot, ".drift-guard")); !os.IsNotExist(err) {
		t.Fatal("canary secret dir written into the project")
	}
	if _, err := os.Stat(filepath.Join(canaryRoot, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatal("events written into the canary project")
	}
	logged, err := os.ReadFile(cp.Log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), canary) || strings.Contains(string(logged), strings.TrimSpace(string(secret))) || strings.Contains(string(logged), canaryRoot) {
		t.Fatalf("log leaked canary or project path:\n%s", logged)
	}
	found, err := LoadEvents(cp.Log)
	if err != nil {
		t.Fatal(err)
	}
	var activated bool
	for _, ev := range found {
		if ev.Kind == "session_active" && ev.SessionID == "can1" && ev.Activation == "project+tag" && ev.Source == "beforeSubmitPrompt" {
			activated = true
		}
	}
	if !activated {
		t.Fatalf("canary activation: %#v", found)
	}
	// A later stop still sees the project rules even without workspace_roots.
	// The saved root is outside this empty p.Root.
	if cp.Root == canaryRoot {
		t.Fatal("test mutated the caller's root permanently; the next call starts from Paths")
	}
}

func TestOptInProjectRulesSurviveLaterEvents(t *testing.T) {
	enableOptIn(t)
	proj := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "drift-guard.rules.json"), []byte(`{"require_markers":true,"reply_language":"ru"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Paths{
		Log:   filepath.Join(outside, "events.jsonl"),
		Dump:  filepath.Join(outside, "dump.jsonl"),
		State: filepath.Join(outside, "state"),
		Root:  t.TempDir(),
	}
	mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"keep","prompt":"no tag","workspace_roots":[`+jsonString(proj)+`]}`)
	// Next call does not repeat workspace_roots. Rules still apply.
	bare := "This reply is long enough to be a final answer and it cites nothing."
	out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"keep","text":`+jsonString(bare)+`}`)
	msg, _ := out["followup_message"].(string)
	if !strings.Contains(msg, "добавьте") || strings.Contains(msg, "add non-empty lines") {
		t.Fatalf("saved project language: %q", msg)
	}
}

func TestFollowupLanguage(t *testing.T) {
	t.Setenv("DRIFT_GUARD_ACTIVATION", "")
	t.Setenv("DRIFT_GUARD_REPLY_LANGUAGE", "")
	bare := "This reply is long enough to be a final answer and it cites nothing."

	p := testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"require_markers":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	en := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"en","text":`+jsonString(bare)+`}`)
	msg, _ := en["followup_message"].(string)
	if !strings.Contains(msg, "add non-empty lines") || !strings.Contains(msg, "ЦЕЛЬ:") {
		t.Fatalf("english markers: %q", msg)
	}

	p = testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"require_markers":true,"reply_language":"ru"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ru := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"ru","text":`+jsonString(bare)+`}`)
	msg, _ = ru["followup_message"].(string)
	if !strings.Contains(msg, "добавьте непустые строки") || strings.Contains(msg, "add non-empty lines") {
		t.Fatalf("russian markers: %q", msg)
	}
	if utf8.RuneCountInString(msg) > 240 || len(msg) > 240 {
		t.Fatalf("marker cap: %d bytes %d runes", len(msg), utf8.RuneCountInString(msg))
	}

	p = testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"reply_language":"RU","test_command":"pytest","test_path_globs":["**/*.py"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	marked := "ЦЕЛЬ: ship\nИСТОЧНИК: a.py:1\nДОКАЗАНО: pytest\n"
	testMsg := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"pytest","text":`+jsonString(marked)+`,"changed_paths":["a.py"]}`)
	msg, _ = testMsg["followup_message"].(string)
	if !strings.Contains(msg, "Запустите") || !strings.Contains(msg, "pytest") || strings.Contains(msg, "did not exit 0") {
		t.Fatalf("russian test followup: %q", msg)
	}

	p = testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"require_markers":true,"reply_language":"ru","canary":{"files":["AGENTS.md"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "AGENTS.md"), []byte("drift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"can-ru"}`)
	canaryMsg := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"can-ru","text":`+jsonString(marked)+`}`)
	msg, _ = canaryMsg["followup_message"].(string)
	if !strings.Contains(msg, "перечитайте") || !strings.Contains(msg, "AGENTS.md") || strings.Contains(msg, "reread") {
		t.Fatalf("russian canary: %q", msg)
	}
	if utf8.RuneCountInString(msg) > 240 {
		t.Fatalf("canary rune cap: %d", utf8.RuneCountInString(msg))
	}

	t.Setenv("DRIFT_GUARD_REPLY_LANGUAGE", "ru")
	p = testPaths(t)
	if err := os.WriteFile(p.Rules, []byte(`{"require_markers":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fromEnv := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"env","text":`+jsonString(bare)+`}`)
	msg, _ = fromEnv["followup_message"].(string)
	if !strings.Contains(msg, "добавьте") {
		t.Fatalf("env language: %q", msg)
	}

	if err := os.WriteFile(p.Rules, []byte(`{"require_markers":true,"reply_language":"en"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rulesWin := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"rules-win","text":`+jsonString(bare)+`}`)
	msg, _ = rulesWin["followup_message"].(string)
	if !strings.Contains(msg, "add non-empty lines") {
		t.Fatalf("rules should beat env: %q", msg)
	}
	payloadWins := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"payload","reply_language":"ru","text":`+jsonString(bare)+`}`)
	msg, _ = payloadWins["followup_message"].(string)
	if !strings.Contains(msg, "добавьте") {
		t.Fatalf("payload should beat rules: %q", msg)
	}

	// Marker text, test text, and a long canary list together stay inside the cap
	// and do not split a rune.
	longCmd := strings.Repeat("y", 80)
	var files []string
	for i := 0; i < 30; i++ {
		files = append(files, "docs/rules/file-"+strings.Repeat("й", 8)+".md")
	}
	rules, err := json.Marshal(map[string]any{
		"require_markers": true,
		"reply_language":  "ru",
		"test_command":    longCmd,
		"test_path_globs": []string{"**/*.py"},
		"canary":          map[string]any{"files": files},
	})
	if err != nil {
		t.Fatal(err)
	}
	p = testPaths(t)
	if err := os.WriteFile(p.Rules, rules, 0o644); err != nil {
		t.Fatal(err)
	}
	mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"cap-ru"}`)
	capped := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"cap-ru","text":"short","changed_paths":["a.py"]}`)
	msg, _ = capped["followup_message"].(string)
	if msg == "" || len(msg) > 500 || !utf8.ValidString(msg) {
		t.Fatalf("cap len=%d valid=%v msg=%q", len(msg), utf8.ValidString(msg), msg)
	}
}

func TestReportSessionCounts(t *testing.T) {
	events := []Event{
		{TS: "2026-11-10T00:00:00Z", Kind: "session_inactive", SessionID: "quiet", Source: "sessionStart"},
		{TS: "2026-11-10T00:00:01Z", Kind: "session_inactive", SessionID: "later", Source: "stop"},
		{TS: "2026-11-10T00:00:02Z", Kind: "session_active", SessionID: "later", Source: "beforeSubmitPrompt", Activation: "tag"},
		{TS: "2026-11-10T00:00:03Z", Kind: "session_active", SessionID: "proj", Source: "sessionStart", Activation: "project"},
		{TS: "2026-11-10T00:00:04Z", Kind: "verdict", SessionID: "proj", Source: "hook-stop", Mode: "log"},
	}
	page := ReportHTML(events)
	if !strings.Contains(page, "Active sessions: 2.") || !strings.Contains(page, "Inactive sessions: 1.") {
		t.Fatalf("counts:\n%s", page)
	}
	if !strings.Contains(page, ">later<") || !strings.Contains(page, ">tag<") || !strings.Contains(page, ">project<") {
		t.Fatalf("table:\n%s", page)
	}
	if strings.Contains(page, ">quiet<") {
		t.Fatal("inactive session listed in the active table")
	}
}
