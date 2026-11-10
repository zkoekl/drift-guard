package drift

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zkoekl/drift-guard/internal/rules"
)

func TestExtractAnchors(t *testing.T) {
	got := ExtractAnchors(`Update widget.go so ParseWidget uses max_retry from internal/drift/scope.go. Keep "retry-once" and 12.`)
	want := []string{"retry-once", "internal/drift/scope.go", "widget.go", "scope.go", "max_retry", "ParseWidget", "12"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("anchors %v", got)
	}
	if len(ExtractAnchors("Please explain the flow.")) != 0 {
		t.Fatal("plain prose should not yield anchors")
	}
	if got := ExtractAnchors("The user's goal was a normal reply."); len(got) != 0 {
		t.Fatalf("possessive became an anchor: %v", got)
	}
	if got := ExtractAnchors("A written 'keys only' rule."); len(got) != 1 || got[0] != "keys only" {
		t.Fatalf("quoted phrase: %v", got)
	}
}

func TestAnswerMarkers(t *testing.T) {
	cfg := DefaultSemantic()
	text := "Done.\nЦЕЛЬ: ship it\nИСТОЧНИК: widget.go:3\nДОКАЗАНО: go test\n"
	if missing := missingMarkers(text, cfg); len(missing) != 0 {
		t.Fatalf("missing %v", missing)
	}
	en := "Done.\nGOAL: ship it\nSOURCE: widget.go:3\nPROOF: go test\n"
	if missing := missingMarkers(en, cfg); len(missing) != 0 {
		t.Fatalf("english aliases missing %v", missing)
	}
	empty := "ЦЕЛЬ:\nИСТОЧНИК: widget.go:3\nДОКАЗАНО: go test\n"
	missing := missingMarkers(empty, cfg)
	if len(missing) != 1 || missing[0] != "ЦЕЛЬ" {
		t.Fatalf("empty goal: %v", missing)
	}
	custom := SemanticConfig{Goal: []string{"TASK"}, Source: []string{"FROM"}, Proof: []string{"CHECK"}}
	if missing := missingMarkers("TASK: a\nFROM: b\nCHECK: c\n", custom); len(missing) != 0 {
		t.Fatalf("custom names %v", missing)
	}
	if missing := missingMarkers(text, custom); len(missing) != 3 {
		t.Fatalf("custom names should not accept the defaults: %v", missing)
	}
}

func TestCitationAndMemory(t *testing.T) {
	cfg := DefaultSemantic()
	text := "ЦЕЛЬ: ship\nИСТОЧНИК: widget.go:3\nДОКАЗАНО: go test\n"
	if ok, reason := citationDispute(text, cfg, []string{"widget.go"}, []rules.Command{{Command: "go test ./...", ExitCode: 0}}); ok {
		t.Fatal(reason)
	}
	if ok, _ := citationDispute(text, cfg, nil, []rules.Command{{Command: "go test", ExitCode: 0}}); !ok {
		t.Fatal("unread source should be disputed")
	}
	if ok, _ := citationDispute(text, cfg, []string{"widget.go"}, []rules.Command{{Command: "go test", ExitCode: 1}}); !ok {
		t.Fatal("nonzero proof should be disputed")
	}
	if ok, _ := citationDispute(text, cfg, []string{"widget.go"}, nil); !ok {
		t.Fatal("missing proof command should be disputed")
	}
	if ok, reason := answeredFromMemory("widget.go exports ParseWidget.", nil); !ok {
		t.Fatal("expected memory finding")
	} else if !strings.Contains(reason, "widget.go") {
		t.Fatal(reason)
	}
	if ok, reason := answeredFromMemory("widget.go exports ParseWidget.", []string{"src/widget.go"}); ok {
		t.Fatal(reason)
	}
	if ok, reason := answeredFromMemory("The module is 1.2.3.", nil); !ok || !strings.Contains(reason, "1.2.3") {
		t.Fatalf("version: %v %s", ok, reason)
	}
	if ok, reason := answeredFromMemory("The module is 1.2.3, read from notes.md.", []string{"notes.md"}); ok {
		t.Fatal(reason)
	}
}

func TestAnchorMissThreshold(t *testing.T) {
	anchors := []string{"widget.go", "12"}
	if ok, _ := AnchorMiss(anchors, 1, "", "widget.go now returns 12", nil); ok {
		t.Fatal("both anchors are in the answer")
	}
	if ok, _ := AnchorMiss(anchors, 1, "", "done", []string{"pkg/widget.go"}); ok {
		t.Fatal("changed path shares widget.go and one hit meets the minimum")
	}
	if ok, reason := AnchorMiss(anchors, 2, "", "done", []string{"pkg/widget.go"}); !ok {
		t.Fatal("threshold 2 needs both anchors")
	} else if !strings.Contains(reason, "need 2") {
		t.Fatal(reason)
	}
	if ok, _ := AnchorMiss(nil, 1, "", "done", nil); ok {
		t.Fatal("no anchors is not a miss")
	}
}

func TestSycophancyPhrasesAndReversal(t *testing.T) {
	ok, _ := Sycophancy([]Message{
		{Role: "assistant", Text: "The limit is correct and should stay."},
		{Role: "user", Text: "ты не прав"},
		{Role: "assistant", Text: "Вы правы, the limit is not correct."},
	}, SemanticConfig{})
	if !ok {
		t.Fatal("expected russian pushback plus agreement")
	}
	ok, _ = Sycophancy([]Message{
		{Role: "assistant", Text: "The limit is correct and stays."},
		{Role: "user", Text: "are you sure"},
		{Role: "assistant", Text: "The limit is not correct and it changes."},
	}, SemanticConfig{})
	if !ok {
		t.Fatal("expected a reversal without an agreement phrase")
	}
	ok, _ = Sycophancy([]Message{
		{Role: "assistant", Text: "The limit is correct and stays."},
		{Role: "user", Text: "are you sure"},
		{Role: "assistant", Text: "The limit is correct and stays."},
	}, SemanticConfig{})
	if ok {
		t.Fatal("holding the claim is not sycophancy")
	}
	ok, _ = Sycophancy([]Message{
		{Role: "assistant", Text: "The limit is correct and stays."},
		{Role: "user", Text: "а разве"},
		{Role: "tool", Text: "", ToolCalls: []string{"Read"}, Reads: []string{"widget.go"}},
		{Role: "assistant", Text: "Вы правы, the limit is not correct."},
	}, SemanticConfig{})
	if ok {
		t.Fatal("a read between the answers is new evidence")
	}
}

func TestStopMarkerFollowupAndDisputed(t *testing.T) {
	p := testPaths(t)
	bare := "This reply is long enough to be a final answer and it cites nothing."
	first := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"markers","text":`+jsonString(bare)+`}`)
	msg, _ := first["followup_message"].(string)
	if !strings.Contains(msg, "ЦЕЛЬ:") || !strings.Contains(msg, "ИСТОЧНИК:") || !strings.Contains(msg, "ДОКАЗАНО:") {
		t.Fatalf("marker followup: %#v", first)
	}
	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(events[len(events)-1], "answer-markers") || events[len(events)-1].Disputed {
		t.Fatalf("marker event: %#v", events[len(events)-1])
	}

	// Same session, second follow-up, then the cap. No third.
	second := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"markers","text":`+jsonString(bare)+`}`)
	if second["followup_message"] == "" || second["followup_message"] == nil {
		t.Fatalf("second marker followup: %#v", second)
	}
	third := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"markers","text":`+jsonString(bare)+`}`)
	if _, ok := third["followup_message"]; ok {
		t.Fatalf("third followup should be absent: %#v", third)
	}

	cited := "The file says so.\nЦЕЛЬ: read the file\nИСТОЧНИК: notes.md:4\nДОКАЗАНО: go test\n"
	disputed := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"cite","text":`+jsonString(cited)+`}`)
	if _, ok := disputed["followup_message"]; ok {
		t.Fatalf("citation dispute must not follow up: %#v", disputed)
	}
	events, err = LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if !last.Disputed || !hasRule(last, "marker-citation") {
		t.Fatalf("citation event: %#v", last)
	}
	if last.Mode == "block" {
		t.Fatal("disputed citation blocked")
	}
}

func TestHookAnchorsMemoryAndSycophancy(t *testing.T) {
	p := testPaths(t)
	mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"sem","prompt":"Update widget.go so ParseWidget returns 12."}`)
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sem","tool_name":"Read","tool_input":{"path":"widget.go"}}`)
	mustHandle(t, p, `{"hook_event_name":"postToolUse","conversation_id":"sem","tool_name":"Shell","tool_input":{"command":"go test"},"tool_output":{"exitCode":0}}`)
	clean := "ParseWidget in widget.go now returns 12.\nЦЕЛЬ: Update widget.go so ParseWidget returns 12\nИСТОЧНИК: widget.go:3\nДОКАЗАНО: go test\n"
	out := mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"sem","text":`+jsonString(clean)+`}`)
	if _, ok := out["followup_message"]; ok {
		t.Fatalf("clean reply followed up: %#v", out)
	}
	events, err := LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Violations != 0 {
		t.Fatalf("clean findings: %#v", events[len(events)-1])
	}

	mustHandle(t, p, `{"hook_event_name":"afterAgentResponse","conversation_id":"flip","text":"The limit is correct and stays in widget.go."}`)
	mustHandle(t, p, `{"hook_event_name":"beforeSubmitPrompt","conversation_id":"flip","prompt":"you're wrong about widget.go"}`)
	agree := "You are right, the limit is not correct.\nЦЕЛЬ: widget.go\nИСТОЧНИК: widget.go:1\nДОКАЗАНО: go test\n"
	// No read and no successful command, so citation is disputed and sycophancy fires.
	// Markers are present, so there is no follow-up.
	out = mustHandle(t, p, `{"hook_event_name":"stop","conversation_id":"flip","text":`+jsonString(agree)+`}`)
	if _, ok := out["followup_message"]; ok {
		t.Fatalf("sycophancy followed up: %#v", out)
	}
	events, err = LoadEvents(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if !hasRule(last, "sycophancy") || !last.Disputed {
		t.Fatalf("sycophancy event: %#v", last)
	}
	if !hasRule(last, "answered-from-memory") {
		t.Fatalf("unread widget.go should be disputed memory: %#v", last)
	}

	page := ReportHTML(events)
	if !strings.Contains(page, "Disputed") || !strings.Contains(page, "sycophancy") {
		t.Fatal("report missing disputed sycophancy")
	}
}

func TestScanDisputedDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/t.json"
	body := map[string]any{
		"origin":     "synthetic",
		"session_id": "mem",
		"messages": []map[string]string{
			{"role": "user", "text": "What is in config.go and the limit 12?"},
			{"role": "assistant", "text": "The port is 1.2.3."},
		},
	}
	b, _ := json.Marshal(body)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ScanFile(path, dir+"/events.jsonl", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if res.Exit != 0 {
		t.Fatalf("scan exit %d", res.Exit)
	}
	if strings.Join(res.RuleIDs, ",") != "answer-markers,anchor-miss,answered-from-memory" {
		t.Fatalf("rules %v", res.RuleIDs)
	}
	if strings.Join(res.DisputedIDs, ",") != "anchor-miss,answered-from-memory" {
		t.Fatalf("disputed %v", res.DisputedIDs)
	}
}

func TestAssumptionMarkerOptional(t *testing.T) {
	cfg := DefaultSemantic()
	if strings.Join(cfg.Assumption, ",") != "ДОПУЩЕНИЕ,ASSUMPTION" {
		t.Fatalf("defaults %v", cfg.Assumption)
	}
	base := "ЦЕЛЬ: ship\nИСТОЧНИК: widget.go:3\nДОКАЗАНО: go test\n"
	if missing := missingMarkers(base, cfg); len(missing) != 0 {
		t.Fatalf("assumption is not required: %v", missing)
	}
	ru := base + "ДОПУЩЕНИЕ: voice was not specified\n"
	if got := markerValues(ru, cfg.Assumption); len(got) != 1 || got[0] != "voice was not specified" {
		t.Fatalf("ru %v", got)
	}
	en := base + "assumption: encoding not in the spec\n"
	if got := markerValues(en, cfg.Assumption); len(got) != 1 || got[0] != "encoding not in the spec" {
		t.Fatalf("en %v", got)
	}
	if got := markerValues(base+"ДОПУЩЕНИЕ:\n", cfg.Assumption); len(got) != 0 {
		t.Fatalf("empty value counts: %v", got)
	}
	custom := fillSemantic(SemanticConfig{Assumption: []string{"GUESS"}})
	if got := markerValues(base+"GUESS: six cells\n", custom.Assumption); len(got) != 1 || got[0] != "six cells" {
		t.Fatalf("custom %v", got)
	}
	if got := markerValues(ru, custom.Assumption); len(got) != 0 {
		t.Fatalf("custom names replace the defaults: %v", got)
	}
	fromEmpty := semanticFromRules(ruleFile{})
	if strings.Join(fromEmpty.Assumption, ",") != "ДОПУЩЕНИЕ,ASSUMPTION" {
		t.Fatalf("rules default %v", fromEmpty.Assumption)
	}
	rf := ruleFile{}
	rf.Markers.Assumption = []string{"GUESS"}
	if got := semanticFromRules(rf).Assumption; len(got) != 1 || got[0] != "GUESS" {
		t.Fatalf("rules json %v", got)
	}
	t.Setenv("DRIFT_GUARD_RULES", t.TempDir()+"/missing.json")
	raw := []byte(`{"assistant_text":"ЦЕЛЬ: ship\nИСТОЧНИК: a.go:1\nДОКАЗАНО: go test\nGUESS: from transcript\n","markers":{"assumption":["GUESS"]}}`)
	var in FileInput
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	sem := semanticInput(in, in.AssistantText)
	if got := markerValues(sem.Text, sem.Config.Assumption); len(got) != 1 || got[0] != "from transcript" {
		t.Fatalf("transcript override %v cfg %v", got, sem.Config.Assumption)
	}
	if missing := missingMarkers(sem.Text, sem.Config); len(missing) != 0 {
		t.Fatalf("custom assumption must not join the required set: %v", missing)
	}
}
