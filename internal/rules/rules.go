// Package rules is the two deterministic checks the prototype measures and ships.
// No model, no network.
package rules

import (
	"path"
	"strings"
	"unicode"
)

// Command is one finished shell invocation from the tool log.
type Command struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
}

// IsGitPush reports whether command would run git push.
// Separators are &&, ||, ;, |, and newlines, outside quotes.
// A shell -c argument is inspected the same way.
// git -C <dir> push counts. A mention inside another program's argument does not.
func IsGitPush(command string) bool {
	for _, seg := range splitTop(command) {
		if segmentPushes(seg) {
			return true
		}
	}
	return false
}

func splitTop(command string) []string {
	var parts []string
	var b strings.Builder
	var quote byte
	for i := 0; i < len(command); i++ {
		c := command[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			b.WriteByte(c)
			continue
		}
		if c == '\n' || c == ';' {
			parts = append(parts, b.String())
			b.Reset()
			continue
		}
		if c == '&' && i+1 < len(command) && command[i+1] == '&' {
			parts = append(parts, b.String())
			b.Reset()
			i++
			continue
		}
		if c == '|' && i+1 < len(command) && command[i+1] == '|' {
			parts = append(parts, b.String())
			b.Reset()
			i++
			continue
		}
		if c == '|' {
			parts = append(parts, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	parts = append(parts, b.String())
	return parts
}

func tokenize(seg string) []string {
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
			if c == quote {
				quote = 0
				flush()
			} else {
				b.WriteByte(c)
			}
			continue
		}
		if c == '\'' || c == '"' {
			flush()
			quote = c
			continue
		}
		if c == ' ' || c == '\t' {
			flush()
			continue
		}
		b.WriteByte(c)
	}
	flush()
	return toks
}

func segmentPushes(seg string) bool {
	return tokensPush(tokenize(seg))
}

func tokensPush(toks []string) bool {
	i := 0
	for i < len(toks) && isEnvAssign(toks[i]) {
		i++
	}
	if i < len(toks) && strings.EqualFold(toks[i], "sudo") {
		i++
		for i < len(toks) && isEnvAssign(toks[i]) {
			i++
		}
	}
	if i < len(toks) && isShell(toks[i]) {
		for j := i + 1; j < len(toks); j++ {
			if toks[j] == "-c" || toks[j] == "-lc" {
				if j+1 < len(toks) && IsGitPush(toks[j+1]) {
					return true
				}
			}
		}
	}
	if i >= len(toks) || !strings.EqualFold(toks[i], "git") {
		return false
	}
	i++
	for i < len(toks) {
		t := toks[i]
		if strings.EqualFold(t, "push") {
			return true
		}
		switch strings.ToLower(t) {
		case "-c", "-C", "--git-dir", "--namespace", "--work-tree":
			i += 2
			continue
		}
		if strings.HasPrefix(t, "-") {
			i++
			continue
		}
		return false
	}
	return false
}

func isShell(tok string) bool {
	switch strings.ToLower(tok) {
	case "sh", "bash", "dash":
		return true
	default:
		return false
	}
}

func isEnvAssign(s string) bool {
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

// DoneWithoutTests fires when a named test command never exited 0.
// It fires only when a rule names testCommand, at least one changed path matches
// globs (any changed path when globs is empty), and the command log has no
// exit-0 invocation whose command string contains testCommand.
func DoneWithoutTests(testCommand string, globs, changed []string, commands []Command) bool {
	testCommand = strings.TrimSpace(testCommand)
	if testCommand == "" || !anyChanged(globs, changed) {
		return false
	}
	needle := strings.ToLower(testCommand)
	for _, c := range commands {
		if c.ExitCode == 0 && strings.Contains(strings.ToLower(c.Command), needle) {
			return false
		}
	}
	return true
}

func anyChanged(globs, changed []string) bool {
	if len(changed) == 0 {
		return false
	}
	if len(globs) == 0 {
		return true
	}
	for _, p := range changed {
		for _, g := range globs {
			if MatchGlob(g, p) {
				return true
			}
		}
	}
	return false
}

// ReplyLanguageViolation fires when the reply misses a named language.
// required is "ru" or "en". Anything else, including an empty rule, does not fire.
// Texts with fewer than 40 letters are not judged. No model is used:
// Russian is a Cyrillic letter ratio under 0.4, English is that ratio over 0.4.
func ReplyLanguageViolation(required, text string) bool {
	switch strings.ToLower(strings.TrimSpace(required)) {
	case "ru", "en":
	default:
		return false
	}
	var letters, cyr int
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.Is(unicode.Cyrillic, r) {
			cyr++
		}
	}
	if letters < 40 {
		return false
	}
	ratio := float64(cyr) / float64(letters)
	if strings.EqualFold(strings.TrimSpace(required), "ru") {
		return ratio < 0.4
	}
	return ratio > 0.4
}

// MatchGlob matches a slash-separated glob. * stays inside one segment.
// ** spans segments. The match is case-sensitive.
func MatchGlob(pattern, name string) bool {
	pattern = strings.Trim(path.Clean("/"+pattern), "/")
	name = strings.Trim(path.Clean("/"+name), "/")
	if pattern == "." {
		pattern = ""
	}
	if name == "." {
		name = ""
	}
	return matchParts(splitSeg(pattern), splitSeg(name))
}

func splitSeg(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func matchParts(pat, name []string) bool {
	if len(pat) == 0 {
		return len(name) == 0
	}
	if pat[0] == "**" {
		if matchParts(pat[1:], name) {
			return true
		}
		if len(name) == 0 {
			return false
		}
		return matchParts(pat, name[1:])
	}
	if len(name) == 0 || !matchSeg(pat[0], name[0]) {
		return false
	}
	return matchParts(pat[1:], name[1:])
}

func matchSeg(pattern, seg string) bool {
	if pattern == "*" {
		return true
	}
	ok, err := path.Match(pattern, seg)
	return err == nil && ok
}
