package drift

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Canary code length is 8 hex characters (32 bits of HMAC-SHA256).
// It is short so a reply can quote it. It is not a security boundary.
const canaryCodeLen = 8

// CanaryCode is the session code for one required file.
// It is HMAC-SHA256(secret, safeSessionID || NUL || normalizedPath), truncated.
// The same session, path, and secret always produce the same code.
// Another session id does not. The hook does not print this value.
func CanaryCode(sessionID, filePath, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(safeID(sessionID)))
	mac.Write([]byte{0})
	mac.Write([]byte(normPath(filePath)))
	return hex.EncodeToString(mac.Sum(nil)[:canaryCodeLen/2])
}

// CanaryInput is one reply plus the files the rules require.
// Reads is the session read log. Root is only used when rewriting files.
type CanaryInput struct {
	Text         string
	Files        []string
	SessionID    string
	Secret       string
	Reads        []string
	SkipFollowup bool
}

// CheckCanary compares ПРОЧИТАНО / READ quotes to this session's codes.
// A missing or wrong code is a follow-up finding. It does not block the process.
// A matching code whose file is absent from Reads is disputed and does not follow up.
// An empty file list means the check is off.
// A matching code shows the file was in context this session, not that its rules were applied.
// SkipFollowup is set when this session has not refreshed the canary lines, so
// the files may still say "pending". In that case a missing or stale quote is
// not a follow-up: never ask an agent to quote pending.
func CheckCanary(in CanaryInput) []Finding {
	files := canaryFiles(in.Files)
	if len(files) == 0 {
		return nil
	}
	quotes := canaryQuotes(in.Text)
	var stale, unread []string
	for _, file := range files {
		want := ""
		if in.Secret != "" && safeID(in.SessionID) != "" {
			want = CanaryCode(in.SessionID, file, in.Secret)
		}
		if !quoteMatches(file, want, quotes) {
			if !in.SkipFollowup {
				stale = append(stale, file)
			}
			continue
		}
		if !wasOpened(file, in.Reads) {
			unread = append(unread, file)
		}
	}
	var out []Finding
	if len(stale) > 0 {
		out = append(out, Finding{
			ID:       "canary-code",
			Channel:  "conversation",
			Mode:     "log",
			Followup: true,
			Reason:   "canary missing or stale: " + strings.Join(stale, ", "),
		})
	}
	if len(unread) > 0 {
		out = append(out, Finding{
			ID:       "canary-read",
			Channel:  "conversation",
			Mode:     "log",
			Disputed: true,
			Reason:   "canary read log missing: " + strings.Join(unread, ", "),
		})
	}
	return out
}

func quoteMatches(file, want string, quotes []canaryQuote) bool {
	if want == "" {
		return false
	}
	for _, q := range quotes {
		if !wasOpened(file, []string{q.path}) {
			continue
		}
		if strings.EqualFold(q.code, want) {
			return true
		}
	}
	return false
}

type canaryQuote struct {
	path string
	code string
}

var canaryQuoteRe = regexp.MustCompile(`(?:^|[\s])(?:ПРОЧИТАНО|(?i:READ)):\s*(\S+)\s+([0-9a-fA-F]{8})\b`)

func canaryQuotes(text string) []canaryQuote {
	var out []canaryQuote
	for _, m := range canaryQuoteRe.FindAllStringSubmatch(text, -1) {
		out = append(out, canaryQuote{path: m[1], code: strings.ToLower(m[2])})
	}
	return out
}

func canaryFiles(files []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		n := normPath(f)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// canaryFollowup asks for a reread. It does not include the session code.
// lang ru uses the Russian sentence. Any other value stays English.
func canaryFollowup(findings []Finding, lang string) string {
	for _, f := range findings {
		if f.ID != "canary-code" || !f.Followup {
			continue
		}
		files := strings.TrimPrefix(f.Reason, "canary missing or stale: ")
		msg := "drift-guard: reread " + files + " and quote each drift-guard-canary line as ПРОЧИТАНО: <file> <code>."
		if russian(lang) {
			msg = "drift-guard: перечитайте " + files + " и процитируйте каждую строку drift-guard-canary как ПРОЧИТАНО: <файл> <код>."
		}
		return truncateRunes(msg, 240)
	}
	return ""
}

// CanaryRefresh is the result of rewriting canary lines. Codes are not listed.
type CanaryRefresh struct {
	Off     bool
	Updated []string
	Missing []string
}

// RunCanaryRefresh loads the rules file, creates the local secret if needed,
// and rewrites each required file's canary line for this session.
// The returned lists are project-relative paths. They do not contain codes.
func RunCanaryRefresh(root, rulesPath, secretFile, sessionID string) (CanaryRefresh, error) {
	rf := loadRules(rulesPath)
	if len(rf.Canary.Files) == 0 {
		return CanaryRefresh{Off: true}, nil
	}
	secret, err := loadOrCreateSecret(secretFile)
	if err != nil {
		return CanaryRefresh{}, err
	}
	return RefreshCanaries(root, rf.Canary.Files, sessionID, secret)
}

// RefreshCanaries rewrites drift-guard-canary lines. A missing file is skipped,
// not created. A path outside root is skipped. The secret is not written into the file.
func RefreshCanaries(root string, files []string, sessionID, secret string) (CanaryRefresh, error) {
	if strings.TrimSpace(secret) == "" {
		return CanaryRefresh{}, errSecret
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	var out CanaryRefresh
	for _, file := range canaryFiles(files) {
		abs, err := resolveInside(root, file)
		if err != nil {
			out.Missing = append(out.Missing, file)
			continue
		}
		body, err := os.ReadFile(abs)
		if err != nil {
			out.Missing = append(out.Missing, file)
			continue
		}
		code := CanaryCode(sessionID, file, secret)
		next := setCanaryLine(string(body), code)
		if next == string(body) {
			out.Updated = append(out.Updated, file)
			continue
		}
		if err := os.WriteFile(abs, []byte(next), 0o644); err != nil {
			out.Missing = append(out.Missing, file)
			continue
		}
		out.Updated = append(out.Updated, file)
	}
	return out, nil
}

var errSecret = errString("canary secret is empty")

type errString string

func (e errString) Error() string { return string(e) }

var canaryLineRe = regexp.MustCompile(`(?m)^([ \t]*)drift-guard-canary:[ \t]*\S*[ \t]*$`)

func setCanaryLine(body, code string) string {
	if !canaryLineRe.MatchString(body) {
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		return body + "drift-guard-canary: " + code + "\n"
	}
	return canaryLineRe.ReplaceAllString(body, "${1}drift-guard-canary: "+code)
}

func loadOrCreateSecret(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errSecret
	}
	if b, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(b))
		if isSecret(s) {
			return s, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	s := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(s+"\n"), 0o600); err != nil {
		return "", err
	}
	return s, nil
}

func isSecret(s string) bool {
	// Generated secrets are hex. A non-empty single token is kept so a file
	// is not rotated just because it is not hex. Short leftovers are replaced.
	if len(s) < 8 || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	return true
}

func resolveInside(root, rel string) (string, error) {
	rel = normPath(rel)
	if rel == "" {
		return "", errString("empty canary path")
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	joined := filepath.Clean(filepath.Join(absRoot, filepath.FromSlash(rel)))
	relTo, err := filepath.Rel(absRoot, joined)
	if err != nil {
		return "", err
	}
	if relTo == ".." || strings.HasPrefix(relTo, ".."+string(filepath.Separator)) {
		return "", errString("canary path escapes root")
	}
	return joined, nil
}

func secretPath(p Paths) string {
	dir := p.State
	if dir == "" {
		dir = ".drift-guard"
	}
	return filepath.Join(dir, "canary-secret")
}

func refreshesCanary(event string) bool {
	switch event {
	case "sessionStart", "SessionStart", "beforeSubmitPrompt", "UserPromptSubmit":
		return true
	default:
		return false
	}
}

func isToolHook(event string) bool {
	switch event {
	case "preToolUse", "PreToolUse", "postToolUse", "PostToolUse",
		"beforeShellExecution", "afterShellExecution",
		"afterFileEdit", "beforeReadFile":
		return true
	default:
		return false
	}
}

// maybeRefreshCanary rewrites canary lines at session start and on each
// prompt, and lazily on the first tool hook when those events did not run
// (Cursor cloud agents do not fire sessionStart).
func maybeRefreshCanary(event string, state *sessionState, p Paths, rf ruleFile, sid string) {
	if state == nil || !rf.RequireMarkers || len(rf.Canary.Files) == 0 {
		return
	}
	if !refreshesCanary(event) && (state.CanaryRefreshed || !isToolHook(event)) {
		return
	}
	refreshCanaryFiles(p, rf, sid)
	state.CanaryRefreshed = true
}

func refreshCanaryFiles(p Paths, rf ruleFile, sid string) {
	if len(rf.Canary.Files) == 0 {
		return
	}
	secret, err := loadOrCreateSecret(secretPath(p))
	if err != nil {
		return
	}
	root := p.Root
	if root == "" {
		root = "."
	}
	_, _ = RefreshCanaries(root, rf.Canary.Files, sid, secret)
}

// transcriptReads pulls read paths from a transcript file when one is present.
// A missing or unreadable file returns nothing. This does not parse file bodies.
func transcriptReads(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil || len(bytesTrim(b)) == 0 {
		return nil
	}
	if len(b) > 8<<20 {
		b = b[len(b)-2<<20:]
	}
	var out []string
	trim := bytesTrim(b)
	if len(trim) > 0 && (trim[0] == '{' || trim[0] == '[') {
		var v any
		if json.Unmarshal(trim, &v) == nil {
			out = append(out, readsFromValue(v)...)
		}
	}
	for _, line := range strings.Split(string(trim), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		out = append(out, readsFromValue(m)...)
	}
	return out
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func readsFromValue(v any) []string {
	switch t := v.(type) {
	case map[string]any:
		var out []string
		out = append(out, stringList(t["reads"])...)
		if isReadTool(stringField(t, "tool_name")) || isReadTool(stringField(t, "name")) {
			if p := inputPath(t); p != "" {
				out = append(out, p)
			}
		}
		for _, key := range []string{"message", "tool_input"} {
			if nested := t[key]; nested != nil {
				out = append(out, readsFromValue(nested)...)
			}
		}
		if arr, ok := t["messages"].([]any); ok {
			out = append(out, readsFromValue(arr)...)
		}
		return out
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, readsFromValue(item)...)
		}
		return out
	default:
		return nil
	}
}
