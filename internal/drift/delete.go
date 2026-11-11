package drift

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/zkoekl/drift-guard/internal/rules"
)

const kindUnrequestedDelete = "unrequested-delete"

const (
	kindRm     = "rm"
	kindGitRm  = "git-rm"
	kindUnlink = "unlink"
	kindMvAway = "mv-away"
	kindMvOver = "mv-over"
	kindTrunc  = "trunc"
	kindTool   = "tool"
)

const (
	actSkip = iota
	actAsk
	actLog
)

// deleteOp is one path a tool is about to remove or replace.
type deleteOp struct {
	Path    string
	Kind    string
	Tool    string
	Command string
}

type pathStat func(string) (exists bool, dir bool)

type unrequestedDeleteEvent struct {
	TS          string   `json:"ts"`
	Kind        string   `json:"kind"`
	SessionID   string   `json:"session_id,omitempty"`
	Source      string   `json:"source,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Path        string   `json:"path"`
	Command     string   `json:"command,omitempty"`
	Tool        string   `json:"tool,omitempty"`
	PromptKnown bool     `json:"prompt_known"`
	VaguePrompt bool     `json:"vague_prompt"`
	Disputed    bool     `json:"disputed"`
	Reason      string   `json:"reason,omitempty"`
	RuleIDs     []string `json:"rule_ids,omitempty"`
}

func unrequestedDeleteMode(rf ruleFile) string {
	switch strings.ToLower(strings.TrimSpace(rf.UnrequestedDelete)) {
	case "log":
		return "log"
	case "off":
		return "off"
	default:
		return "ask"
	}
}

// guardUnrequestedDelete logs unrequested deletes. Ask mode returns a
// permission message, except in a Cursor cloud agent, where ask is not
// honored and the hook denies instead. A bare destructive verb, with no
// file named, is logged disputed and the tool is allowed.
func guardUnrequestedDelete(event string, payload map[string]any, state *sessionState, rf ruleFile, p Paths, sid string, now time.Time) (string, bool) {
	if !isBefore(event) || state == nil || unrequestedDeleteMode(rf) == "off" {
		return "", false
	}
	ops := collectDeleteOps(event, payload)
	if len(ops) == 0 {
		return "", false
	}
	prompt := ""
	if state.PromptSeen {
		prompt = state.UserText
	}
	mode := unrequestedDeleteMode(rf)
	vague := state.PromptSeen && state.Vague
	cloud := cursorCloudAgent()
	var askPaths []string
	for _, op := range selectUnrequested(ops, state.Created, p.Root, nil) {
		action, disputed := deletionAction(prompt, state.PromptSeen, op.Path)
		if action == actSkip {
			continue
		}
		evMode := mode
		if action == actLog {
			evMode = "log"
		} else if action == actAsk && mode == "ask" && cloud {
			evMode = "deny"
		}
		reason := deleteReason(op.Path, vague && action == actAsk, disputed, action == actLog)
		if evMode == "deny" {
			reason += "; cursor cloud agent cannot honor ask"
		}
		_ = AppendJSONL(p.Log, unrequestedDeleteEvent{
			TS: stamp(now), Kind: kindUnrequestedDelete, SessionID: sid, Source: event,
			Mode: evMode, Path: op.Path, Command: op.Command, Tool: op.Tool,
			PromptKnown: state.PromptSeen, VaguePrompt: vague && action == actAsk,
			Disputed: disputed, Reason: reason,
			RuleIDs: []string{kindUnrequestedDelete},
		})
		if action == actAsk && mode == "ask" {
			askPaths = append(askPaths, op.Path)
		}
	}
	if len(askPaths) == 0 {
		return "", false
	}
	lang := resolveReplyLanguage(rf, payload)
	if cloud {
		return deleteCloudDenyMessage(askPaths, vague, lang), true
	}
	return deleteAskMessage(askPaths, vague, lang), false
}

// cursorCloudAgent reports a Cursor cloud or worker VM, not the local CLI.
// Basis: Cursor cloud agents have no user to answer a permission "ask", so
// the hook must deny. Detection uses CURSOR_AGENT_WORKER_ID or
// CURSOR_WORKER_POOL_NAME, which cursor-agent sets on a worker/pool run.
// The local CLI sets CURSOR_AGENT and CURSOR_AGENT_CLI_LOCAL_MODE instead
// and must not match. CURSOR_CODE_REMOTE is also ignored: it covers remote
// SSH workspaces where ask can still reach the user.
func cursorCloudAgent() bool {
	return strings.TrimSpace(os.Getenv("CURSOR_AGENT_WORKER_ID")) != "" ||
		strings.TrimSpace(os.Getenv("CURSOR_WORKER_POOL_NAME")) != ""
}

func deleteCloudDenyMessage(paths []string, vague bool, lang string) string {
	msg := deleteAskMessage(paths, vague, lang)
	if russian(lang) {
		msg += " В облачном агенте Cursor подтверждение недоступно, поэтому удаление запрещено."
	} else {
		msg += " A Cursor cloud agent cannot prompt the user, so the delete is denied."
	}
	return capBytes(msg, 400)
}

// deletionAction applies the latest prompt. A named file, or a path under a
// named directory, is requested. A destructive verb with no file named is
// logged and allowed. Anything else is unrequested.
func deletionAction(prompt string, known bool, pathStr string) (int, bool) {
	if !known {
		return actAsk, true
	}
	if covers(prompt, pathStr) {
		return actSkip, false
	}
	if hasDestructiveIntent(prompt) && len(promptNames(prompt)) == 0 {
		return actLog, true
	}
	return actAsk, false
}

func deleteReason(pathStr string, vague, disputed, verbOnly bool) string {
	msg := "unrequested delete of " + pathStr
	if vague {
		msg += "; vague request, should have asked"
	}
	if verbOnly {
		msg += "; destructive verb names no file"
	} else if disputed {
		msg += "; prompt unknown"
	}
	return msg
}

func deleteAskMessage(paths []string, vague bool, lang string) string {
	file := strings.Join(paths, ", ")
	if len(paths) > 6 {
		file = strings.Join(paths[:6], ", ") + ", …"
	}
	if russian(lang) {
		msg := "Удаление/замена " + file + " не было в задаче."
		if vague {
			msg += " Размытый запрос, нужно было спросить."
		}
		msg += " Уточни у пользователя, прежде чем удалять."
		return capBytes(msg, 400)
	}
	msg := "Deleting or replacing " + file + " was not part of the task."
	if vague {
		msg += " Vague request, should have asked."
	}
	msg += " Ask the user before deleting it."
	return capBytes(msg, 400)
}

func askJSON(event, msg string) []byte {
	var v any
	if isClaude(event) {
		v = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName": "PreToolUse", "permissionDecision": "ask", "permissionDecisionReason": msg,
			},
		}
	} else {
		v = map[string]any{"permission": "ask", "user_message": msg, "agent_message": msg}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}\n")
	}
	return append(b, '\n')
}

func selectUnrequested(ops []deleteOp, created []string, root string, stat pathStat) []deleteOp {
	if stat == nil {
		stat = func(p string) (bool, bool) { return statDeletePath(root, p) }
	}
	var out []deleteOp
	seen := map[string]bool{}
	for _, op := range ops {
		p := normDeletePath(op.Path)
		if p == "" || ignoredDeletePath(p, root) || !looksLikePath(p) || seen[p] {
			continue
		}
		exists, dir := stat(p)
		switch op.Kind {
		case kindTrunc, kindMvOver:
			if !exists || dir {
				continue
			}
		default:
			if !exists && !createdEarlier(root, created, p) {
				continue
			}
		}
		seen[p] = true
		op.Path = p
		out = append(out, op)
	}
	return out
}

func createdEarlier(root string, created []string, p string) bool {
	p = normDeletePath(p)
	abs := p
	if root != "" && !strings.HasPrefix(p, "/") {
		abs = normDeletePath(filepath.Join(root, filepath.FromSlash(p)))
	}
	for _, c := range created {
		c = normDeletePath(c)
		if c == p || c == abs {
			return true
		}
		if root != "" && !strings.HasPrefix(c, "/") {
			c = normDeletePath(filepath.Join(root, filepath.FromSlash(c)))
		}
		if c == abs {
			return true
		}
	}
	return false
}

func ignoredDeletePath(pathStr, root string) bool {
	n := normDeletePath(pathStr)
	if n == "" {
		return true
	}
	if badPath(n) {
		return true
	}
	abs := n
	if root != "" && !strings.HasPrefix(n, "/") {
		abs = normDeletePath(filepath.Join(root, filepath.FromSlash(n)))
	}
	if badPath(abs) {
		return true
	}
	rn := normDeletePath(root)
	if root != "" && rn != "." && rn != "/" && (abs == rn || strings.HasPrefix(abs, rn+"/")) {
		return false
	}
	return isTemp(n) || isTemp(abs)
}

func badPath(p string) bool {
	low := strings.ToLower(p)
	if low == "/dev" || strings.HasPrefix(low, "/dev/") {
		return true
	}
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		switch strings.ToLower(seg) {
		case "bin", "dist", "node_modules", ".git":
			return true
		}
	}
	return false
}

func isTemp(p string) bool {
	if p == "" || !strings.HasPrefix(p, "/") {
		return false
	}
	low := strings.ToLower(p)
	for _, t := range []string{"/tmp", "/var/tmp", "/private/tmp", filepath.ToSlash(os.TempDir())} {
		t = strings.TrimRight(t, "/")
		if t == "" || t == "/" || t == "." {
			continue
		}
		tl := strings.ToLower(t)
		if low == tl || strings.HasPrefix(low, tl+"/") {
			return true
		}
	}
	return false
}

func normDeletePath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 && ((p[0] == '"' && p[len(p)-1] == '"') || (p[0] == '\'' && p[len(p)-1] == '\'')) {
		p = strings.TrimSpace(p[1 : len(p)-1])
	}
	if p == "" || p == "." || p == ".." {
		return p
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	if cleaned == "." {
		return "."
	}
	return strings.TrimPrefix(cleaned, "./")
}

func statDeletePath(root, p string) (bool, bool) {
	n := normDeletePath(p)
	if n == "" {
		return false, false
	}
	c := filepath.FromSlash(n)
	if !strings.HasPrefix(n, "/") && root != "" && root != "." {
		c = filepath.Join(root, c)
	}
	fi, err := os.Stat(c)
	if err != nil {
		return false, false
	}
	return true, fi.IsDir()
}

func covers(prompt, pathStr string) bool {
	pathStr = normDeletePath(pathStr)
	if pathStr == "" {
		return false
	}
	base := path.Base(pathStr)
	for _, raw := range promptNames(prompt) {
		n := strings.TrimSuffix(normDeletePath(raw), "/")
		if n == "" || n == "." || n == ".." {
			continue
		}
		if n == pathStr || (!strings.Contains(n, "/") && n == base) {
			return true
		}
		if strings.Contains(raw, "/") && strings.HasPrefix(pathStr, n+"/") {
			return true
		}
	}
	return false
}

func promptNames(prompt string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		s := b.String()
		b.Reset()
		if strings.Contains(s, ".") || strings.Contains(s, "/") {
			out = append(out, s)
		}
	}
	for _, r := range prompt {
		if isNameChar(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func isNameChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' || r == '/' || r == '@'
}

func hasDestructiveIntent(prompt string) bool {
	for _, w := range splitWords(strings.ToLower(prompt)) {
		if _, ok := intentWords[w]; ok {
			return true
		}
	}
	return false
}

func vaguePrompt(prompt string) bool {
	low := strings.ToLower(prompt)
	for _, phrase := range vaguePhrases {
		if strings.Contains(low, phrase) {
			return true
		}
	}
	return false
}

func splitWords(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

var intentWords = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, w := range strings.Fields(strings.ToLower(
		"удали удалить удалите убери убрать уберите замени заменить замена " +
			"перепиши переписать перенеси перенести переименуй переименовать " +
			"delete deleted deleting remove removed removing replace replaced replacing " +
			"rewrite rewritten rewriting migrate migrated migrating rename renamed renaming unlink")) {
		m[w] = struct{}{}
	}
	return m
}()

var vaguePhrases = []string{
	"сделай что нужно", "ты понял", "заодно", "как считаешь нужным",
	"на твоё усмотрение", "на твое усмотрение", "сам реши",
	"do whatever is needed", "you know what i mean", "as you see fit", "while you're at it",
}

// shellDeletions lists rm, unlink, git rm, mv, and `>` truncation.
// It reuses the quote-aware split from internal/rules. Globs, variables,
// sudo, sh -c, and command substitution are left alone.
func shellDeletions(command string) []deleteOp {
	command = strings.TrimSpace(command)
	if command == "" || len(command) > 8192 {
		return nil
	}
	var ops []deleteOp
	for _, seg := range rules.Segments(command) {
		ops = append(ops, commandOps(rules.Tokens(seg), command)...)
		ops = append(ops, redirectOps(seg, command)...)
	}
	return ops
}

func commandOps(toks []string, full string) []deleteOp {
	i := 0
	for i < len(toks) && isAssign(toks[i]) {
		i++
	}
	if i >= len(toks) {
		return nil
	}
	switch cmdBase(toks[i]) {
	case "rm":
		return opsFor(kindRm, "rm", full, operands(toks[i+1:]))
	case "unlink":
		return opsFor(kindUnlink, "unlink", full, operands(toks[i+1:]))
	case "mv":
		args := operands(toks[i+1:])
		if len(args) < 2 {
			return nil
		}
		ops := opsFor(kindMvAway, "mv", full, args[:len(args)-1])
		return append(ops, opsFor(kindMvOver, "mv", full, args[len(args)-1:])...)
	case "git":
		sub, rest := gitRest(toks[i:])
		switch sub {
		case "rm":
			return opsFor(kindGitRm, "git rm", full, operands(rest))
		case "mv":
			args := operands(rest)
			if len(args) < 2 {
				return nil
			}
			ops := opsFor(kindMvAway, "git mv", full, args[:len(args)-1])
			return append(ops, opsFor(kindMvOver, "git mv", full, args[len(args)-1:])...)
		}
	}
	return nil
}

func gitRest(toks []string) (string, []string) {
	i := 1
	for i < len(toks) {
		t := toks[i]
		switch t {
		case "-C", "-c", "--git-dir", "--work-tree", "--namespace":
			i += 2
			continue
		}
		if strings.HasPrefix(t, "-") {
			i++
			continue
		}
		return strings.ToLower(t), toks[i+1:]
	}
	return "", nil
}

func operands(toks []string) []string {
	var out []string
	dash := false
	for _, t := range toks {
		if !dash && t == "--" {
			dash = true
			continue
		}
		if !dash && strings.HasPrefix(t, "-") && t != "-" {
			continue
		}
		if t == "#" || strings.HasPrefix(t, "#") {
			break
		}
		if !looksLikePath(t) {
			continue
		}
		out = append(out, t)
		if len(out) >= 32 {
			break
		}
	}
	return out
}

func opsFor(kind, tool, full string, paths []string) []deleteOp {
	cmd := capBytes(strings.TrimSpace(full), 240)
	var out []deleteOp
	for _, p := range paths {
		n := normDeletePath(p)
		if n == "" || !looksLikePath(n) {
			continue
		}
		out = append(out, deleteOp{Path: n, Kind: kind, Tool: tool, Command: cmd})
	}
	return out
}

func looksLikePath(p string) bool {
	if p == "" || p == "-" || strings.ContainsAny(p, "*?[$`") {
		return false
	}
	for _, r := range p {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("./_-@", r) {
			return true
		}
	}
	return false
}

func redirectOps(seg, full string) []deleteOp {
	var out []string
	var quote byte
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			continue
		}
		if c != '>' || (i+1 < len(seg) && seg[i+1] == '>') {
			if c == '>' {
				i++
			}
			continue
		}
		j := i + 1
		if j < len(seg) && seg[j] == '&' {
			j++
			if j < len(seg) && seg[j] >= '0' && seg[j] <= '9' {
				continue
			}
		}
		for j < len(seg) && (seg[j] == ' ' || seg[j] == '\t') {
			j++
		}
		start := j
		for j < len(seg) && seg[j] != ' ' && seg[j] != '\t' && seg[j] != '\n' && seg[j] != '|' && seg[j] != '&' && seg[j] != ';' {
			j++
		}
		if start < j {
			out = append(out, seg[start:j])
		}
		i = j - 1
	}
	return opsFor(kindTrunc, "redirect", full, out)
}

func isAssign(s string) bool {
	eq := strings.IndexByte(s, '=')
	if eq <= 0 || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s[:eq] {
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func cmdBase(tok string) string {
	tok = strings.ToLower(path.Base(filepath.ToSlash(strings.TrimSpace(tok))))
	return strings.TrimSuffix(tok, ".exe")
}

func collectDeleteOps(event string, payload map[string]any) []deleteOp {
	if cmd, ok := shellCommand(event, payload); ok && strings.TrimSpace(cmd) != "" {
		return shellDeletions(cmd)
	}
	return toolDeletions(payload)
}

func toolDeletions(payload map[string]any) []deleteOp {
	name := strings.TrimSpace(stringField(payload, "tool_name"))
	if name == "" || isShellTool(name) {
		return nil
	}
	in := asMap(payload["tool_input"])
	low := strings.ToLower(name)
	var ops []deleteOp
	switch low {
	case "delete", "delete_file", "deletefile":
		ops = append(ops, opsFor(kindTool, name, "", toolPaths(payload, in))...)
	}
	if isWipe(low, in) {
		ops = append(ops, opsFor(kindTrunc, name, "", toolPaths(payload, in))...)
	}
	return ops
}

func toolPaths(payload map[string]any, in map[string]any) []string {
	var out []string
	if p := inputPath(payload); p != "" {
		out = append(out, p)
	}
	if arr, ok := in["paths"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func isWipe(name string, in map[string]any) bool {
	switch name {
	case "write", "write_file", "edit", "edit_file":
	default:
		return false
	}
	for _, key := range []string{"contents", "content"} {
		s, ok := in[key].(string)
		if ok && s == "" {
			return true
		}
	}
	return false
}

func isWriteTool(name string) bool {
	switch strings.ToLower(name) {
	case "write", "write_file":
		return true
	default:
		return false
	}
}

func rememberCreated(state *sessionState, pathStr string) {
	pathStr = strings.TrimSpace(pathStr)
	if pathStr == "" || ignoredDeletePath(pathStr, "") {
		return
	}
	state.Created = mergeUnique(state.Created, []string{pathStr})
	if len(state.Created) > 200 {
		state.Created = state.Created[len(state.Created)-200:]
	}
}
