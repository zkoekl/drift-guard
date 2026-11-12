package drift

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

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
