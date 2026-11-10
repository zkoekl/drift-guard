package drift

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const rulesFileName = "drift-guard.rules.json"

const (
	kindSessionInactive = "session_inactive"
	kindSessionActive   = "session_active"
)

var (
	errHookJSON = errors.New("invalid hook json")
	emptyJSON   = []byte("{}\n")
)

// sessionNote is the only record written for an inactive session, and the
// one record written when a session becomes active. It has no prompt and no path.
type sessionNote struct {
	TS         string `json:"ts"`
	Kind       string `json:"kind"`
	SessionID  string `json:"session_id,omitempty"`
	Source     string `json:"source,omitempty"`
	Activation string `json:"activation,omitempty"`
}

func optIn() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("DRIFT_GUARD_ACTIVATION")), "opt-in")
}

func activationTag() string {
	tag := strings.TrimSpace(os.Getenv("DRIFT_GUARD_TAG"))
	if tag == "" {
		return "#dg"
	}
	return tag
}

// optInGate handles an inactive session and returns done.
// An active session falls through with p.Rules and p.Root pointed at the project.
func optInGate(raw []byte, p *Paths, now time.Time) ([]byte, int, bool) {
	meta, err := scanHookMeta(raw)
	if err != nil || meta.Event == "" {
		return emptyJSON, 0, true
	}
	sid := metaSessionID(meta)
	st := loadState(p.State, sid)
	if st.Active {
		applySavedProject(p, &st, sid, meta)
		return nil, 0, false
	}
	why, rulesPath, root := activationWhy(*p, meta)
	if why != "" {
		if rulesPath != "" {
			p.Rules = rulesPath
			p.Root = root
			st.RulesPath = rulesPath
			st.ProjectRoot = root
		}
		st.Active = true
		st.Activation = why
		if claimOnce(p.State, "active-", sid) {
			_ = appendSessionNote(*p, sessionNote{
				TS:         stamp(now),
				Kind:       kindSessionActive,
				SessionID:  sid,
				Source:     meta.Event,
				Activation: why,
			})
		}
		_ = saveState(p.State, sid, st)
		return nil, 0, false
	}
	if st.InactiveNoted || inactiveMarked(p.State, sid) {
		return neutralJSON(meta.Event), 0, true
	}
	if !claimOnce(p.State, "inactive-", sid) {
		return neutralJSON(meta.Event), 0, true
	}
	note := sessionNote{
		TS:        stamp(now),
		Kind:      kindSessionInactive,
		SessionID: sid,
		Source:    meta.Event,
	}
	if err := appendSessionNote(*p, note); err != nil {
		releaseOnce(p.State, "inactive-", sid)
		return neutralJSON(meta.Event), 0, true
	}
	st.InactiveNoted = true
	_ = saveState(p.State, sid, st)
	return neutralJSON(meta.Event), 0, true
}

func activationWhy(p Paths, meta hookMeta) (why, rulesPath, root string) {
	var parts []string
	if rp, r, ok := findProjectRules(p, meta.WorkspaceRoots); ok {
		parts = append(parts, "project")
		rulesPath = rp
		root = r
	}
	if isPromptEvent(meta.Event) && hasActivationTag(meta.prompt(), activationTag()) {
		parts = append(parts, "tag")
	}
	return strings.Join(parts, "+"), rulesPath, root
}

func applySavedProject(p *Paths, st *sessionState, sid string, meta hookMeta) {
	if st.ProjectRoot != "" {
		p.Root = st.ProjectRoot
	}
	if st.RulesPath != "" {
		p.Rules = st.RulesPath
	}
	rp, root, ok := findProjectRules(*p, meta.WorkspaceRoots)
	if !ok {
		return
	}
	p.Rules = rp
	p.Root = root
	if st.RulesPath == rp && st.ProjectRoot == root {
		return
	}
	st.RulesPath = rp
	st.ProjectRoot = root
	_ = saveState(p.State, sid, *st)
}

func findProjectRules(p Paths, workspace []string) (rulesPath, root string, ok bool) {
	for _, r := range candidateRoots(p, workspace) {
		path := filepath.Join(r, rulesFileName)
		st, err := os.Stat(path)
		if err == nil && !st.IsDir() {
			return path, r, true
		}
	}
	return "", "", false
}

func candidateRoots(p Paths, extra []string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(r string) {
		r = strings.TrimSpace(r)
		if r == "" {
			return
		}
		r = filepath.Clean(r)
		if _, ok := seen[r]; ok {
			return
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	add(os.Getenv("DRIFT_GUARD_ROOT"))
	add(os.Getenv("CURSOR_PROJECT_DIR"))
	for _, r := range extra {
		add(r)
	}
	add(p.Root)
	return out
}

func neutralJSON(event string) []byte {
	if isBefore(event) {
		return allowJSON(event)
	}
	return emptyJSON
}

func metaSessionID(m hookMeta) string {
	if m.ConversationID != "" {
		return safeID(m.ConversationID)
	}
	if m.SessionID != "" {
		return safeID(m.SessionID)
	}
	return "unknown"
}

func isPromptEvent(event string) bool {
	return event == "beforeSubmitPrompt" || event == "UserPromptSubmit"
}

func (m hookMeta) prompt() string {
	if strings.TrimSpace(m.Prompt) != "" {
		return m.Prompt
	}
	if strings.TrimSpace(m.UserMessage) != "" {
		return m.UserMessage
	}
	return m.Text
}

// hasActivationTag reports a token match. The tag is compared case-insensitively.
// A letter, digit, or underscore on either side keeps it from matching.
func hasActivationTag(text, tag string) bool {
	if tag == "" || len(text) < len(tag) {
		return false
	}
	n := len(tag)
	for i := 0; i+n <= len(text); i++ {
		if !strings.EqualFold(text[i:i+n], tag) {
			continue
		}
		if i > 0 && tagWord(text[i-1]) {
			continue
		}
		if i+n < len(text) && tagWord(text[i+n]) {
			continue
		}
		return true
	}
	return false
}

func tagWord(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func claimOnce(dir, prefix, sid string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	path := filepath.Join(dir, prefix+safeID(sid))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func releaseOnce(dir, prefix, sid string) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, prefix+safeID(sid)))
}

func inactiveMarked(dir, sid string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "inactive-"+safeID(sid)))
	return err == nil
}

func appendSessionNote(p Paths, note sessionNote) error {
	if p.Log == "" {
		return nil
	}
	dir := filepath.Dir(p.Log)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return AppendJSONL(p.Log, note)
}

type hookMeta struct {
	Event          string
	ConversationID string
	SessionID      string
	Prompt         string
	UserMessage    string
	Text           string
	WorkspaceRoots []string
}

type rawSpan struct {
	ok    bool
	start int
}

// scanHookMeta reads the few top-level fields the opt-in gate needs.
// Other values, including large tool output, are skipped and not retained.
// Prompt text is decoded only for prompt events.
func scanHookMeta(raw []byte) (hookMeta, error) {
	var m hookMeta
	var promptAt, userAt, textAt rawSpan
	i := skipWS(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return m, errHookJSON
	}
	i++
	for {
		i = skipWS(raw, i)
		if i >= len(raw) {
			return m, errHookJSON
		}
		if raw[i] == '}' {
			break
		}
		if raw[i] != '"' {
			return m, errHookJSON
		}
		key, next, err := takeString(raw, i)
		if err != nil {
			return m, err
		}
		i = skipWS(raw, next)
		if i >= len(raw) || raw[i] != ':' {
			return m, errHookJSON
		}
		i = skipWS(raw, i+1)
		switch key {
		case "hook_event_name":
			m.Event, i, err = takeString(raw, i)
		case "conversation_id":
			m.ConversationID, i, err = takeString(raw, i)
		case "session_id":
			m.SessionID, i, err = takeString(raw, i)
		case "workspace_roots":
			m.WorkspaceRoots, i, err = takeRoots(raw, i)
		case "prompt":
			if i < len(raw) && raw[i] == '"' {
				promptAt = rawSpan{ok: true, start: i}
			}
			i, err = skipValue(raw, i)
		case "user_message":
			if i < len(raw) && raw[i] == '"' {
				userAt = rawSpan{ok: true, start: i}
			}
			i, err = skipValue(raw, i)
		case "text":
			if i < len(raw) && raw[i] == '"' {
				textAt = rawSpan{ok: true, start: i}
			}
			i, err = skipValue(raw, i)
		default:
			i, err = skipValue(raw, i)
		}
		if err != nil {
			return m, err
		}
		i = skipWS(raw, i)
		if i >= len(raw) {
			return m, errHookJSON
		}
		if raw[i] == ',' {
			i++
			continue
		}
		if raw[i] == '}' {
			break
		}
		return m, errHookJSON
	}
	if !isPromptEvent(m.Event) {
		return m, nil
	}
	var err error
	if promptAt.ok {
		m.Prompt, _, err = takeString(raw, promptAt.start)
		if err != nil {
			return m, err
		}
	}
	if strings.TrimSpace(m.Prompt) == "" && userAt.ok {
		m.UserMessage, _, err = takeString(raw, userAt.start)
		if err != nil {
			return m, err
		}
	}
	if strings.TrimSpace(m.Prompt) == "" && strings.TrimSpace(m.UserMessage) == "" && textAt.ok {
		m.Text, _, err = takeString(raw, textAt.start)
		if err != nil {
			return m, err
		}
	}
	return m, nil
}

func takeRoots(b []byte, i int) ([]string, int, error) {
	if i < len(b) && b[i] == '"' {
		s, n, err := takeString(b, i)
		if err != nil || s == "" {
			return nil, n, err
		}
		return []string{s}, n, nil
	}
	if i < len(b) && b[i] == 'n' {
		n, err := skipLiteral(b, i, "null")
		return nil, n, err
	}
	return takeStringArray(b, i)
}

func takeStringArray(b []byte, i int) ([]string, int, error) {
	if i >= len(b) || b[i] != '[' {
		return nil, i, errHookJSON
	}
	i++
	var out []string
	for {
		i = skipWS(b, i)
		if i >= len(b) {
			return nil, i, errHookJSON
		}
		if b[i] == ']' {
			return out, i + 1, nil
		}
		if b[i] == '"' {
			s, n, err := takeString(b, i)
			if err != nil {
				return nil, i, err
			}
			if s != "" {
				out = append(out, s)
			}
			i = n
		} else {
			n, err := skipValue(b, i)
			if err != nil {
				return nil, i, err
			}
			i = n
		}
		i = skipWS(b, i)
		if i < len(b) && b[i] == ',' {
			i++
			continue
		}
		if i < len(b) && b[i] == ']' {
			return out, i + 1, nil
		}
		return nil, i, errHookJSON
	}
}

func takeString(b []byte, i int) (string, int, error) {
	if i >= len(b) || b[i] != '"' {
		return "", i, errHookJSON
	}
	i++
	var buf strings.Builder
	for i < len(b) {
		c := b[i]
		if c == '"' {
			return buf.String(), i + 1, nil
		}
		if c != '\\' {
			buf.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(b) {
			return "", i, errHookJSON
		}
		switch b[i+1] {
		case '"', '\\', '/':
			buf.WriteByte(b[i+1])
			i += 2
		case 'b':
			buf.WriteByte('\b')
			i += 2
		case 'f':
			buf.WriteByte('\f')
			i += 2
		case 'n':
			buf.WriteByte('\n')
			i += 2
		case 'r':
			buf.WriteByte('\r')
			i += 2
		case 't':
			buf.WriteByte('\t')
			i += 2
		case 'u':
			if i+6 > len(b) {
				return "", i, errHookJSON
			}
			r, err := strconv.ParseUint(string(b[i+2:i+6]), 16, 16)
			if err != nil {
				return "", i, errHookJSON
			}
			buf.WriteRune(rune(r))
			i += 6
		default:
			return "", i, errHookJSON
		}
	}
	return "", i, errHookJSON
}

func skipWS(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func skipValue(b []byte, i int) (int, error) {
	if i >= len(b) {
		return i, errHookJSON
	}
	switch b[i] {
	case '"':
		return skipString(b, i)
	case '{', '[':
		return skipContainer(b, i)
	case 't':
		return skipLiteral(b, i, "true")
	case 'f':
		return skipLiteral(b, i, "false")
	case 'n':
		return skipLiteral(b, i, "null")
	default:
		if b[i] == '-' || (b[i] >= '0' && b[i] <= '9') {
			return skipNumber(b, i)
		}
		return i, errHookJSON
	}
}

func skipContainer(b []byte, i int) (int, error) {
	depth := 1
	i++
	for i < len(b) && depth > 0 {
		c := b[i]
		if c == '"' {
			n, err := skipString(b, i)
			if err != nil {
				return i, err
			}
			i = n
			continue
		}
		if c == '{' || c == '[' {
			depth++
		} else if c == '}' || c == ']' {
			depth--
		}
		i++
	}
	if depth != 0 {
		return i, errHookJSON
	}
	return i, nil
}

func skipString(b []byte, i int) (int, error) {
	if i >= len(b) || b[i] != '"' {
		return i, errHookJSON
	}
	i++
	for i < len(b) {
		c := b[i]
		if c == '\\' {
			if i+1 >= len(b) {
				return i, errHookJSON
			}
			i += 2
			continue
		}
		if c == '"' {
			return i + 1, nil
		}
		i++
	}
	return i, errHookJSON
}

func skipNumber(b []byte, i int) (int, error) {
	start := i
	if b[i] == '-' {
		i++
	}
	for i < len(b) && ((b[i] >= '0' && b[i] <= '9') || b[i] == '.' || b[i] == 'e' || b[i] == 'E' || b[i] == '+' || b[i] == '-') {
		i++
	}
	if i == start {
		return i, errHookJSON
	}
	return i, nil
}

func skipLiteral(b []byte, i int, lit string) (int, error) {
	if i+len(lit) > len(b) || string(b[i:i+len(lit)]) != lit {
		return i, errHookJSON
	}
	return i + len(lit), nil
}
