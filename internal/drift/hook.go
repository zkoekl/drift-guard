package drift

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	AssistantText string          `json:"assistant_text"`
	PrevAssistant string          `json:"prev_assistant,omitempty"`
	UserText      string          `json:"user_text,omitempty"`
	PromptSeen    bool            `json:"prompt_seen,omitempty"`
	Vague         bool            `json:"vague,omitempty"`
	Created       []string        `json:"created,omitempty"`
	DeleteSeen    []deleteSeen    `json:"delete_seen,omitempty"`
	Pushback      bool            `json:"pushback,omitempty"`
	Evidence      bool            `json:"evidence,omitempty"`
	Anchors       []string        `json:"anchors,omitempty"`
	Reads         []string        `json:"reads,omitempty"`
	Followups     int             `json:"followups"`
	Commands      []rules.Command `json:"commands"`
	Files         []fileNote      `json:"files"`
	Active        bool            `json:"active,omitempty"`
	Activation    string          `json:"activation,omitempty"`
	InactiveNoted bool            `json:"inactive_noted,omitempty"`
	RulesPath     string          `json:"rules_path,omitempty"`
	ProjectRoot   string          `json:"project_root,omitempty"`
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
	_ = writeDump(p, dumpLine{TS: stamp(now), Event: event, Payload: redacted, Transcript: transcript})
	if isPromptEvent(event) {
		notePrompt(&state, payload, semanticFromRules(ruleset))
	}
	observe(event, payload, &state, now)
	var askMsg string
	if isBefore(event) {
		askMsg = guardUnrequestedDelete(event, payload, &state, ruleset, p, sid, now)
	}
	if err := saveState(p.State, sid, state); err != nil {
		_ = err
	}
	if refreshesCanary(event) {
		refreshCanaryFiles(p, ruleset, sid)
	}

	elapsed := time.Since(start).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}

	if cmd, ok := shellCommand(event, payload); ok && isBefore(event) && rules.IsGitPush(cmd) {
		ev := baseVerdict(now, sid, "hook-shell", elapsed)
		ev.Channel = "code"
		ev.Mode = "block"
		ev.Violations = 1
		ev.RuleIDs = []string{"git-push"}
		ev.Reason = "git push is denied"
		_ = AppendJSONL(p.Log, ev)
		return denyJSON(event), 0
	}

	if isBefore(event) {
		if askMsg != "" {
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
	if len(ruleset.Canary.Files) > 0 {
		secret, err := loadOrCreateSecret(secretPath(p))
		if err != nil {
			secret = ""
		}
		reads := append([]string{}, state.Reads...)
		if transcript != nil {
			reads = mergeUnique(reads, transcriptReads(transcript.Path))
		}
		findings = append(findings, CheckCanary(CanaryInput{
			Text:      assistant,
			Files:     ruleset.Canary.Files,
			SessionID: sid,
			Secret:    secret,
			Reads:     reads,
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

func wantsFollowup(findings []Finding) bool {
	for _, f := range findings {
		if f.Followup {
			return true
		}
	}
	return false
}

func composeFollowup(findings []Finding, testCommand string, cfg SemanticConfig, lang string) string {
	var parts []string
	if Block(findings) {
		parts = append(parts, followupText(testCommand, lang))
	}
	if hasFollowupID(findings, "answer-markers") {
		parts = append(parts, markerFollowup(cfg, lang))
	}
	if msg := canaryFollowup(findings, lang); msg != "" {
		parts = append(parts, msg)
	}
	return capBytes(strings.Join(parts, " "), 500)
}

func hasFollowupID(findings []Finding, id string) bool {
	for _, f := range findings {
		if f.Followup && f.ID == id {
			return true
		}
	}
	return false
}

func markerFollowup(cfg SemanticConfig, lang string) string {
	cfg = fillSemantic(cfg)
	var msg string
	if russian(lang) {
		msg = "drift-guard: добавьте непустые строки " + cfg.Goal[0] + ":, " + cfg.Source[0] +
			": (файл:строка или команда) и " + cfg.Proof[0] + ": (команда с кодом 0)."
	} else {
		msg = "drift-guard: add non-empty lines " + cfg.Goal[0] + ":, " + cfg.Source[0] +
			": (file:line or command), and " + cfg.Proof[0] + ": (a command that exited 0)."
	}
	return capBytes(msg, 240)
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

func russian(lang string) bool {
	return strings.EqualFold(strings.TrimSpace(lang), "ru")
}

func capBytes(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
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

func followupText(testCommand, lang string) string {
	cmd := strings.TrimSpace(testCommand)
	if cmd == "" {
		if russian(lang) {
			cmd = "нужную тестовую команду"
		} else {
			cmd = "the required test command"
		}
	}
	cmd = capBytes(cmd, 80)
	var msg string
	if russian(lang) {
		msg = "drift-guard: изменённые файлы есть, а `" + cmd + "` не завершилась с кодом 0. Запустите её до остановки."
	} else {
		msg = "drift-guard: edited paths changed and `" + cmd + "` did not exit 0. Run it before stopping."
	}
	return capBytes(msg, 500)
}

func estTokens(s string) int {
	if s == "" {
		return 0
	}
	n := len(s) / 4
	if len(s)%4 != 0 {
		n++
	}
	return n
}

func joinReasons(findings []Finding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		if f.Reason != "" {
			parts = append(parts, f.ID+": "+f.Reason)
		}
	}
	return strings.Join(parts, "; ")
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
	case "beforeShellExecution", "preToolUse", "PreToolUse":
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

func denyJSON(event string) []byte {
	if isClaude(event) {
		b, _ := json.Marshal(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": "git push is denied by drift-guard.",
			},
		})
		return append(b, '\n')
	}
	b, _ := json.Marshal(map[string]any{
		"permission":    "deny",
		"user_message":  "git push is denied by drift-guard.",
		"agent_message": "git push is denied. Do not push.",
	})
	return append(b, '\n')
}

func allowJSON(event string) []byte {
	if isClaude(event) {
		return []byte("{}\n")
	}
	return []byte("{\"permission\":\"allow\"}\n")
}

func stopJSON(event, follow string) []byte {
	if follow == "" {
		return []byte("{}\n")
	}
	if isClaude(event) {
		b, _ := json.Marshal(map[string]any{
			"decision": "block",
			"reason":   follow,
		})
		return append(b, '\n')
	}
	b, _ := json.Marshal(map[string]string{"followup_message": follow})
	return append(b, '\n')
}

func shellCommand(event string, payload map[string]any) (string, bool) {
	switch event {
	case "beforeShellExecution", "afterShellExecution":
		return stringField(payload, "command"), true
	case "preToolUse", "postToolUse", "PreToolUse", "PostToolUse":
		name := stringField(payload, "tool_name")
		if !isShellTool(name) {
			return "", false
		}
		in := asMap(payload["tool_input"])
		return stringField(in, "command"), true
	default:
		return "", false
	}
}

func isShellTool(name string) bool {
	switch strings.ToLower(name) {
	case "shell", "bash", "powershell":
		return true
	default:
		return false
	}
}

func observe(event string, payload map[string]any, state *sessionState, now time.Time) {
	switch event {
	case "postToolUse", "PostToolUse", "afterShellExecution":
		state.Evidence = true
		if cmd, ok := shellCommand(event, payload); ok && cmd != "" {
			exitCode := -1
			if n, ok := exitFromOutput(payload["tool_output"]); ok {
				exitCode = n
			} else if n, ok := exitFromOutput(payload["output"]); ok {
				exitCode = n
			}
			state.Commands = append(state.Commands, rules.Command{Command: cmd, ExitCode: exitCode})
			state.Reads = mergeUnique(state.Reads, shellReads(cmd))
		}
		if isReadTool(stringField(payload, "tool_name")) {
			if path := inputPath(payload); path != "" {
				state.Reads = mergeUnique(state.Reads, []string{path})
			}
		}
		if isWriteTool(stringField(payload, "tool_name")) {
			if path := inputPath(payload); path != "" {
				rememberCreated(state, path)
			}
		}
		if path := toolPath(payload); path != "" {
			state.Files = append(state.Files, fileNote{Path: path, At: now.UTC()})
		}
	case "afterFileEdit":
		state.Evidence = true
		if path := stringField(payload, "file_path"); path != "" {
			state.Files = append(state.Files, fileNote{Path: path, At: now.UTC()})
			rememberCreated(state, path)
		}
	}
}

func isReadTool(name string) bool {
	switch strings.ToLower(name) {
	case "read", "read_file", "readfile", "open", "open_file", "view":
		return true
	default:
		return false
	}
}

func inputPath(payload map[string]any) string {
	in := asMap(payload["tool_input"])
	for _, key := range []string{"path", "file_path", "target_file"} {
		if p := stringField(in, key); p != "" {
			return p
		}
	}
	return stringField(payload, "file_path")
}

func shellReads(command string) []string {
	fields := strings.Fields(command)
	i := 0
	for i < len(fields) && isEnvAssignLocal(fields[i]) {
		i++
	}
	if i >= len(fields) || !isReader(fields[i]) {
		return nil
	}
	var out []string
	for _, f := range fields[i+1:] {
		if strings.HasPrefix(f, "-") {
			continue
		}
		if strings.Contains(f, "/") || fileRe.MatchString(f) {
			out = append(out, f)
		}
	}
	return out
}

func isReader(tok string) bool {
	base := tok
	if i := strings.LastIndex(tok, "/"); i >= 0 {
		base = tok[i+1:]
	}
	switch strings.ToLower(base) {
	case "cat", "head", "tail", "less", "more", "bat", "nl", "open", "sed", "awk", "type":
		return true
	default:
		return false
	}
}

func isEnvAssignLocal(s string) bool {
	eq := strings.IndexByte(s, '=')
	return eq > 0 && !strings.Contains(s[:eq], "/")
}

func toolPath(payload map[string]any) string {
	name := strings.ToLower(stringField(payload, "tool_name"))
	switch name {
	case "write", "edit", "strreplace", "delete":
	default:
		return ""
	}
	in := asMap(payload["tool_input"])
	for _, key := range []string{"path", "file_path", "target_file"} {
		if p := stringField(in, key); p != "" {
			return p
		}
	}
	return ""
}

func exitFromOutput(v any) (int, bool) {
	switch t := v.(type) {
	case map[string]any:
		if n, ok := t["exitCode"]; ok {
			return num(n), true
		}
		if n, ok := t["exit_code"]; ok {
			return num(n), true
		}
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(t), &m) == nil {
			return exitFromOutput(m)
		}
	}
	return -1, false
}

func assistantText(payload map[string]any) string {
	for _, key := range []string{"last_assistant_message", "text"} {
		if s := stringField(payload, key); s != "" {
			return s
		}
	}
	return ""
}

func changedPaths(payload map[string]any, state sessionState) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range stringList(payload["modified_files"]) {
		add(p)
	}
	for _, note := range state.Files {
		add(note.Path)
	}
	return out
}

func loadRules(path string) ruleFile {
	if path == "" {
		return ruleFile{}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ruleFile{}
	}
	var rf ruleFile
	if json.Unmarshal(b, &rf) != nil {
		return ruleFile{}
	}
	return rf
}

func sessionID(payload map[string]any) string {
	for _, key := range []string{"conversation_id", "session_id"} {
		if s := stringField(payload, key); s != "" {
			return safeID(s)
		}
	}
	return "unknown"
}

func safeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func statePath(dir, id string) string {
	return filepath.Join(dir, "session-"+safeID(id)+".json")
}

func loadState(dir, id string) sessionState {
	b, err := os.ReadFile(statePath(dir, id))
	if err != nil {
		return sessionState{}
	}
	var st sessionState
	if json.Unmarshal(b, &st) != nil {
		return sessionState{}
	}
	return st
}

func saveState(dir, id string, st sessionState) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir, id), append(b, '\n'), 0o644)
}

func writeDump(p Paths, line dumpLine) error {
	if p.Dump == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.Dump), 0o755); err != nil {
		return err
	}
	return AppendJSONL(p.Dump, line)
}

func inspectTranscript(path, assistant string) TranscriptStatus {
	st := TranscriptStatus{Path: path}
	if strings.TrimSpace(path) == "" {
		st.Reason = "transcript_path is empty"
		return st
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if len(b) == 0 {
			st.Reason = "transcript file does not exist"
			return st
		}
	}
	if err != nil {
		st.Reason = "transcript file does not exist"
		return st
	}
	st.Exists = true
	if len(bytes.TrimSpace(b)) == 0 {
		st.Reason = "transcript file is empty"
		return st
	}
	if len(b) > 8<<20 {
		b = b[len(b)-2<<20:]
	}
	st.EndsWithAssistant = endsWithAssistant(b)
	text := strings.TrimSpace(assistant)
	if text == "" {
		st.Reason = "no assistant text captured for this turn, so completeness cannot be confirmed"
		return st
	}
	if bytes.Contains(b, []byte(text)) {
		st.Complete = true
		st.Reason = "transcript contains the assistant text from this turn"
		return st
	}
	st.Reason = "transcript exists but does not contain the assistant text from this turn"
	return st
}

func endsWithAssistant(b []byte) bool {
	trim := bytes.TrimSpace(b)
	if len(trim) == 0 {
		return false
	}
	if trim[0] == '[' {
		var arr []map[string]any
		if json.Unmarshal(trim, &arr) == nil && len(arr) > 0 {
			return isAssistantRecord(arr[len(arr)-1])
		}
	}
	lines := bytes.Split(trim, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if json.Unmarshal(line, &rec) != nil {
			return false
		}
		return isAssistantRecord(rec)
	}
	return false
}

func isAssistantRecord(rec map[string]any) bool {
	for _, key := range []string{"role", "type"} {
		if strings.EqualFold(stringField(rec, key), "assistant") {
			return true
		}
	}
	if msg := asMap(rec["message"]); msg != nil {
		if strings.EqualFold(stringField(msg, "role"), "assistant") {
			return true
		}
	}
	return false
}

func redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if strings.EqualFold(k, "user_email") {
				out[k] = ""
				continue
			}
			out[k] = redact(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redact(val)
		}
		return out
	default:
		return v
	}
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func stringList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
}

func commandList(v any) []rules.Command {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []rules.Command
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, rules.Command{
			Command:  stringField(m, "command"),
			ExitCode: numField(m, "exit_code"),
		})
	}
	return out
}

func asMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(t), &m) == nil {
			return m
		}
	}
	return nil
}

func numField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	return num(m[key])
}

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
