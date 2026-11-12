package drift

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zkoekl/drift-guard/internal/rules"
)

// Rule ids of the three hard guards. They deny in any active session and
// log a verdict with mode "block", like the push guard.
const (
	ruleDestructive = "destructive-command"
	ruleSecretRead  = "secret-access"
	ruleGuardTamper = "guard-tamper"
)

var guardMessages = map[string]string{
	ruleDestructive: "Destructive command denied. Ask the user to run it, or use a narrower command.",
	ruleSecretRead:  "Secrets files are off limits. Ask the user to run it or to give you only the values you need.",
	ruleGuardTamper: "Hook and drift-guard settings are not yours to change. Ask the user to make this change.",
}

// pipeToShell matches a download piped into a shell, or into an interpreter that reads
// its program from stdin, or process-substituted into a shell.
var pipeToShell = regexp.MustCompile(`\b(curl|wget)\b[^|;&]*\|\s*(sudo(\s+-\S+)*\s+)?(\S*/)?((ba|z|da|k)?sh\b|(python[0-9.]*|perl|ruby|node)(\s+-)?\s*($|[;&|)]))|\b(ba|z|da|k)?sh\b[^|;&]*[<$]\(\s*(curl|wget)\b`)

// guardVerdict returns the rule id that denies this hook call, or "".
// The order is destructive, secrets, then the guard's own files.
func guardVerdict(event string, payload map[string]any, rf ruleFile, root string) string {
	if cmd, ok := shellCommand(event, payload); ok {
		return shellVerdict(cmd, rf, root, 0)
	}
	return toolVerdict(event, payload, rf, root)
}

func logGuard(p Paths, sid, source, id string, now time.Time, elapsed int64) {
	ev := baseVerdict(now, sid, source, elapsed)
	ev.Channel, ev.Mode, ev.Violations = "code", "block", 1
	ev.RuleIDs = []string{id}
	ev.Reason = guardMessages[id]
	_ = AppendJSONL(p.Log, ev)
}

func shellVerdict(command string, rf ruleFile, root string, depth int) string {
	if pipeToShell.MatchString(command) {
		return ruleDestructive
	}
	for _, seg := range rules.Segments(command) {
		toks := rules.Tokens(seg)
		i := 0
		for i < len(toks) {
			t := strings.ToLower(cmdBase(toks[i]))
			if isAssign(toks[i]) {
				if secretShell("", []string{toks[i][strings.IndexByte(toks[i], '=')+1:]}, rf, root) {
					return ruleSecretRead
				}
				i++
			} else if strings.HasPrefix(toks[i], "-") || t == "sudo" || t == "env" || t == "command" || t == "nohup" || t == "time" {
				i++
			} else {
				break
			}
		}
		if i >= len(toks) {
			continue
		}
		base, args := cmdBase(toks[i]), toks[i+1:]
		for base == "xargs" { // the command it runs is the first word that is not an option
			j := 0
			for j < len(args) && strings.HasPrefix(args[j], "-") {
				j++
			}
			if j == len(args) {
				break
			}
			toks, base, args = args[j:], cmdBase(args[j]), args[j+1:]
			i = 0
		}
		if depth < 3 {
			for words, k := inlineWords(base, args), 0; k < len(words); k++ {
				if destructiveCommand(cmdBase(words[k]), words[k:], root) {
					return ruleDestructive
				}
			}
			if sub := nestedShell(base, args); sub != "" {
				if id := shellVerdict(sub, rf, root, depth+1); id != "" {
					return id
				}
			}
		}
		if destructiveCommand(base, toks[i:], root) {
			return ruleDestructive
		}
		if secretShell(base, toks[i+1:], rf, root) {
			return ruleSecretRead
		}
		if selfShell(base, toks[i:], rf) {
			return ruleGuardTamper
		}
	}
	return ""
}

// nestedShell returns the script of `sh -c script` or `eval script`.
func nestedShell(base string, args []string) string {
	switch base {
	case "eval":
		return strings.Join(args, " ")
	case "sh", "bash", "zsh", "dash", "ksh":
		for j := 0; j+1 < len(args); j++ {
			if f := args[j]; strings.HasPrefix(f, "-") && !strings.HasPrefix(f, "--") && strings.Contains(f, "c") {
				return args[j+1]
			}
		}
	}
	return ""
}

// inlineWords splits the code of `python -c`, `node -e` and the like into words,
// call punctuation as spaces, so os.system('rm -rf /') reads as a shell line.
func inlineWords(base string, args []string) []string {
	switch base {
	case "python", "python3", "node", "perl", "ruby":
		for j := 0; j+1 < len(args); j++ {
			if args[j] == "-c" || args[j] == "-e" {
				return rules.Tokens(callSyntax.Replace(args[j+1]))
			}
		}
	}
	return nil
}

var callSyntax = strings.NewReplacer("(", " ", ")", " ", "'", " ", `"`, " ", ",", " ", ";", " ; ")

func destructiveCommand(base string, toks []string, root string) bool {
	args := toks[1:]
	switch base {
	case "git":
		sub, rest := gitRest(toks)
		flags, pos := splitFlags(rest)
		switch sub {
		case "reset":
			return hasLong(flags, "--hard") || hasLong(flags, "--merge")
		case "clean":
			return (hasLong(flags, "--force") || hasShort(flags, 'f')) && !hasLong(flags, "--dry-run") && !hasShort(flags, 'n')
		case "checkout", "restore":
			if hasLong(flags, "--force") || hasShort(flags, 'f') {
				return true
			}
			staged := (hasLong(flags, "--staged") || hasShort(flags, 'S')) && !hasLong(flags, "--worktree") && !hasShort(flags, 'W')
			return !staged && anyOf(pos, ".", "./", "*")
		case "stash":
			return len(pos) > 0 && (pos[0] == "drop" || pos[0] == "clear")
		}
	case "rm":
		flags, pos := splitFlags(args)
		if !hasLong(flags, "--recursive") && !hasShort(flags, 'r') && !hasShort(flags, 'R') {
			return false
		}
		for _, t := range pos {
			if wholeTree(t, root) {
				return true
			}
		}
	}
	return false
}

const systemDirs = " /bin /boot /dev /etc /home /lib /lib64 /opt /proc /root /sbin /srv /sys /usr /var "

// wholeTree reports a target that is the filesystem root, a top-level system directory, the home directory,
// the repository root, a parent directory, or everything in one of them.
func wholeTree(t, root string) bool {
	if t == "*" {
		return true
	}
	s := strings.TrimSuffix(strings.TrimSpace(t), "/*")
	if s == "" {
		return true
	}
	n := guardPath(s)
	return strings.Contains(systemDirs, " "+n+" ") || n == "~" || n == "$PWD" || n == "${PWD}" || strings.Trim(n, "./") == "" || (filepath.IsAbs(n) && n == path.Clean(filepath.ToSlash(root)))
}

func secretShell(base string, args []string, rf ruleFile, root string) bool {
	if base == "echo" || base == "printf" {
		args = redirTargets(args)
	}
	if looksOnly(base, args) {
		return false
	}
	for _, a := range args {
		for _, c := range candidates(a) {
			if secretOrLinked(c, root) && !guardAllowed(rf, c) {
				return true
			}
		}
	}
	return false
}

// looksOnly reports ls, stat or test on a path: it reads names and metadata, not
// contents. A redirect or substitution in the arguments could still read or write.
func looksOnly(base string, args []string) bool {
	line := strings.Join(args, " ")
	return (base == "ls" || base == "stat" || base == "test" || base == "[") &&
		!strings.ContainsAny(line, "<>`") && !strings.Contains(line, "$(")
}

func selfShell(base string, toks []string, rf ruleFile) bool {
	hit := func(p string, dirs bool) bool { return selfPath(p, dirs) && !guardAllowed(rf, p) }
	args := toks[1:]
	for _, t := range redirTargets(args) {
		if hit(t, false) {
			return true
		}
	}
	flags, pos := splitFlags(args)
	dirs := false
	switch base {
	case "rm", "mv":
		dirs = true
	case "unlink", "tee", "truncate", "chmod", "chown", "dd":
	case "cp", "install", "ln":
		if len(pos) > 0 {
			pos = pos[len(pos)-1:]
		}
	case "sed", "perl":
		inPlace := hasShort(flags, 'i')
		for _, f := range flags {
			inPlace = inPlace || strings.HasPrefix(f, "--in-place")
		}
		if !inPlace {
			return false
		}
	case "git":
		sub, rest := gitRest(toks)
		switch sub {
		case "rm", "mv":
			dirs = true
		case "restore", "checkout":
		default:
			return false
		}
		_, pos = splitFlags(rest)
	default:
		return false
	}
	for _, p := range pos {
		for _, c := range candidates(p) {
			if hit(c, dirs) {
				return true
			}
		}
	}
	return false
}

// toolVerdict checks file tools. Secrets: any tool that names such a path,
// including Cursor beforeReadFile. The guard's own files: write tools only.
func toolVerdict(event string, payload map[string]any, rf ruleFile, root string) string {
	name := strings.ToLower(stringField(payload, "tool_name"))
	if isShellTool(name) {
		return ""
	}
	in := asMap(payload["tool_input"])
	var paths []string
	for _, m := range []map[string]any{in, payload} {
		for _, k := range []string{"path", "file_path", "target_file", "notebook_path"} {
			if p := stringField(m, k); p != "" {
				paths = append(paths, p)
			}
		}
		paths = append(paths, stringList(m["paths"])...)
	}
	if event == "beforeReadFile" {
		paths = append(paths, attachmentPaths(payload)...)
	}
	write, remove := writeTools[name], name == "delete" || name == "delete_file" || name == "deletefile"
	for _, p := range paths {
		if guardAllowed(rf, p) {
			continue
		}
		if secretOrLinked(p, root) {
			return ruleSecretRead
		}
		if write && selfPath(p, remove) {
			return ruleGuardTamper
		}
	}
	return ""
}

var writeTools = map[string]bool{
	"write": true, "edit": true, "multiedit": true, "strreplace": true, "str_replace": true,
	"notebookedit": true, "edit_file": true, "write_file": true, "search_replace": true,
	"delete": true, "delete_file": true, "deletefile": true,
}

var envSafeSuffix = []string{".example", ".sample", ".template"}

// isSecretPath reports a file that holds credentials.
func isSecretPath(p string) bool {
	n := guardPath(p)
	if n == "" {
		return false
	}
	b := strings.ToLower(path.Base(n))
	if strings.HasSuffix(b, ".pub") {
		return false // public keys
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".ssh" || seg == ".aws" {
			return true
		}
	}
	if n == "~/.config/gcloud" || strings.HasPrefix(n, "~/.config/gcloud/") {
		return true
	}
	switch {
	case b == ".env", b == ".npmrc", b == ".netrc":
		return true
	case strings.HasPrefix(b, ".env."):
		for _, s := range envSafeSuffix {
			if strings.HasSuffix(b, s) {
				return false
			}
		}
		return true
	case strings.HasPrefix(b, ".env") && strings.ContainsAny(b, "*?["):
		return true
	}
	return strings.HasSuffix(b, ".pem") || strings.HasSuffix(b, ".key") || strings.HasPrefix(b, "id_rsa")
}

// secretOrLinked is isSecretPath, also for the file a symlink points to.
func secretOrLinked(p, root string) bool {
	if isSecretPath(p) {
		return true
	}
	if h, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, "~/") {
		p = h + p[1:]
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	r, err := filepath.EvalSymlinks(p)
	return err == nil && r != p && isSecretPath(r)
}

// selfPath reports a hook config, the rules file, or ~/.drift-guard. With
// dirs, the .cursor and .claude directories count, because removing one
// removes the config inside it.
func selfPath(p string, dirs bool) bool {
	n := guardPath(p)
	if n == "" {
		return false
	}
	if n == "~/.drift-guard" || strings.HasPrefix(n, "~/.drift-guard/") || path.Base(n) == "drift-guard.rules.json" {
		return true
	}
	for _, f := range []string{".cursor/hooks.json", ".claude/settings.json", ".claude/settings.local.json"} {
		if n == f || strings.HasSuffix(n, "/"+f) {
			return true
		}
	}
	b := path.Base(n)
	return dirs && (b == ".cursor" || b == ".claude")
}

// guardPath expands the home directory to "~" and cleans the path.
func guardPath(p string) string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if p == "" {
		return ""
	}
	homes := []string{"${HOME}", "$HOME"}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		homes = append(homes, filepath.ToSlash(h))
	}
	for _, h := range homes {
		if p == h || strings.HasPrefix(p, h+"/") {
			p = "~" + p[len(h):]
			break
		}
	}
	return path.Clean(p)
}

func guardAllowed(rf ruleFile, p string) bool {
	n := guardPath(p)
	for _, g := range rf.GuardAllow {
		if ok, _ := path.Match(g, n); ok {
			return true
		}
		if ok, _ := path.Match(g, path.Base(n)); ok {
			return true
		}
	}
	return false
}

// substSyntax drops quotes, backticks and $( ) so cat $(echo .env) shows the path.
var substSyntax = strings.NewReplacer("`", "", `"`, "", "'", "", "$(", "", ")", "")

// candidates also tries the part after = < > @, so --file=.env, of=.env, -d @.env and
// >.env are seen.
func candidates(tok string) []string {
	tok = substSyntax.Replace(tok)
	out := []string{tok}
	if i := strings.LastIndexAny(tok, "=<>@"); i >= 0 && i+1 < len(tok) {
		out = append(out, tok[i+1:])
	}
	return out
}

// redirTargets lists the words after > or >> .
func redirTargets(toks []string) []string {
	var out []string
	for j, t := range toks {
		i := strings.LastIndex(t, ">")
		switch {
		case i < 0:
		case i+1 < len(t):
			out = append(out, t[i+1:])
		case j+1 < len(toks):
			out = append(out, toks[j+1])
		}
	}
	return out
}

// splitFlags separates option words from operands. Words after -- are operands.
func splitFlags(toks []string) (flags, pos []string) {
	dash := false
	for _, t := range toks {
		switch {
		case dash:
			pos = append(pos, t)
		case t == "--":
			dash = true
		case strings.HasPrefix(t, "-") && t != "-":
			flags = append(flags, t)
		default:
			pos = append(pos, t)
		}
	}
	return flags, pos
}

func hasLong(flags []string, name string) bool {
	for _, f := range flags {
		if f == name {
			return true
		}
	}
	return false
}

func hasShort(flags []string, c byte) bool {
	for _, f := range flags {
		if !strings.HasPrefix(f, "--") && strings.IndexByte(f, c) > 0 {
			return true
		}
	}
	return false
}

func anyOf(list []string, want ...string) bool {
	for _, s := range list {
		for _, w := range want {
			if s == w {
				return true
			}
		}
	}
	return false
}
