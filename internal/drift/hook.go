package drift

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zkoekl/drift-guard/internal/rules"
)

// Paths are local files. Logs and dumps are gitignored.
type Paths struct {
	Log   string
	Dump  string
	State string
	Rules string
	Root  string
}

// PathsFromEnv resolves log, dump, state, and the optional rules file.
func PathsFromEnv() Paths {
	logPath := os.Getenv("DRIFT_GUARD_LOG")
	if logPath == "" {
		logPath = "events.jsonl"
	}
	state := os.Getenv("DRIFT_GUARD_STATE")
	if state == "" {
		state = ".drift-guard"
	}
	dump := os.Getenv("DRIFT_GUARD_DUMP")
	if dump == "" {
		dump = filepath.Join(state, "hook-dump.jsonl")
	}
	rulesPath := os.Getenv("DRIFT_GUARD_RULES")
	if rulesPath == "" {
		cands := []string{"drift-guard.rules.json"}
		if root := os.Getenv("CURSOR_PROJECT_DIR"); root != "" {
			cands = append(cands, filepath.Join(root, "drift-guard.rules.json"))
		}
		for _, cand := range cands {
			st, err := os.Stat(cand)
			if err == nil && !st.IsDir() {
				rulesPath = cand
				break
			}
		}
	}
	root := os.Getenv("DRIFT_GUARD_ROOT")
	if root == "" {
		root = os.Getenv("CURSOR_PROJECT_DIR")
	}
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		} else {
			root = "."
		}
	}
	return Paths{Log: logPath, Dump: dump, State: state, Rules: rulesPath, Root: root}
}

type ruleFile struct {
	ReplyLanguage     string   `json:"reply_language"`
	UnrequestedDelete string   `json:"unrequested_delete"`
	TestCommand       string   `json:"test_command"`
	TestPathGlobs     []string `json:"test_path_globs"`
	GuardAllow        []string `json:"guard_allow"`
	RequireMarkers    bool     `json:"require_markers"`
	Markers           struct {
		Goal       []string `json:"goal"`
		Source     []string `json:"source"`
		Proof      []string `json:"proof"`
		Assumption []string `json:"assumption"`
	} `json:"markers"`
	AnchorMinShared  int      `json:"anchor_min_shared"`
	PushbackPhrases  []string `json:"pushback_phrases"`
	AgreementPhrases []string `json:"agreement_phrases"`
	Canary           struct {
		Files []string `json:"files"`
	} `json:"canary"`
}

type sessionState struct {
	AssistantText   string          `json:"assistant_text"`
	PrevAssistant   string          `json:"prev_assistant,omitempty"`
	UserText        string          `json:"user_text,omitempty"`
	PromptSeen      bool            `json:"prompt_seen,omitempty"`
	Vague           bool            `json:"vague,omitempty"`
	Created         []string        `json:"created,omitempty"`
	Pushback        bool            `json:"pushback,omitempty"`
	Evidence        bool            `json:"evidence,omitempty"`
	Anchors         []string        `json:"anchors,omitempty"`
	Reads           []string        `json:"reads,omitempty"`
	Followups       int             `json:"followups"`
	Commands        []rules.Command `json:"commands"`
	Files           []fileNote      `json:"files"`
	Active          bool            `json:"active,omitempty"`
	Activation      string          `json:"activation,omitempty"`
	InactiveNoted   bool            `json:"inactive_noted,omitempty"`
	RulesPath       string          `json:"rules_path,omitempty"`
	ProjectRoot     string          `json:"project_root,omitempty"`
	CanaryRefreshed bool            `json:"canary_refreshed,omitempty"`
}

type fileNote struct {
	Path string    `json:"path"`
	At   time.Time `json:"at"`
}

// TranscriptStatus is the stop-hook note on transcript_path.
type TranscriptStatus struct {
	Path              string `json:"path"`
	Exists            bool   `json:"exists"`
	Complete          bool   `json:"complete"`
	Reason            string `json:"reason"`
	EndsWithAssistant bool   `json:"ends_with_assistant"`
}

type dumpLine struct {
	TS         string            `json:"ts"`
	Event      string            `json:"event"`
	Payload    any               `json:"payload,omitempty"`
	Transcript *TranscriptStatus `json:"transcript,omitempty"`
}

// Handle reads one hook payload and returns the stdout JSON and the process exit code.
// user_email is removed before the payload is written to the dump log.
func Handle(raw []byte, p Paths, now time.Time) ([]byte, int) {
	start := now
	if now.IsZero() {
		start = time.Now().UTC()
		now = start
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return []byte("{}\n"), 0
	}
	if optIn() {
		if out, code, done := optInGate(raw, &p, now); done {
			return out, code
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		_ = writeDump(p, dumpLine{TS: stamp(now), Event: "unparsed", Payload: map[string]string{"error": "invalid json"}})
		return []byte("{}\n"), 0
	}
	event, _ := payload["hook_event_name"].(string)
	redacted, _ := redact(payload).(map[string]any)
	if redacted == nil {
		redacted = map[string]any{}
	}

	ruleset := loadRules(p.Rules)
	sid := sessionID(payload)
	state := loadState(p.State, sid)
	assistant := assistantText(payload)
	if assistant != "" && (event == "afterAgentResponse" || isStop(event)) {
		state.AssistantText = assistant
	}
	if assistant == "" {
		assistant = state.AssistantText
	}
	var transcript *TranscriptStatus
	if isStop(event) {
		st := inspectTranscript(stringField(payload, "transcript_path"), assistant)
		transcript = &st
	}
	if event == "beforeReadFile" {
		delete(redacted, "content") // the whole file, possibly a secret
	}
	_ = writeDump(p, dumpLine{TS: stamp(now), Event: event, Payload: redacted, Transcript: transcript})
	if isPromptEvent(event) {
		notePrompt(&state, payload, semanticFromRules(ruleset))
	}
	observe(event, payload, &state, now)
	maybeRefreshCanary(event, &state, p, ruleset, sid)
	var askMsg string
	var deleteDeny bool
	if isBefore(event) {
		askMsg, deleteDeny = guardUnrequestedDelete(event, payload, &state, ruleset, p, sid, now)
	}
	if err := saveState(p.State, sid, state); err != nil {
		_ = err
	}

	elapsed := time.Since(start).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}

	if isBefore(event) {
		if id := guardVerdict(event, payload, ruleset, p.Root); id != "" {
			source := "hook-tool"
			if _, ok := shellCommand(event, payload); ok {
				source = "hook-shell"
			}
			logGuard(p, sid, source, id, now, elapsed)
			msg := guardMessages[id]
			return denyPermissionJSON(event, msg, msg), 0
		}
	}

	if cmd, ok := shellCommand(event, payload); ok && isBefore(event) && rules.PushDenied(cmd, gitPushContext(p.Root)) {
		ev := baseVerdict(now, sid, "hook-shell", elapsed)
		ev.Channel = "code"
		ev.Mode = "block"
		ev.Violations = 1
		ev.RuleIDs = []string{"git-push"}
		ev.Reason = "protected git push is denied"
		_ = AppendJSONL(p.Log, ev)
		return denyJSON(event), 0
	}

	if isBefore(event) {
		if askMsg != "" {
			if deleteDeny {
				return denyPermissionJSON(event, askMsg, askMsg), 0
			}
			return askJSON(event, askMsg), 0
		}
		return allowJSON(event), 0
	}

	if !isStop(event) {
		return []byte("{}\n"), 0
	}

	in := Input{
		AssistantText: assistant,
		ReplyLanguage: resolveReplyLanguage(ruleset, payload),
		TestCommand:   ruleset.TestCommand,
		TestGlobs:     ruleset.TestPathGlobs,
		ChangedPaths:  changedPaths(payload, state),
		Commands:      append([]rules.Command{}, state.Commands...),
	}
	// A payload may carry the same fields the check command uses. Hook tests and
	// the stop experiment use that. Real Cursor stop payloads leave them empty.
	if extra := stringField(payload, "test_command"); extra != "" {
		in.TestCommand = extra
	}
	if globs := stringList(payload["test_path_globs"]); len(globs) > 0 {
		in.TestGlobs = globs
	}
	if paths := stringList(payload["changed_paths"]); len(paths) > 0 {
		in.ChangedPaths = append(in.ChangedPaths, paths...)
	}
	in.Commands = append(in.Commands, commandList(payload["commands"])...)

	semCfg := semanticFromRules(ruleset)
	opened := mergeUnique(append([]string{}, state.Reads...), in.ChangedPaths)
	findings := Evaluate(in)
	findings = append(findings, Semantic(SemanticInput{
		Text:     assistant,
		Changed:  in.ChangedPaths,
		Commands: in.Commands,
		Reads:    append([]string{}, state.Reads...),
		Opened:   opened,
		Anchors:  append([]string{}, state.Anchors...),
		Config:   semCfg,
		Pushback: state.Pushback,
		Evidence: state.Evidence,
		Prev:     state.PrevAssistant,
		User:     state.UserText,
	})...)
	if ruleset.RequireMarkers && len(ruleset.Canary.Files) > 0 {
		secret, err := loadOrCreateSecret(secretPath(p))
		if err != nil {
			secret = ""
		}
		reads := append([]string{}, state.Reads...)
		if transcript != nil {
			reads = mergeUnique(reads, transcriptReads(transcript.Path))
		}
		findings = append(findings, CheckCanary(CanaryInput{
			Text:         assistant,
			Files:        ruleset.Canary.Files,
			SessionID:    sid,
			Secret:       secret,
			Reads:        reads,
			SkipFollowup: !state.CanaryRefreshed,
		})...)
	}
	ev := baseVerdict(now, sid, "hook-stop", time.Since(start).Milliseconds())
	ev.Channel = Channel(findings)
	ev.Mode = AggregateMode(findings)
	ev.Violations = len(findings)
	ev.RuleIDs = IDs(findings)
	ev.Reason = joinReasons(findings)
	ev.Disputed = anyDisputed(findings)
	ev.DisputedIDs = disputedIDs(findings)
	ev.TimeToDetectMS = timeToDetect(now, state, findings, in.ChangedPaths)

	follow := ""
	loop := numField(payload, "loop_count")
	if (Block(findings) || wantsFollowup(findings)) && loop < 2 && state.Followups < 2 {
		follow = composeFollowup(findings, in.TestCommand, semCfg, in.ReplyLanguage)
		if follow != "" {
			state.Followups++
			_ = saveState(p.State, sid, state)
			ev.TokensFollowupEstimate = estTokens(follow)
		}
	}
	_ = AppendJSONL(p.Log, ev)
	return stopJSON(event, follow), 0
}

func semanticFromRules(rf ruleFile) SemanticConfig {
	cfg := DefaultSemantic()
	cfg.RequireMarkers = rf.RequireMarkers
	if len(rf.Markers.Goal) > 0 {
		cfg.Goal = rf.Markers.Goal
	}
	if len(rf.Markers.Source) > 0 {
		cfg.Source = rf.Markers.Source
	}
	if len(rf.Markers.Proof) > 0 {
		cfg.Proof = rf.Markers.Proof
	}
	if len(rf.Markers.Assumption) > 0 {
		cfg.Assumption = rf.Markers.Assumption
	}
	if rf.AnchorMinShared > 0 {
		cfg.AnchorMin = rf.AnchorMinShared
	}
	if len(rf.PushbackPhrases) > 0 {
		cfg.Pushback = rf.PushbackPhrases
	}
	if len(rf.AgreementPhrases) > 0 {
		cfg.Agreement = rf.AgreementPhrases
	}
	return cfg
}

func notePrompt(state *sessionState, payload map[string]any, cfg SemanticConfig) {
	prompt := firstNonEmpty(
		stringField(payload, "prompt"),
		stringField(payload, "text"),
		stringField(payload, "user_message"),
	)
	if strings.TrimSpace(state.AssistantText) != "" {
		state.PrevAssistant = state.AssistantText
	}
	state.UserText = truncateRunes(prompt, 2000)
	state.PromptSeen = true
	state.Vague = vaguePrompt(state.UserText)
	state.Pushback = containsPhrase(prompt, cfg.Pushback)
	state.Evidence = false
	state.Anchors = mergeUnique(state.Anchors, ExtractAnchors(prompt))
	for _, p := range attachmentPaths(payload) {
		state.Anchors = mergeUnique(state.Anchors, []string{p})
		state.Anchors = mergeUnique(state.Anchors, ExtractAnchors(p))
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func attachmentPaths(payload map[string]any) []string {
	arr, ok := payload["attachments"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"file_path", "path"} {
			if p := stringField(m, key); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func resolveReplyLanguage(rf ruleFile, payload map[string]any) string {
	if extra := stringField(payload, "reply_language"); extra != "" {
		return extra
	}
	if rf.ReplyLanguage != "" {
		return rf.ReplyLanguage
	}
	return os.Getenv("DRIFT_GUARD_REPLY_LANGUAGE")
}

func baseVerdict(now time.Time, sid, source string, elapsed int64) Event {
	return Event{
		TS:                     stamp(now),
		Kind:                   "verdict",
		SessionID:              sid,
		Source:                 source,
		ElapsedMS:              elapsed,
		TokensJudge:            0,
		TokensRulesInjected:    0,
		TokensFollowupEstimate: 0,
		RuleIDs:                []string{},
	}
}

func timeToDetect(now time.Time, state sessionState, findings []Finding, changed []string) *int64 {
	if !Block(findings) || len(changed) == 0 {
		return nil
	}
	var earliest time.Time
	for _, note := range state.Files {
		for _, p := range changed {
			if note.Path == p && !note.At.IsZero() && (earliest.IsZero() || note.At.Before(earliest)) {
				earliest = note.At
			}
		}
	}
	if earliest.IsZero() {
		return nil
	}
	ms := now.Sub(earliest).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return &ms
}

func stamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func isStop(event string) bool {
	return event == "stop" || event == "Stop"
}

func isBefore(event string) bool {
	switch event {
	case "beforeShellExecution", "beforeReadFile", "preToolUse", "PreToolUse":
		return true
	default:
		return false
	}
}

func isClaude(event string) bool {
	switch event {
	case "PreToolUse", "PostToolUse", "Stop", "PostToolUseFailure":
		return true
	default:
		return false
	}
}
