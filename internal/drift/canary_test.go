package drift

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCanaryCodeIsSessionStable(t *testing.T) {
	const secret = "test-secret"
	a := CanaryCode("sess-a", "AGENTS.md", secret)
	if a != CanaryCode("sess-a", "./AGENTS.md", secret) {
		t.Fatal("normalized path must not change the code")
	}
	if a != CanaryCode("sess-a", "AGENTS.md", secret) {
		t.Fatal("code must be stable")
	}
	if a == CanaryCode("sess-b", "AGENTS.md", secret) {
		t.Fatal("another session must not reuse the code")
	}
	if a == CanaryCode("sess-a", "skills/example/SKILL.md", secret) {
		t.Fatal("another file must not reuse the code")
	}
	if len(a) != 8 {
		t.Fatalf("code length %d", len(a))
	}
}

func TestCanaryQuotes(t *testing.T) {
	const secret = "test-secret"
	code := CanaryCode("sess-a", "AGENTS.md", secret)
	stale := CanaryCode("sess-b", "AGENTS.md", secret)
	skill := CanaryCode("sess-a", "skills/example/SKILL.md", secret)
	files := []string{"AGENTS.md", "skills/example/SKILL.md"}

	clean := CheckCanary(CanaryInput{
		Text:      "ПРОЧИТАНО: AGENTS.md " + code + "\nREAD: skills/example/SKILL.md " + skill + "\n",
		Files:     files,
		SessionID: "sess-a",
		Secret:    secret,
		Reads:     []string{"AGENTS.md", "skills/example/SKILL.md"},
	})
	if len(clean) != 0 {
		t.Fatalf("correct codes should pass: %#v", clean)
	}

	readAlias := CheckCanary(CanaryInput{
		Text:      "read: AGENTS.md " + strings.ToUpper(code),
		Files:     []string{"AGENTS.md"},
		SessionID: "sess-a",
		Secret:    secret,
		Reads:     []string{"AGENTS.md"},
	})
	if len(readAlias) != 0 {
		t.Fatalf("READ alias should pass: %#v", readAlias)
	}

	wrong := CheckCanary(CanaryInput{
		Text:      "ПРОЧИТАНО: AGENTS.md " + stale,
		Files:     []string{"AGENTS.md"},
		SessionID: "sess-a",
		Secret:    secret,
		Reads:     []string{"AGENTS.md"},
	})
	if len(wrong) != 1 || wrong[0].ID != "canary-code" || !wrong[0].Followup || wrong[0].Disputed {
		t.Fatalf("stale session code: %#v", wrong)
	}
	if strings.Contains(wrong[0].Reason, code) || strings.Contains(canaryFollowup(wrong, ""), code) {
		t.Fatal("follow-up leaked the session code")
	}

	missing := CheckCanary(CanaryInput{
		Text:      "done",
		Files:     []string{"AGENTS.md"},
		SessionID: "sess-a",
		Secret:    secret,
		Reads:     []string{"AGENTS.md"},
	})
	if len(missing) != 1 || missing[0].ID != "canary-code" || !missing[0].Followup {
		t.Fatalf("missing marker: %#v", missing)
	}

	noRead := CheckCanary(CanaryInput{
		Text:      "ПРОЧИТАНО: AGENTS.md " + code,
		Files:     []string{"AGENTS.md"},
		SessionID: "sess-a",
		Secret:    secret,
	})
	if len(noRead) != 1 || noRead[0].ID != "canary-read" || !noRead[0].Disputed || noRead[0].Followup {
		t.Fatalf("marker without read log: %#v", noRead)
	}

	if got := CheckCanary(CanaryInput{Text: "ПРОЧИТАНО: AGENTS.md " + code, SessionID: "sess-a", Secret: secret}); got != nil {
		t.Fatal("empty file list must stay off")
	}
}

func TestCanaryRefreshRewritesOnlyInsideRoot(t *testing.T) {
	root := t.TempDir()
	secret := "test-secret"
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# Rules\n\ndrift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("drift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := RefreshCanaries(root, []string{"AGENTS.md", "../" + filepath.Base(outside)}, "sess-a", secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "AGENTS.md" {
		t.Fatalf("updated %#v", res)
	}
	if len(res.Missing) != 1 {
		t.Fatalf("escape should be skipped: %#v", res.Missing)
	}
	body, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	code := CanaryCode("sess-a", "AGENTS.md", secret)
	if !strings.Contains(string(body), "drift-guard-canary: "+code) {
		t.Fatalf("rewritten file: %s", body)
	}
	if strings.Contains(string(body), "pending") {
		t.Fatalf("placeholder left in place: %s", body)
	}
	again, err := RefreshCanaries(root, []string{"AGENTS.md"}, "sess-a", secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Updated) != 1 {
		t.Fatal(again)
	}
	body2, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if string(body2) != string(body) {
		t.Fatal("refresh in the same session should be idempotent")
	}
	other := CanaryCode("sess-b", "AGENTS.md", secret)
	if _, err := RefreshCanaries(root, []string{"AGENTS.md"}, "sess-b", secret); err != nil {
		t.Fatal(err)
	}
	body3, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body3), other) || strings.Contains(string(body3), code) {
		t.Fatalf("other session: %s", body3)
	}
	out, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), code) || strings.Contains(string(out), other) {
		t.Fatal("refresh wrote outside the root")
	}

	missing, err := RefreshCanaries(root, []string{"skills/example/SKILL.md"}, "sess-a", secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing.Missing) != 1 || len(missing.Updated) != 0 {
		t.Fatalf("missing file should not be created: %#v", missing)
	}
	if _, err := os.Stat(filepath.Join(root, "skills/example/SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("missing skill file was created")
	}
}

func TestCanaryHookStop(t *testing.T) {
	secret := "test-secret"
	writeCanaryProject := func(t *testing.T) Paths {
		t.Helper()
		p := testPaths(t)
		if err := os.MkdirAll(p.State, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(secretPath(p), []byte(secret+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		rules := `{"canary":{"files":["AGENTS.md","skills/example/SKILL.md"]}}`
		if err := os.WriteFile(p.Rules, []byte(rules), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.Root, "AGENTS.md"), []byte("# Rules\n\ndrift-guard-canary: pending\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		skillDir := filepath.Join(p.Root, "skills", "example")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("drift-guard-canary: pending\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("correct code passes", func(t *testing.T) {
		p := writeCanaryProject(t)
		start := mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"sess-a"}`)
		raw, _ := json.Marshal(start)
		code := CanaryCode("sess-a", "AGENTS.md", secret)
		skill := CanaryCode("sess-a", "skills/example/SKILL.md", secret)
		if strings.Contains(string(raw), code) || strings.Contains(string(raw), skill) || strings.Contains(string(raw), secret) {
			t.Fatalf("stdout leaked the code: %s", raw)
		}
		dump, err := os.ReadFile(p.Dump)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(dump), code) || strings.Contains(string(dump), secret) {
			t.Fatal("dump leaked the code or the secret")
		}
		body, err := os.ReadFile(filepath.Join(p.Root, "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "drift-guard-canary: "+code) {
			t.Fatalf("file was not refreshed: %s", body)
		}
		mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"sess-a","prompt":"Please continue."}`)
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-a","tool_name":"Read","tool_input":{"path":"AGENTS.md"}}`)
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-a","tool_name":"Read","tool_input":{"path":"skills/example/SKILL.md"}}`)
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-a","tool_name":"Shell","tool_input":{"command":"go test"},"tool_output":{"exitCode":0}}`)
		reply := "ПРОЧИТАНО: AGENTS.md " + code + "\nREAD: skills/example/SKILL.md " + skill +
			"\nЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
		out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sess-a","text":`+jsonString(reply)+`}`)
		if _, ok := out["followup_message"]; ok {
			t.Fatalf("correct canary followed up: %#v", out)
		}
		events, err := LoadEvents(p.Log)
		if err != nil {
			t.Fatal(err)
		}
		last := events[len(events)-1]
		if hasRule(last, "canary-code") || hasRule(last, "canary-read") || last.Violations != 0 {
			t.Fatalf("correct stop: %#v", last)
		}
	})

	t.Run("stale code follows up", func(t *testing.T) {
		p := writeCanaryProject(t)
		mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"sess-a"}`)
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-a","tool_name":"Shell","tool_input":{"command":"go test"},"tool_output":{"exitCode":0}}`)
		stale := CanaryCode("sess-b", "AGENTS.md", secret)
		current := CanaryCode("sess-a", "AGENTS.md", secret)
		reply := "ПРОЧИТАНО: AGENTS.md " + stale + "\nПРОЧИТАНО: skills/example/SKILL.md " + stale +
			"\nЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-a","tool_name":"Read","tool_input":{"path":"AGENTS.md"}}`)
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-a","tool_name":"Read","tool_input":{"path":"skills/example/SKILL.md"}}`)
		out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sess-a","text":`+jsonString(reply)+`}`)
		msg, _ := out["followup_message"].(string)
		if !strings.Contains(msg, "reread") || !strings.Contains(msg, "AGENTS.md") {
			t.Fatalf("stale followup: %#v", out)
		}
		if strings.Contains(msg, current) || strings.Contains(msg, stale) || strings.Contains(msg, secret) {
			t.Fatalf("followup leaked a code: %s", msg)
		}
		events, err := LoadEvents(p.Log)
		if err != nil {
			t.Fatal(err)
		}
		last := events[len(events)-1]
		if !hasRule(last, "canary-code") || last.Disputed || last.Mode == "block" {
			t.Fatalf("stale event: %#v", last)
		}
	})

	t.Run("missing marker follows up", func(t *testing.T) {
		p := writeCanaryProject(t)
		mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"sess-c","prompt":"Please continue."}`)
		reply := "ЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-c","tool_name":"Shell","tool_input":{"command":"go test"},"tool_output":{"exitCode":0}}`)
		first := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sess-c","text":`+jsonString(reply)+`}`)
		second := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sess-c","text":`+jsonString(reply)+`}`)
		third := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sess-c","text":`+jsonString(reply)+`}`)
		if first["followup_message"] == "" || second["followup_message"] == "" {
			t.Fatalf("missing marker should follow up twice: %#v %#v", first, second)
		}
		if _, ok := third["followup_message"]; ok {
			t.Fatalf("third followup should be absent: %#v", third)
		}
		events, err := LoadEvents(p.Log)
		if err != nil {
			t.Fatal(err)
		}
		if !hasRule(events[0], "canary-code") {
			t.Fatalf("missing event: %#v", events[0])
		}
	})

	t.Run("marker without read is disputed", func(t *testing.T) {
		p := writeCanaryProject(t)
		mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"sess-d"}`)
		code := CanaryCode("sess-d", "AGENTS.md", secret)
		skill := CanaryCode("sess-d", "skills/example/SKILL.md", secret)
		mustHandle(t, p, `{"hook_event_name":"afterFileEdit","conversation_id":"sess-d","file_path":"AGENTS.md"}`)
		reply := "ПРОЧИТАНО: AGENTS.md " + code + "\nREAD: skills/example/SKILL.md " + skill +
			"\nЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
		mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sess-d","tool_name":"Shell","tool_input":{"command":"go test"},"tool_output":{"exitCode":0}}`)
		out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sess-d","text":`+jsonString(reply)+`}`)
		if _, ok := out["followup_message"]; ok {
			t.Fatalf("read mismatch must not follow up: %#v", out)
		}
		events, err := LoadEvents(p.Log)
		if err != nil {
			t.Fatal(err)
		}
		last := events[len(events)-1]
		if !last.Disputed || !hasRule(last, "canary-read") || hasRule(last, "canary-code") || last.Mode == "block" {
			t.Fatalf("disputed read: %#v", last)
		}
		page := ReportHTML(events)
		if !strings.Contains(page, "canary-read") || !strings.Contains(page, "Disputed") {
			t.Fatal("report missing disputed canary")
		}
	})

	t.Run("transcript read counts", func(t *testing.T) {
		p := writeCanaryProject(t)
		mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"sess-e"}`)
		code := CanaryCode("sess-e", "AGENTS.md", secret)
		transcript := filepath.Join(p.Root, "transcript.json")
		payload := map[string]any{
			"messages": []map[string]any{
				{"role": "tool", "tool_name": "Read", "tool_input": map[string]any{"path": "AGENTS.md"}},
			},
		}
		b, _ := json.Marshal(payload)
		if err := os.WriteFile(transcript, b, 0o644); err != nil {
			t.Fatal(err)
		}
		reply := "ПРОЧИТАНО: AGENTS.md " + code + "\nЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
		// The skill file has no read. Only AGENTS.md is in the transcript.
		// Quote only AGENTS.md so the skill code is missing and would follow up.
		// This subtest checks the transcript path clears the read dispute for the quoted file
		// by using a rules file with one file.
		if err := os.WriteFile(p.Rules, []byte(`{"canary":{"files":["AGENTS.md"]}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sess-e","transcript_path":`+jsonString(transcript)+`,"text":`+jsonString(reply)+`}`)
		if _, ok := out["followup_message"]; ok {
			t.Fatalf("transcript read should satisfy the code and the read log: %#v", out)
		}
		events, err := LoadEvents(p.Log)
		if err != nil {
			t.Fatal(err)
		}
		last := events[len(events)-1]
		if hasRule(last, "canary-code") || hasRule(last, "canary-read") {
			t.Fatalf("transcript read event: %#v", last)
		}
	})
}

func TestScanCanaryFixture(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "fixtures/phase0/transcripts/canary-synthetic.json")
	res, err := ScanFile(path, filepath.Join(t.TempDir(), "events.jsonl"), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if res.Exit != 0 {
		t.Fatalf("exit %d", res.Exit)
	}
	if strings.Join(res.RuleIDs, ",") != "answer-markers,answered-from-memory,canary-read" {
		t.Fatalf("rules %v", res.RuleIDs)
	}
	if strings.Join(res.DisputedIDs, ",") != "answered-from-memory,canary-read" {
		t.Fatalf("disputed %v", res.DisputedIDs)
	}
	stalePath := filepath.Join(t.TempDir(), "stale.json")
	staleCode := CanaryCode("other-session", "AGENTS.md", "fixture-secret")
	body := map[string]any{
		"origin":         "synthetic",
		"session_id":     "canary-fixture",
		"canary_files":   []string{"AGENTS.md"},
		"canary_secret":  "fixture-secret",
		"reads":          []string{"AGENTS.md"},
		"assistant_text": "ПРОЧИТАНО: AGENTS.md " + staleCode + "\n",
	}
	b, _ := json.Marshal(body)
	if err := os.WriteFile(stalePath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = ScanFile(stalePath, filepath.Join(t.TempDir(), "events.jsonl"), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.RuleIDs, ",") != "answer-markers,canary-code" {
		t.Fatalf("stale scan %v", res.RuleIDs)
	}
	for _, id := range res.DisputedIDs {
		if id == "canary-code" {
			t.Fatal("stale code must not be disputed")
		}
	}
}

func TestCanaryRefreshesOnFirstToolHook(t *testing.T) {
	secret := "test-secret"
	p := testPaths(t)
	if err := os.MkdirAll(p.State, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath(p), []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Rules, []byte(`{"canary":{"files":["AGENTS.md"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "AGENTS.md"), []byte("# Rules\n\ndrift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No sessionStart or beforeSubmitPrompt, as in a Cursor cloud agent.
	mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","conversation_id":"lazy","command":"true"}`)
	body, err := os.ReadFile(filepath.Join(p.Root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	code := CanaryCode("lazy", "AGENTS.md", secret)
	if !strings.Contains(string(body), "drift-guard-canary: "+code) {
		t.Fatalf("first tool hook should refresh: %s", body)
	}
	if strings.Contains(string(body), "pending") {
		t.Fatalf("pending left in place: %s", body)
	}
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"lazy","tool_name":"Read","tool_input":{"path":"AGENTS.md"}}`)
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"lazy","tool_name":"Shell","tool_input":{"command":"go test"},"tool_output":{"exitCode":0}}`)
	reply := "ПРОЧИТАНО: AGENTS.md " + code + "\nЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
	out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"lazy","text":`+jsonString(reply)+`}`)
	if _, ok := out["followup_message"]; ok {
		t.Fatalf("lazy refresh should satisfy canary: %#v", out)
	}
}

func TestCanaryDoesNotAskToQuotePending(t *testing.T) {
	secret := "test-secret"
	p := testPaths(t)
	if err := os.MkdirAll(p.State, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath(p), []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Rules, []byte(`{"canary":{"files":["AGENTS.md"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "AGENTS.md"), []byte("drift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// stop with no prior hook, so no refresh. The file still says pending.
	reply := "ПРОЧИТАНО: AGENTS.md pending\nЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
	out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"no-refresh","text":`+jsonString(reply)+`}`)
	msg, _ := out["followup_message"].(string)
	if strings.Contains(strings.ToLower(msg), "pending") {
		t.Fatalf("follow-up asked to quote pending: %s", msg)
	}
	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if hasRule(ev, "canary-code") {
			t.Fatalf("unrefreshed session should not follow up canary-code: %#v", ev)
		}
		if strings.Contains(strings.ToLower(ev.Reason), "pending") {
			t.Fatalf("reason asked to quote pending: %#v", ev)
		}
	}
	// A missing quote after a refresh still follows up, but never says pending.
	mustHandle(t, p, `{"hook_event_name":"sessionStart","conversation_id":"after"}`)
	reply = "ЦЕЛЬ: continue\nИСТОЧНИК: go test\nДОКАЗАНО: go test\n"
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"after","tool_name":"Shell","tool_input":{"command":"go test"},"tool_output":{"exitCode":0}}`)
	out = mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"after","text":`+jsonString(reply)+`}`)
	msg, _ = out["followup_message"].(string)
	if !strings.Contains(msg, "reread") {
		t.Fatalf("missing code after refresh should follow up: %#v", out)
	}
	if strings.Contains(strings.ToLower(msg), "pending") {
		t.Fatalf("follow-up mentioned pending: %s", msg)
	}
}
