package drift

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	kindUnrequestedDelete = "unrequested-delete"
	maxShellScan          = 64 << 10
	maxDeleteOperands     = 40
	deleteDedupeWindow    = 5 * time.Second
)

const (
	kindRm     = "rm"
	kindGitRm  = "git-rm"
	kindUnlink = "unlink"
	kindMvAway = "mv-away"
	kindMvOver = "mv-over"
	kindTrunc  = "trunc"
	kindTool   = "tool"
)

// deleteOp is one path a tool is about to remove or replace.
type deleteOp struct {
	Path    string
	Kind    string
	Tool    string
	Command string
	Seq     int
}

type deleteSeen struct {
	Sig string    `json:"sig"`
	At  time.Time `json:"at"`
}

type pathStat func(path string) (exists bool, dir bool)

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

// unrequestedDeleteMode reads unrequested_delete. Empty or unknown stays ask.
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

// guardUnrequestedDelete logs unrequested deletes and, in ask mode, returns
// the permission message. It does not deny the tool.
func guardUnrequestedDelete(event string, payload map[string]any, state *sessionState, rf ruleFile, p Paths, sid string, now time.Time) string {
	if !isBefore(event) || state == nil {
		return ""
	}
	mode := unrequestedDeleteMode(rf)
	if mode == "off" {
		return ""
	}
	ops := collectDeleteOps(event, payload)
	if len(ops) == 0 {
		return ""
	}
	prompt := ""
	if state.PromptSeen {
		prompt = state.UserText
	}
	flagged := selectUnrequested(ops, prompt, state.PromptSeen, state.Created, p.Root, nil)
	if len(flagged) == 0 {
		return ""
	}
	vague := state.PromptSeen && state.Vague
	disputed := !state.PromptSeen
	for _, op := range flagged {
		sig := op.Tool + "\n" + op.Path + "\n" + op.Command
		if rememberDelete(state, sig, now) {
			continue
		}
		_ = AppendJSONL(p.Log, unrequestedDeleteEvent{
			TS:          stamp(now),
			Kind:        kindUnrequestedDelete,
			SessionID:   sid,
			Source:      event,
			Mode:        mode,
			Path:        op.Path,
			Command:     op.Command,
			Tool:        op.Tool,
			PromptKnown: state.PromptSeen,
			VaguePrompt: vague,
			Disputed:    disputed,
			Reason:      deleteReason(op.Path, vague, disputed),
			RuleIDs:     []string{kindUnrequestedDelete},
		})
	}
	if mode != "ask" {
		return ""
	}
	return deleteAskMessage(flaggedPaths(flagged), vague, resolveReplyLanguage(rf, payload))
}

func rememberDelete(state *sessionState, sig string, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	cutoff := now.Add(-deleteDedupeWindow)
	kept := make([]deleteSeen, 0, len(state.DeleteSeen)+1)
	dup := false
	for _, s := range state.DeleteSeen {
		if s.At.Before(cutoff) {
			continue
		}
		kept = append(kept, s)
		if s.Sig == sig {
			dup = true
		}
	}
	if !dup {
		kept = append(kept, deleteSeen{Sig: sig, At: now})
	}
	if len(kept) > 32 {
		kept = kept[len(kept)-32:]
	}
	state.DeleteSeen = kept
	return dup
}

func rememberCreated(state *sessionState, path string) {
	path = strings.TrimSpace(path)
	if path == "" || ignoredDeletePath(path, "") {
		return
	}
	state.Created = mergeUnique(state.Created, []string{path})
	if len(state.Created) > 200 {
		state.Created = state.Created[len(state.Created)-200:]
	}
}

func flaggedPaths(ops []deleteOp) []string {
	var out []string
	seen := map[string]bool{}
	for _, op := range ops {
		if op.Path == "" || seen[op.Path] {
			continue
		}
		seen[op.Path] = true
		out = append(out, op.Path)
	}
	return out
}

func deleteReason(path string, vague, disputed bool) string {
	msg := "unrequested delete of " + path
	if vague {
		msg += "; vague request, should have asked"
	}
	if disputed {
		msg += "; prompt unknown"
	}
	return msg
}

func deleteAskMessage(paths []string, vague bool, lang string) string {
	file := joinDeletePaths(paths)
	if file == "" {
		file = "file"
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

func joinDeletePaths(paths []string) string {
	if len(paths) > 6 {
		return strings.Join(paths[:6], ", ") + ", …"
	}
	return strings.Join(paths, ", ")
}

func askJSON(event, msg string) []byte {
	var v any
	if isClaude(event) {
		v = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "ask",
				"permissionDecisionReason": msg,
			},
		}
	} else {
		v = map[string]any{
			"permission":    "ask",
			"user_message":  msg,
			"agent_message": msg,
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}\n")
	}
	return append(b, '\n')
}

func collectDeleteOps(event string, payload map[string]any) []deleteOp {
	if cmd, ok := shellCommand(event, payload); ok && strings.TrimSpace(cmd) != "" {
		return shellDeletions(cmd)
	}
	return toolDeletions(payload)
}

// selectUnrequested keeps deletes the latest prompt did not request.
// promptKnown is false when no prompt hook has run for this session.
func selectUnrequested(ops []deleteOp, prompt string, promptKnown bool, created []string, root string, stat pathStat) []deleteOp {
	if stat == nil {
		stat = func(p string) (bool, bool) {
			return statDeletePath(root, p)
		}
	}
	ops = expandMoves(ops, root, stat)
	intent := promptKnown && hasDestructiveIntent(prompt)
	var out []deleteOp
	seen := map[string]bool{}
	for _, op := range ops {
		p := normDeletePath(op.Path)
		if p == "" || ignoredDeletePath(p, root) || !looksLikeDeletePath(p) {
			continue
		}
		if promptKnown && (intent || promptNamesFile(prompt, p)) {
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
		if seen[p] {
			continue
		}
		seen[p] = true
		op.Path = p
		out = append(out, op)
	}
	return out
}

func expandMoves(ops []deleteOp, root string, stat pathStat) []deleteOp {
	aways := map[int][]deleteOp{}
	for _, op := range ops {
		if op.Kind == kindMvAway {
			aways[op.Seq] = append(aways[op.Seq], op)
		}
	}
	var out []deleteOp
	for _, op := range ops {
		if op.Kind != kindMvOver {
			out = append(out, op)
			continue
		}
		dest := normDeletePath(op.Path)
		if dest == "" || ignoredDeletePath(dest, root) {
			continue
		}
		exists, dir := stat(dest)
		if dir {
			for _, src := range aways[op.Seq] {
				base := path.Base(normDeletePath(src.Path))
				if base == "" || base == "." || base == ".." {
					continue
				}
				out = append(out, deleteOp{
					Path:    joinSlash(dest, base),
					Kind:    kindMvOver,
					Tool:    op.Tool,
					Command: op.Command,
					Seq:     op.Seq,
				})
			}
			continue
		}
		if exists {
			op.Path = dest
			out = append(out, op)
		}
	}
	return out
}

func joinSlash(dir, base string) string {
	dir = normDeletePath(dir)
	if dir == "" || dir == "." {
		return base
	}
	return strings.TrimRight(dir, "/") + "/" + base
}

func createdEarlier(root string, created []string, p string) bool {
	for _, c := range created {
		if sameDeletePath(root, c, p) {
			return true
		}
	}
	return false
}

func sameDeletePath(root, a, b string) bool {
	na, nb := normDeletePath(a), normDeletePath(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	if root == "" {
		return false
	}
	ra, rb := absUnder(root, na), absUnder(root, nb)
	return ra != "" && ra == rb
}

func absUnder(root, p string) string {
	if p == "" {
		return ""
	}
	if isAbsPath(p) {
		return normDeletePath(p)
	}
	return normDeletePath(filepath.Join(root, filepath.FromSlash(p)))
}

func ignoredDeletePath(pathStr, root string) bool {
	n := normDeletePath(pathStr)
	if n == "" {
		return true
	}
	if isDevice(n) || hasIgnoredSegment(n) {
		return true
	}
	abs := n
	if root != "" && !isAbsPath(n) {
		abs = normDeletePath(filepath.Join(root, filepath.FromSlash(n)))
	}
	if isDevice(abs) || hasIgnoredSegment(abs) {
		return true
	}
	rootNorm := normDeletePath(root)
	if root != "" && pathInside(abs, rootNorm) {
		return false
	}
	return isTempPath(n) || isTempPath(abs)
}

func hasIgnoredSegment(p string) bool {
	p = strings.Trim(filepath.ToSlash(p), "/")
	if p == "" || p == "." || p == ".." {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		switch strings.ToLower(seg) {
		case "bin", "dist", "node_modules", ".git":
			return true
		}
	}
	return false
}

func isDevice(p string) bool {
	n := strings.ToLower(normDeletePath(p))
	return n == "/dev" || strings.HasPrefix(n, "/dev/")
}

func isTempPath(p string) bool {
	if p == "" || p == "." || p == ".." {
		return false
	}
	slash := filepath.ToSlash(p)
	if !strings.HasPrefix(slash, "/") {
		return false
	}
	lows := strings.ToLower(slash)
	for _, t := range tempPrefixes() {
		t = strings.TrimRight(filepath.ToSlash(t), "/")
		if t == "" || t == "/" || t == "." {
			continue
		}
		tl := strings.ToLower(t)
		if lows == tl || strings.HasPrefix(lows, tl+"/") {
			return true
		}
	}
	return false
}

func tempPrefixes() []string {
	prefs := []string{"/tmp", "/var/tmp", "/private/tmp"}
	if td := os.TempDir(); td != "" {
		prefs = append(prefs, td)
	}
	return prefs
}

func pathInside(pathStr, root string) bool {
	if pathStr == "" || root == "" || root == "." || root == "/" {
		return false
	}
	pathStr = strings.TrimRight(pathStr, "/")
	root = strings.TrimRight(root, "/")
	return pathStr == root || strings.HasPrefix(pathStr, root+"/")
}

func isAbsPath(p string) bool {
	return filepath.IsAbs(filepath.FromSlash(p))
}

func normDeletePath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 {
		if (p[0] == '"' && p[len(p)-1] == '"') || (p[0] == '\'' && p[len(p)-1] == '\'') {
			p = strings.TrimSpace(p[1 : len(p)-1])
		}
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
	var cands []string
	if isAbsPath(n) {
		cands = append(cands, filepath.FromSlash(n))
	} else if root != "" && root != "." {
		cands = append(cands, filepath.Join(root, filepath.FromSlash(n)))
	} else {
		cands = append(cands, filepath.FromSlash(n))
	}
	seen := map[string]bool{}
	for _, c := range cands {
		c = filepath.Clean(c)
		if seen[c] {
			continue
		}
		seen[c] = true
		exists, dir, found := lstatKind(c)
		if found {
			return exists, dir
		}
	}
	return false, false
}

func lstatKind(p string) (exists bool, dir bool, found bool) {
	fi, err := os.Lstat(p)
	if err != nil {
		return false, false, false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		if t, err2 := os.Stat(p); err2 == nil {
			return true, t.IsDir(), true
		}
		return true, false, true
	}
	return true, fi.IsDir(), true
}

func promptNamesFile(prompt, pathStr string) bool {
	if strings.TrimSpace(prompt) == "" {
		return false
	}
	base := path.Base(normDeletePath(pathStr))
	if base == "" || base == "." || base == ".." || base == "/" {
		return false
	}
	if utf8.RuneCountInString(base) < 2 {
		return false
	}
	if containsFileToken(prompt, base) {
		return true
	}
	full := normDeletePath(pathStr)
	if strings.Contains(full, "/") && containsFileToken(prompt, full) {
		return true
	}
	return false
}

func containsFileToken(text, token string) bool {
	if token == "" {
		return false
	}
	hay := []rune(strings.ToLower(text))
	needle := []rune(strings.ToLower(token))
	if len(needle) == 0 || len(hay) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		if string(hay[i:i+len(needle)]) != string(needle) {
			continue
		}
		if i > 0 && isFileChar(hay[i-1]) {
			continue
		}
		end := i + len(needle)
		if end < len(hay) && isFileChar(hay[end]) {
			continue
		}
		return true
	}
	return false
}

func isFileChar(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '.', '_', '-':
		return true
	default:
		return false
	}
}

func hasDestructiveIntent(prompt string) bool {
	if strings.TrimSpace(prompt) == "" {
		return false
	}
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

// Intent words are matched whole, after case folding. A hit marks every
// delete in that prompt as requested.
var intentWords = map[string]struct{}{
	"удали": {}, "удалить": {}, "удалите": {}, "удаляй": {}, "удаляйте": {},
	"удалил": {}, "удалила": {}, "удалили": {}, "удаление": {},
	"убери": {}, "убрать": {}, "уберите": {}, "убрал": {}, "убрала": {}, "убрали": {},
	"замени": {}, "заменить": {}, "замените": {}, "замена": {}, "заменил": {}, "заменила": {}, "заменили": {},
	"перепиши": {}, "переписать": {}, "перепишите": {}, "переписал": {}, "переписала": {}, "переписали": {},
	"перенеси": {}, "перенести": {}, "перенесите": {}, "перенес": {}, "перенёс": {}, "перенесла": {}, "перенесли": {},
	"переименуй": {}, "переименовать": {}, "переименуйте": {}, "переименовал": {},
	"delete": {}, "deleted": {}, "deleting": {}, "deletes": {}, "deletion": {},
	"remove": {}, "removed": {}, "removing": {}, "removes": {}, "removal": {},
	"replace": {}, "replaced": {}, "replacing": {}, "replaces": {}, "replacement": {},
	"rewrite": {}, "rewrote": {}, "rewritten": {}, "rewriting": {}, "rewrites": {},
	"migrate": {}, "migrated": {}, "migrating": {}, "migrates": {}, "migration": {},
	"rename": {}, "renamed": {}, "renaming": {}, "renames": {},
	"unlink": {}, "unlinked": {},
}

// Vague phrases are matched as case-insensitive substrings of the latest prompt.
var vaguePhrases = []string{
	"сделай что нужно",
	"ты понял",
	"заодно",
	"как считаешь нужным",
	"как посчитаешь нужным",
	"на твоё усмотрение",
	"на твое усмотрение",
	"сам реши",
	"do whatever is needed",
	"do whatever you think",
	"you know what i mean",
	"you know what to do",
	"as you see fit",
	"whatever you think is best",
	"while you're at it",
}

// shellDeletions lists destructive operands. It is quote-aware and conservative:
// globs, variables, command substitution, find -delete, and xargs are ignored.
func shellDeletions(command string) []deleteOp {
	command = strings.TrimSpace(command)
	if command == "" || len(command) > maxShellScan {
		return nil
	}
	seq := 0
	return scanShell(command, command, 0, &seq)
}

func scanShell(command, full string, depth int, seq *int) []deleteOp {
	if depth > 2 || strings.TrimSpace(command) == "" || len(command) > maxShellScan {
		return nil
	}
	var ops []deleteOp
	for _, seg := range splitShell(command) {
		ops = append(ops, scanSegment(seg, full, depth, seq)...)
	}
	return ops
}

func scanSegment(seg, full string, depth int, seq *int) []deleteOp {
	toks := stripPreamble(tokenizeShell(seg))
	if script, ok := inlineShell(toks); ok && depth < 2 {
		ops := scanShell(script, full, depth+1, seq)
		ops = append(ops, withSeq(redirectDeletes(seg, full), nextSeq(seq))...)
		return ops
	}
	id := nextSeq(seq)
	ops := withSeq(commandDeletes(toks, full), id)
	ops = append(ops, withSeq(redirectDeletes(seg, full), id)...)
	return ops
}

func nextSeq(seq *int) int {
	*seq++
	return *seq
}

func withSeq(ops []deleteOp, seq int) []deleteOp {
	for i := range ops {
		ops[i].Seq = seq
	}
	return ops
}

func commandDeletes(toks []string, full string) []deleteOp {
	if len(toks) == 0 {
		return nil
	}
	switch cmdBase(toks[0]) {
	case "rm":
		return opsFor(kindRm, "rm", full, operands(toks[1:]))
	case "unlink":
		return opsFor(kindUnlink, "unlink", full, operands(toks[1:]))
	case "mv":
		return mvOps("mv", full, toks)
	case "git":
		return gitDeletes(full, toks)
	default:
		return nil
	}
}

func mvOps(tool, full string, toks []string) []deleteOp {
	sources, dest, ok := parseMv(toks)
	if !ok {
		return nil
	}
	ops := opsFor(kindMvAway, tool, full, sources)
	if dest != "" {
		ops = append(ops, opsFor(kindMvOver, tool, full, []string{dest})...)
	}
	return ops
}

func parseMv(toks []string) (sources []string, dest string, ok bool) {
	var args []string
	target := ""
	dash := false
	for i := 1; i < len(toks); i++ {
		t := toks[i]
		if !dash && t == "--" {
			dash = true
			continue
		}
		if !dash && (t == "-t" || t == "--target-directory") && i+1 < len(toks) {
			target = toks[i+1]
			i++
			continue
		}
		if !dash && strings.HasPrefix(t, "--target-directory=") {
			target = strings.TrimPrefix(t, "--target-directory=")
			continue
		}
		if !dash && strings.HasPrefix(t, "-") && t != "-" {
			continue
		}
		if t == "#" || strings.HasPrefix(t, "#") {
			break
		}
		if !looksLikeDeletePath(t) {
			continue
		}
		args = append(args, t)
		if len(args) >= maxDeleteOperands {
			break
		}
	}
	if target != "" {
		if len(args) == 0 || !looksLikeDeletePath(target) {
			return nil, "", false
		}
		return args, target, true
	}
	if len(args) < 2 {
		return nil, "", false
	}
	return args[:len(args)-1], args[len(args)-1], true
}

func gitDeletes(full string, toks []string) []deleteOp {
	i := 1
	for i < len(toks) {
		t := toks[i]
		switch t {
		case "-C", "-c", "--git-dir", "--namespace", "--work-tree":
			i += 2
			continue
		}
		if strings.HasPrefix(t, "--git-dir=") || strings.HasPrefix(t, "--work-tree=") || strings.HasPrefix(t, "--namespace=") {
			i++
			continue
		}
		if strings.HasPrefix(t, "-") {
			i++
			continue
		}
		break
	}
	if i >= len(toks) {
		return nil
	}
	rest := toks[i+1:]
	switch strings.ToLower(toks[i]) {
	case "rm":
		return opsFor(kindGitRm, "git rm", full, operands(rest))
	case "mv":
		return mvOps("git mv", full, append([]string{"mv"}, rest...))
	default:
		return nil
	}
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
		if !looksLikeDeletePath(t) {
			continue
		}
		out = append(out, t)
		if len(out) >= maxDeleteOperands {
			break
		}
	}
	return out
}

func opsFor(kind, tool, full string, paths []string) []deleteOp {
	if len(paths) == 0 {
		return nil
	}
	cmd := shortCommand(full)
	var out []deleteOp
	for _, p := range paths {
		n := normDeletePath(p)
		if n == "" || !looksLikeDeletePath(n) {
			continue
		}
		out = append(out, deleteOp{Path: n, Kind: kind, Tool: tool, Command: cmd})
	}
	return out
}

func shortCommand(s string) string {
	return capBytes(strings.TrimSpace(s), 500)
}

func looksLikeDeletePath(p string) bool {
	if p == "" || p == "-" {
		return false
	}
	if strings.ContainsAny(p, "*?[`$") || strings.Contains(p, "${") {
		return false
	}
	if strings.HasPrefix(p, "=") {
		return false
	}
	if len(p) == 1 && p[0] >= '0' && p[0] <= '9' {
		return false
	}
	for _, r := range p {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '/' || r == '_' || r == '-' || r == '@' || r == '+' {
			return true
		}
	}
	return false
}

func redirectDeletes(seg, full string) []deleteOp {
	return opsFor(kindTrunc, "redirect", full, scanRedirects(seg))
}

func scanRedirects(seg string) []string {
	var out []string
	var quote byte
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if quote != 0 {
			if c == '\\' && quote == '"' && i+1 < len(seg) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\\' && i+1 < len(seg) {
			i++
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			continue
		}
		if c == '#' && (i == 0 || isShellWS(seg[i-1])) {
			for i+1 < len(seg) && seg[i+1] != '\n' {
				i++
			}
			continue
		}
		if c != '>' {
			continue
		}
		if i+1 < len(seg) && seg[i+1] == '>' {
			i++
			continue
		}
		j := i + 1
		if j < len(seg) && seg[j] == '|' {
			j++
		}
		if j < len(seg) && seg[j] == '&' {
			j++
			if j < len(seg) && seg[j] >= '0' && seg[j] <= '9' {
				continue
			}
		}
		for j < len(seg) && isShellWS(seg[j]) {
			j++
		}
		if j >= len(seg) {
			break
		}
		target, next := readRedirectTarget(seg, j)
		if looksLikeDeletePath(target) {
			out = append(out, target)
			if len(out) >= maxDeleteOperands {
				break
			}
		}
		if next > i {
			i = next - 1
		}
	}
	return out
}

func readRedirectTarget(s string, j int) (string, int) {
	if j >= len(s) {
		return "", j
	}
	if s[j] == '\'' || s[j] == '"' {
		q := s[j]
		j++
		var b strings.Builder
		for j < len(s) {
			if s[j] == '\\' && q == '"' && j+1 < len(s) {
				b.WriteByte(s[j+1])
				j += 2
				continue
			}
			if s[j] == q {
				return b.String(), j + 1
			}
			b.WriteByte(s[j])
			j++
		}
		return b.String(), j
	}
	start := j
	for j < len(s) {
		switch s[j] {
		case ' ', '\t', '\n', '\r', '|', '&', ';', '<', '>', '\'', '"':
			return s[start:j], j
		}
		j++
	}
	return s[start:], j
}

func isShellWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func splitShell(command string) []string {
	var parts []string
	var b strings.Builder
	var quote byte
	flush := func() {
		parts = append(parts, b.String())
		b.Reset()
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == '\\' && quote == '"' && i+1 < len(command) {
				b.WriteByte(command[i+1])
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\\' && i+1 < len(command) {
			b.WriteByte(c)
			b.WriteByte(command[i+1])
			i++
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			b.WriteByte(c)
			continue
		}
		if c == '#' && (i == 0 || isShellWS(command[i-1])) {
			for i+1 < len(command) && command[i+1] != '\n' {
				i++
			}
			continue
		}
		if c == '\n' || c == ';' {
			flush()
			continue
		}
		if c == '&' && i+1 < len(command) && command[i+1] == '&' {
			flush()
			i++
			continue
		}
		if c == '|' && i > 0 && command[i-1] == '>' {
			// >| truncates. It is not a pipe.
			b.WriteByte(c)
			continue
		}
		if c == '|' && i+1 < len(command) && command[i+1] == '|' {
			flush()
			i++
			continue
		}
		if c == '|' {
			flush()
			continue
		}
		b.WriteByte(c)
	}
	flush()
	return parts
}

func tokenizeShell(seg string) []string {
	var toks []string
	var b strings.Builder
	var quote byte
	flush := func() {
		if b.Len() == 0 {
			return
		}
		toks = append(toks, b.String())
		b.Reset()
	}
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if quote != 0 {
			if c == '\\' && quote == '"' && i+1 < len(seg) {
				b.WriteByte(seg[i+1])
				i++
				continue
			}
			if c == quote {
				quote = 0
				flush()
				continue
			}
			b.WriteByte(c)
			continue
		}
		if c == '\\' && i+1 < len(seg) {
			b.WriteByte(seg[i+1])
			i++
			continue
		}
		if c == '\'' || c == '"' {
			flush()
			quote = c
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			flush()
			continue
		}
		b.WriteByte(c)
	}
	flush()
	return toks
}

func stripPreamble(toks []string) []string {
	toks = stripAssigns(toks)
	for n := 0; n < 4 && len(toks) > 0; n++ {
		switch cmdBase(toks[0]) {
		case "sudo":
			toks = stripSudo(toks[1:])
		case "command", "exec", "time", "nohup":
			toks = stripFlags(toks[1:])
		case "env":
			toks = stripEnvCmd(toks[1:])
		default:
			return toks
		}
		toks = stripAssigns(toks)
	}
	return toks
}

func stripAssigns(toks []string) []string {
	i := 0
	for i < len(toks) && isAssign(toks[i]) {
		i++
	}
	return toks[i:]
}

func stripFlags(toks []string) []string {
	i := 0
	for i < len(toks) {
		t := toks[i]
		if t == "--" {
			return toks[i+1:]
		}
		if strings.HasPrefix(t, "-") && t != "-" {
			i++
			continue
		}
		break
	}
	return toks[i:]
}

func stripSudo(toks []string) []string {
	i := 0
	for i < len(toks) {
		t := toks[i]
		if t == "--" {
			return toks[i+1:]
		}
		switch t {
		case "-u", "-g", "-h", "-p", "-C", "-T", "-R", "-D", "-A":
			i += 2
			continue
		}
		if strings.HasPrefix(t, "-") && t != "-" {
			i++
			continue
		}
		break
	}
	if i > len(toks) {
		return nil
	}
	return toks[i:]
}

func stripEnvCmd(toks []string) []string {
	i := 0
	for i < len(toks) {
		t := toks[i]
		if t == "--" {
			return toks[i+1:]
		}
		if isAssign(t) || (strings.HasPrefix(t, "-") && t != "-") {
			i++
			continue
		}
		break
	}
	return toks[i:]
}

func inlineShell(toks []string) (string, bool) {
	if len(toks) == 0 {
		return "", false
	}
	switch cmdBase(toks[0]) {
	case "sh", "bash", "dash", "zsh", "ksh":
	default:
		return "", false
	}
	for i := 1; i < len(toks); i++ {
		if toks[i] == "-c" || toks[i] == "-lc" {
			if i+1 < len(toks) {
				return toks[i+1], true
			}
			return "", true
		}
	}
	return "", false
}

func cmdBase(tok string) string {
	tok = filepath.Base(filepath.ToSlash(strings.TrimSpace(tok)))
	return strings.TrimSuffix(strings.ToLower(tok), ".exe")
}

func isAssign(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") {
		return false
	}
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return false
	}
	for _, r := range s[:eq] {
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
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
	case "delete", "delete_file", "deletefile", "remove_file":
		ops = append(ops, opsFor(kindTool, name, "", toolPaths(payload, in))...)
	}
	if strings.EqualFold(stringField(in, "action"), "delete") || strings.EqualFold(stringField(in, "type"), "delete") {
		ops = append(ops, opsFor(kindTool, name, "", toolPaths(payload, in))...)
	}
	if marked := markerPaths(payload, in); len(marked) > 0 {
		tool := name
		if tool == "" {
			tool = "Edit"
		}
		ops = append(ops, opsFor(kindTool, tool, "", marked)...)
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
	if in == nil {
		return out
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

func markerPaths(payload map[string]any, in map[string]any) []string {
	var out []string
	if in != nil {
		for _, key := range []string{"patch", "diff", "input", "edit"} {
			if s := stringField(in, key); s != "" {
				out = append(out, pathsFromDeleteMarker(s)...)
			}
		}
	}
	if s, ok := payload["tool_input"].(string); ok && in == nil {
		out = append(out, pathsFromDeleteMarker(s)...)
	}
	return out
}

func pathsFromDeleteMarker(s string) []string {
	const key = "delete file:"
	var out []string
	lower := strings.ToLower(s)
	for {
		i := strings.Index(lower, key)
		if i < 0 {
			break
		}
		rest := s[i+len(key):]
		rest = strings.TrimLeft(rest, " \t")
		line := rest
		if n := strings.IndexAny(line, "\r\n"); n >= 0 {
			line = line[:n]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
		next := i + len(key)
		if next > len(s) {
			break
		}
		s = s[next:]
		lower = lower[next:]
	}
	return out
}

func isWipe(name string, in map[string]any) bool {
	switch name {
	case "write", "write_file", "edit", "edit_file":
	default:
		return false
	}
	if in == nil {
		return false
	}
	for _, key := range []string{"contents", "content"} {
		v, ok := in[key]
		if !ok {
			continue
		}
		s, isString := v.(string)
		if isString && s == "" {
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
